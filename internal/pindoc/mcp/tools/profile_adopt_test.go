package tools

import (
	"testing"
)

// TestMCPProfileAdoptPlannerIntegration covers the T13 planner: a
// profile.adopt propose on a software-product project targeting
// game-narrative produces a diff (top-levels to create / reuse / retire)
// and records a change-set — without mutating any area.
func TestMCPProfileAdoptPlannerIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	out := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":        fixture.projectSlug,
		"kind":                "profile.adopt",
		"target_profile_slug": "game-narrative",
	})
	if out.Status != "proposed" || out.ChangeID == "" || out.Diff == nil {
		t.Fatalf("profile.adopt propose = %+v", out)
	}

	diff := out.Diff
	if diff.SourceProfileSlug != "software-product" || diff.TargetProfileSlug != "game-narrative" {
		t.Fatalf("diff profiles = %q -> %q", diff.SourceProfileSlug, diff.TargetProfileSlug)
	}
	// cross-cutting / misc / _unsorted appear in both profiles -> reused.
	assertSliceHas(t, "top_level_reused", diff.TopLevelReused, "cross-cutting", "misc", "_unsorted")
	// game-narrative-only top-levels -> to_create.
	assertSliceHas(t, "top_level_to_create", diff.TopLevelToCreate, "characters", "combat", "narrative", "atlas")
	// software-product-only top-levels -> to_retire.
	retireSlugs := make([]string, 0, len(diff.TopLevelToRetire))
	for _, r := range diff.TopLevelToRetire {
		retireSlugs = append(retireSlugs, r.Slug)
	}
	assertSliceHas(t, "top_level_to_retire", retireSlugs, "strategy", "system", "experience")

	// The planner mutates nothing — no game-narrative area exists yet.
	var created bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM areas WHERE project_id = $1::uuid AND slug = 'characters')
	`, fixture.projectID).Scan(&created); err != nil {
		t.Fatalf("check planner mutation: %v", err)
	}
	if created {
		t.Fatal("profile.adopt planner must not create areas")
	}

	// The change-set row is recorded — kind profile.adopt, still proposed.
	var kind, status, srcProfile, tgtProfile string
	if err := pool.QueryRow(ctx, `
		SELECT kind, status, COALESCE(source_profile_slug, ''), COALESCE(target_profile_slug, '')
		  FROM taxonomy_changes WHERE id = $1::uuid
	`, out.ChangeID).Scan(&kind, &status, &srcProfile, &tgtProfile); err != nil {
		t.Fatalf("read change row: %v", err)
	}
	if kind != "profile.adopt" || status != "proposed" || srcProfile != "software-product" || tgtProfile != "game-narrative" {
		t.Fatalf("change row = kind=%q status=%q %s->%s", kind, status, srcProfile, tgtProfile)
	}
}

func assertSliceHas(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Fatalf("%s %v is missing %q", label, got, w)
		}
	}
}
