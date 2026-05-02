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
	Message             string                 `json:"message,omitempty" jsonschema:"optional probe text echoed back in the response"`
	ProjectSlug         string                 `json:"project_slug,omitempty" jsonschema:"optional project slug for current_project_summary; defaults to server PINDOC_PROJECT when configured"`
	ClientToolsetHash   string                 `json:"client_toolset_hash,omitempty" jsonschema:"optional client-known toolset_version for drift detection"`
	WorkingDirectory    string                 `json:"working_directory,omitempty" jsonschema:"optional client workspace path for diagnostics; server only reads it when the directory is observable from this MCP process"`
	CurrentPindocMD     string                 `json:"current_pindoc_md,omitempty" jsonschema:"optional full client-read PINDOC.md body; ping parses frontmatter only and prefers this over server filesystem probing"`
	PindocMDFrontmatter *pingPindocFrontmatter `json:"pindoc_md_frontmatter,omitempty" jsonschema:"optional PINDOC.md frontmatter collected by the client; preferred over server filesystem probing for harness drift"`
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
	Code                string `json:"code,omitempty"`
	SuggestedCall       string `json:"suggested_call"`
	Reason              string `json:"reason,omitempty"`
	Path                string `json:"path,omitempty"`
	Source              string `json:"source,omitempty"`
	ExpectedProjectSlug string `json:"expected_project_slug,omitempty"`
	FoundProjectSlug    string `json:"found_project_slug,omitempty"`
	SchemaVersion       string `json:"schema_version,omitempty"`
}

