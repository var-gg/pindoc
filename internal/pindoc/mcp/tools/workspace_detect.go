package tools

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type workspaceDetectInput struct {
	WorkspacePath string `json:"workspace_path,omitempty" jsonschema:"optional agent current working directory"`
	GitRemoteURL  string `json:"git_remote_url,omitempty" jsonschema:"optional output of git remote get-url origin"`
	Frontmatter   *struct {
		ProjectSlug string `json:"project_slug,omitempty"`
	} `json:"pindoc_md_frontmatter,omitempty" jsonschema:"optional PINDOC.md frontmatter fields"`
}

type workspaceDetectOutput struct {
	ProjectSlug string   `json:"project_slug,omitempty"`
	Confidence  string   `json:"confidence" jsonschema:"one of high | medium | low | none"`
	Via         string   `json:"via" jsonschema:"one of pindoc_md | git_remote | directory_match | fallback_only_one | fallback_required"`
	Candidates  []string `json:"candidates,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}

func RegisterWorkspaceDetect(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.workspace.detect",
			Description: "Resolve the likely project_slug for the current workspace from PINDOC.md frontmatter, git remote URL, workspace directory name, and visible-project fallback.",
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
	return detectWorkspaceFromSources(in, visible, lookup), nil
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

	if in.Frontmatter != nil {
		slug := normalizeDetectSlug(in.Frontmatter.ProjectSlug)
		if slug != "" {
			if visibleSet[slug] {
				return workspaceDetectOutput{
					ProjectSlug: slug,
					Confidence:  "high",
					Via:         "pindoc_md",
					Reason:      "PINDOC.md frontmatter project_slug is visible to the caller.",
				}
			}
			return workspaceDetectOutput{
				Confidence: "none",
				Via:        "pindoc_md",
				Reason:     "PINDOC.md project_slug is not visible to the caller.",
			}
		}
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
