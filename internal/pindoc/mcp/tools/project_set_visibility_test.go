package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestProjectSetVisibilityRecordsAuditEventIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project visibility MCP DB integration")
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

	fixture := seedMCPVisibilityFixture(t, ctx, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, fixture.projectID)
	})

	owner := &auth.Principal{UserID: fixture.ownerUserID, AgentID: "agent:visibility-test", Source: auth.SourceOAuth}
	updated := callVisibilityTool[projectSetVisibilityOutput](t, ctx, pool, nil, owner, "pindoc.project.set_visibility", map[string]any{
		"project_slug": fixture.projectSlug,
		"visibility":   projects.VisibilityPublic,
	})
	if updated.Status != "ok" || updated.Code != "PROJECT_VISIBILITY_UPDATED" || updated.Affected != 1 {
		t.Fatalf("project.set_visibility updated = %+v", updated)
	}
	assertMCPProjectVisibilityEvent(t, ctx, pool, fixture.projectID, projects.VisibilityOrg, projects.VisibilityPublic, fixture.ownerUserID, "agent:visibility-test", "mcp_project_set_visibility", 1)

	noOp := callVisibilityTool[projectSetVisibilityOutput](t, ctx, pool, nil, owner, "pindoc.project.set_visibility", map[string]any{
		"project_slug": fixture.projectSlug,
		"visibility":   projects.VisibilityPublic,
	})
	if noOp.Status != "informational" || noOp.Code != "PROJECT_VISIBILITY_NO_OP" || noOp.Affected != 0 {
		t.Fatalf("project.set_visibility no-op = %+v", noOp)
	}
	assertMCPProjectVisibilityEvent(t, ctx, pool, fixture.projectID, projects.VisibilityOrg, projects.VisibilityPublic, fixture.ownerUserID, "agent:visibility-test", "mcp_project_set_visibility", 1)
}

func assertMCPProjectVisibilityEvent(t *testing.T, ctx context.Context, pool *db.Pool, projectID, from, to, actorUserID, actorID, origin string, wantCount int) {
	t.Helper()
	var count int
	var gotFrom, gotTo, gotActorUserID, gotActorID, gotOrigin string
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::int,
		       COALESCE(max(payload->>'from'), ''),
		       COALESCE(max(payload->>'to'), ''),
		       COALESCE(max(payload->>'actor_user_id'), ''),
		       COALESCE(max(payload->>'actor_id'), ''),
		       COALESCE(max(payload->>'origin'), '')
		  FROM events
		 WHERE project_id = $1::uuid
		   AND subject_id = $1::uuid
		   AND kind = 'project.visibility_changed'
	`, projectID).Scan(&count, &gotFrom, &gotTo, &gotActorUserID, &gotActorID, &gotOrigin); err != nil {
		t.Fatalf("select project visibility event: %v", err)
	}
	if count != wantCount {
		t.Fatalf("project visibility event count = %d, want %d", count, wantCount)
	}
	if count == 0 {
		return
	}
	got := fmt.Sprintf("%s|%s|%s|%s|%s", gotFrom, gotTo, gotActorUserID, gotActorID, gotOrigin)
	want := fmt.Sprintf("%s|%s|%s|%s|%s", from, to, actorUserID, actorID, origin)
	if got != want {
		t.Fatalf("project visibility event payload = %s, want %s", got, want)
	}
}
