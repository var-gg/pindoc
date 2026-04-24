package tools

import "testing"

func TestProjectCreateAreaSeedsUseConcernSkeleton(t *testing.T) {
	want := []string{
		"strategy",
		"context",
		"experience",
		"system",
		"operations",
		"governance",
		"cross-cutting",
		"misc",
		"_unsorted",
	}

	if len(projectCreateTopLevelAreaSeeds) != len(want) {
		t.Fatalf("seed count = %d, want %d", len(projectCreateTopLevelAreaSeeds), len(want))
	}

	seen := map[string]bool{}
	for i, seed := range projectCreateTopLevelAreaSeeds {
		if seed.Slug != want[i] {
			t.Fatalf("seed %d slug = %q, want %q", i, seed.Slug, want[i])
		}
		if seed.ParentSlug != "" {
			t.Fatalf("top-level seed %q has parent %q", seed.Slug, seed.ParentSlug)
		}
		if seen[seed.Slug] {
			t.Fatalf("duplicate seed slug %q", seed.Slug)
		}
		seen[seed.Slug] = true
	}

	for _, legacy := range []string{"vision", "architecture", "data-model", "mechanisms", "ui", "roadmap", "decisions"} {
		if seen[legacy] {
			t.Fatalf("legacy top-level area %q should not be seeded for new projects", legacy)
		}
	}

	for _, seed := range projectCreateTopLevelAreaSeeds {
		if seed.IsCrossCutting != (seed.Slug == "cross-cutting") {
			t.Fatalf("seed %q is_cross_cutting = %v", seed.Slug, seed.IsCrossCutting)
		}
	}
}

func TestProjectCreateStarterSubAreaSeeds(t *testing.T) {
	wantCounts := map[string]int{
		"context":       6,
		"experience":    5,
		"system":        7,
		"operations":    6,
		"governance":    5,
		"cross-cutting": 6,
	}

	gotCounts := map[string]int{}
	seen := map[string]bool{}
	for _, seed := range projectCreateStarterSubAreaSeeds {
		if seed.ParentSlug == "" {
			t.Fatalf("starter seed %q is missing parent", seed.Slug)
		}
		if seen[seed.Slug] {
			t.Fatalf("duplicate starter slug %q", seed.Slug)
		}
		seen[seed.Slug] = true
		gotCounts[seed.ParentSlug]++
		if seed.IsCrossCutting != (seed.ParentSlug == "cross-cutting") {
			t.Fatalf("seed %s/%s is_cross_cutting = %v", seed.ParentSlug, seed.Slug, seed.IsCrossCutting)
		}
	}

	for parent, want := range wantCounts {
		if got := gotCounts[parent]; got != want {
			t.Fatalf("starter count for %q = %d, want %d", parent, got, want)
		}
	}
}
