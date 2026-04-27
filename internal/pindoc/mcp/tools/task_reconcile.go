package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const reconcileSweepLimit = 100

type ReconcileCandidate struct {
	Slug          string `json:"slug"`
	Status        string `json:"status"`
	AcceptancePct int    `json:"acceptance_pct"`
}

type ReconcileSummary struct {
	LastRunAt         time.Time `json:"last_run_at"`
	TransitionedCount int       `json:"transitioned_count"`
	ResidualCount     int       `json:"residual_count"`
}

type reconcileTaskRow struct {
	ID           string
	Slug         string
	Title        string
	Body         string
	Tags         []string
	Completeness string
	HeadRev      int
}

func reconcileCompletedOpenTasks(ctx context.Context, deps Deps, projectSlug string) (ReconcileSummary, []ReconcileCandidate, error) {
	now := time.Now().UTC()
	candidatesBefore, err := listReconcileCandidates(ctx, deps, projectSlug, 10)
	if err != nil {
		return ReconcileSummary{}, nil, err
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, a.body_markdown, a.tags, a.completeness,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1
		  AND a.type = 'Task'
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
		  AND COALESCE(a.task_meta->>'status', '') = 'open'
		ORDER BY a.updated_at ASC
		LIMIT $2
	`, projectSlug, reconcileSweepLimit)
	if err != nil {
		return ReconcileSummary{}, nil, err
	}
	defer rows.Close()

	var complete []reconcileTaskRow
	for rows.Next() {
		var r reconcileTaskRow
		if err := rows.Scan(&r.ID, &r.Slug, &r.Title, &r.Body, &r.Tags, &r.Completeness, &r.HeadRev); err != nil {
			return ReconcileSummary{}, nil, err
		}
		resolved, total := countAcceptanceResolution(r.Body)
		if total > 0 && resolved == total {
			complete = append(complete, r)
		}
	}
	if err := rows.Err(); err != nil {
		return ReconcileSummary{}, nil, err
	}

	transitioned := 0
	if len(complete) > 0 {
		tx, err := deps.DB.Begin(ctx)
		if err != nil {
			return ReconcileSummary{}, nil, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		for _, r := range complete {
			newRev := r.HeadRev + 1
			tag, err := tx.Exec(ctx, `
				UPDATE artifacts
				   SET task_meta = jsonb_set(COALESCE(task_meta, '{}'::jsonb), '{status}', '"claimed_done"'),
				       author_id = 'pindoc-reconcile-sweeper',
				       updated_at = now()
				 WHERE id = $1::uuid
				   AND COALESCE(task_meta->>'status', '') = 'open'
			`, r.ID)
			if err != nil {
				return ReconcileSummary{}, nil, fmt.Errorf("update reconcile task %s: %w", r.Slug, err)
			}
			if tag.RowsAffected() == 0 {
				continue
			}
			payload, _ := json.Marshal(map[string]any{
				"task_meta": map[string]string{"status": "claimed_done"},
				"source":    "reconcile_sweeper",
			})
			if _, err := tx.Exec(ctx, `
				INSERT INTO artifact_revisions (
					artifact_id, revision_number, title, body_markdown, body_hash, tags,
					completeness, author_kind, author_id, author_version, commit_msg,
					source_session_ref, revision_shape, shape_payload
				) VALUES ($1, $2, $3, $4, $5, $6, $7, 'agent', 'pindoc-reconcile-sweeper', NULL,
				          'auto: acceptance 100% -> claimed_done', $8::jsonb, 'meta_patch', $9::jsonb)
			`, r.ID, newRev, r.Title, r.Body, bodyHash(r.Body), r.Tags, r.Completeness,
				`{"tool":"pindoc.reconcile.sweeper"}`, string(payload)); err != nil {
				return ReconcileSummary{}, nil, fmt.Errorf("insert reconcile revision for %s: %w", r.Slug, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				SELECT project_id, 'task.reconciled_claimed_done', id, jsonb_build_object(
					'revision_number', $2::int,
					'slug', $3::text
				)
				FROM artifacts WHERE id = $1::uuid
			`, r.ID, newRev, r.Slug); err != nil {
				return ReconcileSummary{}, nil, fmt.Errorf("insert reconcile event for %s: %w", r.Slug, err)
			}
			transitioned++
		}
		if err := tx.Commit(ctx); err != nil {
			return ReconcileSummary{}, nil, err
		}
	}

	residual, err := listReconcileCandidates(ctx, deps, projectSlug, 10)
	if err != nil {
		return ReconcileSummary{}, nil, err
	}
	return ReconcileSummary{
		LastRunAt:         now,
		TransitionedCount: transitioned,
		ResidualCount:     len(residual),
	}, candidatesBefore, nil
}

func listReconcileCandidates(ctx context.Context, deps Deps, projectSlug string, limit int) ([]ReconcileCandidate, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT a.slug, a.body_markdown, COALESCE(a.task_meta->>'status', '') AS task_status
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1
		  AND a.type = 'Task'
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
		  AND COALESCE(a.task_meta->>'status', '') = 'open'
		ORDER BY a.updated_at DESC
		LIMIT $2
	`, projectSlug, reconcileSweepLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ReconcileCandidate{}
	for rows.Next() {
		var slug, body, status string
		if err := rows.Scan(&slug, &body, &status); err != nil {
			return nil, err
		}
		resolved, total := countAcceptanceResolution(body)
		if total == 0 || resolved != total {
			continue
		}
		out = append(out, ReconcileCandidate{
			Slug:          slug,
			Status:        status,
			AcceptancePct: 100,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}
