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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type pingInput struct {
	// Message is echoed back so callers can distinguish their probe packets
	// without crafting a second round-trip. Empty string is allowed.
	Message           string `json:"message,omitempty" jsonschema:"optional probe text echoed back in the response"`
	ProjectSlug       string `json:"project_slug,omitempty" jsonschema:"optional project slug for current_project_summary; defaults to server PINDOC_PROJECT when configured"`
	ClientToolsetHash string `json:"client_toolset_hash,omitempty" jsonschema:"optional client-known toolset_version for drift detection"`
	WorkingDirectory  string `json:"working_directory,omitempty" jsonschema:"optional local workspace root; when provided, ping checks PINDOC.md frontmatter drift"`
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
	ToolsetVersion        string                `json:"toolset_version"`
	RequiresResync        *bool                 `json:"requires_resync"`
	SinceLastSeen         []string              `json:"since_last_seen,omitempty"`
	ClientActions         []ToolsetClientAction `json:"client_actions,omitempty"`
	CurrentProjectSummary *PingProjectSummary   `json:"current_project_summary,omitempty"`
	ReconcileCandidates   []ReconcileCandidate  `json:"reconcile_candidates,omitempty"`
	ReconcileSummary      *ReconcileSummary     `json:"reconcile_summary,omitempty"`
	HarnessDriftHint      *HarnessDriftHint     `json:"harness_drift_hint,omitempty"`
	HarnessDriftHints     []HarnessDriftHint    `json:"harness_drift_hints,omitempty"`
	HarnessBlocked        bool                  `json:"harness_blocked,omitempty"`
}

type HarnessDriftHint struct {
	Detected            bool   `json:"detected"`
	Severity            string `json:"severity,omitempty"`
	SuggestedCall       string `json:"suggested_call"`
	Reason              string `json:"reason,omitempty"`
	Path                string `json:"path,omitempty"`
	ExpectedProjectSlug string `json:"expected_project_slug,omitempty"`
	FoundProjectSlug    string `json:"found_project_slug,omitempty"`
	SchemaVersion       string `json:"schema_version,omitempty"`
}

type PingProjectSummary struct {
	ProjectSlug          string `json:"project_slug"`
	AreasCount           int    `json:"areas_count"`
	ArtifactsCount       int    `json:"artifacts_count"`
	OpenTaskCount        int    `json:"open_task_count"`
	ClaimedDoneTaskCount int    `json:"claimed_done_task_count"`
	BlockedTaskCount     int    `json:"blocked_task_count"`
	CancelledTaskCount   int    `json:"cancelled_task_count"`
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
			Description: "Handshake probe. Returns pong + server version + configured user language. Use this to verify the Pindoc MCP connection is live before calling any write tools. Pass client_toolset_hash when you have a cached toolset_version; on mismatch, client_actions tells agents to call runtime.status, refresh ToolSearch, and restart the MCP session if schemas remain stale.",
		},
		func(ctx context.Context, p *auth.Principal, in pingInput) (*sdk.CallToolResult, pingOutput, error) {
			echo := "pong"
			if in.Message != "" {
				echo = fmt.Sprintf("pong: %s", in.Message)
			}
			toolsetVersion := ToolsetVersion()
			requiresResync, sinceLastSeen := toolsetDrift(in.ClientToolsetHash, toolsetVersion)
			out := pingOutput{
				Pong:           echo,
				Version:        deps.Version,
				ServerTime:     time.Now().UTC(),
				UserLanguage:   deps.UserLanguage,
				ToolsetVersion: toolsetVersion,
				RequiresResync: requiresResync,
				SinceLastSeen:  sinceLastSeen,
			}
			if requiresResync != nil && *requiresResync {
				out.ClientActions = toolsetDriftClientActions(in.ClientToolsetHash)
			}
			projectSlug := strings.TrimSpace(in.ProjectSlug)
			if projectSlug == "" {
				projectSlug = strings.TrimSpace(deps.DefaultProjectSlug)
			}
			if strings.TrimSpace(in.WorkingDirectory) != "" {
				hints := detectHarnessDrifts(in.WorkingDirectory, projectSlug)
				if len(hints) > 0 {
					out.HarnessDriftHints = hints
					out.HarnessDriftHint = &out.HarnessDriftHints[0]
					out.HarnessBlocked = harnessDriftBlocked(hints)
				} else {
					out.HarnessDriftHint = detectHarnessDrift(in.WorkingDirectory, projectSlug)
				}
			}
			if projectSlug != "" && deps.DB != nil {
				scope, err := auth.ResolveProject(ctx, deps.DB, p, projectSlug)
				if err != nil {
					return nil, pingOutput{}, fmt.Errorf("ping project summary: %w", err)
				}
				summary, err := pingProjectSummary(ctx, deps, scope.ProjectSlug)
				if err != nil {
					return nil, pingOutput{}, fmt.Errorf("ping project summary: %w", err)
				}
				out.CurrentProjectSummary = &summary
				reconcileSummary, candidates, err := reconcileCompletedOpenTasks(ctx, deps, scope.ProjectSlug)
				if err != nil {
					return nil, pingOutput{}, fmt.Errorf("ping reconcile: %w", err)
				}
				out.ReconcileSummary = &reconcileSummary
				out.ReconcileCandidates = candidates
			}
			return nil, out, nil
		},
	)
}

func toolsetDrift(clientHash, serverHash string) (*bool, []string) {
	clientHash = strings.TrimSpace(clientHash)
	if clientHash == "" {
		return nil, nil
	}
	changed := clientHash != serverHash
	if !changed {
		v := false
		return &v, []string{}
	}
	v := true
	return &v, toolNamesSinceClientHash(clientHash)
}

