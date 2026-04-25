package artifacts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// CachedTranslation is an ordinary artifact connected to a source artifact
// through a translation_of edge and tagged with the requested body_locale.
type CachedTranslation struct {
	ID           string
	Slug         string
	Title        string
	BodyMarkdown string
	BodyLocale   string
	UpdatedAt    time.Time
}

// FindCachedTranslation returns the freshest translation_of neighbor whose
// artifact body locale matches targetLocale. Edges may point either
// translation -> source (the normal artifact.propose relates_to shape) or
// source -> translation (accepted for old/manual graph entries).
func FindCachedTranslation(ctx context.Context, pool *db.Pool, sourceArtifactID, targetLocale string) (*CachedTranslation, error) {
	rows, err := pool.Query(ctx, `
		WITH candidates AS (
			SELECT t.id::text, t.slug, t.title, t.body_markdown, t.body_locale, t.updated_at
			  FROM artifact_edges e
			  JOIN artifacts t ON t.id = e.source_id
			 WHERE e.relation = 'translation_of'
			   AND e.target_id = $1::uuid
			   AND lower(t.body_locale) = lower($2)
			   AND t.status <> 'archived'
			UNION
			SELECT t.id::text, t.slug, t.title, t.body_markdown, t.body_locale, t.updated_at
			  FROM artifact_edges e
			  JOIN artifacts t ON t.id = e.target_id
			 WHERE e.relation = 'translation_of'
			   AND e.source_id = $1::uuid
			   AND lower(t.body_locale) = lower($2)
			   AND t.status <> 'archived'
		)
		SELECT id, slug, title, body_markdown, body_locale, updated_at
		  FROM candidates
		 ORDER BY updated_at DESC
		 LIMIT 1
	`, sourceArtifactID, targetLocale)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out CachedTranslation
	if rows.Next() {
		if err := rows.Scan(&out.ID, &out.Slug, &out.Title, &out.BodyMarkdown, &out.BodyLocale, &out.UpdatedAt); err != nil {
			return nil, err
		}
		return &out, rows.Err()
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, pgx.ErrNoRows
}

func IsNoCachedTranslation(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
