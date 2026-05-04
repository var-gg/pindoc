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

func TestCapabilitiesForVisibleCount(t *testing.T) {
	cases := []struct {
		name              string
		count             int
		wantSwitching     bool
		wantCreateAllowed bool
	}{
		{"empty table", 0, false, true},
		{"single project", 1, false, true},
		{"two visible projects", 2, true, true},
		{"many visible projects", 5, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CapabilitiesForVisibleCount(c.count)
			if got.MultiProjectSwitching != c.wantSwitching {
				t.Errorf("MultiProjectSwitching = %v, want %v", got.MultiProjectSwitching, c.wantSwitching)
			}
			if got.ProjectCreateAllowed != c.wantCreateAllowed {
				t.Errorf("ProjectCreateAllowed = %v, want %v", got.ProjectCreateAllowed, c.wantCreateAllowed)
			}
		})
	}
}

func TestArtifactVisibilityAllowedByProject(t *testing.T) {
	cases := []struct {
		project  string
		artifact string
		want     bool
	}{
		{VisibilityPublic, VisibilityPublic, true},
		{VisibilityPublic, VisibilityOrg, true},
		{VisibilityPublic, VisibilityPrivate, true},
		{VisibilityOrg, VisibilityPublic, false},
		{VisibilityOrg, VisibilityOrg, true},
		{VisibilityOrg, VisibilityPrivate, true},
		{VisibilityPrivate, VisibilityPublic, false},
		{VisibilityPrivate, VisibilityOrg, false},
		{VisibilityPrivate, VisibilityPrivate, true},
		{"deleted", VisibilityPrivate, false},
		{VisibilityPublic, "deleted", false},
	}
	for _, c := range cases {
		t.Run(c.project+"->"+c.artifact, func(t *testing.T) {
			if got := ArtifactVisibilityAllowedByProject(c.project, c.artifact); got != c.want {
				t.Fatalf("ArtifactVisibilityAllowedByProject(%q, %q) = %v, want %v", c.project, c.artifact, got, c.want)
			}
		})
	}
}

// TestBuildVisibilitySelect locks the three branches of the
// visibility-aware listing query so a future change here can't silently
// regress: anonymous/default scopes see only public, members see public
// plus membership-scoped rows, and explicit trusted_local single-user
// self-host callers retain full visibility.
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
			"project_members pm",
			"organization_members om",
		} {
			if !strings.Contains(q, want) {
				t.Errorf("members query missing %q in %q", want, q)
			}
		}
		if len(args) != 6 {
			t.Fatalf("expected 6 args (public, org, org_ids, org, private, user_id), got %d: %#v", len(args), args)
		}
		if args[0] != VisibilityPublic || args[1] != VisibilityOrg {
			t.Errorf("first two args should be public/org tier, got %#v", args[:2])
		}
	})

	t.Run("default scope: public only", func(t *testing.T) {
		q, args := buildVisibilitySelect(ViewerScope{}, base)
		if !strings.Contains(q, "WHERE visibility = $1") {
			t.Fatalf("default scope should fail closed to public-only, got %q", q)
		}
		if len(args) != 1 || args[0] != VisibilityPublic {
			t.Fatalf("default scope args = %#v, want public-only", args)
		}
	})

	t.Run("trusted_local explicit: no WHERE", func(t *testing.T) {
		q, args := buildVisibilitySelect(ViewerScope{TrustedLocal: true}, base)
		if strings.Contains(q, "WHERE") {
			t.Errorf("trusted_local should not add WHERE, got %q", q)
		}
		if args != nil {
			t.Errorf("trusted_local should return nil args, got %#v", args)
		}
	})

	t.Run("legacy user string normalizes as trusted_local", func(t *testing.T) {
		scope := normalizeScope("user-id-string")
		if !scope.TrustedLocal || scope.UserID != "user-id-string" {
			t.Fatalf("legacy non-empty string scope = %#v, want trusted_local with user id", scope)
		}
		q, args := buildVisibilitySelect(scope, base)
		if strings.Contains(q, "WHERE") {
			t.Errorf("legacy string scope should not filter, got %q", q)
		}
		if args != nil {
			t.Errorf("legacy string scope should return nil args, got %#v", args)
		}
	})

	t.Run("empty legacy string is anonymous", func(t *testing.T) {
		scope := normalizeScope(" ")
		if !scope.AnonymousOnly || scope.TrustedLocal {
			t.Fatalf("empty string scope = %#v, want anonymous-only", scope)
		}
		q, args := buildVisibilitySelect(scope, base)
		if !strings.Contains(q, "WHERE visibility = $1") {
			t.Fatalf("empty string scope should be public-only, got %q", q)
		}
		if len(args) != 1 || args[0] != VisibilityPublic {
			t.Fatalf("empty string args = %#v, want public-only", args)
		}
	})
}
