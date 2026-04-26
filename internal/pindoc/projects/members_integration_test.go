package projects

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestProjectMembersIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project_members DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ownerID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Project Member Owner', 'pm-owner@example.invalid', 'pindoc_admin')
		RETURNING id::text
	`).Scan(&ownerID); err != nil {
		t.Fatalf("insert owner user: %v", err)
	}
	assertDefaultBootstrapIdempotent(t, ctx, tx, ownerID)

	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "pm-integration",
		Name:            "Project Members Integration",
		PrimaryLanguage: "en",
		OwnerUserID:     ownerID,
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	assertMemberRole(t, ctx, tx, out.ID, ownerID, ProjectRoleOwner)
	assertInvalidRoleRejected(t, ctx, tx, out.ID, ownerID)
	assertUserIndexUsed(t, ctx, tx, ownerID)
	assertFKActions(t, ctx, tx, out.ID)
}

func assertDefaultBootstrapIdempotent(t *testing.T, ctx context.Context, tx pgx.Tx, userID string) {
	t.Helper()
	for i := 0; i < 2; i++ {
		if err := EnsureDefaultProjectOwnerMembership(ctx, tx, "pindoc", userID); err != nil {
			t.Fatalf("default owner bootstrap attempt %d: %v", i+1, err)
		}
	}
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		  FROM project_members pm
		  JOIN projects p ON p.id = pm.project_id
		 WHERE p.slug = 'pindoc' AND pm.user_id = $1::uuid AND pm.role = $2
	`, userID, ProjectRoleOwner).Scan(&n); err != nil {
		t.Fatalf("count default project membership: %v", err)
	}
	if n != 1 {
		t.Fatalf("default owner bootstrap rows = %d, want 1", n)
	}
}

func assertMemberRole(t *testing.T, ctx context.Context, tx pgx.Tx, projectID, userID, wantRole string) {
	t.Helper()
	var role string
	if err := tx.QueryRow(ctx, `
		SELECT role FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
	`, projectID, userID).Scan(&role); err != nil {
		t.Fatalf("select member role: %v", err)
	}
	if role != wantRole {
		t.Fatalf("member role = %q, want %q", role, wantRole)
	}
}

func assertInvalidRoleRejected(t *testing.T, ctx context.Context, tx pgx.Tx, projectID, userID string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SAVEPOINT invalid_role_check`); err != nil {
		t.Fatalf("savepoint invalid role: %v", err)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'admin')
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, projectID, userID)
	if err == nil {
		t.Fatalf("invalid project_members.role should be rejected")
	}
	if _, rbErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_role_check`); rbErr != nil {
		t.Fatalf("rollback invalid role savepoint: %v", rbErr)
	}
}

func assertUserIndexUsed(t *testing.T, ctx context.Context, tx pgx.Tx, userID string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("disable seqscan: %v", err)
	}
	rows, err := tx.Query(ctx, `EXPLAIN SELECT * FROM project_members WHERE user_id = $1::uuid`, userID)
	if err != nil {
		t.Fatalf("explain project_members user lookup: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan explain: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read explain: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_project_members_user") {
		t.Fatalf("expected idx_project_members_user in plan:\n%s", plan.String())
	}
}

func assertFKActions(t *testing.T, ctx context.Context, tx pgx.Tx, projectID string) {
	t.Helper()
	var inviterID, viewerID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Project Member Inviter', 'pm-inviter@example.invalid', 'pindoc_admin')
		RETURNING id::text
	`).Scan(&inviterID); err != nil {
		t.Fatalf("insert inviter: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Project Member Viewer', 'pm-viewer@example.invalid', 'pindoc_admin')
		RETURNING id::text
	`).Scan(&viewerID); err != nil {
		t.Fatalf("insert viewer: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role, invited_by)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
	`, projectID, viewerID, ProjectRoleViewer, inviterID); err != nil {
		t.Fatalf("insert viewer membership: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, inviterID); err != nil {
		t.Fatalf("delete inviter: %v", err)
	}
	var invitedBy *string
	if err := tx.QueryRow(ctx, `
		SELECT invited_by::text FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
	`, projectID, viewerID).Scan(&invitedBy); err != nil {
		t.Fatalf("select invited_by: %v", err)
	}
	if invitedBy != nil {
		t.Fatalf("invited_by should be NULL after inviter delete, got %q", *invitedBy)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, viewerID); err != nil {
		t.Fatalf("delete viewer: %v", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM project_members
			 WHERE project_id = $1::uuid AND user_id = $2::uuid
		)
	`, projectID, viewerID).Scan(&exists); err != nil {
		t.Fatalf("check viewer cascade: %v", err)
	}
	if exists {
		t.Fatalf("viewer membership should cascade on user delete")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1::uuid`, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	err := tx.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id = $1::uuid LIMIT 1`, projectID).Scan(new(string))
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("project delete should cascade memberships, got err=%v", err)
	}
}
