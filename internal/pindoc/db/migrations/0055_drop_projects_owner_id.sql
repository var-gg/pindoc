-- +goose Up
-- projects.organization_id is now the authoritative ownership boundary.
-- Remove the legacy owner_id text column and its transitional indexes /
-- constraints. The canonical slug uniqueness from migration 0029 stays in
-- place; org-scoped lookup is backed by idx_projects_org_slug.
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_locale_unique;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_key;

DROP INDEX IF EXISTS idx_projects_owner;

ALTER TABLE projects
    DROP COLUMN IF EXISTS owner_id;

-- +goose Down
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_projects_owner ON projects(owner_id);

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_key;
ALTER TABLE projects
    ADD CONSTRAINT projects_owner_slug_key UNIQUE (owner_id, slug);
