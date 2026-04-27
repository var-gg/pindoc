package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ProjectRoleOwner  = "owner"
	ProjectRoleEditor = "editor"
	ProjectRoleViewer = "viewer"
)

// ErrLastOwner is returned by RemoveProjectMember when the request would
// strip the last owner row off a project. The handler maps it to a 422
// LAST_OWNER response so the UI can show "transfer ownership first" guidance
// instead of leaving the project orphaned. Phase D first-pass treats this
// as a hard block; ownership transfer lands in a follow-up task.
var ErrLastOwner = errors.New("projects: cannot remove the last owner of a project")

// ErrMemberNotFound is returned when the project_members row to remove
// doesn't exist. Distinct from ErrLastOwner so the handler can map to
// 404 (member-not-found) rather than 422 (last-owner-block).
var ErrMemberNotFound = errors.New("projects: project member not found")

type membershipExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// MemberRow is the read shape returned by ListProjectMembers. The HTTP
// layer surfaces it directly to the Reader UI; the avatar / chip work
// happens client-side off these fields. invited_by_id intentionally
// stays as the user UUID (not a join) so the UI can resolve display
// names against the same /api/users cache it already uses for assignee
// dropdowns.
type MemberRow struct {
	UserID        string
	DisplayName   string
	GitHubHandle  string
	Role          string
	InvitedByID   string
	JoinedAt      time.Time
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ListProjectMembers returns every project_members row for the project,
// joined with the users table for display fields. Sorted by role
// (owner → editor → viewer) then joined_at ASC so the owner row comes
// first and the most recent invitee shows up at the bottom.
func ListProjectMembers(ctx context.Context, q queryer, projectID string) ([]MemberRow, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("projects: project_id is required")
	}
	rows, err := q.Query(ctx, `
		SELECT pm.user_id::text, COALESCE(u.display_name, ''),
		       COALESCE(u.github_handle, ''), pm.role,
		       COALESCE(pm.invited_by::text, ''), pm.joined_at
		  FROM project_members pm
		  LEFT JOIN users u ON u.id = pm.user_id
		 WHERE pm.project_id = $1::uuid
		 ORDER BY CASE pm.role
		            WHEN 'owner'  THEN 0
		            WHEN 'editor' THEN 1
		            WHEN 'viewer' THEN 2
		            ELSE 3
		          END,
		          pm.joined_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	defer rows.Close()
	out := make([]MemberRow, 0, 8)
	for rows.Next() {
		var m MemberRow
		if err := rows.Scan(
			&m.UserID,
			&m.DisplayName,
			&m.GitHubHandle,
			&m.Role,
			&m.InvitedByID,
			&m.JoinedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project member row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project members: %w", err)
	}
	return out, nil
}

// CountProjectOwners returns the number of owner rows on a project. The
// remove path uses this inside its transaction to refuse the request
// before touching the row when the deletion would leave the project
// without an owner.
func CountProjectOwners(ctx context.Context, q queryer, projectID string) (int, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, errors.New("projects: project_id is required")
	}
	var n int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM project_members
		 WHERE project_id = $1::uuid AND role = 'owner'
	`, projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count project owners: %w", err)
	}
	return n, nil
}

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// RemoveProjectMember deletes one project_members row, refusing the
// operation if the target is the last owner. Wraps the read + delete
// in a transaction so the owner-count check and the delete agree on
// the same snapshot — a concurrent owner removal cannot fool both
// checks into seeing 2 owners.
//
// Returns ErrMemberNotFound when no row matches; ErrLastOwner when the
// row is the project's last owner. Both are sentinel errors the
// handler maps to specific status codes.
func RemoveProjectMember(ctx context.Context, pool beginner, projectID, userID string) error {
	projectID = strings.TrimSpace(projectID)
	userID = strings.TrimSpace(userID)
	if projectID == "" || userID == "" {
		return errors.New("projects: project_id and user_id are required")
	}
	if pool == nil {
		return errors.New("projects: nil DB pool")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin remove member: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role string
	err = tx.QueryRow(ctx, `
		SELECT role FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
		 FOR UPDATE
	`, projectID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("lock project member row: %w", err)
	}
	if role == ProjectRoleOwner {
		var owners int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM project_members
			 WHERE project_id = $1::uuid AND role = 'owner'
		`, projectID).Scan(&owners); err != nil {
			return fmt.Errorf("count owners during remove: %w", err)
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
	`, projectID, userID); err != nil {
		return fmt.Errorf("delete project member: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit remove member: %w", err)
	}
	return nil
}

// EnsureProjectOwnerMembership creates the owner row for a known project id.
// Empty user ids are a no-op so trusted_local installs without
// PINDOC_USER_NAME keep booting and creating projects as before.
func EnsureProjectOwnerMembership(ctx context.Context, exec membershipExecer, projectID, userID string) error {
	projectID = strings.TrimSpace(projectID)
	userID = strings.TrimSpace(userID)
	if exec == nil || projectID == "" || userID == "" {
		return nil
	}
	_, err := exec.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (project_id, user_id) DO NOTHING
	`, projectID, userID, ProjectRoleOwner)
	return err
}

// EnsureDefaultProjectOwnerMembership is the idempotent boot-time
// bootstrap path for existing installs. The migration cannot see
// PINDOC_USER_NAME/EMAIL, so the server fills the default project's owner
// row after the env-derived users row exists.
func EnsureDefaultProjectOwnerMembership(ctx context.Context, exec membershipExecer, projectSlug, userID string) error {
	projectSlug = strings.TrimSpace(projectSlug)
	userID = strings.TrimSpace(userID)
	if exec == nil || projectSlug == "" || userID == "" {
		return nil
	}
	_, err := exec.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		SELECT p.id, $2::uuid, $3
		  FROM projects p
		 WHERE p.slug = $1
		ON CONFLICT (project_id, user_id) DO NOTHING
	`, projectSlug, userID, ProjectRoleOwner)
	return err
}
