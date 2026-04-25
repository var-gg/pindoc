package tools

import (
	"strings"
	"testing"
)

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

	if len(topLevelAreaSeed) != len(want) {
		t.Fatalf("seed count = %d, want %d", len(topLevelAreaSeed), len(want))
	}

	seen := map[string]bool{}
	for i, seed := range topLevelAreaSeed {
		if seed.Slug != want[i] {
			t.Fatalf("seed %d slug = %q, want %q", i, seed.Slug, want[i])
		}
		if seen[seed.Slug] {
			t.Fatalf("duplicate seed slug %q", seed.Slug)
		}
		if seed.Name == "" {
			t.Fatalf("seed %q has empty name", seed.Slug)
		}
		if seed.DescriptionEN == "" || seed.DescriptionKO == "" {
			t.Fatalf("seed %q must carry en/ko descriptions", seed.Slug)
		}
		seen[seed.Slug] = true
	}

	for _, legacy := range []string{"vision", "architecture", "data-model", "mechanisms", "ui", "roadmap", "decisions"} {
		if seen[legacy] {
			t.Fatalf("legacy top-level area %q should not be seeded for new projects", legacy)
		}
	}

	for _, seed := range topLevelAreaSeed {
		if seed.IsCrossCutting != (seed.Slug == "cross-cutting") {
			t.Fatalf("seed %q is_cross_cutting = %v", seed.Slug, seed.IsCrossCutting)
		}
	}
}

func TestProjectCreateDescriptionAdvertisesAreaSeed(t *testing.T) {
	if !strings.Contains(projectCreateDescription, "Auto-creates 9 top-level/project-root areas") {
		t.Fatalf("project_create description should advertise 9 project-root areas")
	}
	if !strings.Contains(projectCreateDescription, "area-구조-top-level-고정-골격-depth-2-sub-area") {
		t.Fatalf("project_create description should reference the governing Decision slug")
	}
}

func TestProjectCreateDescriptionRequiresExplicitImmutableLanguage(t *testing.T) {
	for _, want := range []string{
		"primary_language is required",
		"No default",
		"Supported languages are en, ko, ja",
		"immutable after creation",
		"recreate the project",
	} {
		if !strings.Contains(projectCreateDescription, want) {
			t.Fatalf("project_create description missing locale guidance %q", want)
		}
	}
}

func TestNormalizeProjectLanguage(t *testing.T) {
	for _, raw := range []string{"en", "ko", "ja", " JA "} {
		got, err := normalizeProjectLanguage(raw)
		if err != nil {
			t.Fatalf("normalizeProjectLanguage(%q) returned error: %v", raw, err)
		}
		if got == "" || !isSupportedProjectLanguage(got) {
			t.Fatalf("normalizeProjectLanguage(%q) = %q, want supported language", raw, got)
		}
	}

	for _, raw := range []string{"", "fr"} {
		_, err := normalizeProjectLanguage(raw)
		if err == nil {
			t.Fatalf("normalizeProjectLanguage(%q) expected error", raw)
		}
		msg := err.Error()
		for _, want := range []string{"Supported languages: en, ko, ja", "immutable", "recreate"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("normalizeProjectLanguage(%q) error missing %q: %s", raw, want, msg)
			}
		}
	}
}

func TestLocalizedAreaDescription(t *testing.T) {
	if got := localizedAreaDescription("English", "한국어", "ko"); got != "한국어" {
		t.Fatalf("ko description = %q, want 한국어", got)
	}
	if got := localizedAreaDescription("English", "한국어", "en"); got != "English" {
		t.Fatalf("en description = %q, want English", got)
	}
	if got := localizedAreaDescription("English", "한국어", "ja"); got != "English" {
		t.Fatalf("ja description fallback = %q, want English", got)
	}
	if got := localizedAreaDescription("English", "", "ko"); got != "English" {
		t.Fatalf("missing ko fallback = %q, want English", got)
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
