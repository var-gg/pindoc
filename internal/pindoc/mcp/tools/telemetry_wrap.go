package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

// Handler is Pindoc's tool handler signature — Principal-aware,
// req-blind. AddInstrumentedTool wraps a Handler in the SDK's
// req-aware ToolHandlerFor by running the AuthChain to produce the
// Principal and threading it as an explicit argument. Handlers
// therefore never reach into deps for caller identity / scope and
// never inspect raw HTTP headers — both responsibilities belong to
// the resolver chain (Decision principal-resolver-architecture).
type Handler[I, O any] func(ctx context.Context, p *auth.Principal, in I) (*sdk.CallToolResult, O, error)

// AddInstrumentedTool is the drop-in replacement for sdk.AddTool that
// every Pindoc tool uses. Resolves the calling Principal via
// deps.AuthChain, then wraps the resulting handler with telemetry
// before registering. Telemetry no-ops cleanly when deps.Telemetry is
// nil (test fixtures); chain failures (no resolver matched, malformed
// token) propagate to the caller as plain errors rather than producing
// a degraded Principal.
func AddInstrumentedTool[I, O any](
	server *sdk.Server,
	deps Deps,
	tool *sdk.Tool,
	handler Handler[I, O],
) {
	name := tool.Name
	chain := deps.AuthChain
	store := deps.Telemetry
	sdkHandler := func(ctx context.Context, req *sdk.CallToolRequest, input I) (*sdk.CallToolResult, O, error) {
		p, err := chain.Resolve(ctx, req)
		if err != nil {
			var zero O
			return nil, zero, fmt.Errorf("auth: resolve principal for %q: %w", name, err)
		}
		if store == nil {
			return handler(ctx, p, input)
		}
		return instrumentCall(name, store, p, input, func() (*sdk.CallToolResult, O, error) {
			return handler(ctx, p, input)
		})
	}
	sdk.AddTool(server, tool, sdkHandler)
}

// instrumentCall records one tool-call entry around the supplied
// handler invocation. Latency cost is dominated by two json.Marshal
// calls plus one tiktoken encode; the channel send is non-blocking
// and never waits on IO.
//
// Error code is lifted by reflection from an `ErrorCode string` field
// on the output struct when present (Pindoc's not_ready convention).
// Falls back to "handler_error" when the handler returned a non-nil
// error; empty string for successful calls.
func instrumentCall[I, O any](
	name string,
	store *telemetry.Store,
	p *auth.Principal,
	input I,
	invoke func() (*sdk.CallToolResult, O, error),
) (*sdk.CallToolResult, O, error) {
	start := time.Now()

	inputJSON, _ := json.Marshal(input)
	result, output, err := invoke()
	outputJSON, _ := json.Marshal(output)

	errorCode := ""
	if err != nil {
		errorCode = "handler_error"
	} else if code := extractErrorCode(output); code != "" {
		errorCode = code
	}

	store.Record(telemetryEntry(start, name, p, input, inputJSON, outputJSON, errorCode, store))

	return result, output, err
}

// telemetryEntry packages the per-call fields into a telemetry.Entry,
// pulling user / agent from the resolved Principal and project_slug
// from the tool input via reflection (account-level scope, Decision
// mcp-scope-account-level-industry-standard — Principal no longer
// carries ProjectSlug). The downstream mcp_tool_calls schema stays
// identical to the V1 (deps-fed) shape — the regression check on row
// format is "fields look the same as before this Task". Nil principals
// fall back to empty strings so capability probes that ran before
// chain resolution still record cleanly. Tool inputs without a
// `ProjectSlug` field (instance-wide tools like ping / user.current)
// log empty project_slug, which is the correct semantic.
func telemetryEntry(
	start time.Time,
	name string,
	p *auth.Principal,
	input any,
	inputJSON, outputJSON []byte,
	errorCode string,
	store *telemetry.Store,
) telemetry.Entry {
	var agentID, userID string
	if p != nil {
		agentID = p.AgentID
		userID = p.UserID
	}
	projectSlug := extractProjectSlug(input)
	return telemetry.Entry{
		StartedAt:       start,
		DurationMs:      time.Since(start).Milliseconds(),
		ToolName:        name,
		AgentID:         agentID,
		UserID:          userID,
		ProjectSlug:     projectSlug,
		InputBytes:      len(inputJSON),
		OutputBytes:     len(outputJSON),
		InputChars:      utf8.RuneCountInString(string(inputJSON)),
		OutputChars:     utf8.RuneCountInString(string(outputJSON)),
		InputTokensEst:  store.EstimateTokens(string(inputJSON)),
		OutputTokensEst: store.EstimateTokens(string(outputJSON)),
		ErrorCode:       errorCode,
		ToolsetVersion:  ToolsetVersion(),
	}
}

// extractErrorCode looks for a string-valued ErrorCode field on the
// output struct — Pindoc's not_ready pattern. Uses reflection so the
// wrapper stays generic; Pindoc outputs are modest-sized structs so
// the per-call reflection cost is negligible next to JSON marshalling.
// Returns "" when the output has no such field or the field is empty.
func extractErrorCode(v any) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	f := rv.FieldByName("ErrorCode")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

// extractProjectSlug pulls the input's ProjectSlug field via reflection
// so telemetry stays project-attributed even though Principal no longer
// carries the slug (Decision mcp-scope-account-level-industry-standard
// puts project_slug on every project-scoped tool input). Returns ""
// when the input has no such field — instance-wide tools (ping,
// user.current) log no project_slug, which is the right semantic.
func extractProjectSlug(v any) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	f := rv.FieldByName("ProjectSlug")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}
