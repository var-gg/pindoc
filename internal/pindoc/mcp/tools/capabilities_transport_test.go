package tools

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// TestBuildCapabilities_AlwaysPerCall locks the post-pivot contract:
// account-level scope (Decision mcp-scope-account-level-industry-
// standard) makes scope_mode = "per_call" and
// new_project_requires_reconnect = false on every transport. Transport
// is still echoed for telemetry/debugging but no longer drives scope
// branching. The previous TransportBranching test (stdio →
// fixed_session, streamable_http → per_connection) is now obsolete —
// per_connection was a cover for "URL pin per project" which no
// longer exists.
func TestBuildCapabilities_AlwaysPerCall(t *testing.T) {
	cases := []struct {
		name          string
		transport     string
		wantTransport string
	}{
		{"empty defaults to stdio", "", "stdio"},
		{"explicit stdio", "stdio", "stdio"},
		{"streamable_http daemon", "streamable_http", "streamable_http"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := buildCapabilities(
				Deps{Transport: c.transport},
				&auth.Principal{AuthMode: auth.AuthModeTrustedLocal},
				false,
			)
			if caps.Transport != c.wantTransport {
				t.Errorf("Transport = %q, want %q", caps.Transport, c.wantTransport)
			}
			if caps.ScopeMode != "per_call" {
				t.Errorf("ScopeMode = %q, want per_call (account-level always)", caps.ScopeMode)
			}
			if caps.NewProjectRequiresReconnect {
				t.Errorf("NewProjectRequiresReconnect = true, want false (account-level always)")
			}
			// Invariants — these don't depend on transport, but if a
			// future refactor accidentally drops them the multi-project
			// rollout silently breaks read-side advertisement. Keep
			// these here so the regression bites at this test, not in
			// production logs.
			if caps.AuthMode != "trusted_local" {
				t.Errorf("AuthMode = %q, want trusted_local (transport-independent)", caps.AuthMode)
			}
			if caps.UpdateVia != "update_of" {
				t.Errorf("UpdateVia = %q, want update_of (transport-independent)", caps.UpdateVia)
			}
		})
	}
}

// TestBuildCapabilities_MultiProjectPassThrough locks the read-side
// contract that the `multi_project` bool the call site derives from
// projects.CountVisible flows verbatim into the Capabilities payload.
// Reader UI keys the project switcher off this flag, so a future
// refactor that accidentally hard-codes false here would silently hide
// the switcher even after the operator creates a second project.
func TestBuildCapabilities_MultiProjectPassThrough(t *testing.T) {
	cases := []struct {
		name         string
		multiProject bool
	}{
		{"single project visible", false},
		{"two or more projects visible", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := buildCapabilities(
				Deps{Transport: "stdio"},
				&auth.Principal{AuthMode: auth.AuthModeTrustedLocal},
				c.multiProject,
			)
			if caps.MultiProject != c.multiProject {
				t.Errorf("MultiProject = %v, want %v", caps.MultiProject, c.multiProject)
			}
		})
	}
}

func TestBuildCapabilities_ReceiptExemptionLimit(t *testing.T) {
	caps := buildCapabilities(
		Deps{ReceiptExemptionLimit: 5},
		&auth.Principal{AuthMode: auth.AuthModeTrustedLocal},
		false,
	)
	if caps.ReceiptExemptionLimit != 5 {
		t.Fatalf("ReceiptExemptionLimit = %d, want 5", caps.ReceiptExemptionLimit)
	}
}
