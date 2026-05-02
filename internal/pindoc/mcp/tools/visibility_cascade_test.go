package tools

import "testing"

// TestResolveArtifactVisibility locks the cascade contract artifact.propose
// and the project default setting share. Three cases:
//   - explicit valid: takes precedence over everything
//   - explicit invalid / empty: falls through to project default
//   - both invalid / empty: lands on the global 'org' safe default
func TestResolveArtifactVisibility(t *testing.T) {
	cases := []struct {
		name     string
		explicit string
		project  string
		want     string
	}{
		{"explicit public wins over project private", "public", "private", "public"},
		{"explicit private wins over project public", "private", "public", "private"},
		{"explicit org wins over project public", "org", "public", "org"},
		{"empty explicit, project public", "", "public", "public"},
		{"empty explicit, project private", "", "private", "private"},
		{"empty explicit, project org", "", "org", "org"},
		{"both empty -> 'org' safe default", "", "", "org"},
		{"invalid explicit falls through to project", "deleted", "public", "public"},
		{"both invalid -> 'org' safe default", "garbage", "junk", "org"},
		{"trim + lowercase", "  PUBLIC  ", "", "public"},
		{"explicit takes case-folded value", "Private", "org", "private"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveArtifactVisibility(c.explicit, c.project); got != c.want {
				t.Errorf("resolveArtifactVisibility(%q, %q) = %q, want %q",
					c.explicit, c.project, got, c.want)
			}
		})
	}
}

func TestNormalizeVisibility(t *testing.T) {
	cases := map[string]string{
		"public":      "public",
		"org":         "org",
		"private":     "private",
		"PUBLIC":      "public",
		"  Private  ": "private",
		"":            "",
		"deleted":     "",
		"unknown":     "",
	}
	for in, want := range cases {
		if got := normalizeVisibility(in); got != want {
			t.Errorf("normalizeVisibility(%q) = %q, want %q", in, got, want)
		}
	}
}
