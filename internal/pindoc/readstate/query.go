package readstate

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// ForArtifact returns the raw aggregate for a single (artifact, user_key)
// pair, or nil if the user has no read_events on that artifact yet.
func ForArtifact(ctx context.Context, pool *db.Pool, artifactID, userKey string) (*Aggregate, error) {
	var agg Aggregate
	err := pool.QueryRow(ctx, `
		SELECT artifact_id::text, user_key, first_seen_at, last_seen_at,
		       total_active_seconds, total_idle_seconds, max_scroll_pct, event_count
		FROM artifact_read_states
		WHERE artifact_id = $1 AND user_key = $2
	`, artifactID, userKey).Scan(
		&agg.ArtifactID, &agg.UserKey,
		&agg.FirstSeenAt, &agg.LastSeenAt,
		&agg.TotalActiveSeconds, &agg.TotalIdleSeconds,
		&agg.MaxScrollPct, &agg.EventCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agg, nil
}

// BatchByArtifacts returns aggregates for the given artifact IDs in a
// single query. Missing artifacts (no read_events for that user) are
// simply absent from the map — callers should default to StateUnseen.
func BatchByArtifacts(ctx context.Context, pool *db.Pool, artifactIDs []string, userKey string) (map[string]*Aggregate, error) {
	out := map[string]*Aggregate{}
	if len(artifactIDs) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT artifact_id::text, user_key, first_seen_at, last_seen_at,
		       total_active_seconds, total_idle_seconds, max_scroll_pct, event_count
		FROM artifact_read_states
		WHERE user_key = $1 AND artifact_id = ANY($2::uuid[])
	`, userKey, artifactIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var agg Aggregate
		if err := rows.Scan(
			&agg.ArtifactID, &agg.UserKey,
			&agg.FirstSeenAt, &agg.LastSeenAt,
			&agg.TotalActiveSeconds, &agg.TotalIdleSeconds,
			&agg.MaxScrollPct, &agg.EventCount,
		); err != nil {
			return nil, err
		}
		copy := agg
		out[agg.ArtifactID] = &copy
	}
	return out, rows.Err()
}

// ArtifactState returns the read state for a single artifact, identified
// by id or slug within the project. Returns pgx.ErrNoRows if the artifact
// itself is missing — that's distinct from the "exists but unread" case,
// which yields a State{ReadState: StateUnseen} with no error.
func ArtifactState(ctx context.Context, pool *db.Pool, projectSlug, artifactRef, userKey string) (State, error) {
	var (
		artifactID string
		locale     string
		bodyChars  int
		firstSeen  *time.Time
		lastSeen   *time.Time
		activeSec  float64
		idleSec    float64
		scrollMax  float64
		eventCount int
	)
	err := pool.QueryRow(ctx, `
		SELECT
			a.id::text,
			a.body_locale,
			char_length(COALESCE(a.body_markdown, '')),
			ars.first_seen_at,
			ars.last_seen_at,
			COALESCE(ars.total_active_seconds, 0),
			COALESCE(ars.total_idle_seconds, 0),
			COALESCE(ars.max_scroll_pct, 0),
			COALESCE(ars.event_count, 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		LEFT JOIN artifact_read_states ars
		       ON ars.artifact_id = a.id AND ars.user_key = $3
		WHERE p.slug = $1
		  AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, projectSlug, artifactRef, userKey).Scan(
		&artifactID, &locale, &bodyChars,
		&firstSeen, &lastSeen,
		&activeSec, &idleSec, &scrollMax, &eventCount,
	)
	if err != nil {
		return State{}, err
	}
	var agg *Aggregate
	if eventCount > 0 {
		a := Aggregate{
			ArtifactID:         artifactID,
			UserKey:            userKey,
			TotalActiveSeconds: activeSec,
			TotalIdleSeconds:   idleSec,
			MaxScrollPct:       scrollMax,
			EventCount:         eventCount,
		}
		if firstSeen != nil {
			a.FirstSeenAt = *firstSeen
		}
		if lastSeen != nil {
			a.LastSeenAt = *lastSeen
		}
		agg = &a
	}
	state := ClassifyAggregateFromChars(agg, bodyChars, locale)
	state.ArtifactID = artifactID
	state.UserKey = userKey
	return state, nil
}

// ProjectStates returns the classified read state for every non-archived
// artifact in the project, joining the raw aggregate against
// char_length(body_markdown) + body_locale so ExpectedSeconds can be
// computed without shipping every body. Artifacts the user has never
// touched come back as StateUnseen with EventCount 0.
func ProjectStates(ctx context.Context, pool *db.Pool, projectSlug, userKey string) ([]State, error) {
	rows, err := pool.Query(ctx, `
		SELECT
			a.id::text                                  AS artifact_id,
			a.body_locale,
			char_length(COALESCE(a.body_markdown, ''))  AS body_chars,
			ars.first_seen_at,
			ars.last_seen_at,
			COALESCE(ars.total_active_seconds, 0)       AS total_active_seconds,
			COALESCE(ars.total_idle_seconds, 0)         AS total_idle_seconds,
			COALESCE(ars.max_scroll_pct, 0)             AS max_scroll_pct,
			COALESCE(ars.event_count, 0)                AS event_count
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		LEFT JOIN artifact_read_states ars
		       ON ars.artifact_id = a.id AND ars.user_key = $2
		WHERE p.slug = $1
		  AND a.status <> 'archived'
		ORDER BY a.id
	`, projectSlug, userKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []State{}
	for rows.Next() {
		var (
			artifactID string
			locale     string
			bodyChars  int
			firstSeen  *time.Time
			lastSeen   *time.Time
			activeSec  float64
			idleSec    float64
			scrollMax  float64
			eventCount int
		)
		if err := rows.Scan(
			&artifactID, &locale, &bodyChars,
			&firstSeen, &lastSeen,
			&activeSec, &idleSec, &scrollMax, &eventCount,
		); err != nil {
			return nil, err
		}
		var agg *Aggregate
		if eventCount > 0 {
			a := Aggregate{
				ArtifactID:         artifactID,
				UserKey:            userKey,
				TotalActiveSeconds: activeSec,
				TotalIdleSeconds:   idleSec,
				MaxScrollPct:       scrollMax,
				EventCount:         eventCount,
			}
			if firstSeen != nil {
				a.FirstSeenAt = *firstSeen
			}
			if lastSeen != nil {
				a.LastSeenAt = *lastSeen
			}
			agg = &a
		}
		state := ClassifyAggregateFromChars(agg, bodyChars, locale)
		state.ArtifactID = artifactID
		state.UserKey = userKey
		out = append(out, state)
	}
	return out, rows.Err()
}

// ProjectAggregates returns every (artifact, user_key) aggregate for a
// project — used by Sidebar/Today to color or order artifacts by the
// reader's progress without N+1 queries.
func ProjectAggregates(ctx context.Context, pool *db.Pool, projectSlug, userKey string) (map[string]*Aggregate, error) {
	out := map[string]*Aggregate{}
	rows, err := pool.Query(ctx, `
		SELECT ars.artifact_id::text, ars.user_key,
		       ars.first_seen_at, ars.last_seen_at,
		       ars.total_active_seconds, ars.total_idle_seconds,
		       ars.max_scroll_pct, ars.event_count
		FROM artifact_read_states ars
		JOIN artifacts a ON a.id = ars.artifact_id
		JOIN projects  p ON p.id = a.project_id
		WHERE p.slug = $1
		  AND ars.user_key = $2
		  AND a.status <> 'archived'
	`, projectSlug, userKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var agg Aggregate
		if err := rows.Scan(
			&agg.ArtifactID, &agg.UserKey,
			&agg.FirstSeenAt, &agg.LastSeenAt,
			&agg.TotalActiveSeconds, &agg.TotalIdleSeconds,
			&agg.MaxScrollPct, &agg.EventCount,
		); err != nil {
			return nil, err
		}
		copy := agg
		out[agg.ArtifactID] = &copy
	}
	return out, rows.Err()
}
