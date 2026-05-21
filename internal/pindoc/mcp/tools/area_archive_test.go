package tools

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// TestMCPTaxonomyChangeAreaRetireEmptyIntegration walks an
// area.retire_empty change-set through propose -> approve -> apply: an
// empty area is archived, while an area still holding an artifact is left
// blocked (lifecycle unchanged).
func TestMCPTaxonomyChangeAreaRetireEmptyIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	emptyAreaID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "retire-empty-zone")
	fullAreaID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "retire-full-zone")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, fullAreaID, "retire-blocker-art", projects.VisibilityOrg, fixture.ownerUserID, "ko")

	proposed := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug": fixture.projectSlug,
		"kind":         "area.retire_empty",
		"area_slugs":   []string{"retire-empty-zone", "retire-full-zone"},
	})
	if proposed.Status != "proposed" || proposed.ChangeID == "" {
		t.Fatalf("propose output = %+v", proposed)
	}

	approved := callVisibilityTool[taxonomyChangeApproveOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.approve", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if approved.Status != "approved" {
		t.Fatalf("approve output = %+v", approved)
	}

	applied := callVisibilityTool[taxonomyChangeApplyOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.apply", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if applied.Status != "applied" || applied.ArchivedCount != 1 || applied.BlockedCount != 1 {
		t.Fatalf("apply output = %+v, want applied with archived=1 blocked=1", applied)
	}

	var emptyLifecycle, fullLifecycle string
	if err := pool.QueryRow(ctx, `SELECT lifecycle FROM areas WHERE id = $1::uuid`, emptyAreaID).Scan(&emptyLifecycle); err != nil {
		t.Fatalf("read empty area: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT lifecycle FROM areas WHERE id = $1::uuid`, fullAreaID).Scan(&fullLifecycle); err != nil {
		t.Fatalf("read full area: %v", err)
	}
	if emptyLifecycle != "archived" {
		t.Fatalf("empty area lifecycle = %q, want archived", emptyLifecycle)
	}
	if fullLifecycle != "active" {
		t.Fatalf("area holding an artifact lifecycle = %q, want active (blocked from archive)", fullLifecycle)
	}

	var changeStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM taxonomy_changes WHERE id = $1::uuid`, proposed.ChangeID).Scan(&changeStatus); err != nil {
		t.Fatalf("read change row: %v", err)
	}
	if changeStatus != "applied" {
		t.Fatalf("taxonomy_changes status = %q, want applied", changeStatus)
	}
}
