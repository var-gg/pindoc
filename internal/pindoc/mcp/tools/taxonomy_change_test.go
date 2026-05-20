package tools

import "testing"

// TestValidateTaxonomyChangePropose locks the static gate of
// pindoc.taxonomy.change.propose: slug shape, required fields, facet
// rejection (document form / workflow status / actor), and the one-off
// initiative heuristic. Collision and count-cap checks need a project
// row and are exercised by the integration test.
func TestValidateTaxonomyChangePropose(t *testing.T) {
	base := taxonomyChangeProposeInput{
		CandidateSlug: "characters",
		Name:          "Characters",
		Description:   "Character lore, cast structure, factions, and relationships.",
		Evidence:      "30+ artifacts about characters mis-filed under system/experience over 30 days.",
	}
	if _, nr := validateTaxonomyChangePropose(base); nr != nil {
		t.Fatalf("valid candidate rejected: %+v", nr)
	}

	tests := []struct {
		name string
		mut  func(*taxonomyChangeProposeInput)
		want string
	}{
		{"slug invalid", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "Bad Slug" }, "CANDIDATE_SLUG_INVALID"},
		{"slug too short", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "x" }, "CANDIDATE_SLUG_INVALID"},
		{"name too short", func(in *taxonomyChangeProposeInput) { in.Name = "x" }, "NAME_INVALID"},
		{"description required", func(in *taxonomyChangeProposeInput) { in.Description = "" }, "DESCRIPTION_REQUIRED"},
		{"evidence required", func(in *taxonomyChangeProposeInput) { in.Evidence = "   " }, "EVIDENCE_REQUIRED"},
		{"facet document type", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "decision" }, "CANDIDATE_SLUG_IS_FACET"},
		{"facet workflow status", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "blocked" }, "CANDIDATE_SLUG_IS_FACET"},
		{"facet actor", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "codex" }, "CANDIDATE_SLUG_IS_FACET"},
		{"one-off phase", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "phase-3" }, "CANDIDATE_SLUG_ONE_OFF"},
		{"one-off patch", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "april-patch" }, "CANDIDATE_SLUG_ONE_OFF"},
		{"one-off dated", func(in *taxonomyChangeProposeInput) { in.CandidateSlug = "launch-2026" }, "CANDIDATE_SLUG_ONE_OFF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mut(&in)
			_, nr := validateTaxonomyChangePropose(in)
			if nr == nil {
				t.Fatalf("%s: expected not_ready, got nil", tc.name)
			}
			if nr.ErrorCode != tc.want {
				t.Fatalf("%s: error_code = %q, want %q", tc.name, nr.ErrorCode, tc.want)
			}
		})
	}
}

// TestMCPTaxonomyChangeProposeIntegration exercises the handler end to
// end: a valid proposal is recorded as a taxonomy.top_level_proposed
// event WITHOUT creating an area, a slug colliding with an existing
// top-level is rejected, and a facet slug is rejected.
func TestMCPTaxonomyChangeProposeIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	out := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":   fixture.projectSlug,
		"candidate_slug": "characters",
		"name":           "Characters",
		"description":    "Character lore, cast structure, factions, and relationships.",
		"evidence":       "30+ character artifacts mis-filed under system/experience.",
	})
	if out.Status != "proposed" || out.CandidateSlug != "characters" {
		t.Fatalf("propose output = %+v, want status=proposed slug=characters", out)
	}

	var areaCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM areas WHERE project_id = $1::uuid AND slug = 'characters'
	`, fixture.projectID).Scan(&areaCount); err != nil {
		t.Fatalf("count characters area: %v", err)
	}
	if areaCount != 0 {
		t.Fatalf("taxonomy.change.propose created an area; want 0, got %d", areaCount)
	}

	var eventCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM events
		 WHERE project_id = $1::uuid
		   AND kind = 'taxonomy.top_level_proposed'
		   AND payload->>'candidate_slug' = 'characters'
	`, fixture.projectID).Scan(&eventCount); err != nil {
		t.Fatalf("count proposal event: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("taxonomy.top_level_proposed event count = %d, want 1", eventCount)
	}

	collision := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":   fixture.projectSlug,
		"candidate_slug": "system",
		"name":           "System",
		"description":    "duplicate of an existing top-level area",
		"evidence":       "duplicate",
	})
	if collision.Status != "not_ready" || collision.ErrorCode != "CANDIDATE_SLUG_EXISTS" {
		t.Fatalf("collision output = %+v, want not_ready/CANDIDATE_SLUG_EXISTS", collision)
	}

	facet := callVisibilityTool[taxonomyChangeProposeOutput](t, ctx, pool, nil, owner, "pindoc.taxonomy.change.propose", map[string]any{
		"project_slug":   fixture.projectSlug,
		"candidate_slug": "decision",
		"name":           "Decision",
		"description":    "document form, not a subject concern",
		"evidence":       "n/a",
	})
	if facet.Status != "not_ready" || facet.ErrorCode != "CANDIDATE_SLUG_IS_FACET" {
		t.Fatalf("facet output = %+v, want not_ready/CANDIDATE_SLUG_IS_FACET", facet)
	}
}
