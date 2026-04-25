package httpapi

import (
	"encoding/json"
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
	RevisionShape  string    `json:"revision_shape,omitempty"`
	RevisionType   string    `json:"revision_type,omitempty"`
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
		       commit_msg, completeness, tags, revision_shape, shape_payload, created_at
		FROM artifact_revisions
		WHERE artifact_id = $1
		ORDER BY revision_number ASC
	`, artifactID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []revisionRow{}
	var prevSnapshot diff.RevisionMetaSnapshot
	var prevBodyHash string
	for rows.Next() {
		var r revisionRow
		var authorVer, commit *string
		var tags []string
		var shapePayload []byte
		if err := rows.Scan(&r.RevisionNumber, &r.Title, &r.BodyHash, &r.AuthorID,
			&authorVer, &commit, &r.Completeness, &tags, &r.RevisionShape, &shapePayload,
			&r.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if authorVer != nil {
			r.AuthorVersion = *authorVer
		}
		if commit != nil {
			r.CommitMsg = *commit
		}
		snapshot := diff.RevisionMetaSnapshot{
			RevisionNumber: r.RevisionNumber,
			Tags:           tags,
			Completeness:   r.Completeness,
			Shape:          r.RevisionShape,
			ShapePayload:   json.RawMessage(shapePayload),
		}
		bodyChanged := prevSnapshot.RevisionNumber == 0 || r.BodyHash != prevBodyHash
		metaChanged := diff.MetaChangedBetween(prevSnapshot, snapshot)
		r.RevisionType = diff.ClassifyRevisionType(r.RevisionShape, r.CommitMsg, bodyChanged, metaChanged)
		out = append(out, r)
		prevSnapshot = snapshot
		prevBodyHash = r.BodyHash
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
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
	BodyHash       string    `json:"body_hash,omitempty"`
	AuthorID       string    `json:"author_id"`
	AuthorVersion  string    `json:"author_version,omitempty"`
	CommitMsg      string    `json:"commit_msg,omitempty"`
	RevisionShape  string    `json:"revision_shape,omitempty"`
	RevisionType   string    `json:"revision_type,omitempty"`
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
	snapshots, err := loadMetaSnapshotsHTTP(r, d, artifactID, toRev)
	if err != nil {
		d.Logger.Error("diff meta snapshots", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	metaDelta := diff.MetaDeltaForRange(fromRev, toRev, snapshots)
	revisionType := diff.ClassifyRevisionType(
		to.snapshot.Shape,
		to.meta.CommitMsg,
		from.meta.BodyHash != to.meta.BodyHash,
		len(metaDelta) > 0,
	)
	to.meta.RevisionType = revisionType
	acceptanceChecklist := diff.AcceptanceChecklistSummary(from.body, to.body, to.snapshot.Shape, to.snapshot.ShapePayload)

	writeJSON(w, http.StatusOK, map[string]any{
		"artifact_id":          artifactID,
		"slug":                 slug,
		"from":                 from.meta,
		"to":                   to.meta,
		"stats":                stats,
		"meta_delta":           metaDelta,
		"acceptance_checklist": acceptanceChecklist,
		"revision_type":        revisionType,
		"section_deltas":       deltas,
		"unified_diff":         unified,
	})
}

type loadedRevHTTP struct {
	meta     diffRevOut
	body     string
	snapshot diff.RevisionMetaSnapshot
}

func loadRevHTTP(r *http.Request, d Deps, artifactID string, rev int) (loadedRevHTTP, error) {
	var out loadedRevHTTP
	var authorVer, commitMsg *string
	var tags []string
	var shapePayload []byte
	err := d.DB.QueryRow(r.Context(), `
		SELECT r.revision_number, r.title,
		       COALESCE(
		           r.body_markdown,
		           (
		               SELECT prev.body_markdown
		               FROM artifact_revisions prev
		               WHERE prev.artifact_id = r.artifact_id
		                 AND prev.revision_number < r.revision_number
		                 AND prev.body_markdown IS NOT NULL
		               ORDER BY prev.revision_number DESC
		               LIMIT 1
		           ),
		           ''
		       ) AS body_markdown,
		       r.body_hash, r.author_id, r.author_version, r.commit_msg,
		       r.completeness, r.tags, r.revision_shape, r.shape_payload, r.created_at
		FROM artifact_revisions r
		WHERE r.artifact_id = $1 AND r.revision_number = $2
	`, artifactID, rev).Scan(
		&out.meta.RevisionNumber, &out.meta.Title, &out.body, &out.meta.BodyHash,
		&out.meta.AuthorID, &authorVer, &commitMsg, &out.snapshot.Completeness,
		&tags, &out.meta.RevisionShape, &shapePayload, &out.meta.CreatedAt,
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
	out.snapshot = diff.RevisionMetaSnapshot{
		RevisionNumber: out.meta.RevisionNumber,
		Tags:           tags,
		Completeness:   out.snapshot.Completeness,
		Shape:          out.meta.RevisionShape,
		ShapePayload:   json.RawMessage(shapePayload),
	}
	return out, nil
}

func loadMetaSnapshotsHTTP(r *http.Request, d Deps, artifactID string, toRev int) ([]diff.RevisionMetaSnapshot, error) {
	rows, err := d.DB.Query(r.Context(), `
		SELECT revision_number, tags, completeness, revision_shape, shape_payload
		FROM artifact_revisions
		WHERE artifact_id = $1 AND revision_number <= $2
		ORDER BY revision_number ASC
	`, artifactID, toRev)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []diff.RevisionMetaSnapshot
	for rows.Next() {
		var snap diff.RevisionMetaSnapshot
		var payload []byte
		if err := rows.Scan(&snap.RevisionNumber, &snap.Tags, &snap.Completeness, &snap.Shape, &payload); err != nil {
			return nil, err
		}
		snap.ShapePayload = json.RawMessage(payload)
		out = append(out, snap)
	}
	return out, rows.Err()
}
