package tools

import "testing"

func TestNormalizeBodyLocale(t *testing.T) {
	cases := map[string]string{
		" EN ": "en",
		"ko":   "ko",
		"JA":   "ja",
		"":     "",
	}
	for in, want := range cases {
		if got := normalizeBodyLocale(in); got != want {
			t.Fatalf("normalizeBodyLocale(%q) = %q; want %q", in, got, want)
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
