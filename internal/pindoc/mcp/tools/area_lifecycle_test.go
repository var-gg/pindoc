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

// TestValidateSetAreaTargetLifecycle covers the T8 filing guard: a
// retiring/archived area, or any area under a retiring/archived ancestor,
// is closed to new filing. An empty Lifecycle (hand-built struct) is
// treated as active.
func TestValidateSetAreaTargetLifecycle(t *testing.T) {
	tests := []struct {
		name string
		area setAreaInfo
		want string
	}{
		{"active fileable area passes", setAreaInfo{ID: "a", Slug: "combat", Fileable: true, Lifecycle: "active"}, ""},
		{"empty lifecycle treated as active", setAreaInfo{ID: "a", Slug: "combat", Fileable: true}, ""},
		{"retiring area rejected", setAreaInfo{ID: "a", Slug: "experience", Fileable: true, Lifecycle: "retiring"}, "AREA_NOT_ACTIVE"},
		{"archived area rejected", setAreaInfo{ID: "a", Slug: "old", Fileable: true, Lifecycle: "archived"}, "AREA_NOT_ACTIVE"},
		{"active area under retiring ancestor rejected", setAreaInfo{ID: "a", Slug: "heroes", Fileable: true, Lifecycle: "active", AncestorRetired: true}, "AREA_NOT_ACTIVE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateSetAreaTarget(tc.area); got != tc.want {
				t.Fatalf("validateSetAreaTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAreaLifecycleListingIntegration covers migration 0067 + area.list:
// the default listing shows only active areas; include_retiring and
// include_archived opt the legacy tiers back in, and the lifecycle field
// is surfaced.
func TestAreaLifecycleListingIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	retiringID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "legacy-retiring")
	archivedID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "legacy-archived")
	if _, err := pool.Exec(ctx, `UPDATE areas SET lifecycle = 'retiring' WHERE id = $1::uuid`, retiringID); err != nil {
		t.Fatalf("mark retiring: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE areas SET lifecycle = 'archived', archived_at = now() WHERE id = $1::uuid`, archivedID); err != nil {
		t.Fatalf("mark archived: %v", err)
	}

	def := callVisibilityTool[areaListOutput](t, ctx, pool, nil, owner, "pindoc.area.list", map[string]any{
		"project_slug": fixture.projectSlug,
	})
	if areaListCountBySlug(def, "legacy-retiring") != -1 || areaListCountBySlug(def, "legacy-archived") != -1 {
		t.Fatalf("default area.list must hide retiring/archived areas: %+v", def.Areas)
	}

	withRetiring := callVisibilityTool[areaListOutput](t, ctx, pool, nil, owner, "pindoc.area.list", map[string]any{
		"project_slug":     fixture.projectSlug,
		"include_retiring": true,
	})
	if areaListCountBySlug(withRetiring, "legacy-retiring") == -1 {
		t.Fatal("include_retiring must surface the retiring area")
	}
	if areaListCountBySlug(withRetiring, "legacy-archived") != -1 {
		t.Fatal("include_retiring must not surface archived areas")
	}
	if lc := areaListLifecycleBySlug(withRetiring, "legacy-retiring"); lc != "retiring" {
		t.Fatalf("retiring area lifecycle field = %q, want retiring", lc)
	}

	withArchived := callVisibilityTool[areaListOutput](t, ctx, pool, nil, owner, "pindoc.area.list", map[string]any{
		"project_slug":     fixture.projectSlug,
		"include_archived": true,
	})
	if areaListCountBySlug(withArchived, "legacy-archived") == -1 {
		t.Fatal("include_archived must surface the archived area")
	}
}

func areaListLifecycleBySlug(out areaListOutput, slug string) string {
	for _, a := range out.Areas {
		if a.Slug == slug {
			return a.Lifecycle
		}
	}
	return ""
}

// TestArtifactProposeRejectsRetiringAreaIntegration covers the T8
// propose-side filing guard: a new artifact cannot be created in a
// retiring area, while an active area still accepts it.
func TestArtifactProposeRejectsRetiringAreaIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.propose retiring-area integration")
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
	projectSlug := fmt.Sprintf("area-lifecycle-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	insertContextReceiptArea(t, ctx, pool, projectID, "active-zone")
	retiringID := insertContextReceiptArea(t, ctx, pool, projectID, "legacy-zone")
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()
	if _, err := pool.Exec(ctx, `UPDATE areas SET lifecycle = 'retiring' WHERE id = $1::uuid`, retiringID); err != nil {
		t.Fatalf("mark retiring: %v", err)
	}

	call := newArtifactProposeTestCaller(t, ctx, pool, nil)
	rejected := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     "legacy-zone",
		"type":          "Decision",
		"title":         "Filed into a retiring area",
		"slug":          fmt.Sprintf("retiring-reject-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("x", "y"),
		"author_id":     "codex-test",
	})
	if rejected.Status != "not_ready" || rejected.ErrorCode != "AREA_NOT_ACTIVE" {
		t.Fatalf("propose into retiring area = status=%q code=%q, want not_ready AREA_NOT_ACTIVE",
			rejected.Status, rejected.ErrorCode)
	}

	accepted := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     "active-zone",
		"type":          "Decision",
		"title":         "Filed into an active area",
		"slug":          fmt.Sprintf("active-ok-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("x", "y"),
		"author_id":     "codex-test",
	})
	if accepted.Status != "accepted" || !accepted.Created {
		t.Fatalf("propose into active area = %+v", accepted)
	}
}
