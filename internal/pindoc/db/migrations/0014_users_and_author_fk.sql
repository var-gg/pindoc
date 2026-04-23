-- +goose Up
-- Author identity dual (Decision `decision-author-identity-dual`):
-- Pindoc separates the agent that wrote an artifact (author_id text, e.g.
-- "claude-code") from the human the agent acts for (users.id uuid). V1
-- self-host is single-user, but getting the schema ready now means V1.5
-- GitHub OAuth can upsert into an already-wired users table without a
-- back-compat migration.
--
-- Shape:
--   users.display_name    — free text, 2-60 runes (range validated by the
--                            MCP tool, not DB; Postgres length check would
--                            miscount combined hangul jamo)
--   users.email           — optional, unique-when-not-null
--   users.github_handle   — optional, unique-when-not-null. V1 leaves null;
--                            V1.5 OAuth Task fills it and flips source to
--                            'github_oauth'.
--   users.source          — 'harness_install' | 'pindoc_admin' | 'github_oauth'
--   artifacts.author_user_id — nullable uuid, FK to users(id). Existing rows
--                              stay null (D-slug backfill rule: no retro
--                              edits). Reader falls back to "(unknown) via
--                              {agent_id}" when null.

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name  TEXT NOT NULL,
    email         TEXT,
    github_handle TEXT,
    source        TEXT NOT NULL DEFAULT 'harness_install'
                      CHECK (source IN ('harness_install', 'pindoc_admin', 'github_oauth')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial unique index: lets multiple users leave email/github_handle null
-- during V1 single-user while still guaranteeing uniqueness once either
-- field is set (V1.5 OAuth depends on github_handle uniqueness).
CREATE UNIQUE INDEX idx_users_email_unique
    ON users (email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX idx_users_github_handle_unique
    ON users (github_handle) WHERE github_handle IS NOT NULL;

ALTER TABLE artifacts
    ADD COLUMN author_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_artifacts_author_user ON artifacts (author_user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_artifacts_author_user;
ALTER TABLE artifacts DROP COLUMN IF EXISTS author_user_id;
DROP INDEX IF EXISTS idx_users_github_handle_unique;
DROP INDEX IF EXISTS idx_users_email_unique;
DROP TABLE IF EXISTS users;
