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

// TestPhaseDMembersIntegration exercises ListProjectMembers,
// CountProjectOwners, and RemoveProjectMember end-to-end against a
// temporary DB. The test runs only when PINDOC_TEST_DATABASE_URL is set
// (same skip-gate as the rest of the package's integration suite).
//
// It covers three claims from Phase D's acceptance:
//
//  1. List returns owner first, alphabetised by joined_at within role —
//     so the Reader UI's owner row is always at the top of the panel.
//  2. RemoveProjectMember refuses to take the last owner row off a
//     project (ErrLastOwner) but happily removes additional owners.
//  3. RemoveProjectMember returns ErrMemberNotFound on a stale user_id
//     so the handler can map to 404 rather than 500.
func TestPhaseDMembersIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run Phase D members integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	ownerID := insertUser(t, ctx, tx, "Phase D Owner", "pd-owner@example.invalid")
	editorID := insertUser(t, ctx, tx, "Phase D Editor", "pd-editor@example.invalid")
	viewerID := insertUser(t, ctx, tx, "Phase D Viewer", "pd-viewer@example.invalid")

	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "pd-integration",
		Name:            "Phase D Integration",
		PrimaryLanguage: "en",
		OwnerUserID:     ownerID,
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role, invited_by)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid),
		       ($1::uuid, $5::uuid, $6, $4::uuid)
	`, out.ID, editorID, ProjectRoleEditor, ownerID, viewerID, ProjectRoleViewer); err != nil {
		t.Fatalf("seed editor + viewer: %v", err)
	}

	// 1. ListProjectMembers — owner first, then editor, then viewer.
	rows, err := ListProjectMembers(ctx, tx, out.ID)
	if err != nil {
		t.Fatalf("ListProjectMembers: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("members count = %d, want 3", len(rows))
	}
	wantOrder := []string{ProjectRoleOwner, ProjectRoleEditor, ProjectRoleViewer}
	for i, want := range wantOrder {
		if rows[i].Role != want {
			t.Fatalf("rows[%d].role = %q, want %q", i, rows[i].Role, want)
		}
	}
	// invited_by is set on editor/viewer but not on owner (auto-bootstrapped).
	if rows[0].InvitedByID != "" {
		t.Fatalf("owner.invited_by = %q, want empty", rows[0].InvitedByID)
	}
	if rows[1].InvitedByID != ownerID {
		t.Fatalf("editor.invited_by = %q, want %q", rows[1].InvitedByID, ownerID)
	}

	// 2. CountProjectOwners reflects exactly one owner.
	count, err := CountProjectOwners(ctx, tx, out.ID)
	if err != nil {
		t.Fatalf("CountProjectOwners: %v", err)
	}
	if count != 1 {
		t.Fatalf("owner count = %d, want 1", count)
	}

	// 3. RemoveProjectMember refuses to remove the last owner.
	if err := RemoveProjectMember(ctx, txAdapter{tx}, out.ID, ownerID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("RemoveProjectMember(last owner): err = %v, want ErrLastOwner", err)
	}

	// 4. Promoting editor → second owner unblocks owner removal.
	if _, err := tx.Exec(ctx, `
		UPDATE project_members SET role = 'owner'
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
	`, out.ID, editorID); err != nil {
		t.Fatalf("promote editor to owner: %v", err)
	}
	count, err = CountProjectOwners(ctx, tx, out.ID)
	if err != nil {
		t.Fatalf("CountProjectOwners after promote: %v", err)
	}
	if count != 2 {
		t.Fatalf("owner count after promote = %d, want 2", count)
	}
	if err := RemoveProjectMember(ctx, txAdapter{tx}, out.ID, ownerID); err != nil {
		t.Fatalf("RemoveProjectMember(non-last owner) returned %v", err)
	}

	// 5. Self-leave path — viewer removes themself.
	if err := RemoveProjectMember(ctx, txAdapter{tx}, out.ID, viewerID); err != nil {
		t.Fatalf("RemoveProjectMember(self viewer) returned %v", err)
	}

	// 6. ErrMemberNotFound on stale user_id.
	if err := RemoveProjectMember(ctx, txAdapter{tx}, out.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("RemoveProjectMember(stale user): err = %v, want ErrMemberNotFound", err)
	}
}

func insertUser(t *testing.T, ctx context.Context, tx pgx.Tx, name, email string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, $2, 'pindoc_admin')
		RETURNING id::text
	`, name, email).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", name, err)
	}
	return id
}

// txAdapter exposes pgx.Tx as the beginner interface RemoveProjectMember
// expects. The underlying call is `Begin(ctx) → Tx`; pgx.Tx already
// supports nested Begin via savepoints, so the production behaviour is
// preserved while letting the integration test reuse the outer
// transaction for cleanup.
type txAdapter struct{ pgx.Tx }

func (t txAdapter) Begin(ctx context.Context) (pgx.Tx, error) {
	return t.Tx.Begin(ctx)
}
