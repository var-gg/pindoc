package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ToolsetSchemaVersion is bumped whenever an existing tool's input schema,
// output schema, or description contract changes without adding/removing a
// tool name. RegisteredTools catches catalog membership drift; this salt
// catches same-name surface drift that client-side schema caches otherwise
// cannot see.
const ToolsetSchemaVersion = "2026-04-30-dry-run-evidence-toolset-v2"

// RegisteredTools is the canonical list of MCP tool names this package
// exposes — kept in sync with the Register*(…) calls in
// internal/pindoc/mcp/server.go. ToolsetVersion() hashes this list plus
// ToolsetSchemaVersion so agents can detect drift when a new tool lands or
// an existing tool's schema/description changes between sessions: Claude
// Code's schema cache binds tool definitions at session start, and a version
// bump in a live response is the cheapest way to tell the user "restart me,
// the server grew a tool surface you can't see yet".
var RegisteredTools = []string{
	"pindoc.ping",
	"pindoc.runtime.status",
	"pindoc.project.current",
	"pindoc.project.create",
	"pindoc.project_export",
	"pindoc.workspace.detect",
	"pindoc.area.list",
	"pindoc.area.create",
	"pindoc.artifact.read",
	"pindoc.artifact.translate",
	"pindoc.artifact.propose",
	"pindoc.artifact.wording_fix",
	"pindoc.artifact.add_pin",
	"pindoc.harness.install",
	"pindoc.artifact.search",
	"pindoc.context.for_task",
	"pindoc.artifact.revisions",
	"pindoc.artifact.diff",
	"pindoc.artifact.summary_since",
	"pindoc.artifact.read_state",
	"pindoc.user.current",
	"pindoc.user.update",
	"pindoc.scope.in_flight",
	"pindoc.task.queue",
	"pindoc.task.acceptance.transition",
	"pindoc.task.assign",
	"pindoc.task.bulk_assign",
	"pindoc.task.claim_done",
}

// ToolsetVersion returns a stable short string identifying the current tool
// surface: "<count>:<hash8>" where hash8 is the first 8 hex chars of
// sha256(newline-joined schema salt + sorted tool names). Agents compare
// across sessions; any difference means the catalog/schema changed and a
// reconnect is needed to see the current tools.
func ToolsetVersion() string {
	sorted := make([]string, len(RegisteredTools))
	copy(sorted, RegisteredTools)
	sort.Strings(sorted)
	payload := append([]string{"schema:" + ToolsetSchemaVersion}, sorted...)
	h := sha256.Sum256([]byte(strings.Join(payload, "\n")))
	return fmt.Sprintf("%d:%s", len(sorted), hex.EncodeToString(h[:])[:8])
}
