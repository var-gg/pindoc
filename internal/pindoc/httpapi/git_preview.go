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
	CommitInfo *pgit.CommitInfo   `json:"commit_info,omitempty"`
	Files      []pgit.ChangedFile `json:"files,omitempty"`
}

type commitResponse struct {
	GitPreview gitPreviewEnvelope `json:"git_preview"`
	RepoID     string             `json:"repo_id"`
	Commit     string             `json:"commit"`
	CommitInfo *pgit.CommitInfo   `json:"commit_info,omitempty"`
}

type gitRepoSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	GitRemoteURL  string `json:"git_remote_url,omitempty"`
}

type gitReposResponse struct {
	ProjectSlug string           `json:"project_slug"`
	Repos       []gitRepoSummary `json:"repos"`
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

func (d Deps) handleGitRepos(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	var projectID string
	if err := d.DB.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE slug = $1`, projectSlug).Scan(&projectID); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	repos, err := pgit.LoadProjectRepos(r.Context(), d.DB, projectID)
	if err != nil {
		d.Logger.Error("git repos lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "git repos lookup failed")
		return
	}
	out := make([]gitRepoSummary, 0, len(repos))
	for _, repo := range repos {
		out = append(out, gitRepoSummary{
			ID:            repo.ID,
			Name:          repo.Name,
			DefaultBranch: repo.DefaultBranch,
			GitRemoteURL:  repo.GitRemoteURL,
		})
	}
	writeJSON(w, http.StatusOK, gitReposResponse{ProjectSlug: projectSlug, Repos: out})
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
	info, infoErr := provider.CommitInfo(r.Context(), repo, commit)
	files, err := provider.ChangedFiles(r.Context(), repo, commit)
	if err != nil {
		d.writeGitPreviewError(w, http.StatusOK, repo, commit, "", err, changedFilesResponse{})
		return
	}
	if infoErr == nil {
		commit = info.SHA
	}
	writeJSON(w, http.StatusOK, changedFilesResponse{
		GitPreview: gitPreviewEnvelope{Available: true},
		RepoID:     repo.ID,
		Commit:     commit,
		CommitInfo: commitInfoPtr(info, infoErr),
		Files:      files,
	})
}

func (d Deps) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	repo, ok := d.gitRepoFromRequest(w, r)
	if !ok {
		return
	}
	commit := strings.TrimSpace(r.URL.Query().Get("commit"))
	if commit == "" {
		commit = "HEAD"
	}
	provider := pgit.LocalGitProvider{}
	info, err := provider.CommitInfo(r.Context(), repo, commit)
	if err != nil {
		d.writeGitPreviewError(w, http.StatusOK, repo, commit, "", err, commitResponse{})
		return
	}
	writeJSON(w, http.StatusOK, commitResponse{
		GitPreview: gitPreviewEnvelope{Available: true},
		RepoID:     repo.ID,
		Commit:     info.SHA,
		CommitInfo: &info,
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

type gitCommitReference struct {
	ArtifactID  string `json:"artifact_id"`
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	AreaSlug    string `json:"area_slug"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	LinesStart  int    `json:"lines_start,omitempty"`
	LinesEnd    int    `json:"lines_end,omitempty"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

type gitCommitReferencesResponse struct {
	ProjectSlug string               `json:"project_slug"`
	Commit      string               `json:"commit"`
	References  []gitCommitReference `json:"references"`
}

func (d Deps) handleGitCommitReferences(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	sha := strings.TrimSpace(r.PathValue("sha"))
	if len(sha) < 7 {
		writeError(w, http.StatusBadRequest, "commit sha must be at least 7 characters")
		return
	}
	rows, err := d.DB.Query(r.Context(), `
		SELECT a.id::text, a.slug, a.type, a.title, ar.slug,
		       ap.kind, ap.path, COALESCE(ap.lines_start, 0), COALESCE(ap.lines_end, 0)
		  FROM artifact_pins ap
		  JOIN artifacts a ON a.id = ap.artifact_id
		  JOIN areas ar ON ar.id = a.area_id
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.status <> 'archived'
		   AND ap.commit_sha IS NOT NULL
		   AND (ap.commit_sha = $2 OR ap.commit_sha LIKE $2 || '%' OR $2 LIKE ap.commit_sha || '%')
		 ORDER BY a.updated_at DESC, a.slug, ap.path
		 LIMIT 100
	`, projectSlug, sha)
	if err != nil {
		d.Logger.Error("git commit references lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "commit references lookup failed")
		return
	}
	defer rows.Close()
	refs := make([]gitCommitReference, 0)
	for rows.Next() {
		var ref gitCommitReference
		if err := rows.Scan(
			&ref.ArtifactID, &ref.Slug, &ref.Type, &ref.Title, &ref.AreaSlug,
			&ref.Kind, &ref.Path, &ref.LinesStart, &ref.LinesEnd,
		); err != nil {
			d.Logger.Error("git commit references scan failed", "err", err)
			writeError(w, http.StatusInternalServerError, "commit references scan failed")
			return
		}
		ref.HumanURL = httpArtifactURL(projectSlug, ref.Slug)
		ref.HumanURLAbs = d.httpArtifactURLAbs(projectSlug, ref.Slug)
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		d.Logger.Error("git commit references rows failed", "err", err)
		writeError(w, http.StatusInternalServerError, "commit references rows failed")
		return
	}
	writeJSON(w, http.StatusOK, gitCommitReferencesResponse{
		ProjectSlug: projectSlug,
		Commit:      sha,
		References:  refs,
	})
}

func commitInfoPtr(info pgit.CommitInfo, err error) *pgit.CommitInfo {
	if err != nil || strings.TrimSpace(info.SHA) == "" {
		return nil
	}
	return &info
}

func httpArtifactURL(projectSlug, slug string) string {
	return "/p/" + projectSlug + "/wiki/" + slug
}

func (d Deps) httpArtifactURLAbs(projectSlug, slug string) string {
	if d.Settings == nil {
		return ""
	}
	base := strings.TrimRight(d.Settings.Get().PublicBaseURL, "/")
	if base == "" {
		return ""
	}
	return base + httpArtifactURL(projectSlug, slug)
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
	commit = strings.TrimSpace(commit)
	if commit == "" {
		commit = repo.DefaultBranch
	}
	for _, raw := range append([]string{repo.GitRemoteOriginal, repo.GitRemoteURL}, repo.URLs...) {
		if p == "" {
			if u := githubCommitURL(raw, commit); u != "" {
				return u
			}
			continue
		}
		if u := githubBlobURL(raw, commit, p); u != "" {
			return u
		}
	}
	return ""
}

func githubBlobURL(raw, commit, p string) string {
	owner, name := githubRepoParts(raw)
	if owner == "" || name == "" {
		return ""
	}
	return "https://github.com/" + owner + "/" + name + "/blob/" + url.PathEscape(commit) + "/" + strings.TrimLeft(p, "/")
}

func githubCommitURL(raw, commit string) string {
	owner, name := githubRepoParts(raw)
	if owner == "" || name == "" {
		return ""
	}
	if commit == "" {
		return "https://github.com/" + owner + "/" + name
	}
	return "https://github.com/" + owner + "/" + name + "/commit/" + url.PathEscape(commit)
}

func githubRepoParts(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	if !strings.Contains(raw, "://") && strings.HasPrefix(raw, "github.com/") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return "", ""
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
