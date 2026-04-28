package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RegisteredTools is the canonical list of MCP tool names this package
// exposes — kept in sync with the Register*(…) calls in
// internal/pindoc/mcp/server.go. ToolsetVersion() hashes this list so
// agents can detect drift when a new tool lands between sessions:
// Claude Code's schema cache binds tool definitions at session start,
// and a version bump in a live response is the cheapest way to tell
// the user "restart me, the server grew a tool you can't see yet".
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

// ToolsetVersion returns a stable short string identifying the current
// tool surface: "<count>:<hash8>" where hash8 is the first 8 hex chars
// of sha256(newline-joined-sorted-tool-names). Agents compare across
// sessions; any difference means the catalog changed and a reconnect
// is needed to see the new tools.
func ToolsetVersion() string {
	sorted := make([]string, len(RegisteredTools))
	copy(sorted, RegisteredTools)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return fmt.Sprintf("%d:%s", len(sorted), hex.EncodeToString(h[:])[:8])
}
