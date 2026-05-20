package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type workspaceDetectInput struct {
	WorkspacePath string                 `json:"workspace_path,omitempty" jsonschema:"optional agent current working directory"`
	GitRemoteURL  string                 `json:"git_remote_url,omitempty" jsonschema:"optional output of git remote get-url origin"`
	Frontmatter   *pingPindocFrontmatter `json:"pindoc_md_frontmatter,omitempty" jsonschema:"optional PINDOC.md frontmatter fields; workspace.detect uses project_slug and accepts schema_version for compatibility with ping harness drift probes"`
}

type workspaceDetectOutput struct {
	ProjectSlug    string                     `json:"project_slug,omitempty"`
	Confidence     string                     `json:"confidence" jsonschema:"one of high | medium | low | none"`
	Via            string                     `json:"via" jsonschema:"one of pindoc_md | git_remote | directory_match | fallback_only_one | fallback_required"`
	Candidates     []string                   `json:"candidates,omitempty"`
	Reason         string                     `json:"reason,omitempty"`
	Warnings       []string                   `json:"warnings,omitempty" jsonschema:"non-blocking warnings; current codes: PROJECT_REPOS_EMPTY_FOR_PINDOC_MD_MATCH"`
	NextAction     *workspaceDetectNextAction `json:"next_action,omitempty" jsonschema:"suggested tool call to close a recoverable gap (e.g. register a missing project_repos row)"`
	ToolsetVersion string                     `json:"toolset_version,omitempty"`
}

// workspaceDetectNextAction is the hint shape the harness can replay
// directly: tool name + args ready for CallTool. Reason is the human-
// readable rationale surfaced when the agent declines or the user wants
// to know why the suggestion appeared.
type workspaceDetectNextAction struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Reason string         `json:"reason"`
}

func RegisterWorkspaceDetect(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.workspace.detect",
			Description: "Resolve the likely project_slug for the current workspace from client-provided PINDOC.md frontmatter, git remote URL, workspace directory name, and visible-project fallback. This tool does not require the MCP server to read the client's local PINDOC.md file.",
		},
		func(ctx context.Context, p *auth.Principal, in workspaceDetectInput) (*sdk.CallToolResult, workspaceDetectOutput, error) {
			out, err := handleWorkspaceDetect(ctx, deps, p, in)
			return nil, out, err
		},
	)
}

func handleWorkspaceDetect(ctx context.Context, deps Deps, p *auth.Principal, in workspaceDetectInput) (workspaceDetectOutput, error) {
	visible, err := visibleProjectSlugs(ctx, deps, p)
	if err != nil {
		return workspaceDetectOutput{}, err
	}
	lookup := func(rawRemote string) (string, bool) {
		slug, err := projects.LookupProjectSlugByGitRemoteURL(ctx, deps.DB, rawRemote)
		if err != nil {
			if isNoProjectRepoMatch(err) {
				return "", false
			}
			return "", false
		}
		return slug, true
	}
	out := detectWorkspaceFromSources(in, visible, lookup)
	enrichWorkspaceDetectWithRegisterHint(ctx, deps, p, in, &out)
	return out, nil
}

