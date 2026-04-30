package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
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

type toolReannounce struct {
	name     string
	register func()
}

var (
	toolReannounceMu       sync.RWMutex
	toolReannounceByServer = map[*sdk.Server]toolReannounce{}
)

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
		projectDefault := applyProjectSlugDefaulting(ctx, deps, p, &input)
		if projectSlugFieldValue(input) == "" && projectDefault.Via != "" && projectDefault.ProjectSlug == "" {
			if output, ok := projectSlugDefaultNotReady[O](projectDefault); ok {
				output = stampToolsetVersion(output)
				output = applyMCPErrorContract(output, deps.UserLanguage)
				return nil, output, nil
			}
		}
		if store == nil {
			result, output, err := handler(ctx, p, input)
			output = stampToolsetVersion(output)
			output = applyMCPErrorContract(output, deps.UserLanguage)
			return result, output, err
		}
		return instrumentCall(name, store, deps.UserLanguage, p, input, func() (*sdk.CallToolResult, O, error) {
			return handler(ctx, p, input)
		})
	}
	sdk.AddTool(server, tool, sdkHandler)
	rememberToolReannounce(server, name, func() {
		sdk.AddTool(server, tool, sdkHandler)
	})
}

func rememberToolReannounce(server *sdk.Server, name string, register func()) {
	if server == nil || register == nil || name == "" {
		return
	}
	toolReannounceMu.Lock()
	defer toolReannounceMu.Unlock()
	if _, exists := toolReannounceByServer[server]; exists {
		return
	}
	toolReannounceByServer[server] = toolReannounce{
		name:     name,
		register: register,
	}
}

// ReannounceToolListChanged re-adds one already-registered tool to trigger
// the SDK's standard notifications/tools/list_changed path without changing
// Pindoc's tool catalog. The SDK treats AddTool as a replacement and emits
// the notification to active sessions; the handler is identical to the
// original registration, so calls continue to route normally.
func ReannounceToolListChanged(server *sdk.Server) (name string, ok bool) {
	toolReannounceMu.RLock()
	entry, exists := toolReannounceByServer[server]
	toolReannounceMu.RUnlock()
	if !exists {
		return "", false
	}
	defer func() {
		if recover() != nil {
			name, ok = "", false
		}
	}()
	entry.register()
	return entry.name, true
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
	lang string,
	p *auth.Principal,
	input I,
	invoke func() (*sdk.CallToolResult, O, error),
) (*sdk.CallToolResult, O, error) {
	start := time.Now()

	inputJSON, _ := json.Marshal(input)
	result, output, err := invoke()
	output = stampToolsetVersion(output)
	output = applyMCPErrorContract(output, lang)
	outputJSON, _ := json.Marshal(output)

	errorCode := ""
	if err != nil {
		errorCode = "handler_error"
	} else if code := extractErrorCode(output); code != "" {
		errorCode = code
	}

	entry := telemetryEntry(start, name, p, input, inputJSON, outputJSON, errorCode, store)
	entry.Metadata = extractToolMetadata(name, input, outputJSON)
	store.Record(entry)

	return result, output, err
}

func stampToolsetVersion[O any](output O) O {
	rv := reflect.ValueOf(&output).Elem()
	for rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return output
		}
		elem := rv.Elem()
		if elem.Kind() == reflect.Struct && !elem.CanSet() {
			copy := reflect.New(elem.Type()).Elem()
			copy.Set(elem)
			if setToolsetVersionField(copy) {
				rv.Set(copy)
			}
			return output
		}
		rv = elem
	}
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return output
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return output
	}
	setToolsetVersionField(rv)
	return output
}

