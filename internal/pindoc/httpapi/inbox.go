package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type inboxResp struct {
	ProjectSlug string        `json:"project_slug"`
	Count       int           `json:"count"`
	Items       []artifactRow `json:"items"`
}

type inboxReviewRequest struct {
	Decision   string `json:"decision"`
	ReviewerID string `json:"reviewer_id,omitempty"`
	CommitMsg  string `json:"commit_msg,omitempty"`
}

type inboxReviewResp struct {
	Status      string `json:"status"`
	ArtifactID  string `json:"artifact_id"`
	Slug        string `json:"slug"`
	ReviewState string `json:"review_state"`
	RowStatus   string `json:"row_status"`
}

func (d Deps) handleInbox(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	rows, err := d.DB.Query(r.Context(), `
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(NULLIF(a.body_locale, ''), NULLIF(p.primary_language, ''), 'en'),
			a.completeness, a.status, a.review_state,
			a.author_id, a.published_at, a.updated_at, a.task_meta, a.artifact_meta,
			'[]'::jsonb AS recent_warnings,
			u.id::text, u.display_name, u.github_handle
		FROM artifacts a
		JOIN projects p  ON p.id  = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		LEFT JOIN users u ON u.id = a.author_user_id
		WHERE p.slug = $1
		  AND a.review_state = 'pending_review'
		  AND a.status <> 'archived'
		ORDER BY a.updated_at DESC
		LIMIT 200
	`, slug)
	if err != nil {
		d.Logger.Error("inbox query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	items := []artifactRow{}
	for rows.Next() {
		var a artifactRow
		var publishedAt *time.Time
		var taskMeta, artifactMeta, recentWarnings []byte
		var userID, userDisplay, userGithub *string
		if err := rows.Scan(
			&a.ID, &a.Slug, &a.Type, &a.Title, &a.AreaSlug,
			&a.BodyLocale,
			&a.Completeness, &a.Status, &a.ReviewState,
			&a.AuthorID, &publishedAt, &a.UpdatedAt, &taskMeta, &artifactMeta,
			&recentWarnings,
			&userID, &userDisplay, &userGithub,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if publishedAt != nil {
			a.PublishedAt = *publishedAt
		}
		if len(taskMeta) > 0 {
			a.TaskMeta = json.RawMessage(taskMeta)
		}
		if len(artifactMeta) > 0 {
			a.ArtifactMeta = json.RawMessage(artifactMeta)
		}
		if userID != nil && userDisplay != nil {
			ref := &authorUserRef{ID: *userID, DisplayName: *userDisplay}
			if userGithub != nil {
				ref.GithubHandle = *userGithub
			}
			a.AuthorUser = ref
		}
		items = append(items, a)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	writeJSON(w, http.StatusOK, inboxResp{
		ProjectSlug: slug,
		Count:       len(items),
		Items:       items,
	})
}

func (d Deps) handleInboxReview(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	ref := strings.TrimSpace(r.PathValue("idOrSlug"))
	if ref == "" {
		writeError(w, http.StatusBadRequest, "missing id or slug")
		return
	}
	var in inboxReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}

	decision := strings.ToLower(strings.TrimSpace(in.Decision))
	nextReviewState := ""
	nextStatus := ""
	eventKind := ""
	switch decision {
	case "approve", "approved":
		nextReviewState = "approved"
		nextStatus = "published"
		eventKind = "review.approved"
	case "reject", "rejected":
		nextReviewState = "rejected"
		nextStatus = "archived"
		eventKind = "review.rejected"
	default:
		writeError(w, http.StatusBadRequest, "decision must be approve or reject")
		return
	}

	tx, err := d.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "begin failed")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var out inboxReviewResp
	err = tx.QueryRow(r.Context(), `
		WITH target AS (
			SELECT a.id
			  FROM artifacts a
			  JOIN projects p ON p.id = a.project_id
			 WHERE p.slug = $1
			   AND (a.id::text = $2 OR a.slug = $2)
			   AND a.review_state = 'pending_review'
			 LIMIT 1
		)
		UPDATE artifacts a
		   SET review_state = $3,
		       status       = $4,
		       updated_at   = now()
		  FROM target
		 WHERE a.id = target.id
		RETURNING a.id::text, a.slug, a.review_state, a.status
	`, slug, ref, nextReviewState, nextStatus).Scan(
		&out.ArtifactID, &out.Slug, &out.ReviewState, &out.RowStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pending review not found")
		return
	}
	if err != nil {
		d.Logger.Error("inbox review update", "err", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	reviewer := strings.TrimSpace(in.ReviewerID)
	if reviewer == "" {
		reviewer = "reader"
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO events (project_id, kind, subject_id, payload)
		SELECT p.id, $3, $2::uuid, jsonb_build_object(
			'reviewer_id', $4::text,
			'commit_msg',  $5::text
		)
		  FROM projects p
		 WHERE p.slug = $1
	`, slug, out.ArtifactID, eventKind, reviewer, strings.TrimSpace(in.CommitMsg)); err != nil {
		d.Logger.Error("inbox review event", "err", err)
		writeError(w, http.StatusInternalServerError, "event failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	out.Status = "accepted"
	writeJSON(w, http.StatusOK, out)
}
