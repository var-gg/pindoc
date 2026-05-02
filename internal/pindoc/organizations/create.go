package organizations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Kinds an Organization can take. 'personal' is the auto-created Org tied
// 1:1 to a user — its slug equals the user's username and it cannot be
// transferred. 'team' is the multi-member shared Org used for actual
// SaaS billing. The CHECK constraint on organizations.kind enforces
// exactly these two values at the DB layer.
const (
	KindPersonal = "personal"
	KindTeam     = "team"
)

// Roles inside organization_members. Mirrors GitHub/Slack: owner has
// billing + member-management rights, admin manages members, member just
// participates. Project-level membership is layered separately via
// project_members (existing migration 0032) so a user can have lower
// access to specific Projects than the Org-wide role implies.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// DefaultOrgSlug is the bootstrap Org seeded by migration 0049. Existing
// self-host installs route through it until a real owner identity is
// set; the slug is a reserved sentinel and cannot be claimed by signup.
const DefaultOrgSlug = "default"

// CreateInput is the entrypoint-agnostic projection of "create Org".
// MCP tool / REST handler / CLI all build one of these from their native
// input shape.
type CreateInput struct {
	Slug        string
	Name        string
	Kind        string // 'personal' or 'team'; defaults to 'team'
	Description string // optional
	OwnerUserID string // required for kind='personal', optional for 'team'
}

// Output carries the post-create facts every entrypoint needs.
type Output struct {
	ID          string
	Slug        string
	Name        string
	Kind        string
	Description string
}

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Create inserts an organizations row + an organization_members owner
// row in the same transaction. Caller owns Begin/Commit/Rollback (the
// MCP tool already runs in-tx; REST/CLI begin their own).
//
// For kind='personal', OwnerUserID is mandatory and the personal-Org
// uniqueness index will reject a second personal Org per user. For
// kind='team', OwnerUserID is optional but recommended (creates the
// initial owner member row).
func Create(ctx context.Context, tx pgx.Tx, in CreateInput) (Output, error) {
	var zero Output

	slug := NormalizeSlug(in.Slug)
	name := strings.TrimSpace(in.Name)
	kind := strings.TrimSpace(in.Kind)
	desc := strings.TrimSpace(in.Description)
	ownerUserID := strings.TrimSpace(in.OwnerUserID)

	if kind == "" {
		kind = KindTeam
	}
	if kind != KindPersonal && kind != KindTeam {
		return zero, fmt.Errorf("%w: kind must be 'personal' or 'team', got %q", ErrKindInvalid, in.Kind)
	}
	if err := ValidateSlug(slug); err != nil {
		return zero, err
	}
	if name == "" {
		return zero, fmt.Errorf("%w: name is required", ErrNameRequired)
	}
	if kind == KindPersonal && ownerUserID == "" {
		return zero, fmt.Errorf("%w: personal Org requires owner_user_id", ErrKindInvalid)
	}

	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	var (
		orgID       string
		ownerForCol any
	)
	if ownerUserID == "" {
		ownerForCol = nil
	} else {
		ownerForCol = ownerUserID
	}

	err := tx.QueryRow(ctx, `
		INSERT INTO organizations (slug, name, kind, description, owner_user_id)
		VALUES ($1, $2, $3, $4, $5::uuid)
		RETURNING id::text
	`, slug, name, kind, descPtr, ownerForCol).Scan(&orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Distinguish "slug already taken" from "this user already has a
			// personal Org" by inspecting the constraint name: organizations_
			// slug_key vs idx_organizations_personal_one_per_user.
			if strings.Contains(pgErr.ConstraintName, "personal_one_per_user") {
				return zero, fmt.Errorf("%w: user already has a personal Org", ErrKindInvalid)
			}
			return zero, fmt.Errorf("%w: organization slug %q is already in use", ErrSlugTaken, slug)
		}
		return zero, fmt.Errorf("organization insert: %w", err)
	}

	if ownerUserID != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members (organization_id, user_id, role)
			VALUES ($1::uuid, $2::uuid, 'owner')
			ON CONFLICT (organization_id, user_id) DO NOTHING
		`, orgID, ownerUserID); err != nil {
			return zero, fmt.Errorf("seed owner membership: %w", err)
		}
	}

	return Output{
		ID:          orgID,
		Slug:        slug,
		Name:        name,
		Kind:        kind,
		Description: desc,
	}, nil
}

// EnsurePersonal creates the personal Org for a user if one doesn't
// already exist, otherwise returns the existing one. Idempotent: safe
// to call from any user-bootstrap path (OAuth signup, MCP user.update
// after username assignment, harness_install). The slug equals the
// username and the Org's kind is locked to 'personal'.
//
// Returns ErrSlugReserved if the username collides with a system-reserved
// slug (the caller should reject the username earlier in the flow, but
// this is the last-line guard).
func EnsurePersonal(ctx context.Context, tx pgx.Tx, userID, username, displayName string) (Output, error) {
	var zero Output

	username = NormalizeSlug(username)
	if err := ValidateSlug(username); err != nil {
		return zero, err
	}
	if userID == "" {
		return zero, fmt.Errorf("%w: user_id is required", ErrKindInvalid)
	}

	// Check for an existing personal Org first so the function is idempotent
	// across re-bootstrap. Match by owner_user_id rather than slug — slug
	// can change (rename), the owner relationship cannot.
	var existing Output
	err := tx.QueryRow(ctx, `
		SELECT id::text, slug, name, kind, COALESCE(description, '')
		  FROM organizations
		 WHERE owner_user_id = $1::uuid AND kind = 'personal' AND deleted_at IS NULL
		 LIMIT 1
	`, userID).Scan(&existing.ID, &existing.Slug, &existing.Name, &existing.Kind, &existing.Description)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return zero, fmt.Errorf("lookup existing personal org: %w", err)
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = username
	}

	return Create(ctx, tx, CreateInput{
		Slug:        username,
		Name:        name,
		Kind:        KindPersonal,
		OwnerUserID: userID,
	})
}
