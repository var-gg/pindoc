-- +goose Up
-- Revision history for artifacts. Append-only; artifacts.body_markdown keeps
-- the head so the read path stays a single-table query. Each propose that
-- supplies update_of inserts a new row here, increments
-- (artifact_id, revision_number), and re-embeds chunks.
--
-- body_hash is sha256 of body_markdown. Clients can dedupe no-op updates
-- cheaply and — further out — we can chunk/embed by hash to skip re-embed
-- when only title changes.

CREATE TABLE artifact_revisions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id        UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    revision_number    INT  NOT NULL,
    title              TEXT NOT NULL,
    body_markdown      TEXT NOT NULL,
    body_hash          TEXT NOT NULL,
    tags               TEXT[] NOT NULL DEFAULT '{}',
    completeness       TEXT NOT NULL,
    author_kind        TEXT NOT NULL,
    author_id          TEXT NOT NULL,
    author_version     TEXT,
    source_session_ref JSONB,
    commit_msg         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, revision_number)
);

CREATE INDEX idx_artifact_revisions_artifact_created
    ON artifact_revisions(artifact_id, created_at DESC);
CREATE INDEX idx_artifact_revisions_body_hash
    ON artifact_revisions(body_hash);

-- Backfill revision 1 for every existing artifact so the invariant
-- "every artifact has at least one revision" holds from 0004 forward.
INSERT INTO artifact_revisions (
    artifact_id, revision_number, title, body_markdown, body_hash, tags,
    completeness, author_kind, author_id, author_version,
    source_session_ref, commit_msg, created_at
)
SELECT
    id,
    1,
    title,
    body_markdown,
    encode(sha256(convert_to(body_markdown, 'UTF8')), 'hex'),
    tags,
    completeness,
    author_kind,
    author_id,
    author_version,
    source_session_ref,
    'backfill: rev 1 synthesised by migration 0004',
    COALESCE(published_at, created_at)
FROM artifacts
WHERE NOT EXISTS (
    SELECT 1 FROM artifact_revisions r WHERE r.artifact_id = artifacts.id
);

-- +goose Down
DROP TABLE IF EXISTS artifact_revisions;
