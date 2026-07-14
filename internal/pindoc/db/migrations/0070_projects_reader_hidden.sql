-- +goose Up
-- Reader fixture isolation used to be inferred from slug prefixes in HTTP
-- handlers. Persist the intent on projects so downstream tests and plugins can
-- create fixtures without patching Pindoc source code.

ALTER TABLE projects
    ADD COLUMN reader_hidden BOOLEAN NOT NULL DEFAULT FALSE;

-- One-time compatibility backfill for fixture families created by released
-- integration tests. Runtime code does not retain this slug heuristic; all new
-- fixture creation paths must set reader_hidden explicitly.
UPDATE projects
SET reader_hidden = TRUE
WHERE lower(slug) LIKE ANY (ARRAY[
    'oauth-it-%',
    'invite-http-%',
    'workspace-detect-%',
    'vis-http-%',
    'vis-mcp-%',
    'artifact-audit-%',
    'task-flow-a-%',
    'task-flow-b-%',
    'task-queue-across-a-%',
    'task-queue-across-b-%',
    'asset-local-path-%',
    'cmdk-current-%',
    'cmdk-sister-%',
    'register-hint-%',
    'set-repo-%'
])
   OR lower(slug) ~ '^pindoc-[0-9a-f]{16}$';

CREATE INDEX idx_projects_reader_visible
    ON projects(created_at)
    WHERE reader_hidden = FALSE;

-- +goose Down
DROP INDEX IF EXISTS idx_projects_reader_visible;
ALTER TABLE projects DROP COLUMN IF EXISTS reader_hidden;
