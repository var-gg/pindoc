package auth

import (
	"context"
	"errors"
	"testing"
)

// TestProjectScopeCan_RoleActionMatrix locks the role × action
// permission table for V1. New roles or actions that flip this matrix
// should land here first so the regression bites at the table, not in
// production auth bypasses.
func TestProjectScopeCan_RoleActionMatrix(t *testing.T) {
	cases := []struct {
		role   string
		action string
		want   bool
	}{
		// Owner: V1 default. Satisfies every defined action.
		{role: "owner", action: "read.project", want: true},
		{role: "owner", action: "read.artifact", want: true},
		{role: "owner", action: "read.area", want: true},
		{role: "owner", action: "write.artifact", want: true},
		{role: "owner", action: "write.area", want: true},
		{role: "owner", action: "write.task", want: true},
		{role: "owner", action: "write.project", want: true},
		{role: "owner", action: "read.capabilities", want: true},

		// Editor: V1.5+. Reads and most writes; cannot create projects.
		{role: "editor", action: "read.artifact", want: true},
		{role: "editor", action: "write.artifact", want: true},
		{role: "editor", action: "write.task", want: true},
		{role: "editor", action: "write.project", want: false},

		// Viewer: V1.5+. Reads only.
		{role: "viewer", action: "read.project", want: true},
		{role: "viewer", action: "read.artifact", want: true},
		{role: "viewer", action: "write.artifact", want: false},
		{role: "viewer", action: "write.task", want: false},

		// Unknown action — fails closed regardless of role.
		{role: "owner", action: "bogus.action", want: false},
		{role: "owner", action: "", want: false},

		// Unknown role — also fails closed.
		{role: "robot", action: "read.project", want: false},
		{role: "", action: "read.project", want: false},
	}
	for _, c := range cases {
		t.Run(c.role+":"+c.action, func(t *testing.T) {
			s := &ProjectScope{Role: c.role}
			if got := s.Can(c.action); got != c.want {
				t.Fatalf("Can(%q) for role %q = %v; want %v", c.action, c.role, got, c.want)
			}
		})
	}
}

// TestProjectScopeCan_NilReceiver guards against panics when handlers
// forget to check ResolveProject's err — Can() must always return
// false rather than panic so the failure mode is "auth blocks the
// call" rather than "process crashes mid-request".
func TestProjectScopeCan_NilReceiver(t *testing.T) {
	var s *ProjectScope
	if s.Can("read.project") {
		t.Fatal("Can on nil receiver returned true; want false")
	}
}

// TestResolveProject_EmptySlug verifies the cheap synchronous path: an
// empty project_slug input must surface ErrProjectSlugRequired before
// any DB roundtrip so handlers can map it to PROJECT_SLUG_REQUIRED
// without sniffing error strings.
func TestResolveProject_EmptySlug(t *testing.T) {
	cases := []string{"", "  ", "\t\n"}
	for _, slug := range cases {
		_, err := ResolveProject(context.Background(), nil, &Principal{UserID: "u"}, slug)
		if !errors.Is(err, ErrProjectSlugRequired) {
			t.Fatalf("ResolveProject(slug=%q) err = %v; want ErrProjectSlugRequired", slug, err)
		}
	}
}

// TestResolveProject_NilPool is the boot-error path — handlers reach
// into auth.ResolveProject before the daemon's DB pool is wired.
// Returning a plain error keeps the failure surface obvious instead of
// a downstream nil-deref panic in the SQL roundtrip.
func TestResolveProject_NilPool(t *testing.T) {
	_, err := ResolveProject(context.Background(), nil, &Principal{UserID: "u"}, "pindoc")
	if err == nil {
		t.Fatal("expected error on nil pool; got nil")
	}
	if errors.Is(err, ErrProjectSlugRequired) || errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("nil-pool error should not collide with sentinels; got %v", err)
	}
}

// TestResolveRole_TrustedLocalAlwaysOwner pins the V1 contract: any
// resolved Principal under trusted_local mode gets owner role on
// every project. V1.5 OAuth will branch on AuthMode; this test should
// extend rather than disappear.
func TestResolveRole_TrustedLocalAlwaysOwner(t *testing.T) {
	for _, mode := range []string{AuthModeTrustedLocal, "", "anything-else"} {
		got := resolveRole(&Principal{AuthMode: mode})
		if got != RoleOwner {
			t.Fatalf("resolveRole(mode=%q) = %q; want %q", mode, got, RoleOwner)
		}
	}
	if got := resolveRole(nil); got != "" {
		t.Fatalf("resolveRole(nil) = %q; want empty string", got)
	}
}
