package tools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

const (
	projectSlugDefaultEnv       = "PINDOC_PROJECT"
	projectSlugDefaultWorkspace = "workspace"
)

type projectSlugDefaultResult struct {
	ProjectSlug string
	Via         string
	Candidates  []string
	Reason      string
}

// applyProjectSlugDefaulting mutates the local tool input copy before the
// handler runs. Explicit project_slug always wins. Empty project_slug falls
// back to PINDOC_PROJECT, then to a unique repo/workspace mapping derived
// from the server-visible RepoRoot. If no conservative answer exists, the
// handler runs unchanged and auth.ResolveProject returns PROJECT_SLUG_REQUIRED
// as before.
func applyProjectSlugDefaulting(ctx context.Context, deps Deps, p *auth.Principal, input any) projectSlugDefaultResult {
	rv := reflect.ValueOf(input)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return projectSlugDefaultResult{}
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return projectSlugDefaultResult{}
	}
	f := rv.FieldByName("ProjectSlug")
	if !f.IsValid() || !f.CanSet() || f.Kind() != reflect.String {
		return projectSlugDefaultResult{}
	}
	if strings.TrimSpace(f.String()) != "" {
		return projectSlugDefaultResult{}
	}

	res := resolveDefaultProjectSlug(ctx, deps, p)
	if res.ProjectSlug != "" {
		f.SetString(res.ProjectSlug)
	}
	return res
}

func projectSlugFieldValue(input any) string {
	rv := reflect.ValueOf(input)
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
	return strings.TrimSpace(f.String())
}

func projectSlugDefaultNotReady[O any](res projectSlugDefaultResult) (O, bool) {
	var output O
	rv := reflect.ValueOf(&output).Elem()
	for rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return output, false
		}
		rv = rv.Elem()
	}
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return output, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return output, false
	}
	status := rv.FieldByName("Status")
	if !status.IsValid() || !status.CanSet() || status.Kind() != reflect.String {
		return output, false
	}
	status.SetString("not_ready")
	setStringField(rv, "ErrorCode", "PROJECT_SLUG_REQUIRED")
	setStringSliceField(rv, "Failed", []string{"PROJECT_SLUG_REQUIRED"})
	setStringSliceField(rv, "Checklist", []string{projectSlugRequiredHint(res)})
	setStringSliceField(rv, "SuggestedActions", []string{
		"Pass project_slug explicitly, set PINDOC_PROJECT, or run pindoc.workspace.detect and retry with the resolved project_slug.",
	})
	return output, true
}

func resolveDefaultProjectSlug(ctx context.Context, deps Deps, p *auth.Principal) projectSlugDefaultResult {
	if slug := strings.TrimSpace(deps.DefaultProjectSlug); slug != "" {
		return projectSlugDefaultResult{
			ProjectSlug: slug,
			Via:         projectSlugDefaultEnv,
			Reason:      "project_slug omitted; using PINDOC_PROJECT fallback.",
		}
	}

	repoRoot := strings.TrimSpace(deps.RepoRoot)
	if repoRoot == "" || deps.DB == nil {
		return projectSlugDefaultResult{
			Via:    "missing",
			Reason: "project_slug omitted and neither PINDOC_PROJECT nor RepoRoot workspace signals are configured.",
		}
	}

	if slugs, err := projectSlugsByLocalPath(ctx, deps, p, repoRoot); err == nil && len(slugs) > 0 {
		return uniqueProjectSlugDefault(slugs, "workspace_path")
	}

	rawRemote, err := gitRemoteFromWorkdir(ctx, deps, repoRoot)
	if err != nil || strings.TrimSpace(rawRemote) == "" {
		return projectSlugDefaultResult{
			Via:    "missing",
			Reason: "project_slug omitted; RepoRoot exists but no git remote mapping could be read.",
		}
	}
	slugs, err := projectSlugsByGitRemote(ctx, deps, p, rawRemote)
	if err != nil {
		return projectSlugDefaultResult{
			Via:    "missing",
			Reason: "project_slug omitted; git remote did not match project_repos.",
		}
	}
	return uniqueProjectSlugDefault(slugs, projectSlugDefaultWorkspace)
}

func uniqueProjectSlugDefault(slugs []string, via string) projectSlugDefaultResult {
	slugs = normalizedSlugList(slugs)
	switch len(slugs) {
	case 0:
		return projectSlugDefaultResult{
			Via:    "missing",
			Reason: "project_slug omitted; workspace mapping returned no visible projects.",
		}
	case 1:
		return projectSlugDefaultResult{
			ProjectSlug: slugs[0],
			Via:         via,
			Reason:      "project_slug omitted; workspace mapping resolved exactly one visible project.",
		}
	default:
		return projectSlugDefaultResult{
			Via:        "ambiguous",
			Candidates: slugs,
			Reason:     "project_slug omitted; workspace mapping is ambiguous.",
		}
	}
}

func projectSlugsByLocalPath(ctx context.Context, deps Deps, p *auth.Principal, localPath string) ([]string, error) {
	clean := strings.TrimSpace(localPath)
	if clean == "" || deps.DB == nil {
		return nil, pgx.ErrNoRows
	}
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	clean = filepath.Clean(clean)
	rows, err := deps.DB.Query(ctx, `
		SELECT DISTINCT p.slug
		  FROM project_repos pr
		  JOIN projects p ON p.id = pr.project_id
		 WHERE $1 = ANY(pr.local_paths)
		 ORDER BY p.slug
	`, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return visibleSlugsFromRows(ctx, deps, p, rows)
}

func projectSlugsByGitRemote(ctx context.Context, deps Deps, p *auth.Principal, rawRemote string) ([]string, error) {
	normalized, err := projects.NormalizeGitRemoteURL(rawRemote)
	if err != nil {
		return nil, err
	}
	if normalized == "" || deps.DB == nil {
		return nil, pgx.ErrNoRows
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT DISTINCT p.slug
		  FROM project_repos pr
		  JOIN projects p ON p.id = pr.project_id
		 WHERE pr.git_remote_url = $1
		 ORDER BY p.slug
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return visibleSlugsFromRows(ctx, deps, p, rows)
}

type slugRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func visibleSlugsFromRows(ctx context.Context, deps Deps, p *auth.Principal, rows slugRows) ([]string, error) {
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func projectSlugRequiredHint(res projectSlugDefaultResult) string {
	if len(res.Candidates) > 0 {
		return fmt.Sprintf("project_slug is required; workspace mapping was ambiguous. Candidates: %s", strings.Join(res.Candidates, ", "))
	}
	if res.Reason != "" {
		return "project_slug is required; " + res.Reason
	}
	return "project_slug is required; pass project_slug explicitly or set PINDOC_PROJECT."
}

func setStringField(rv reflect.Value, name, value string) {
	f := rv.FieldByName(name)
	if f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(value)
	}
}

func setStringSliceField(rv reflect.Value, name string, value []string) {
	f := rv.FieldByName(name)
	if !f.IsValid() || !f.CanSet() || f.Kind() != reflect.Slice || f.Type().Elem().Kind() != reflect.String {
		return
	}
	f.Set(reflect.ValueOf(value))
}
