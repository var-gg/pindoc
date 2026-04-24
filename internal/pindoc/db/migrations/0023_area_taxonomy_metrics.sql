-- +goose Up
-- Phase 5 area taxonomy operating metrics.
--
-- This function is intentionally SQL-only so operators can run the same
-- query from psql, pindoc-admin, or a future Reader admin surface without
-- inventing another metrics definition.

CREATE OR REPLACE FUNCTION area_taxonomy_metrics(
    p_project_slug TEXT,
    p_window_days  INTEGER DEFAULT 30
)
RETURNS TABLE (
    project_slug                 TEXT,
    computed_at                  TIMESTAMPTZ,
    window_days                  INTEGER,
    artifact_total               INTEGER,
    misc_count                   INTEGER,
    cross_cutting_count          INTEGER,
    misc_ratio                   NUMERIC,
    cross_cutting_ratio          NUMERIC,
    rehomed_artifacts_30d        INTEGER,
    rehome_rate_30d              NUMERIC,
    suggestion_events            INTEGER,
    suggestion_resolved_events   INTEGER,
    suggestion_hits              INTEGER,
    agent_suggestion_accuracy    NUMERIC
)
LANGUAGE sql
STABLE
AS $$
WITH project_row AS (
    SELECT id, slug
    FROM projects
    WHERE slug = p_project_slug
),
visible_artifacts AS (
    SELECT a.id, ar.slug AS area_slug, ar.is_cross_cutting
    FROM artifacts a
    JOIN areas ar ON ar.id = a.area_id
    JOIN project_row p ON p.id = a.project_id
    WHERE a.status <> 'archived'
      AND a.slug NOT LIKE '\_template\_%' ESCAPE '\'
),
artifact_counts AS (
    SELECT
        COUNT(*)::INTEGER AS total,
        COUNT(*) FILTER (WHERE area_slug = 'misc')::INTEGER AS misc,
        COUNT(*) FILTER (WHERE is_cross_cutting)::INTEGER AS cross_cutting
    FROM visible_artifacts
),
rehome_counts AS (
    SELECT COUNT(DISTINCT e.subject_id)::INTEGER AS rehomed
    FROM events e
    JOIN project_row p ON p.id = e.project_id
    WHERE e.kind = 'artifact.area_relabelled'
      AND e.created_at >= now() - (GREATEST(p_window_days, 0) * INTERVAL '1 day')
),
suggested AS (
    SELECT
        e.payload->>'correlation_id' AS correlation_id,
        e.payload->'suggested_areas' AS suggested_areas
    FROM events e
    JOIN project_row p ON p.id = e.project_id
    WHERE e.kind = 'agent.area_suggestion_proposed'
      AND e.created_at >= now() - (GREATEST(p_window_days, 0) * INTERVAL '1 day')
      AND COALESCE(e.payload->>'correlation_id', '') <> ''
),
resolved AS (
    SELECT
        e.payload->>'correlation_id' AS correlation_id,
        e.payload->>'final_area_slug' AS final_area_slug
    FROM events e
    JOIN project_row p ON p.id = e.project_id
    WHERE e.kind = 'agent.area_suggestion_resolved'
      AND e.created_at >= now() - (GREATEST(p_window_days, 0) * INTERVAL '1 day')
      AND COALESCE(e.payload->>'correlation_id', '') <> ''
      AND COALESCE(e.payload->>'final_area_slug', '') <> ''
),
suggestion_join AS (
    SELECT
        r.correlation_id,
        r.final_area_slug,
        s.suggested_areas,
        EXISTS (
            SELECT 1
            FROM jsonb_array_elements(COALESCE(s.suggested_areas, '[]'::jsonb)) AS elem
            WHERE elem->>'area_slug' = r.final_area_slug
        ) AS hit
    FROM resolved r
    JOIN suggested s ON s.correlation_id = r.correlation_id
),
suggestion_counts AS (
    SELECT
        (SELECT COUNT(*)::INTEGER FROM suggested) AS proposed,
        COUNT(*)::INTEGER AS resolved,
        COUNT(*) FILTER (WHERE hit)::INTEGER AS hits
    FROM suggestion_join
)
SELECT
    p.slug AS project_slug,
    now() AS computed_at,
    GREATEST(p_window_days, 0) AS window_days,
    c.total AS artifact_total,
    c.misc AS misc_count,
    c.cross_cutting AS cross_cutting_count,
    COALESCE(c.misc::NUMERIC / NULLIF(c.total, 0), 0) AS misc_ratio,
    COALESCE(c.cross_cutting::NUMERIC / NULLIF(c.total, 0), 0) AS cross_cutting_ratio,
    r.rehomed AS rehomed_artifacts_30d,
    COALESCE(r.rehomed::NUMERIC / NULLIF(c.total, 0), 0) AS rehome_rate_30d,
    s.proposed AS suggestion_events,
    s.resolved AS suggestion_resolved_events,
    s.hits AS suggestion_hits,
    s.hits::NUMERIC / NULLIF(s.resolved, 0) AS agent_suggestion_accuracy
FROM project_row p
CROSS JOIN artifact_counts c
CROSS JOIN rehome_counts r
CROSS JOIN suggestion_counts s;
$$;

-- +goose Down
DROP FUNCTION IF EXISTS area_taxonomy_metrics(TEXT, INTEGER);
