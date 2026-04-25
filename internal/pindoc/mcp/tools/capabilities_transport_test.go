package tools

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// TestBuildCapabilities_TransportBranching locks the contract that
// `pindoc.project.current` advertises capability values keyed off the
// MCP transport that built the Server. Stdio (the legacy
// subprocess-per-session path) keeps `fixed_session` + reconnect=true so
// existing agents don't change behaviour. Streamable-HTTP daemons advertise
// `per_connection` + reconnect=false because the transport already binds
// a different project per URL — switching projects is "open another url"
// not "tear down the daemon". Decision
// pindoc-mcp-transport-streamable-http-per-connection-scope.
//
// Transport now flows through *auth.Principal rather than tools.Deps —
// the Principal carries every per-call caller-context value (Decision
// principal-resolver-architecture). The capability shape is unchanged.
func TestBuildCapabilities_TransportBranching(t *testing.T) {
	cases := []struct {
		name              string
		transport         string
		wantTransport     string
		wantScopeMode     string
		wantRequiresReco  bool
	}{
		{
			name:             "empty defaults to stdio",
			transport:        "",
			wantTransport:    "stdio",
			wantScopeMode:    "fixed_session",
			wantRequiresReco: true,
		},
		{
			name:             "explicit stdio",
			transport:        "stdio",
			wantTransport:    "stdio",
			wantScopeMode:    "fixed_session",
			wantRequiresReco: true,
		},
		{
			name:             "streamable_http daemon",
			transport:        "streamable_http",
			wantTransport:    "streamable_http",
			wantScopeMode:    "per_connection",
			wantRequiresReco: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := buildCapabilities(Deps{}, &auth.Principal{Transport: c.transport, AuthMode: auth.AuthModeTrustedLocal})
			if caps.Transport != c.wantTransport {
				t.Errorf("Transport = %q, want %q", caps.Transport, c.wantTransport)
			}
			if caps.ScopeMode != c.wantScopeMode {
				t.Errorf("ScopeMode = %q, want %q", caps.ScopeMode, c.wantScopeMode)
			}
			if caps.NewProjectRequiresReconnect != c.wantRequiresReco {
				t.Errorf("NewProjectRequiresReconnect = %v, want %v",
					caps.NewProjectRequiresReconnect, c.wantRequiresReco)
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
