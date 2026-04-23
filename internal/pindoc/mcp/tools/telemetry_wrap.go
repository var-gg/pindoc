package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

// AddInstrumentedTool is the drop-in replacement for sdk.AddTool that
// every Pindoc tool uses. Wraps handler with Instrument() before
// registering so telemetry lands on every call without the registration
// site duplicating the boilerplate. Nil deps.Telemetry no-ops the
// wrap, so tests that leave it unset still compile and run.
func AddInstrumentedTool[I, O any](
	server *sdk.Server,
	deps Deps,
	tool *sdk.Tool,
	handler sdk.ToolHandlerFor[I, O],
) {
	sdk.AddTool(server, tool, Instrument(tool.Name, deps.Telemetry, deps, handler))
}

// Instrument wraps a tool handler with async telemetry. The returned
// handler is a drop-in replacement — same signature, same semantics,
// plus a fire-and-forget Record() on the way out. Latency cost is
// dominated by two json.Marshal calls + one tiktoken encode; the
// channel send itself is non-blocking and never waits on IO.
//
// Error code is lifted by reflection from an `ErrorCode string` field
// on the output struct when present (Pindoc's not_ready convention).
// Falls back to "handler_error" when the handler returned a non-nil
// error; empty string for successful calls.
func Instrument[I, O any](
	name string,
	store *telemetry.Store,
	deps Deps,
	handler sdk.ToolHandlerFor[I, O],
) sdk.ToolHandlerFor[I, O] {
	if store == nil {
		return handler
	}
	return func(ctx context.Context, req *sdk.CallToolRequest, input I) (*sdk.CallToolResult, O, error) {
		start := time.Now()

		inputJSON, _ := json.Marshal(input)
		result, output, err := handler(ctx, req, input)
		outputJSON, _ := json.Marshal(output)

		errorCode := ""
		if err != nil {
			errorCode = "handler_error"
		} else if code := extractErrorCode(output); code != "" {
			errorCode = code
		}

		store.Record(telemetry.Entry{
			StartedAt:       start,
			DurationMs:      time.Since(start).Milliseconds(),
			ToolName:        name,
			AgentID:         deps.AgentID,
			UserID:          deps.UserID,
			ProjectSlug:     deps.ProjectSlug,
			InputBytes:      len(inputJSON),
			OutputBytes:     len(outputJSON),
			InputChars:      utf8.RuneCountInString(string(inputJSON)),
			OutputChars:     utf8.RuneCountInString(string(outputJSON)),
			InputTokensEst:  store.EstimateTokens(string(inputJSON)),
			OutputTokensEst: store.EstimateTokens(string(outputJSON)),
			ErrorCode:       errorCode,
			ToolsetVersion:  ToolsetVersion(),
		})

		return result, output, err
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
