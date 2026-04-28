package git

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

type RepoQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ResolvePinRepoID maps an incoming pin to the canonical project_repos.id.
// Failure is graceful: callers store NULL and surface a registration warning.
func ResolvePinRepoID(ctx context.Context, q RepoQueryer, projectID, explicitRepoID, repoName, pinPath, repoRoot string) (string, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if q == nil || projectID == "" {
		return "", false, nil
	}
	explicitRepoID = strings.TrimSpace(explicitRepoID)
	if explicitRepoID != "" {
		var id string
		err := q.QueryRow(ctx, `
			SELECT id::text
			  FROM project_repos
			 WHERE project_id = $1::uuid AND id = $2::uuid
			 LIMIT 1
		`, projectID, explicitRepoID).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}

	repos, err := LoadProjectRepos(ctx, q, projectID)
	if err != nil {
		return "", false, err
	}
	if len(repos) == 0 {
		return "", false, nil
	}

	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		repoName = "origin"
	}
	if id, ok := matchRepoNameOrURL(repos, repoName); ok {
		return id, true, nil
	}
	if id, ok := matchRepoPath(repos, repoRoot); ok {
		return id, true, nil
	}
	if id, ok := matchRepoPath(repos, pinPath); ok {
		return id, true, nil
	}
	if id, ok := matchRepoNameOrURL(repos, pinPath); ok {
		return id, true, nil
	}
	if repoName == "origin" && len(repos) == 1 {
		return repos[0].ID, true, nil
	}
	return "", false, nil
}

func LoadProjectRepos(ctx context.Context, q RepoQueryer, projectID string) ([]Repo, error) {
	rows, err := q.Query(ctx, `
		SELECT id::text,
		       COALESCE(name, ''),
		       git_remote_url,
		       git_remote_url_original,
		       default_branch,
		       local_paths,
		       urls
		  FROM project_repos
		 WHERE project_id = $1::uuid
		 ORDER BY created_at ASC
	`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Repo
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Name, &r.GitRemoteURL, &r.GitRemoteOriginal, &r.DefaultBranch, &r.LocalPaths, &r.URLs); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func matchRepoNameOrURL(repos []Repo, raw string) (string, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "", false
	}
	for _, repo := range repos {
		if raw == strings.ToLower(strings.TrimSpace(repo.Name)) ||
			raw == strings.ToLower(strings.TrimSpace(repo.GitRemoteURL)) ||
			raw == strings.ToLower(strings.TrimSpace(repo.GitRemoteOriginal)) {
			return repo.ID, true
		}
		for _, u := range repo.URLs {
			if raw == strings.ToLower(strings.TrimSpace(u)) {
				return repo.ID, true
			}
		}
	}
	return "", false
}

func matchRepoPath(repos []Repo, rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || !filepath.IsAbs(rawPath) {
		return "", false
	}
	candidate := cleanAbs(rawPath)
	for _, repo := range repos {
		for _, p := range repo.LocalPaths {
			root := cleanAbs(p)
			if root == "" {
				continue
			}
			if candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator)) {
				return repo.ID, true
			}
		}
	}
	return "", false
}

func cleanAbs(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}
