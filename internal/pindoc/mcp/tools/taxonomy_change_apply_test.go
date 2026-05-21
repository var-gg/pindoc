package tools

import (
	"context"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// TestMCPTaxonomyChangeLifecycleIntegration walks a top_level.add
// change-set through propose -> approve -> apply and verifies the
// top-level area is created only by apply — not by propose or approve.
func TestMCPTaxonomyChangeLifecycleIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	proposed := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":   fixture.projectSlug,
		"candidate_slug": "playtest",
		"name":           "Playtest",
		"description":    "Playtest sessions, feedback, and balance findings.",
		"evidence":       "Recurring playtest notes are scattered across misc and experience with no durable home.",
		"fileable":       true,
		"max_depth":      1,
	})
	if proposed.Status != "proposed" || proposed.ChangeID == "" || proposed.PlanHash == "" {
		t.Fatalf("propose output = %+v", proposed)
	}
	assertTopLevelAreaAbsent(t, ctx, pool, fixture.projectID, "playtest")

	approved := callVisibilityTool[taxonomyChangeApproveOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.approve", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if approved.Status != "approved" || approved.ChangeStatus != "approved" {
		t.Fatalf("approve output = %+v", approved)
	}
	assertTopLevelAreaAbsent(t, ctx, pool, fixture.projectID, "playtest")

	applied := callVisibilityTool[taxonomyChangeApplyOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.apply", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if applied.Status != "applied" || applied.AreaSlug != "playtest" || applied.AreaID == "" {
		t.Fatalf("apply output = %+v", applied)
	}

	var lifecycle string
	var parentID *string
	if err := pool.QueryRow(ctx, `
		SELECT lifecycle, parent_id::text FROM areas
		 WHERE project_id = $1::uuid AND slug = 'playtest'
	`, fixture.projectID).Scan(&lifecycle, &parentID); err != nil {
		t.Fatalf("read created area: %v", err)
	}
	if lifecycle != "active" || parentID != nil {
		t.Fatalf("created area lifecycle=%q parent=%v, want active top-level", lifecycle, parentID)
	}
	var changeStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM taxonomy_changes WHERE id = $1::uuid`, proposed.ChangeID).Scan(&changeStatus); err != nil {
		t.Fatalf("read change row: %v", err)
	}
	if changeStatus != "applied" {
		t.Fatalf("taxonomy_changes status = %q, want applied", changeStatus)
	}

	// A second apply is rejected — the change-set is no longer approved.
	reapplied := callVisibilityTool[taxonomyChangeApplyOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.apply", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if reapplied.Status != "not_ready" || reapplied.ErrorCode != "CHANGE_NOT_APPROVED" {
		t.Fatalf("re-apply output = %+v, want not_ready CHANGE_NOT_APPROVED", reapplied)
	}
}

// TestMCPTaxonomyChangeApplyStaleIntegration covers drift detection: a
// change-set whose candidate slug is taken between approve and apply is
// marked stale instead of applied.
func TestMCPTaxonomyChangeApplyStaleIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	proposed := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":   fixture.projectSlug,
		"candidate_slug": "telemetry",
		"name":           "Telemetry",
		"description":    "Runtime telemetry and metrics surfaces.",
		"evidence":       "Telemetry artifacts keep landing in misc with no durable home.",
		"fileable":       true,
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

	// The slug is taken out from under the approved change-set.
	if _, err := pool.Exec(ctx, `
		INSERT INTO areas (project_id, slug, name) VALUES ($1::uuid, 'telemetry', 'Telemetry')
	`, fixture.projectID); err != nil {
		t.Fatalf("insert colliding area: %v", err)
	}

	stale := callVisibilityTool[taxonomyChangeApplyOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.apply", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    proposed.ChangeID,
	})
	if stale.Status != "not_ready" || stale.ErrorCode != "CHANGE_STALE" {
		t.Fatalf("apply on drifted change = %+v, want not_ready CHANGE_STALE", stale)
	}
	var changeStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM taxonomy_changes WHERE id = $1::uuid`, proposed.ChangeID).Scan(&changeStatus); err != nil {
		t.Fatalf("read change row: %v", err)
	}
	if changeStatus != "stale" {
		t.Fatalf("taxonomy_changes status = %q, want stale", changeStatus)
	}
}

func assertTopLevelAreaAbsent(t *testing.T, ctx context.Context, pool *db.Pool, projectID, slug string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM areas WHERE project_id = $1::uuid AND slug = $2)
	`, projectID, slug).Scan(&exists); err != nil {
		t.Fatalf("check area %s: %v", slug, err)
	}
	if exists {
		t.Fatalf("area %q exists but should not exist yet", slug)
	}
}
