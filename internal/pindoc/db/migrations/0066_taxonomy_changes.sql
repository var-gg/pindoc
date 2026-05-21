-- +goose Up
-- Decision taxonomy-change-operation (Phase 2): taxonomy_changes is the
-- operation record for every owner-approved taxonomy change-set. The four
-- MCP tools — taxonomy.change.propose / diff / approve / apply — all read
-- and write this row. The events table only carries an audit copy: apply
-- re-derives the dry-run and compares plan_hash against this row, so the
-- row (not the event) is the source of truth for what gets applied.
--
-- kind:   top_level.add | profile.adopt | area.retire_empty
-- status: proposed -> approved -> applied;  rejected / stale are terminal.

CREATE TABLE taxonomy_changes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL
        CHECK (kind IN ('top_level.add', 'profile.adopt', 'area.retire_empty')),
    status              TEXT NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed', 'approved', 'applied', 'rejected', 'stale')),
    source_profile_slug TEXT,
    target_profile_slug TEXT,
    plan_json           JSONB NOT NULL,
    diff_json           JSONB NOT NULL,
    plan_hash           TEXT NOT NULL,
    created_by          TEXT NOT NULL,
    approved_by         TEXT,
    approved_at         TIMESTAMPTZ,
    applied_by          TEXT,
    applied_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_taxonomy_changes_project
    ON taxonomy_changes (project_id, status, created_at DESC);
CREATE INDEX idx_taxonomy_changes_plan_hash
    ON taxonomy_changes (plan_hash);

-- +goose Down
DROP INDEX IF EXISTS idx_taxonomy_changes_plan_hash;
DROP INDEX IF EXISTS idx_taxonomy_changes_project;
DROP TABLE IF EXISTS taxonomy_changes;