type pingPindocFrontmatter struct {
	ProjectSlug   string `json:"project_slug,omitempty"`
	SchemaVersion string `json:"schema_version,omitempty"`
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
			Description: "Handshake probe. Returns pong + server version + configured user language. Use this to verify the Pindoc MCP connection is live before calling any write tools. Pass client_toolset_hash when you have a cached toolset_version; on mismatch, client_actions tells agents to call runtime.status, refresh ToolSearch, and restart the MCP session if schemas remain stale. For harness drift checks, prefer client-provided current_pindoc_md or pindoc_md_frontmatter; working_directory is only a best-effort server-filesystem diagnostic and unobservable paths are not treated as missing files.",
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
			if hasHarnessDriftProbeInput(in) {
				hints, clean := evaluateHarnessDrift(harnessDriftProbe{
					WorkingDirectory:    in.WorkingDirectory,
					ExpectedProjectSlug: projectSlug,
					CurrentPindocMD:     in.CurrentPindocMD,
					Frontmatter:         in.PindocMDFrontmatter,
				})
				if len(hints) > 0 {
					out.HarnessDriftHints = hints
					out.HarnessDriftHint = &out.HarnessDriftHints[0]
					out.HarnessBlocked = harnessDriftBlocked(hints)
				} else {
					out.HarnessDriftHint = &clean
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

type harnessDriftProbe struct {
	WorkingDirectory    string
	ExpectedProjectSlug string
	CurrentPindocMD     string
	Frontmatter         *pingPindocFrontmatter
}

type harnessPindocSnapshot struct {
	meta map[string]string
	base HarnessDriftHint
	hint *HarnessDriftHint
}

const (
	harnessDriftCodeMissingProjectSlug  = "pindoc_md_project_slug_missing"
	harnessDriftCodeMissingSchema       = "pindoc_md_schema_version_missing"
	harnessDriftCodeMissingPindoc       = "pindoc_md_missing"
	harnessDriftCodeProjectSlugMismatch = "pindoc_md_project_slug_mismatch"
	harnessDriftCodeRootUnreadable      = "workspace_root_unreadable"
	harnessDriftCodeServerUnobservable  = "server_cannot_observe_workspace"
	harnessDriftCodeUnreadablePindoc    = "pindoc_md_unreadable"

	harnessDriftSourceClientBody        = "client_current_pindoc_md"
	harnessDriftSourceClientFrontmatter = "client_pindoc_md_frontmatter"
	harnessDriftSourceServerFilesystem  = "server_filesystem"

	harnessSuggestedNone                 = "none"
	harnessSuggestedWorkspaceDetect      = "pindoc.workspace.detect"
	harnessSuggestedHarnessInstall       = "pindoc.harness.install"
	harnessSuggestedHarnessInstallClient = "pindoc.harness.install(current_pindoc_md=...)"
)

func hasHarnessDriftProbeInput(in pingInput) bool {
	return strings.TrimSpace(in.WorkingDirectory) != "" ||
		strings.TrimSpace(in.CurrentPindocMD) != "" ||
		in.PindocMDFrontmatter != nil
}

func detectHarnessDrift(workingDirectory, expectedProjectSlug string) *HarnessDriftHint {
	hints, clean := evaluateHarnessDrift(harnessDriftProbe{
		WorkingDirectory:    workingDirectory,
		ExpectedProjectSlug: expectedProjectSlug,
	})
	if len(hints) > 0 {
		return &hints[0]
	}
	return &clean
}

func detectHarnessDrifts(workingDirectory, expectedProjectSlug string) []HarnessDriftHint {
	hints, _ := evaluateHarnessDrift(harnessDriftProbe{
		WorkingDirectory:    workingDirectory,
		ExpectedProjectSlug: expectedProjectSlug,
	})
	return hints
}

func evaluateHarnessDrift(probe harnessDriftProbe) ([]HarnessDriftHint, HarnessDriftHint) {
	snapshot := readHarnessPindocSnapshot(probe)
	if snapshot.hint != nil {
		return []HarnessDriftHint{*snapshot.hint}, HarnessDriftHint{}
	}

	clean := snapshot.base
	clean.Detected = false
	clean.SuggestedCall = harnessSuggestedNone
	if snapshot.meta != nil {
		clean.FoundProjectSlug = snapshot.meta["project_slug"]
		clean.SchemaVersion = snapshot.meta["schema_version"]
	}

	if snapshot.meta == nil {
		return nil, clean
	}

	hints := harnessDriftsFromMeta(snapshot.base, snapshot.meta)
	if len(hints) == 0 {
		return nil, clean
	}
	return hints, HarnessDriftHint{}
}

func readHarnessPindocSnapshot(probe harnessDriftProbe) harnessPindocSnapshot {
	displayPath := pindocMDPath(probe.WorkingDirectory)
	base := HarnessDriftHint{
		Detected:            true,
		SuggestedCall:       harnessSuggestedHarnessInstall,
		Path:                displayPath,
		ExpectedProjectSlug: strings.TrimSpace(probe.ExpectedProjectSlug),
	}

	if strings.TrimSpace(probe.CurrentPindocMD) != "" {
		base.Source = harnessDriftSourceClientBody
		base.SuggestedCall = harnessSuggestedHarnessInstallClient
		return harnessPindocSnapshot{
			meta: parsePindocFrontmatter(probe.CurrentPindocMD),
			base: base,
		}
	}
	if probe.Frontmatter != nil {
		base.Source = harnessDriftSourceClientFrontmatter
		base.SuggestedCall = harnessSuggestedHarnessInstallClient
		return harnessPindocSnapshot{
			meta: map[string]string{
				"project_slug":   strings.TrimSpace(probe.Frontmatter.ProjectSlug),
				"schema_version": strings.TrimSpace(probe.Frontmatter.SchemaVersion),
			},
			base: base,
		}
	}
	if strings.TrimSpace(probe.WorkingDirectory) == "" {
		return harnessPindocSnapshot{base: base}
	}

	base.Source = harnessDriftSourceServerFilesystem
	root := filepath.Clean(strings.TrimSpace(probe.WorkingDirectory))
	info, err := os.Stat(root)
	if err != nil {
		hint := base
		hint.SuggestedCall = harnessSuggestedWorkspaceDetect
		if os.IsNotExist(err) {
			hint.Code = harnessDriftCodeServerUnobservable
			hint.Severity = "info"
			hint.Reason = "MCP server cannot observe the client workspace root; this usually means the client and server filesystems differ. Provide pindoc_md_frontmatter/current_pindoc_md or call workspace.detect instead of treating PINDOC.md as missing."
		} else {
			hint.Code = harnessDriftCodeRootUnreadable
			hint.Severity = "warning"
			hint.Reason = fmt.Sprintf("MCP server could not inspect the client workspace root: %v", err)
		}
		return harnessPindocSnapshot{base: base, hint: &hint}
	}
	if !info.IsDir() {
		hint := base
		hint.Code = harnessDriftCodeRootUnreadable
		hint.Severity = "warning"
		hint.SuggestedCall = harnessSuggestedWorkspaceDetect
		hint.Reason = "working_directory is observable by the MCP server but is not a directory."
		return harnessPindocSnapshot{base: base, hint: &hint}
	}

	body, err := os.ReadFile(filepath.Join(root, "PINDOC.md"))
	if err != nil {
		hint := base
		if os.IsNotExist(err) {
			hint.Code = harnessDriftCodeMissingPindoc
			hint.Reason = "PINDOC.md is missing in the server-observed workspace root."
			hint.Severity = "info"
		} else {
			hint.Code = harnessDriftCodeUnreadablePindoc
			hint.Reason = fmt.Sprintf("PINDOC.md could not be read by the MCP server: %v", err)
			hint.Severity = "warning"
		}
		return harnessPindocSnapshot{base: base, hint: &hint}
	}
	return harnessPindocSnapshot{
		meta: parsePindocFrontmatter(string(body)),
		base: base,
	}
}

func harnessDriftsFromMeta(base HarnessDriftHint, meta map[string]string) []HarnessDriftHint {
	foundProjectSlug := meta["project_slug"]
	schemaVersion := meta["schema_version"]
	mk := func(code, severity, reason string) HarnessDriftHint {
		hint := base
		hint.Code = code
		hint.Severity = severity
		hint.Reason = reason
		hint.FoundProjectSlug = foundProjectSlug
		hint.SchemaVersion = schemaVersion
		return hint
	}
	var hints []HarnessDriftHint
	switch {
	case strings.TrimSpace(foundProjectSlug) == "":
		hints = append(hints, mk(harnessDriftCodeMissingProjectSlug, "warning", "PINDOC.md frontmatter is missing project_slug."))
	case base.ExpectedProjectSlug != "" && foundProjectSlug != base.ExpectedProjectSlug:
		hints = append(hints, mk(harnessDriftCodeProjectSlugMismatch, "blocking", fmt.Sprintf("PINDOC.md project_slug=%q does not match expected project_slug=%q.", foundProjectSlug, base.ExpectedProjectSlug)))
	}
	if strings.TrimSpace(schemaVersion) == "" {
		hints = append(hints, mk(harnessDriftCodeMissingSchema, "info", "PINDOC.md frontmatter is missing schema_version."))
	}
	sortHarnessDriftHints(hints)
	return hints
}

func pindocMDPath(workingDirectory string) string {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if workingDirectory == "" {
		return ""
	}
	workingDirectory = strings.TrimRight(workingDirectory, `\/`)
	if isWindowsLikePath(workingDirectory) {
		return strings.ReplaceAll(workingDirectory, "/", `\`) + `\PINDOC.md`
	}
	return filepath.Join(workingDirectory, "PINDOC.md")
}

func isWindowsLikePath(path string) bool {
	return len(path) >= 2 && path[1] == ':' || strings.Contains(path, `\`)
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