func setToolsetVersionField(rv reflect.Value) bool {
	f := rv.FieldByName("ToolsetVersion")
	if !f.IsValid() || !f.CanSet() || f.Kind() != reflect.String || f.String() != "" {
		return false
	}
	f.SetString(ToolsetVersion())
	return true
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

// extractToolMetadata builds the per-tool metadata payload that lands
// in mcp_tool_calls.metadata. V1 covers the four highest-value tools
// the codex DX feedback (Decision mcp-dx-외부-리뷰-codex-1차-피드백-
// 6항목 발견 4) called out by name:
//
//   - workspace.detect → "via" priority chain branch
//   - area.list        → "include_templates" flag usage
//   - artifact.propose → "shape" + "artifact_type" buckets
//   - artifact.search  → "top_k" + "include_templates" + "hits_count"
//
// All other tools return a nil payload and the row defaults to '{}'.
// Returning bytes (json.RawMessage) lets the writer pass them straight
// to pgx without a second marshal round-trip.
func extractToolMetadata(toolName string, input any, outputJSON []byte) json.RawMessage {
	md := map[string]any{}

	switch toolName {
	case "pindoc.workspace.detect":
		// "via" lives on the structured output. Decode lazily into a
		// minimal envelope so an unrelated output-shape change doesn't
		// break this extractor.
		var out struct {
			Via string `json:"via"`
		}
		_ = json.Unmarshal(outputJSON, &out)
		if out.Via != "" {
			md["via"] = out.Via
		}

	case "pindoc.area.list":
		if v, ok := metaBoolField(input, "IncludeTemplates"); ok {
			md["include_templates"] = v
		}

	case "pindoc.artifact.propose":
		if v := metaStringField(input, "Shape"); v != "" {
			md["shape"] = v
		}
		if v := metaStringField(input, "Type"); v != "" {
			// Avoid clashing with the column name "tool_name" if a
			// future query joins both — store under artifact_type.
			md["artifact_type"] = v
		}
		if v := metaStringField(input, "AreaSlug"); v != "" {
			md["area_slug"] = v
		}

	case "pindoc.artifact.search":
		if v := metaIntField(input, "TopK"); v > 0 {
			md["top_k"] = v
		}
		if v, ok := metaBoolField(input, "IncludeTemplates"); ok {
			md["include_templates"] = v
		}
		var out struct {
			Hits []json.RawMessage `json:"hits"`
		}
		_ = json.Unmarshal(outputJSON, &out)
		md["hits_count"] = len(out.Hits)
	}

	if len(md) == 0 {
		return nil
	}
	buf, err := json.Marshal(md)
	if err != nil {
		return nil
	}
	return buf
}

// metaStringField reads a string-valued struct field by name, traversing
// a single pointer level. Returns "" when the field is missing, of the
// wrong kind, or empty. Renamed from stringField to avoid colliding
// with the same-name helper in error_contract.go (which takes a
// reflect.Value directly rather than any).
func metaStringField(v any, name string) string {
	rv := metaReflectStruct(v)
	if !rv.IsValid() {
		return ""
	}
	f := rv.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

// metaBoolField reads a bool field by name. Second return distinguishes
// "field is present and false" (true, false) from "field missing"
// (false, false) so callers can decide whether to record the value.
func metaBoolField(v any, name string) (bool, bool) {
	rv := metaReflectStruct(v)
	if !rv.IsValid() {
		return false, false
	}
	f := rv.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.Bool {
		return false, false
	}
	return f.Bool(), true
}

// metaIntField reads an int / int64 / int32 field by name. Returns 0
// when missing or not a signed integer kind. The caller decides
// whether 0 is meaningful.
func metaIntField(v any, name string) int {
	rv := metaReflectStruct(v)
	if !rv.IsValid() {
		return 0
	}
	f := rv.FieldByName(name)
	if !f.IsValid() {
		return 0
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(f.Int())
	}
	return 0
}

// metaReflectStruct dereferences a single pointer and returns the
// underlying struct value (or invalid Value when the input isn't a
// struct).
func metaReflectStruct(v any) reflect.Value {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return reflect.Value{}
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return rv
}
