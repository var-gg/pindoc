package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// TestArtifactProposeAppliesToAreasValidationIntegration covers Decision
// area-taxonomy-profiled-skeleton T6: artifact_meta.applies_to_areas is
// plain JSONB with no foreign key, so preflight rejects a scope whose
// slug is not a real area in the project — an unknown slug makes the rule
// silently no-op. A real slug, a real wildcard scope, and "*" all pass.
func TestArtifactProposeAppliesToAreasValidationIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.propose applies_to_areas DB integration")
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

	suffix := time.Now().UnixNano()
	projectSlug := fmt.Sprintf("propose-areas-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	insertContextReceiptArea(t, ctx, pool, projectID, "mcp")
	insertContextReceiptArea(t, ctx, pool, projectID, "ui")
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	call := newArtifactProposeTestCaller(t, ctx, pool, nil)

	// An applies_to_areas scope pointing at a slug that is not a real
	// area is blocked — the rule would otherwise silently match nothing.
	// The list leads with a real slug to prove the loop keeps scanning
	// past a valid entry before it reaches the unknown one.
	beforeArtifacts := countRows(t, ctx, pool, "artifacts", projectID)
	unknown := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     "mcp",
		"type":          "Decision",
		"title":         "Rule scoped to a ghost area",
		"slug":          fmt.Sprintf("areas-unknown-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("x", "y"),
		"author_id":     "codex-test",
		"artifact_meta": map[string]any{
			"applies_to_areas": []string{"mcp", "ghost-area"},
		},
	})
	if unknown.Status != "not_ready" || unknown.ErrorCode != "META_APPLIES_AREA_UNKNOWN" {
		t.Fatalf("unknown applies_to_areas slug = status=%q code=%q, want not_ready META_APPLIES_AREA_UNKNOWN",
			unknown.Status, unknown.ErrorCode)
	}
	if got := countRows(t, ctx, pool, "artifacts", projectID); got != beforeArtifacts {
		t.Fatalf("artifact count after blocked propose = %d, want %d", got, beforeArtifacts)
	}

	// A real area slug, a wildcard scope on a real area, and the project
	// wildcard "*" all resolve, so the propose is accepted.
	accepted := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     "mcp",
		"type":          "Decision",
		"title":         "Rule scoped to real areas",
		"slug":          fmt.Sprintf("areas-known-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("x", "y"),
		"author_id":     "codex-test",
		"artifact_meta": map[string]any{
			"applies_to_areas": []string{"mcp", "ui/*", "*"},
		},
	})
	if accepted.Status != "accepted" || !accepted.Created {
		t.Fatalf("known applies_to_areas scopes = status=%q created=%v error_code=%q",
			accepted.Status, accepted.Created, accepted.ErrorCode)
	}
}
