-- +goose Up
-- Preserve externally shared artifact URLs when a maintenance pass rewrites
-- slugs. Each alias is project-scoped and points at the canonical artifact
-- row; Reader share routes redirect /p/{project}/wiki/{old_slug} to the
-- artifact's current slug.

CREATE TABLE artifact_slug_aliases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    old_slug    TEXT NOT NULL,
    created_by  TEXT NOT NULL DEFAULT 'system',
    reason      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, old_slug)
);

CREATE INDEX idx_artifact_slug_aliases_artifact
    ON artifact_slug_aliases(artifact_id);

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_slug_aliases_artifact;
DROP TABLE IF EXISTS artifact_slug_aliases;
