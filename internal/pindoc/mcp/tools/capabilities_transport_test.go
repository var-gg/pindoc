package tools

import (
	"reflect"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
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
				&auth.Principal{Source: auth.SourceLoopback},
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
			if caps.UpdateVia != "update_of" {
				t.Errorf("UpdateVia = %q, want update_of (transport-independent)", caps.UpdateVia)
			}
			if caps.BindAddr != config.DefaultBindAddr {
				t.Errorf("BindAddr = %q, want %q (default)", caps.BindAddr, config.DefaultBindAddr)
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
				&auth.Principal{Source: auth.SourceLoopback},
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
		&auth.Principal{Source: auth.SourceLoopback},
		false,
	)
	if caps.ReceiptExemptionLimit != 5 {
		t.Fatalf("ReceiptExemptionLimit = %d, want 5", caps.ReceiptExemptionLimit)
	}
}

// TestBuildCapabilities_ProvidersFromConfig verifies AuthProviders /
// BindAddr flow from Deps to Capabilities verbatim. Decision
// `decision-auth-model-loopback-and-providers` retired the auth_mode
// enum field in favour of these two axes.
func TestBuildCapabilities_ProvidersFromConfig(t *testing.T) {
	deps := Deps{
		AuthProviders: []string{config.AuthProviderGitHub},
		BindAddr:      "0.0.0.0:5830",
	}
	caps := buildCapabilities(deps, &auth.Principal{Source: auth.SourceOAuth}, false)
	wantProviders := []string{config.AuthProviderGitHub}
	if !reflect.DeepEqual(caps.AuthProviders, wantProviders) {
		t.Fatalf("AuthProviders = %#v, want %#v", caps.AuthProviders, wantProviders)
	}
	if caps.BindAddr != "0.0.0.0:5830" {
		t.Fatalf("BindAddr = %q, want 0.0.0.0:5830", caps.BindAddr)
	}
}

// TestBuildCapabilities_ProvidersEmptySerialisesAsArray ensures the
// JSON wire surface always emits a `providers` array (Reader / agents
// can iterate without a nil guard) even when no IdP is configured.
func TestBuildCapabilities_ProvidersEmptySerialisesAsArray(t *testing.T) {
	caps := buildCapabilities(Deps{}, &auth.Principal{Source: auth.SourceLoopback}, false)
	if caps.AuthProviders == nil {
		t.Fatalf("AuthProviders = nil; want non-nil empty slice for stable JSON shape")
	}
	if len(caps.AuthProviders) != 0 {
		t.Fatalf("AuthProviders = %#v, want empty", caps.AuthProviders)
	}
}
