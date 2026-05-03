package projects

import (
	"errors"
	"strings"
	"testing"
)

// TestTopLevelAreaSeed_ConcernSkeleton locks the canonical 9-row order +
// bilingual descriptions. Renaming or reordering rows changes URLs for
// every dogfood project, so this test is the regression baseline for the
// taxonomy frozen by Decision area-구조-top-level-고정-골격-depth-2-sub-area
// 만-프로젝트별-자유.
func TestTopLevelAreaSeed_ConcernSkeleton(t *testing.T) {
	want := []string{
		"strategy", "context", "experience", "system",
		"operations", "governance", "cross-cutting",
		"misc", "_unsorted",
	}

	if len(TopLevelAreaSeed) != len(want) {
		t.Fatalf("seed count = %d, want %d", len(TopLevelAreaSeed), len(want))
	}

	seen := map[string]bool{}
	for i, seed := range TopLevelAreaSeed {
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

	for _, seed := range TopLevelAreaSeed {
		if seed.IsCrossCutting != (seed.Slug == "cross-cutting") {
			t.Fatalf("seed %q is_cross_cutting = %v", seed.Slug, seed.IsCrossCutting)
		}
	}
}

// TestStarterSubAreaSeeds locks the per-parent counts + the
// "every cross-cutting child is_cross_cutting=true" invariant. New
// sub-areas under existing parents land here so dogfood projects stay
// in sync with self-host installs.
func TestStarterSubAreaSeeds(t *testing.T) {
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
	for _, seed := range StarterSubAreaSeeds {
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

// TestNormalizeLanguage covers the supported set, whitespace/case tolerance,
// and the two failure modes (empty → ErrLangRequired, unsupported →
// ErrLangInvalid). Error messages must surface the supported list and
// the immutable/recreate guidance — agents read these and pass the
// hint up to the user.
func TestNormalizeLanguage(t *testing.T) {
	for _, raw := range []string{"en", "ko", "ja", " JA "} {
		got, err := NormalizeLanguage(raw)
		if err != nil {
			t.Fatalf("NormalizeLanguage(%q) returned error: %v", raw, err)
		}
		if got == "" || !isSupportedLanguage(got) {
			t.Fatalf("NormalizeLanguage(%q) = %q, want supported language", raw, got)
		}
	}

	emptyCases := []struct {
		raw    string
		target error
	}{
		{"", ErrLangRequired},
		{"   ", ErrLangRequired},
		{"fr", ErrLangInvalid},
		{"EN-GB", ErrLangInvalid},
	}
	for _, c := range emptyCases {
		_, err := NormalizeLanguage(c.raw)
		if err == nil {
			t.Fatalf("NormalizeLanguage(%q) expected error", c.raw)
		}
		if !errors.Is(err, c.target) {
			t.Fatalf("NormalizeLanguage(%q) error = %v, want wrapping %v", c.raw, err, c.target)
		}
		msg := err.Error()
		for _, want := range []string{"Supported languages: en, ko, ja", "immutable", "recreate"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("NormalizeLanguage(%q) error missing %q: %s", c.raw, want, msg)
			}
		}
	}
}

func TestNormalizeVisibility(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want string
	}{
		{"public", VisibilityPublic},
		{" ORG ", VisibilityOrg},
		{"Private", VisibilityPrivate},
	} {
		if got := NormalizeVisibility(c.raw); got != c.want {
			t.Fatalf("NormalizeVisibility(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
	for _, raw := range []string{"", "deleted", "members"} {
		if got := NormalizeVisibility(raw); got != "" {
			t.Fatalf("NormalizeVisibility(%q) = %q, want empty", raw, got)
		}
	}
}

// TestValidateProjectSlug locks the regex shape + reserved-word block.
// New reserved words go in reservedSlugs, and this test smoke-checks a
// representative subset — exhaustive enumeration would just duplicate
// the map.
func TestValidateProjectSlug(t *testing.T) {
	valid := []string{"x1", "var-gg", "trading-platform", "abc-123-def"}
	for _, s := range valid {
		if err := ValidateProjectSlug(s); err != nil {
			t.Fatalf("ValidateProjectSlug(%q) returned %v, want nil", s, err)
		}
	}

	// Auto-lowering is part of the contract — agents that pass "Var-GG"
	// shouldn't fail validation, they should get a normalized "var-gg"
	// project slug. Documented at the regex declaration.
	for _, s := range []string{"FOO", "Var-GG", "  ABC  "} {
		if err := ValidateProjectSlug(s); err != nil {
			t.Fatalf("ValidateProjectSlug(%q) (auto-lowered) returned %v, want nil", s, err)
		}
	}

	invalidShape := []string{
		"",                      // empty
		"X",                     // single char (still single after lower)
		"-foo",                  // leading dash
		"1foo",                  // leading digit
		"foo_bar",               // underscore
		"foo.bar",               // dot
		strings.Repeat("a", 41), // too long
	}
	for _, s := range invalidShape {
		err := ValidateProjectSlug(s)
		if err == nil {
			t.Fatalf("ValidateProjectSlug(%q) returned nil, want error", s)
		}
		if !errors.Is(err, ErrSlugInvalid) {
			t.Fatalf("ValidateProjectSlug(%q) error = %v, want wrapping ErrSlugInvalid", s, err)
		}
	}

	// "p" is in reservedSlugs but unreachable — the regex blocks it
	// first as too-short. Single-char inputs never reach the reserved
	// check. Reserved entries 2+ chars are what actually fires.
	for _, reserved := range []string{"admin", "api", "wiki", "ui", "design"} {
		err := ValidateProjectSlug(reserved)
		if err == nil {
			t.Fatalf("ValidateProjectSlug(%q) returned nil, want error", reserved)
		}
		if !errors.Is(err, ErrSlugReserved) {
			t.Fatalf("ValidateProjectSlug(%q) error = %v, want wrapping ErrSlugReserved", reserved, err)
		}
	}
}

// TestLocalizedAreaDescription locks the ko-prefers-ko-otherwise-en
// fallback. Any future locale that wants its own descriptions adds a new
// branch in LocalizedAreaDescription and a case here.
func TestLocalizedAreaDescription(t *testing.T) {
	if got := LocalizedAreaDescription("English", "한국어", "ko"); got != "한국어" {
		t.Fatalf("ko description = %q, want 한국어", got)
	}
	if got := LocalizedAreaDescription("English", "한국어", "en"); got != "English" {
		t.Fatalf("en description = %q, want English", got)
	}
	if got := LocalizedAreaDescription("English", "한국어", "ja"); got != "English" {
		t.Fatalf("ja description fallback = %q, want English", got)
	}
	if got := LocalizedAreaDescription("English", "", "ko"); got != "English" {
		t.Fatalf("missing ko fallback = %q, want English", got)
	}
}

// TestTemplateSeeds_V1Set locks the Tier A templates plus the optional
// SessionHandoff convention template. Adding Feature / APIEndpoint /
// Screen / DataModel templates lands here when V1.x picks up the Web-SaaS
// Domain Pack.
func TestTemplateSeeds_V1Set(t *testing.T) {
	want := []string{
		"_template_debug",
		"_template_decision",
		"_template_analysis",
		"_template_task",
		"_template_session_handoff",
	}
	if len(TemplateSeeds) != len(want) {
		t.Fatalf("template count = %d, want %d", len(TemplateSeeds), len(want))
	}
	for i, w := range want {
		if TemplateSeeds[i].Slug != w {
			t.Fatalf("template %d slug = %q, want %q", i, TemplateSeeds[i].Slug, w)
		}
		if TemplateSeeds[i].Body == "" {
			t.Fatalf("template %q has empty body", w)
		}
	}
}
