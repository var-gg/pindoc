package projects

import "testing"

// TestIsMultiProject locks the rule MCP capabilities + HTTP /api/config
// share: the Reader project switcher only appears once a second project
// row exists. Single-project installs (count == 1, the freshly seeded
// `pindoc` row) stay chrome-less; zero is treated the same way so a
// transient empty table never spuriously flips the switcher on. The
// rule is duplicated nowhere else — both call sites import this
// function so a future change here is one edit, not three.
func TestIsMultiProject(t *testing.T) {
	cases := []struct {
		name  string
		count int
		want  bool
	}{
		{"empty table", 0, false},
		{"single seeded project", 1, false},
		{"two projects (operator created a second)", 2, true},
		{"many projects", 5, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsMultiProject(c.count); got != c.want {
				t.Errorf("IsMultiProject(%d) = %v, want %v", c.count, got, c.want)
			}
		})
	}
}
