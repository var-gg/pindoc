package titleguide

import (
	"strings"
	"testing"
)

func TestResolveFallback(t *testing.T) {
	cases := map[string]string{
		"":            "en",
		"en":          "en",
		"EN":          "en",
		"ko":          "ko",
		"ko-KR":       "ko",
		"ko_KR":       "ko",
		"ja":          "ja",
		"ja-JP":       "ja",
		"zh-Hant-TW":  "en",
		"unknown":     "en",
		"  ko  ":      "ko",
	}
	for input, want := range cases {
		if got := Resolve(input).Locale; got != want {
			t.Errorf("Resolve(%q).Locale = %q, want %q", input, got, want)
		}
	}
}

func TestEvaluateTitle_LengthBounds(t *testing.T) {
	// Korean MaxRunes=60, MinRunes=8.
	tooShort := "짧음"
	got := EvaluateTitle(tooShort, "ko", ProjectOverride{})
	if !hasCodePrefix(got, "TITLE_TOO_SHORT") {
		t.Errorf("expected TITLE_TOO_SHORT for %q under ko, got %v", tooShort, got)
	}

	tooLong := strings.Repeat("가", 80)
	got = EvaluateTitle(tooLong, "ko", ProjectOverride{})
	if !hasCodePrefix(got, "TITLE_TOO_LONG") {
		t.Errorf("expected TITLE_TOO_LONG for 80-char ko title, got %v", got)
	}

	// Same length but locale=en (max 80) — should NOT fire long.
	got = EvaluateTitle(tooLong, "en", ProjectOverride{})
	if hasCodePrefix(got, "TITLE_TOO_LONG") {
		t.Errorf("80 runes should be in-band for en, got %v", got)
	}
}

func TestEvaluateTitle_EmptyDoesNotFireTooShort(t *testing.T) {
	// The propose pipeline already rejects empty titles via TITLE_EMPTY,
	// so EvaluateTitle treats 0 runes as "out of scope" — no double
	// signal.
	got := EvaluateTitle("", "ko", ProjectOverride{})
	if hasCodePrefix(got, "TITLE_TOO_SHORT") {
		t.Errorf("empty title should not produce TITLE_TOO_SHORT; got %v", got)
	}
}

func TestEvaluateTitle_GenericTokens(t *testing.T) {
	got := EvaluateTitle("기타 변경 사항 처리", "ko", ProjectOverride{})
	if !hasCodePrefix(got, "TITLE_GENERIC_TOKENS") {
		t.Errorf("expected TITLE_GENERIC_TOKENS for ko jargon-heavy title, got %v", got)
	}

	got = EvaluateTitle("Various stuff to handle", "en", ProjectOverride{})
	if !hasCodePrefix(got, "TITLE_GENERIC_TOKENS") {
		t.Errorf("expected TITLE_GENERIC_TOKENS for en jargon-heavy title, got %v", got)
	}

	// Specific title in en should not trip generic.
	got = EvaluateTitle("Add retry backoff to payment gateway", "en", ProjectOverride{})
	if hasCodePrefix(got, "TITLE_GENERIC_TOKENS") {
		t.Errorf("specific title should not trip TITLE_GENERIC_TOKENS, got %v", got)
	}
}

func TestEvaluateTitle_ProjectOverride(t *testing.T) {
	// Project says "Layer 2" is jargon for them. Baseline doesn't list
	// it (project-specific). Override should make it fire.
	override := ProjectOverride{ExtraJargon: []string{"Layer 2"}}
	got := EvaluateTitle("Wiki Sidebar에 Layer 2 read state 카운터 노출", "ko", override)
	if !hasCodePrefix(got, "TITLE_GENERIC_TOKENS") {
		t.Errorf("project override should flag 'Layer 2' as jargon, got %v", got)
	}
	// matched token must appear in the code suffix so the agent knows
	// what to fix.
	for _, w := range got {
		if strings.HasPrefix(w.Code, "TITLE_GENERIC_TOKENS:") && !strings.Contains(w.Code, "layer 2") {
			t.Errorf("TITLE_GENERIC_TOKENS code must list matched token, got %q", w.Code)
		}
	}
}

func TestEvaluateTitle_OverrideAndBaselineDedup(t *testing.T) {
	// Override repeating a baseline token must not produce a duplicate
	// match in the code suffix.
	override := ProjectOverride{ExtraJargon: []string{"기타", "기타"}}
	got := EvaluateTitle("기타 처리", "ko", override)
	for _, w := range got {
		if !strings.HasPrefix(w.Code, "TITLE_GENERIC_TOKENS:") {
			continue
		}
		// Count "기타" — must appear exactly once in the suffix.
		suffix := strings.TrimPrefix(w.Code, "TITLE_GENERIC_TOKENS:")
		count := strings.Count(suffix, "기타")
		if count != 1 {
			t.Errorf("duplicate '기타' in %q (count=%d)", w.Code, count)
		}
	}
}

func TestSlugVerboseThreshold(t *testing.T) {
	// ko: max 60 → threshold 30
	if got := SlugVerboseThreshold("ko"); got != 30 {
		t.Errorf("SlugVerboseThreshold(ko) = %d, want 30", got)
	}
	// en: max 80 → 40
	if got := SlugVerboseThreshold("en"); got != 40 {
		t.Errorf("SlugVerboseThreshold(en) = %d, want 40", got)
	}
	// unknown locale falls through to en threshold.
	if got := SlugVerboseThreshold("xx"); got != 40 {
		t.Errorf("SlugVerboseThreshold(xx) = %d, want 40 (en fallback)", got)
	}
}

func hasCodePrefix(warnings []Warning, prefix string) bool {
	for _, w := range warnings {
		if strings.HasPrefix(w.Code, prefix) {
			return true
		}
	}
	return false
}
