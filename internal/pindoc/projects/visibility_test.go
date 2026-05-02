package projects

import (
	"strings"
	"testing"
)

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

// TestBuildVisibilitySelect locks the three branches of the
// visibility-aware listing query so a future change here can't silently
// regress: anonymous viewers see only public, members see public+org
// scoped to their orgs, and the trusted_local single-user self-host
// fallback returns everything for compatibility with existing callers
// that haven't migrated to the scoped API.
func TestBuildVisibilitySelect(t *testing.T) {
	const base = "SELECT count(*) FROM projects"

	t.Run("anonymous: public only", func(t *testing.T) {
		q, args := buildVisibilitySelect(ViewerScope{AnonymousOnly: true}, base)
		if !strings.Contains(q, "WHERE visibility = $1") {
			t.Fatalf("expected anon WHERE visibility = $1, got %q", q)
		}
		if len(args) != 1 || args[0] != VisibilityPublic {
			t.Fatalf("expected single 'public' arg, got %#v", args)
		}
	})

	t.Run("members: public OR org-scoped", func(t *testing.T) {
		scope := ViewerScope{
			UserID: "u-1",
			OrgIDs: []string{"org-uuid-a", "org-uuid-b"},
		}
		q, args := buildVisibilitySelect(scope, base)
		for _, want := range []string{
			"visibility = $1",
			"visibility = $2",
			"organization_id::text = ANY($3)",
		} {
			if !strings.Contains(q, want) {
				t.Errorf("members query missing %q in %q", want, q)
			}
		}
		if len(args) != 3 {
			t.Fatalf("expected 3 args (public, org, org_ids), got %d: %#v", len(args), args)
		}
		if args[0] != VisibilityPublic || args[1] != VisibilityOrg {
			t.Errorf("first two args should be public/org tier, got %#v", args[:2])
		}
	})

	t.Run("trusted_local fallback: no WHERE", func(t *testing.T) {
		q, args := buildVisibilitySelect(ViewerScope{}, base)
		if strings.Contains(q, "WHERE") {
			t.Errorf("trusted_local fallback should not add WHERE, got %q", q)
		}
		if args != nil {
			t.Errorf("trusted_local fallback should return nil args, got %#v", args)
		}
	})

	t.Run("legacy string scope behaves as trusted_local", func(t *testing.T) {
		scope := normalizeScope("user-id-string")
		q, args := buildVisibilitySelect(scope, base)
		if strings.Contains(q, "WHERE") {
			t.Errorf("legacy string scope should not filter, got %q", q)
		}
		if args != nil {
			t.Errorf("legacy string scope should return nil args, got %#v", args)
		}
	})
}
