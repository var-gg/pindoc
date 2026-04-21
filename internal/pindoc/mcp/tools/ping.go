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
)

type PingDeps struct {
	Version      string
	UserLanguage string
}

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
}

// RegisterPing wires pindoc.ping — the Phase-1 handshake tool. Its job is
// small and fixed: prove the stdio transport works and return a few server
// facts an agent can surface in a startup log line.
func RegisterPing(server *sdk.Server, deps PingDeps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.ping",
			Description: "Handshake probe. Returns pong + server version + configured user language. Use this to verify the Pindoc MCP connection is live before calling any write tools.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in pingInput) (*sdk.CallToolResult, pingOutput, error) {
			echo := "pong"
			if in.Message != "" {
				echo = fmt.Sprintf("pong: %s", in.Message)
			}
			return nil, pingOutput{
				Pong:         echo,
				Version:      deps.Version,
				ServerTime:   time.Now().UTC(),
				UserLanguage: deps.UserLanguage,
			}, nil
		},
	)
}
