-- +goose Up
-- Phase 16: add owner_id to projects for future multi-owner deployments.
--
-- A single Pindoc instance today hosts one or more projects; every project
-- is globally unique by slug. Real-world self-host is almost always one
-- human or one small team sharing a box, so the current model is fine
-- for V1.
--
-- But backfilling an ownership column onto an existing projects table
-- full of artifacts, chunks, edges, pins, revisions, and events is a huge
-- migration. Adding it now (populated with a harmless constant) means
-- downstream schemas that want to cascade ownership — future user
-- accounts, shared workspaces, permission scopes — can reference
-- projects.owner_id from day zero instead of forcing a whole-DB
-- rewrite later.
--
-- Naming: owner_id is intentionally generic. It is NOT "tenant_id"
-- (SaaS-flavoured) nor "user_id" (implies a user table we don't have).
-- Think of it as "the principal that owns this project" — a single
-- self-host box has one logical owner, larger deployments might have
-- many. The default value 'default' is a stable sentinel; every query
-- that doesn't care about ownership keeps working unchanged.
--
-- Slug uniqueness: previously a project slug was globally unique. With
-- owner_id, two different owners can legitimately pick the same slug
-- ("api" on owner alice and "api" on owner bob are not the same). Swap
-- the global UNIQUE(slug) for UNIQUE(owner_id, slug). The /p/{slug}
-- URL resolution stays the same for single-owner deployments because
-- there's only one row to match.

ALTER TABLE projects
    ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'default';

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_slug_key;

ALTER TABLE projects
    ADD CONSTRAINT projects_owner_slug_key UNIQUE (owner_id, slug);

CREATE INDEX idx_projects_owner ON projects(owner_id);

-- +goose Down
DROP INDEX IF EXISTS idx_projects_owner;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_key;
ALTER TABLE projects ADD CONSTRAINT projects_slug_key UNIQUE (slug);
ALTER TABLE projects DROP COLUMN IF EXISTS owner_id;
