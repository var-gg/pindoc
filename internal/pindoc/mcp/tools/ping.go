// Package tools holds Pindoc's MCP tool implementations.
//
// Each file exposes a Register<Name>(server, deps) function the server
// package calls during startup. Keeping each tool in its own file makes
// the Phase-by-Phase growth visible in the diff: ping.go (Phase 1),
// harness_install.go / project_current.go / ... (Phase 2), search.go /
// context_for_task.go (Phase 3), and so on.
package tools

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type pingInput struct {
	// Message is echoed back so callers can distinguish their probe packets
	// without crafting a second round-trip. Empty string is allowed.
	Message string `json:"message,omitempty" jsonschema:"optional probe text echoed back in the response"`
}

type pingOutput struct {
	Pong         string    `json:"pong"`
	Version      string    `json:"version"`
	ServerTime   time.Time `json:"server_time"`
	UserLanguage string    `json:"user_language"`
	// ToolsetVersion is a stable short hash of the current MCP tool
	// catalog. Agents compare this across sessions to detect drift — if
	// the server grew a new tool (e.g. pindoc.scope.in_flight landed in
	// Phase F), this value changes and the agent's schema cache is stale
	// until the session restarts. Phase H drift notice.
	ToolsetVersion string `json:"toolset_version"`
}

// RegisterPing wires pindoc.ping — the Phase-1 handshake tool. Its job is
// small and fixed: prove the stdio transport works and return a few server
// facts an agent can surface in a startup log line. Goes through the same
// AddInstrumentedTool path as every other tool so the auth chain runs
// (trusted_local always matches) and telemetry records the call.
func RegisterPing(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.ping",
			Description: "Handshake probe. Returns pong + server version + configured user language. Use this to verify the Pindoc MCP connection is live before calling any write tools.",
		},
		func(_ context.Context, _ *auth.Principal, in pingInput) (*sdk.CallToolResult, pingOutput, error) {
			echo := "pong"
			if in.Message != "" {
				echo = fmt.Sprintf("pong: %s", in.Message)
			}
			return nil, pingOutput{
				Pong:           echo,
				Version:        deps.Version,
				ServerTime:     time.Now().UTC(),
				UserLanguage:   deps.UserLanguage,
				ToolsetVersion: ToolsetVersion(),
			}, nil
		},
	)
}
