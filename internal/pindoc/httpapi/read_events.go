package httpapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type readEventRequest struct {
	ArtifactID    string    `json:"artifact_id"`
	ArtifactSlug  string    `json:"artifact_slug"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	ActiveSeconds float64   `json:"active_seconds"`
	ScrollMaxPct  float64   `json:"scroll_max_pct"`
	IdleSeconds   float64   `json:"idle_seconds"`
	Locale        string    `json:"locale"`
}

type readEventResponse struct {
	ID            string  `json:"id"`
	ArtifactID    string  `json:"artifact_id"`
	ActiveSeconds float64 `json:"active_seconds"`
	ScrollMaxPct  float64 `json:"scroll_max_pct"`
}

func (d Deps) handleReadEvent(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	var input readEventRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := validateReadEvent(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	artifactID, err := d.resolveReadEventArtifact(r, projectSlug, input)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		d.Logger.Error("read event artifact lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	var eventID string
	err = d.DB.QueryRow(r.Context(), `
		INSERT INTO read_events (
			artifact_id, user_id, user_key, started_at, ended_at,
			active_seconds, scroll_max_pct, idle_seconds, locale
		)
		VALUES ($1, NULL, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))
		RETURNING id::text
	`, artifactID, readerUserKey(r), input.StartedAt, input.EndedAt,
		input.ActiveSeconds, input.ScrollMaxPct, input.IdleSeconds,
		strings.TrimSpace(input.Locale),
	).Scan(&eventID)
	if err != nil {
		d.Logger.Error("read event insert", "err", err)
		writeError(w, http.StatusInternalServerError, "write failed")
		return
	}

	// Note: read_events does NOT update reader_watermarks. Reading one
	// artifact ≠ reviewing today's Change Group stream. Watermark
	// updates are driven by Today viewport observer + explicit
	// "Mark all read" only — see docs/06-ui-flows.md Flow 1a.

	writeJSON(w, http.StatusOK, readEventResponse{
		ID:            eventID,
		ArtifactID:    artifactID,
		ActiveSeconds: input.ActiveSeconds,
		ScrollMaxPct:  input.ScrollMaxPct,
	})
}

func (d Deps) resolveReadEventArtifact(r *http.Request, projectSlug string, input readEventRequest) (string, error) {
	var artifactID string
	err := d.DB.QueryRow(r.Context(), `
		SELECT a.id::text
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1
		  AND (
			(NULLIF($2, '') IS NOT NULL AND a.id::text = $2)
			OR (NULLIF($3, '') IS NOT NULL AND a.slug = $3)
		  )
		LIMIT 1
	`, projectSlug, strings.TrimSpace(input.ArtifactID), strings.TrimSpace(input.ArtifactSlug)).Scan(&artifactID)
	return artifactID, err
}

func validateReadEvent(input readEventRequest) error {
	if strings.TrimSpace(input.ArtifactID) == "" && strings.TrimSpace(input.ArtifactSlug) == "" {
		return errors.New("artifact_id or artifact_slug is required")
	}
	if input.StartedAt.IsZero() || input.EndedAt.IsZero() {
		return errors.New("started_at and ended_at are required")
	}
	if input.EndedAt.Before(input.StartedAt) {
		return errors.New("ended_at must be after started_at")
	}
	duration := input.EndedAt.Sub(input.StartedAt).Seconds()
	if duration > 24*60*60 {
		return errors.New("read event duration is too long")
	}
	if invalidFloat(input.ActiveSeconds) || input.ActiveSeconds < 0 {
		return errors.New("active_seconds must be non-negative")
	}
	if invalidFloat(input.IdleSeconds) || input.IdleSeconds < 0 {
		return errors.New("idle_seconds must be non-negative")
	}
	if input.ActiveSeconds > duration {
		return errors.New("active_seconds must not exceed session duration")
	}
	if invalidFloat(input.ScrollMaxPct) || input.ScrollMaxPct < 0 || input.ScrollMaxPct > 1 {
		return errors.New("scroll_max_pct must be between 0 and 1")
	}
	if len(strings.TrimSpace(input.Locale)) > 16 {
		return errors.New("locale is too long")
	}
	return nil
}

func invalidFloat(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0)
}
