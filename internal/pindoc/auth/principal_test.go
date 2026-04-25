package auth

import (
	"testing"
	"time"
)

// TestPrincipalCan_RoleActionMatrix locks the role × action permission
// table for V1. New roles or actions that flip this matrix should land
// here first so the regression bites at the table, not in production
// auth bypasses.
func TestPrincipalCan_RoleActionMatrix(t *testing.T) {
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
		{role: "owner", action: "write.user_self", want: true},
		{role: "owner", action: "read.capabilities", want: true},

		// Editor: V1.5+. Reads and most writes; cannot create projects.
		{role: "editor", action: "read.artifact", want: true},
		{role: "editor", action: "write.artifact", want: true},
		{role: "editor", action: "write.task", want: true},
		{role: "editor", action: "write.project", want: false},
		{role: "editor", action: "write.user_self", want: true},

		// Viewer: V1.5+. Reads only.
		{role: "viewer", action: "read.project", want: true},
		{role: "viewer", action: "read.artifact", want: true},
		{role: "viewer", action: "write.artifact", want: false},
		{role: "viewer", action: "write.task", want: false},
		{role: "viewer", action: "write.user_self", want: false},

		// Unknown action — fails closed regardless of role.
		{role: "owner", action: "bogus.action", want: false},
		{role: "owner", action: "", want: false},

		// Unknown role — also fails closed.
		{role: "robot", action: "read.project", want: false},
		{role: "", action: "read.project", want: false},
	}
	for _, c := range cases {
		t.Run(c.role+":"+c.action, func(t *testing.T) {
			p := &Principal{Role: c.role}
			if got := p.Can(c.action); got != c.want {
				t.Fatalf("Can(%q) for role %q = %v; want %v", c.action, c.role, got, c.want)
			}
		})
	}
}

// TestPrincipalCan_NilReceiver guards against panics when handlers
// forget to check for a nil Principal — Can() must always return false
// rather than panic so the failure mode is "auth blocks the call"
// rather than "process crashes mid-request".
func TestPrincipalCan_NilReceiver(t *testing.T) {
	var p *Principal
	if p.Can("read.project") {
		t.Fatal("Can on nil receiver returned true; want false")
	}
}

// TestPrincipalIsExpired covers the three states: zero time (no
// expiry), future time (still valid), past time (expired). The boundary
// case is "now == ExpiresAt": defined as expired so token leaks at the
// exact deadline don't get one extra free request.
func TestPrincipalIsExpired(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "zero never expires", expiresAt: time.Time{}, want: false},
		{name: "past is expired", expiresAt: now.Add(-time.Hour), want: true},
		{name: "future is valid", expiresAt: now.Add(time.Hour), want: false},
		{name: "exactly now is expired", expiresAt: now, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Principal{ExpiresAt: c.expiresAt}
			if got := p.IsExpired(now); got != c.want {
				t.Fatalf("IsExpired(now) with ExpiresAt=%v = %v; want %v", c.expiresAt, got, c.want)
			}
		})
	}

	// Nil receiver also reports not expired so the failure mode for a
	// missing Principal is "auth blocks at Can() check" rather than
	// "expiry check panics".
	var nilP *Principal
	if nilP.IsExpired(now) {
		t.Fatal("IsExpired on nil receiver returned true; want false")
	}
}
