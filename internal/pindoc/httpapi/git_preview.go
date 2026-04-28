package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	pgit "github.com/var-gg/pindoc/internal/pindoc/git"
)

type gitPreviewEnvelope struct {
	Available   bool   `json:"available"`
	Reason      string `json:"reason,omitempty"`
	FallbackURL string `json:"fallback_url,omitempty"`
}

type changedFilesResponse struct {
	GitPreview gitPreviewEnvelope `json:"git_preview"`
	RepoID     string             `json:"repo_id"`
	Commit     string             `json:"commit"`
	Files      []pgit.ChangedFile `json:"files,omitempty"`
}

type blobResponse struct {
	GitPreview gitPreviewEnvelope `json:"git_preview"`
	RepoID     string             `json:"repo_id"`
	Commit     string             `json:"commit"`
	Blob       *pgit.Blob         `json:"blob,omitempty"`
}

type diffResponse struct {
	GitPreview gitPreviewEnvelope `json:"git_preview"`
	RepoID     string             `json:"repo_id"`
	Commit     string             `json:"commit"`
	Path       string             `json:"path,omitempty"`
	Diff       string             `json:"diff,omitempty"`
}

func (d Deps) handleGitChangedFiles(w http.ResponseWriter, r *http.Request) {
	repo, ok := d.gitRepoFromRequest(w, r)
	if !ok {
		return
	}
	commit := strings.TrimSpace(r.URL.Query().Get("commit"))
	if commit == "" {
		commit = "HEAD"
	}
	provider := pgit.LocalGitProvider{}
	files, err := provider.ChangedFiles(r.Context(), repo, commit)
	if err != nil {
		d.writeGitPreviewError(w, http.StatusOK, repo, commit, "", err, changedFilesResponse{})
		return
	}
	writeJSON(w, http.StatusOK, changedFilesResponse{
		GitPreview: gitPreviewEnvelope{Available: true},
		RepoID:     repo.ID,
		Commit:     commit,
		Files:      files,
	})
}

func (d Deps) handleGitBlob(w http.ResponseWriter, r *http.Request) {
	repo, ok := d.gitRepoFromRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	commit := strings.TrimSpace(q.Get("commit"))
	if commit == "" {
		commit = "HEAD"
	}
	path := strings.TrimSpace(q.Get("path"))
	provider := pgit.LocalGitProvider{}
	blob, err := provider.Blob(r.Context(), repo, commit, path, pgit.BlobSizeCap)
	if err != nil {
		if errors.Is(err, pgit.ErrSizeCapExceeded) {
			writeJSON(w, http.StatusOK, blobResponse{
				GitPreview: d.gitPreview(repo, commit, path, "size_cap_exceeded"),
				RepoID:     repo.ID,
				Commit:     commit,
				Blob:       &blob,
			})
			return
		}
		d.writeGitPreviewError(w, http.StatusOK, repo, commit, path, err, blobResponse{})
		return
	}
	writeJSON(w, http.StatusOK, blobResponse{
		GitPreview: gitPreviewEnvelope{Available: true},
		RepoID:     repo.ID,
		Commit:     commit,
		Blob:       &blob,
	})
}

func (d Deps) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	repo, ok := d.gitRepoFromRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	commit := strings.TrimSpace(q.Get("commit"))
	if commit == "" {
		commit = "HEAD"
	}
	path := strings.TrimSpace(q.Get("path"))
	provider := pgit.LocalGitProvider{}
	diff, err := provider.Diff(r.Context(), repo, commit, path)
	if err != nil {
		d.writeGitPreviewError(w, http.StatusOK, repo, commit, path, err, diffResponse{})
		return
	}
	writeJSON(w, http.StatusOK, diffResponse{
		GitPreview: gitPreviewEnvelope{Available: true},
		RepoID:     repo.ID,
		Commit:     commit,
		Path:       path,
		Diff:       diff,
	})
}

