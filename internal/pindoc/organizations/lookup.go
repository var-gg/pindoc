package organizations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Org is the read shape every other package consumes. Distinct from the
// CreateInput / Output pair because lookup queries don't always need
// the description and the returned slug is always canonical.
type Org struct {
	ID          string
	Slug        string
	Name        string
	Kind        string
	OwnerUserID string // empty string when the Org has no owner_user_id (default Org)
	Description string
}

// LookupQueryer is the read interface every Resolve* function consumes.
// Both *db.Pool and pgx.Tx satisfy it, so callers can run lookups inside
// or outside an existing transaction without ceremony.
type LookupQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ResolveBySlug returns the active Org with the given slug, or
// ErrNotFound. Slug is normalized before the query so callers don't
// have to remember to lower-case.
func ResolveBySlug(ctx context.Context, q LookupQueryer, slug string) (*Org, error) {
	slug = NormalizeSlug(slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: empty slug", ErrNotFound)
	}
	var (
		out      Org
		ownerPtr *string
		descPtr  *string
	)
	err := q.QueryRow(ctx, `
		SELECT id::text, slug, name, kind, owner_user_id::text, description
		  FROM organizations
		 WHERE slug = $1 AND deleted_at IS NULL
		 LIMIT 1
	`, slug).Scan(&out.ID, &out.Slug, &out.Name, &out.Kind, &ownerPtr, &descPtr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: slug %q", ErrNotFound, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup org by slug: %w", err)
	}
	if ownerPtr != nil {
		out.OwnerUserID = *ownerPtr
	}
	if descPtr != nil {
		out.Description = *descPtr
	}
	return &out, nil
}

// ResolveByID is the UUID-keyed counterpart to ResolveBySlug. Used by
// Project lookup paths that already hold the organization_id FK and
// want to enrich with slug/name for URL building.
func ResolveByID(ctx context.Context, q LookupQueryer, id string) (*Org, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: empty id", ErrNotFound)
	}
	var (
		out      Org
		ownerPtr *string
		descPtr  *string
	)
	err := q.QueryRow(ctx, `
		SELECT id::text, slug, name, kind, owner_user_id::text, description
		  FROM organizations
		 WHERE id = $1::uuid AND deleted_at IS NULL
		 LIMIT 1
	`, id).Scan(&out.ID, &out.Slug, &out.Name, &out.Kind, &ownerPtr, &descPtr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id %q", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup org by id: %w", err)
	}
	if ownerPtr != nil {
		out.OwnerUserID = *ownerPtr
	}
	if descPtr != nil {
		out.Description = *descPtr
	}
	return &out, nil
}

// ResolveDefaultID returns the UUID of the bootstrap 'default' Org seeded
// by migration 0049. Project create/upsert paths that don't yet have a
// real Organization context fall back to this so organization_id stays
// populated without a caller-visible owner label.
func ResolveDefaultID(ctx context.Context, q LookupQueryer) (string, error) {
	var id string
	err := q.QueryRow(ctx, `
		SELECT id::text FROM organizations
		 WHERE slug = $1 AND deleted_at IS NULL
		 LIMIT 1
	`, DefaultOrgSlug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: bootstrap default org missing — migration 0049 not applied?", ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("lookup default org id: %w", err)
	}
	return id, nil
}
