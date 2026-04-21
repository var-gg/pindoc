package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/diff"
)

type revisionRow struct {
	RevisionNumber int       `json:"revision_number"`
	Title          string    `json:"title"`
	BodyHash       string    `json:"body_hash"`
	AuthorID       string    `json:"author_id"`
	AuthorVersion  string    `json:"author_version,omitempty"`
	CommitMsg      string    `json:"commit_msg,omitempty"`
	Completeness   string    `json:"completeness"`
	CreatedAt      time.Time `json:"created_at"`
}

func (d Deps) handleArtifactRevisions(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	ref := r.PathValue("idOrSlug")
	if ref == "" {
		writeError(w, http.StatusBadRequest, "missing id or slug")
		return
	}
	var artifactID, slug, title string
	err := d.DB.QueryRow(r.Context(), `
		SELECT a.id::text, a.slug, a.title
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
	`, projectSlug, ref).Scan(&artifactID, &slug, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		d.Logger.Error("revisions head lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	rows, err := d.DB.Query(r.Context(), `
		SELECT revision_number, title, body_hash, author_id, author_version,
		       commit_msg, completeness, created_at
		FROM artifact_revisions
		WHERE artifact_id = $1
		ORDER BY revision_number DESC
	`, artifactID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []revisionRow{}
	for rows.Next() {
		var r revisionRow
		var authorVer, commit *string
		if err := rows.Scan(&r.RevisionNumber, &r.Title, &r.BodyHash, &r.AuthorID,
			&authorVer, &commit, &r.Completeness, &r.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if authorVer != nil {
			r.AuthorVersion = *authorVer
		}
		if commit != nil {
			r.CommitMsg = *commit
		}
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"artifact_id": artifactID,
		"slug":        slug,
		"title":       title,
		"revisions":   out,
	})
}

type diffRevOut struct {
	RevisionNumber int       `json:"revision_number"`
	Title          string    `json:"title"`
	AuthorID       string    `json:"author_id"`
	AuthorVersion  string    `json:"author_version,omitempty"`
	CommitMsg      string    `json:"commit_msg,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func (d Deps) handleArtifactDiff(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	ref := r.PathValue("idOrSlug")
	if ref == "" {
		writeError(w, http.StatusBadRequest, "missing id or slug")
		return
	}
	fromRev, _ := strconv.Atoi(r.URL.Query().Get("from"))
	toRev, _ := strconv.Atoi(r.URL.Query().Get("to"))

	var artifactID, slug string
	var latest int
	err := d.DB.QueryRow(r.Context(), `
		SELECT a.id::text, a.slug,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
	`, projectSlug, ref).Scan(&artifactID, &slug, &latest)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if latest == 0 {
		writeError(w, http.StatusNotFound, "artifact has no revisions")
		return
	}
	if toRev == 0 {
		toRev = latest
	}
	if fromRev == 0 {
		fromRev = toRev - 1
		if fromRev < 1 {
			fromRev = 1
		}
	}

	from, errA := loadRevHTTP(r, d, artifactID, fromRev)
	to, errB := loadRevHTTP(r, d, artifactID, toRev)
	if errA != nil || errB != nil {
		writeError(w, http.StatusNotFound, "revision not found")
		return
	}

	stats, deltas := diff.Summary(from.body, to.body)
	unified := diff.Unified(slug, from.body, to.body)

	writeJSON(w, http.StatusOK, map[string]any{
		"artifact_id": artifactID,
		"slug":        slug,
		"from":        from.meta,
		"to":          to.meta,
		"stats":       stats,
		"section_deltas": deltas,
		"unified_diff":   unified,
	})
}

type loadedRevHTTP struct {
	meta diffRevOut
	body string
}

func loadRevHTTP(r *http.Request, d Deps, artifactID string, rev int) (loadedRevHTTP, error) {
	var out loadedRevHTTP
	var authorVer, commitMsg *string
	err := d.DB.QueryRow(r.Context(), `
		SELECT revision_number, title, body_markdown, author_id,
		       author_version, commit_msg, created_at
		FROM artifact_revisions
		WHERE artifact_id = $1 AND revision_number = $2
	`, artifactID, rev).Scan(
		&out.meta.RevisionNumber, &out.meta.Title, &out.body,
		&out.meta.AuthorID, &authorVer, &commitMsg, &out.meta.CreatedAt,
	)
	if err != nil {
		return out, err
	}
	if authorVer != nil {
		out.meta.AuthorVersion = *authorVer
	}
	if commitMsg != nil {
		out.meta.CommitMsg = *commitMsg
	}
	return out, nil
}
