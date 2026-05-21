package tools

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// TestMCPProfileAdoptApplyIntegration covers the T14 apply: a
// software-product project adopts game-narrative through
// propose -> approve -> apply. After apply the target top-levels exist
// and are active, the non-overlapping legacy top-levels are no longer
// active, the project profile pin moved, and a pre-existing artifact in a
// reused area survives untouched.
func TestMCPProfileAdoptApplyIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	var miscID string
	if err := pool.QueryRow(ctx, `
		SELECT id::text FROM areas WHERE project_id = $1::uuid AND slug = 'misc'
	`, fixture.projectID).Scan(&miscID); err != nil {
		t.Fatalf("resolve misc area: %v", err)
	}
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, miscID, "adopt-survivor", projects.VisibilityOrg, fixture.ownerUserID, "ko")

	proposed := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":        fixture.projectSlug,
		"kind":                "profile.adopt",
		"target_profile_slug": "game-narrative",
	})
	if proposed.Status != "proposed" || proposed.ChangeID == "" {
		t.Fatalf("propose = %+v", proposed)
	}
	approved := callVisibilityTool[taxonomyChangeApproveOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.approve", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if approved.Status != "approved" {
		t.Fatalf("approve = %+v", approved)
	}
	applied := callVisibilityTool[taxonomyChangeApplyOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.apply", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if applied.Status != "applied" {
		t.Fatalf("apply = %+v", applied)
	}

	lifecycleOf := func(slug string) string {
		t.Helper()
		var lc string
		if err := pool.QueryRow(ctx, `
			SELECT lifecycle FROM areas WHERE project_id = $1::uuid AND slug = $2
		`, fixture.projectID, slug).Scan(&lc); err != nil {
			t.Fatalf("read area %s: %v", slug, err)
		}
		return lc
	}
	// game-narrative top-levels were created and are active.
	if lc := lifecycleOf("characters"); lc != "active" {
		t.Fatalf("characters lifecycle = %q, want active", lc)
	}
	// cross-cutting overlaps both profiles — reused, still active.
	if lc := lifecycleOf("cross-cutting"); lc != "active" {
		t.Fatalf("cross-cutting lifecycle = %q, want active", lc)
	}
	// a software-product-only top-level is no longer active.
	if lc := lifecycleOf("strategy"); lc == "active" {
		t.Fatalf("strategy lifecycle = %q, want retiring or archived", lc)
	}

	var pin string
	if err := pool.QueryRow(ctx, `
		SELECT taxonomy_profile_slug FROM projects WHERE id = $1::uuid
	`, fixture.projectID).Scan(&pin); err != nil {
		t.Fatalf("read profile pin: %v", err)
	}
	if pin != "game-narrative" {
		t.Fatalf("project profile pin = %q, want game-narrative", pin)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM taxonomy_changes WHERE id = $1::uuid`, proposed.ChangeID).Scan(&status); err != nil {
		t.Fatalf("read change status: %v", err)
	}
	if status != "applied" {
		t.Fatalf("change-set status = %q, want applied", status)
	}

	// The pre-existing artifact in the reused misc area is untouched.
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "adopt-survivor", "misc")
}
