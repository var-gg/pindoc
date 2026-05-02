package projects

import "testing"

// TestIsAccessibleAnonymously locks the visibility-tier access rule the
// /pindoc.org/{org}/p/{slug} public route depends on. Only 'public'
// passes — 'org' (safe default) and 'private' both block, so the
// route layer can emit a 404 rather than 403 (existence-leak).
func TestIsAccessibleAnonymously(t *testing.T) {
	cases := []struct {
		visibility string
		want       bool
	}{
		{VisibilityPublic, true},
		{VisibilityOrg, false},
		{VisibilityPrivate, false},
		{"", false},
		{"unexpected", false},
	}
	for _, c := range cases {
		t.Run(c.visibility, func(t *testing.T) {
			r := &ResolveResult{ProjectVisibility: c.visibility}
			if got := r.IsAccessibleAnonymously(); got != c.want {
				t.Errorf("IsAccessibleAnonymously(%q) = %v, want %v", c.visibility, got, c.want)
			}
		})
	}

	t.Run("nil receiver returns false", func(t *testing.T) {
		var r *ResolveResult
		if r.IsAccessibleAnonymously() {
			t.Error("nil ResolveResult should not be accessible")
		}
	})
}
