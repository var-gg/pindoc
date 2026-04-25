// probe-mcp-http connects to a running pindoc-server -http daemon, calls
// pindoc.project.current on /mcp, and prints the capabilities block.
// Manual-QA helper for the streamable-HTTP transport rollout.
// Not part of the production tool set — keep usage minimal so it stays
// trivial to read. Build:
//
//	go build -o bin/probe-mcp-http.exe ./cmd/probe-mcp-http
//	./bin/probe-mcp-http.exe http://127.0.0.1:5830/mcp pindoc
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: probe-mcp-http <endpoint-url> [project_slug]")
		os.Exit(2)
	}
	endpoint := os.Args[1]
	args := map[string]any{}
	if len(os.Args) >= 3 {
		args["project_slug"] = os.Args[2]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := sdk.NewClient(&sdk.Implementation{
		Name:    "probe-mcp-http",
		Version: "0.0.1",
	}, nil)

	cs, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect failed: %v\n", err)
		os.Exit(1)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "pindoc.project.current",
		Arguments: args,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "call failed: %v\n", err)
		os.Exit(1)
	}

	out := map[string]any{
		"endpoint":           endpoint,
		"is_error":           res.IsError,
		"structured_content": res.StructuredContent,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
