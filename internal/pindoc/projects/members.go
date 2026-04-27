package projects

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ProjectRoleOwner  = "owner"
	ProjectRoleEditor = "editor"
	ProjectRoleViewer = "viewer"
)

type membershipExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
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