func toolNamesSinceClientHash(clientHash string) []string {
	parts := strings.SplitN(clientHash, ":", 2)
	if len(parts) != 2 {
		return append([]string{}, RegisteredTools...)
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 0 || n > len(RegisteredTools) {
		return append([]string{}, RegisteredTools...)
	}
	return append([]string{}, RegisteredTools[n:]...)
}

func pingProjectSummary(ctx context.Context, deps Deps, projectSlug string) (PingProjectSummary, error) {
	var out PingProjectSummary
	out.ProjectSlug = projectSlug
	err := deps.DB.QueryRow(ctx, `
		SELECT
			(SELECT count(*)::int FROM areas ar JOIN projects p ON p.id = ar.project_id WHERE p.slug = $1),
			(SELECT count(*)::int FROM artifacts a JOIN projects p ON p.id = a.project_id WHERE p.slug = $1 AND a.status <> 'archived' AND a.status <> 'superseded'),
			(SELECT count(*)::int FROM artifacts a JOIN projects p ON p.id = a.project_id WHERE p.slug = $1 AND a.type = 'Task' AND a.status <> 'archived' AND a.status <> 'superseded' AND COALESCE(NULLIF(a.task_meta->>'status', ''), 'open') = 'open'),
			(SELECT count(*)::int FROM artifacts a JOIN projects p ON p.id = a.project_id WHERE p.slug = $1 AND a.type = 'Task' AND a.status <> 'archived' AND a.status <> 'superseded' AND a.task_meta->>'status' = 'claimed_done'),
			(SELECT count(*)::int FROM artifacts a JOIN projects p ON p.id = a.project_id WHERE p.slug = $1 AND a.type = 'Task' AND a.status <> 'archived' AND a.status <> 'superseded' AND a.task_meta->>'status' = 'blocked'),
			(SELECT count(*)::int FROM artifacts a JOIN projects p ON p.id = a.project_id WHERE p.slug = $1 AND a.type = 'Task' AND a.status <> 'archived' AND a.status <> 'superseded' AND a.task_meta->>'status' = 'cancelled')
	`, projectSlug).Scan(
		&out.AreasCount,
		&out.ArtifactsCount,
		&out.OpenTaskCount,
		&out.ClaimedDoneTaskCount,
		&out.BlockedTaskCount,
		&out.CancelledTaskCount,
	)
	return out, err
}

func detectHarnessDrift(workingDirectory, expectedProjectSlug string) *HarnessDriftHint {
	hints := detectHarnessDrifts(workingDirectory, expectedProjectSlug)
	path := filepath.Join(strings.TrimSpace(workingDirectory), "PINDOC.md")
	hint := &HarnessDriftHint{
		Detected:            false,
		SuggestedCall:       "pindoc.harness.install",
		Path:                path,
		ExpectedProjectSlug: strings.TrimSpace(expectedProjectSlug),
	}
	if len(hints) > 0 {
		return &hints[0]
	}
	body, err := os.ReadFile(path)
	if err == nil {
		meta := parsePindocFrontmatter(string(body))
		hint.FoundProjectSlug = meta["project_slug"]
		hint.SchemaVersion = meta["schema_version"]
	}
	return hint
}

func detectHarnessDrifts(workingDirectory, expectedProjectSlug string) []HarnessDriftHint {
	path := filepath.Join(strings.TrimSpace(workingDirectory), "PINDOC.md")
	base := HarnessDriftHint{
		Detected:            true,
		SuggestedCall:       "pindoc.harness.install",
		Path:                path,
		ExpectedProjectSlug: strings.TrimSpace(expectedProjectSlug),
	}
	body, err := os.ReadFile(path)
	if err != nil {
		hint := base
		if os.IsNotExist(err) {
			hint.Reason = "PINDOC.md is missing in the workspace root."
			hint.Severity = "info"
		} else {
			hint.Reason = fmt.Sprintf("PINDOC.md could not be read: %v", err)
			hint.Severity = "warning"
		}
		return []HarnessDriftHint{hint}
	}
	meta := parsePindocFrontmatter(string(body))
	foundProjectSlug := meta["project_slug"]
	schemaVersion := meta["schema_version"]
	mk := func(severity, reason string) HarnessDriftHint {
		hint := base
		hint.Severity = severity
		hint.Reason = reason
		hint.FoundProjectSlug = foundProjectSlug
		hint.SchemaVersion = schemaVersion
		return hint
	}
	var hints []HarnessDriftHint
	switch {
	case strings.TrimSpace(foundProjectSlug) == "":
		hints = append(hints, mk("warning", "PINDOC.md frontmatter is missing project_slug."))
	case base.ExpectedProjectSlug != "" && foundProjectSlug != base.ExpectedProjectSlug:
		hints = append(hints, mk("blocking", fmt.Sprintf("PINDOC.md project_slug=%q does not match expected project_slug=%q.", foundProjectSlug, base.ExpectedProjectSlug)))
	}
	if strings.TrimSpace(schemaVersion) == "" {
		hints = append(hints, mk("info", "PINDOC.md frontmatter is missing schema_version."))
	}
	sortHarnessDriftHints(hints)
	return hints
}

func harnessDriftBlocked(hints []HarnessDriftHint) bool {
	for _, hint := range hints {
		if hint.Severity == "blocking" {
			return true
		}
	}
	return false
}

func sortHarnessDriftHints(hints []HarnessDriftHint) {
	sort.SliceStable(hints, func(i, j int) bool {
		return harnessDriftSeverityRank(hints[i].Severity) < harnessDriftSeverityRank(hints[j].Severity)
	})
}

func harnessDriftSeverityRank(severity string) int {
	switch severity {
	case "blocking":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func parsePindocFrontmatter(body string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return out
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			out[key] = value
		}
	}
	return out
}