// enrichWorkspaceDetectWithRegisterHint surfaces a missing project_repos
// row for the resolved project: it emits a PROJECT_REPOS_EMPTY_FOR_PINDOC_MD_MATCH
// warning and, for callers who can actually act on it, a replayable
// project.set_repo next_action. Closes the dogfood gap where pin.add_pin
// emits PIN_REPO_NOT_REGISTERED for a workspace that workspace.detect
// resolved via PINDOC.md frontmatter.
//
// Deliberately conservative — only fires when:
//   - Confidence == "high" (PINDOC.md frontmatter). Medium/low directory or
//     fallback matches are too ambiguous to risk pointing a mutating
//     set_repo call at the wrong project.
//   - the caller provided a parseable git_remote_url, and
//   - the project owns no row for that normalized remote.
//
// The replayable next_action is attached only when the caller holds
// write.project (set_repo is owner-only); non-owners still get the warning
// so they know to ask a project owner.
func enrichWorkspaceDetectWithRegisterHint(ctx context.Context, deps Deps, p *auth.Principal, in workspaceDetectInput, out *workspaceDetectOutput) {
	if out == nil || out.ProjectSlug == "" {
		return
	}
	// git_remote already confirmed the row; medium/low matches are too
	// ambiguous to attach a mutating hint to.
	if out.Confidence != "high" || out.Via == "git_remote" {
		return
	}
	rawRemote := strings.TrimSpace(in.GitRemoteURL)
	if rawRemote == "" {
		return
	}
	normalized, err := projects.NormalizeGitRemoteURL(rawRemote)
	if err != nil || normalized == "" {
		return
	}
	if deps.DB == nil {
		return
	}
	var exists bool
	if err := deps.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM project_repos pr
			  JOIN projects p ON p.id = pr.project_id
			 WHERE p.slug = $1
			   AND pr.git_remote_url = $2
		)
	`, out.ProjectSlug, normalized).Scan(&exists); err != nil {
		// Non-blocking enrichment — never fail the detect call over it,
		// but leave a trace so a missing hint can be told apart from a
		// genuinely registered repo.
		if deps.Logger != nil {
			deps.Logger.Warn("workspace.detect register-hint lookup failed",
				"project_slug", out.ProjectSlug, "error", err)
		}
		return
	}
	if exists {
		return
	}

	// The remote is unregistered for this project — surface the gap to
	// every caller regardless of role.
	out.Warnings = appendUniqueWarning(out.Warnings, "PROJECT_REPOS_EMPTY_FOR_PINDOC_MD_MATCH")

	// project.set_repo is owner-only; only hand back a replayable
	// next_action when the caller can run it. Non-owners keep the warning.
	scope, err := auth.ResolveProject(ctx, deps.DB, p, out.ProjectSlug)
	if err != nil || !scope.Can("write.project") {
		return
	}

	args := map[string]any{
		"project_slug":   out.ProjectSlug,
		"git_remote_url": rawRemote,
	}
	if local := strings.TrimSpace(in.WorkspacePath); local != "" {
		args["local_paths"] = []string{local}
	}
	out.NextAction = &workspaceDetectNextAction{
		Tool: "pindoc.project.set_repo",
		Args: args,
		Reason: "project_repos has no row mapping " + normalized +
			" to project_slug " + out.ProjectSlug +
			". Register the workspace so pin.repo_id auto-mapping and PIN_REPO_NOT_REGISTERED both resolve.",
	}
}

func appendUniqueWarning(existing []string, code string) []string {
	for _, w := range existing {
		if w == code {
			return existing
		}
	}
	return append(existing, code)
}

func detectWorkspaceFromSources(
	in workspaceDetectInput,
	visible []string,
	remoteLookup func(string) (string, bool),
) workspaceDetectOutput {
	visible = normalizedSlugList(visible)
	visibleSet := make(map[string]bool, len(visible))
	for _, slug := range visible {
		visibleSet[slug] = true
	}

	if out, ok := workspaceDetectFromFrontmatter(in.Frontmatter, visibleSet, "PINDOC.md frontmatter project_slug is visible to the caller."); ok {
		return out
	}
	if out, ok := workspaceDetectFromFrontmatter(
		readWorkspacePindocFrontmatter(in.WorkspacePath),
		visibleSet,
		"workspace_path/PINDOC.md frontmatter project_slug is visible to the caller.",
	); ok {
		return out
	}

	if remote := strings.TrimSpace(in.GitRemoteURL); remote != "" && remoteLookup != nil {
		if slug, ok := remoteLookup(remote); ok {
			slug = normalizeDetectSlug(slug)
			if visibleSet[slug] {
				return workspaceDetectOutput{
					ProjectSlug: slug,
					Confidence:  "high",
					Via:         "git_remote",
					Reason:      "git remote URL matched project_repos.",
				}
			}
		}
	}

	base := workspaceBasename(in.WorkspacePath)
	if base != "" {
		if visibleSet[base] {
			return workspaceDetectOutput{
				ProjectSlug: base,
				Confidence:  "medium",
				Via:         "directory_match",
				Reason:      "workspace directory name exactly matches a visible project slug.",
			}
		}
		candidates := directoryCandidates(base, visible)
		if len(candidates) > 0 {
			out := workspaceDetectOutput{
				Confidence: "low",
				Via:        "directory_match",
				Candidates: candidates,
				Reason:     "workspace directory name partially matches visible project slugs.",
			}
			if len(candidates) == 1 {
				out.ProjectSlug = candidates[0]
			}
			return out
		}
	}

	switch len(visible) {
	case 0:
		return workspaceDetectOutput{
			Confidence: "none",
			Via:        "fallback_required",
			Reason:     "no visible projects for the caller.",
		}
	case 1:
		return workspaceDetectOutput{
			ProjectSlug: visible[0],
			Confidence:  "medium",
			Via:         "fallback_only_one",
			Reason:      "only one visible project is available to the caller.",
		}
	default:
		return workspaceDetectOutput{
			Confidence: "none",
			Via:        "fallback_required",
			Candidates: visible,
			Reason:     "multiple visible projects require an explicit project_slug.",
		}
	}
}

func workspaceDetectFromFrontmatter(frontmatter *pingPindocFrontmatter, visibleSet map[string]bool, visibleReason string) (workspaceDetectOutput, bool) {
	if frontmatter == nil {
		return workspaceDetectOutput{}, false
	}
	slug := normalizeDetectSlug(frontmatter.ProjectSlug)
	if slug == "" {
		return workspaceDetectOutput{}, false
	}
	if visibleSet[slug] {
		return workspaceDetectOutput{
			ProjectSlug: slug,
			Confidence:  "high",
			Via:         "pindoc_md",
			Reason:      visibleReason,
		}, true
	}
	return workspaceDetectOutput{
		Confidence: "none",
		Via:        "pindoc_md",
		Reason:     "PINDOC.md project_slug is not visible to the caller.",
	}, true
}

func readWorkspacePindocFrontmatter(workspacePath string) *pingPindocFrontmatter {
	root := strings.TrimSpace(workspacePath)
	if root == "" {
		return nil
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, "PINDOC.md"))
	if err != nil {
		return nil
	}
	meta := parsePindocFrontmatter(string(body))
	if len(meta) == 0 {
		return nil
	}
	return &pingPindocFrontmatter{
		ProjectSlug:   meta["project_slug"],
		SchemaVersion: meta["schema_version"],
	}
}

func visibleProjectSlugs(ctx context.Context, deps Deps, p *auth.Principal) ([]string, error) {
	if deps.DB == nil {
		return nil, errors.New("workspace.detect: database pool not configured")
	}
	rows, err := deps.DB.Query(ctx, `SELECT slug FROM projects ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		if _, err := auth.ResolveProject(ctx, deps.DB, p, slug); err == nil {
			out = append(out, slug)
		} else if !errors.Is(err, auth.ErrProjectAccessDenied) && !errors.Is(err, auth.ErrProjectNotFound) {
			return nil, err
		}
	}
	return normalizedSlugList(out), rows.Err()
}

func normalizedSlugList(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, slug := range in {
		slug = normalizeDetectSlug(slug)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

func normalizeDetectSlug(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func workspaceBasename(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return normalizeDetectSlug(filepath.Base(filepath.Clean(path)))
}

func directoryCandidates(base string, visible []string) []string {
	var out []string
	for _, slug := range visible {
		if slug == "" || slug == base {
			continue
		}
		if strings.HasPrefix(base, slug+"-") || strings.HasSuffix(base, "-"+slug) {
			out = append(out, slug)
		}
	}
	return normalizedSlugList(out)
}

func isNoProjectRepoMatch(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, projects.ErrGitRemoteURLInvalid)
}
