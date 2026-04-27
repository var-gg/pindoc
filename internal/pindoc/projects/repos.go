package projects

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrGitRemoteURLInvalid = errors.New("GIT_REMOTE_URL_INVALID")

type repoQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ProjectRepoInput struct {
	ProjectID     string
	GitRemoteURL  string
	Name          string
	DefaultBranch string
}

// NormalizeGitRemoteURL converts common Git remote shapes into the DB lookup
// key workspace.detect can compare against. The canonical form is
// host/owner/repo, lowercased, without scheme, auth, port, or .git suffix.
func NormalizeGitRemoteURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	s = strings.TrimRight(s, "/")

	var host, path string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("%w: parse %q: %v", ErrGitRemoteURLInvalid, raw, err)
		}
		host = u.Hostname()
		path = u.Path
	} else if i := strings.Index(s, ":"); i > 0 && !strings.Contains(s[:i], "/") {
		// SCP-like SSH syntax: git@github.com:owner/repo.git
		host = s[:i]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		path = s[i+1:]
	} else {
		parts := strings.SplitN(strings.TrimPrefix(s, "//"), "/", 2)
		if len(parts) == 2 {
			host = parts[0]
			path = parts[1]
		}
	}

	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.ToLower(strings.Trim(path, "/"))
	if host == "" || path == "" || !strings.Contains(path, "/") {
		return "", fmt.Errorf("%w: git remote URL must resolve to host/owner/repo: %q", ErrGitRemoteURLInvalid, raw)
	}
	return host + "/" + path, nil
}

func AddProjectRepo(ctx context.Context, q repoQueryer, in ProjectRepoInput) (string, error) {
	if q == nil {
		return "", errors.New("project repo insert: nil queryer")
	}
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return "", errors.New("project repo insert: project id is required")
	}
	normalized, err := NormalizeGitRemoteURL(in.GitRemoteURL)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", nil
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "origin"
	}
	branch := strings.TrimSpace(in.DefaultBranch)
	if branch == "" {
		branch = "main"
	}

	var id string
	if err := q.QueryRow(ctx, `
		INSERT INTO project_repos (
			project_id, git_remote_url, git_remote_url_original, name, default_branch
		) VALUES ($1::uuid, $2, $3, $4, $5)
		ON CONFLICT (project_id, git_remote_url) DO UPDATE SET
			git_remote_url_original = EXCLUDED.git_remote_url_original,
			name = EXCLUDED.name,
			default_branch = EXCLUDED.default_branch
		RETURNING id::text
	`, projectID, normalized, strings.TrimSpace(in.GitRemoteURL), name, branch).Scan(&id); err != nil {
		return "", fmt.Errorf("project repo insert: %w", err)
	}
	return id, nil
}

func EnsureDefaultProjectRepo(ctx context.Context, q repoQueryer, projectSlug, rawRemote string) error {
	if q == nil {
		return nil
	}
	projectSlug = strings.TrimSpace(projectSlug)
	if projectSlug == "" || strings.TrimSpace(rawRemote) == "" {
		return nil
	}
	normalized, err := NormalizeGitRemoteURL(rawRemote)
	if err != nil {
		return err
	}
	if normalized == "" {
		return nil
	}
	var id string
	err = q.QueryRow(ctx, `
		INSERT INTO project_repos (
			project_id, git_remote_url, git_remote_url_original, name, default_branch
		)
		SELECT p.id, $2, $3, 'origin', 'main'
		  FROM projects p
		 WHERE p.slug = $1
		   AND NOT EXISTS (
			 SELECT 1 FROM project_repos pr WHERE pr.project_id = p.id
		   )
		ON CONFLICT (project_id, git_remote_url) DO NOTHING
		RETURNING id::text
	`, projectSlug, normalized, strings.TrimSpace(rawRemote)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("default project repo bootstrap: %w", err)
	}
	return nil
}

func LookupProjectSlugByGitRemoteURL(ctx context.Context, q repoQueryer, rawRemote string) (string, error) {
	if q == nil {
		return "", errors.New("project repo lookup: nil queryer")
	}
	normalized, err := NormalizeGitRemoteURL(rawRemote)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", pgx.ErrNoRows
	}
	var slug string
	if err := q.QueryRow(ctx, `
		SELECT p.slug
		  FROM project_repos pr
		  JOIN projects p ON p.id = pr.project_id
		 WHERE pr.git_remote_url = $1
		 ORDER BY pr.created_at ASC
		 LIMIT 1
	`, normalized).Scan(&slug); err != nil {
		return "", err
	}
	return slug, nil
}

func GitRemoteURLFromWorkdir(ctx context.Context, workdir string) (string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		workdir = "."
	}
	out, err := exec.CommandContext(ctx, "git", "-C", workdir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func BootstrapDefaultProjectRepoFromWorkdir(ctx context.Context, q repoQueryer, projectSlug, workdir string) (string, error) {
	raw, err := GitRemoteURLFromWorkdir(ctx, workdir)
	if err != nil {
		return "", err
	}
	normalized, err := NormalizeGitRemoteURL(raw)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", nil
	}
	if err := EnsureDefaultProjectRepo(ctx, q, projectSlug, raw); err != nil {
		return "", err
	}
	return normalized, nil
}
