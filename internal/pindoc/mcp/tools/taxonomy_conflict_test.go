package tools

import "testing"

// TestMCPProfileAdoptConflictDetectionIntegration covers the T15
// structural conflict detector: a clean adoption reports no conflict,
// while an adoption whose target top-level slug collides with an existing
// area reports a structural conflict and is refused at apply.
func TestMCPProfileAdoptConflictDetectionIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	// Clean case: a plain software-product project adopting game-narrative
	// has no structural conflict, and no semantic candidate — the overlaps
	// are the universal misc / _unsorted / cross-cutting areas.
	clean := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":        fixture.projectSlug,
		"kind":                "profile.adopt",
		"target_profile_slug": "game-narrative",
	})
	if clean.Status != "proposed" || clean.Diff == nil {
		t.Fatalf("clean propose = %+v", clean)
	}
	if len(clean.Diff.StructuralConflicts) != 0 {
		t.Fatalf("clean adoption structural conflicts = %v, want none", clean.Diff.StructuralConflicts)
	}
	if len(clean.Diff.SemanticConflictCandidates) != 0 {
		t.Fatalf("clean adoption semantic candidates = %v, want none", clean.Diff.SemanticConflictCandidates)
	}

	// Conflict case: a sub-area whose slug collides with a game-narrative
	// top-level makes the adoption structurally unsafe.
	insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "combat")
	conflicted := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":        fixture.projectSlug,
		"kind":                "profile.adopt",
		"target_profile_slug": "game-narrative",
	})
	if conflicted.Status != "proposed" || conflicted.Diff == nil {
		t.Fatalf("conflicted propose = %+v", conflicted)
	}
	if len(conflicted.Diff.StructuralConflicts) == 0 {
		t.Fatal("adoption with a colliding 'combat' sub-area must report a structural conflict")
	}

	// apply refuses a structurally-conflicted change-set.
	approved := callVisibilityTool[taxonomyChangeApproveOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.approve", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    conflicted.ChangeID,
	})
	if approved.Status != "approved" {
		t.Fatalf("approve = %+v", approved)
	}
	applied := callVisibilityTool[taxonomyChangeApplyOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.apply", map[string]any{
		"project_slug": fixture.projectSlug,
		"change_id":    conflicted.ChangeID,
	})
	if applied.Status != "not_ready" || applied.ErrorCode != "CHANGE_STALE" {
		t.Fatalf("apply of a structurally-conflicted change = %+v, want not_ready CHANGE_STALE", applied)
	}
}