func (d Deps) gitRepoFromRequest(w http.ResponseWriter, r *http.Request) (pgit.Repo, bool) {
	repoID := strings.TrimSpace(r.URL.Query().Get("repo_id"))
	if repoID == "" {
		writeError(w, http.StatusBadRequest, "repo_id is required")
		return pgit.Repo{}, false
	}
	repo, err := d.loadGitRepo(r.Context(), projectSlugFrom(r), repoID)
	if errors.Is(err, errGitRepoNotFound) {
		writeError(w, http.StatusNotFound, "repo not found")
		return pgit.Repo{}, false
	}
	if err != nil {
		d.Logger.Error("git repo lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "repo lookup failed")
		return pgit.Repo{}, false
	}
	if strings.TrimSpace(d.RepoRoot) != "" && len(repo.LocalPaths) == 0 {
		repo.LocalPaths = append(repo.LocalPaths, d.RepoRoot)
	}
	return repo, true
}

var errGitRepoNotFound = errors.New("git_repo_not_found")

func (d Deps) loadGitRepo(ctx context.Context, projectSlug, repoID string) (pgit.Repo, error) {
	var projectID string
	if err := d.DB.QueryRow(ctx, `SELECT id::text FROM projects WHERE slug = $1`, projectSlug).Scan(&projectID); err != nil {
		return pgit.Repo{}, errGitRepoNotFound
	}
	repos, err := pgit.LoadProjectRepos(ctx, d.DB, projectID)
	if err != nil {
		return pgit.Repo{}, err
	}
	for _, repo := range repos {
		if repo.ID == repoID {
			return repo, nil
		}
	}
	return pgit.Repo{}, errGitRepoNotFound
}

func (d Deps) writeGitPreviewError(w http.ResponseWriter, defaultStatus int, repo pgit.Repo, commit, path string, err error, _ any) {
	status := defaultStatus
	reason := gitPreviewReason(err)
	switch reason {
	case "commit_too_short", "path_rejected":
		status = http.StatusBadRequest
	case "commit_not_found", "path_not_found":
		status = http.StatusNotFound
	case "no_provider_for_repo":
		status = http.StatusGone
	}
	writeJSON(w, status, map[string]any{
		"git_preview": d.gitPreview(repo, commit, path, reason),
		"repo_id":     repo.ID,
		"commit":      commit,
	})
}

func (d Deps) gitPreview(repo pgit.Repo, commit, path, reason string) gitPreviewEnvelope {
	return gitPreviewEnvelope{
		Available:   false,
		Reason:      reason,
		FallbackURL: githubFallbackURL(repo, commit, path),
	}
}

func gitPreviewReason(err error) string {
	switch {
	case errors.Is(err, pgit.ErrNoProviderForRepo):
		return "no_provider_for_repo"
	case errors.Is(err, pgit.ErrCommitNotFound):
		return "commit_not_found"
	case errors.Is(err, pgit.ErrPathNotFound):
		return "path_not_found"
	case errors.Is(err, pgit.ErrPathRejected):
		return "path_rejected"
	case errors.Is(err, pgit.ErrCommitTooShort):
		return "commit_too_short"
	case errors.Is(err, pgit.ErrSizeCapExceeded):
		return "size_cap_exceeded"
	default:
		return "provider_error"
	}
}

func githubFallbackURL(repo pgit.Repo, commit, p string) string {
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" {
		return ""
	}
	commit = strings.TrimSpace(commit)
	if commit == "" {
		commit = repo.DefaultBranch
	}
	for _, raw := range append([]string{repo.GitRemoteOriginal, repo.GitRemoteURL}, repo.URLs...) {
		if u := githubBlobURL(raw, commit, p); u != "" {
			return u
		}
	}
	return ""
}

func githubBlobURL(raw, commit, p string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	if !strings.Contains(raw, "://") && strings.HasPrefix(raw, "github.com/") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return "https://github.com/" + parts[0] + "/" + parts[1] + "/blob/" + url.PathEscape(commit) + "/" + strings.TrimLeft(p, "/")
}
