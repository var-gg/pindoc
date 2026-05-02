package tools

import "testing"

func TestNormalizeBodyLocale(t *testing.T) {
	cases := map[string]string{
		" EN ":  "en",
		"ko":    "ko",
		"JA":    "ja",
		"en-US": "en-us",
		"":      "",
	}
	for in, want := range cases {
		if got := normalizeBodyLocale(in); got != want {
			t.Fatalf("normalizeBodyLocale(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestValidBodyLocaleSafeSubset(t *testing.T) {
	valid := []string{"ko", "en", "ja", "ko-KR", "en-US", "en-GB", "ja-JP", " EN-us "}
	for _, in := range valid {
		if !validBodyLocale(in) {
			t.Fatalf("validBodyLocale(%q) = false; want true", in)
		}
	}
	invalid := []string{"", "fr", "ko-Hang", "zh-Hans", "en-Latn-US", "english"}
	for _, in := range invalid {
		if validBodyLocale(in) {
			t.Fatalf("validBodyLocale(%q) = true; want false", in)
		}
	}
}

func TestArtifactTranslateToolRegistered(t *testing.T) {
	found := false
	for _, name := range RegisteredTools {
		if name == "pindoc.artifact.translate" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("pindoc.artifact.translate missing from RegisteredTools")
	}
}
