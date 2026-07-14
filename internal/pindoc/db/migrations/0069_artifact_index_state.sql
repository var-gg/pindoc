-- +goose Up
-- Index provenance is separate from artifact_chunks so a failed refresh can
-- describe the current artifact revision without deleting the last known-good
-- searchable chunks. Legacy rows are deliberately marked unknown: the old
-- schema cannot prove that their chunks match the current title/body.

CREATE TABLE artifact_index_state (
    artifact_id       UUID PRIMARY KEY REFERENCES artifacts(id) ON DELETE CASCADE,
    revision_number   INT NOT NULL DEFAULT 0,
    body_hash         TEXT NOT NULL,
    title_hash        TEXT NOT NULL,
    model_name        TEXT NOT NULL DEFAULT '',
    model_id          TEXT NOT NULL DEFAULT '',
    model_dim         INT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'unknown'
                      CHECK (status IN ('unknown', 'indexed', 'failed')),
    attempt_count     INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_attempt_at   TIMESTAMPTZ,
    indexed_at        TIMESTAMPTZ,
    last_error        TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_artifact_index_state_status
    ON artifact_index_state(status, updated_at);

INSERT INTO artifact_index_state (
    artifact_id, revision_number, body_hash, title_hash, status
)
SELECT
    a.id,
    COALESCE(r.revision_number, 0),
    encode(sha256(convert_to(a.body_markdown, 'UTF8')), 'hex'),
    encode(sha256(convert_to(a.title, 'UTF8')), 'hex'),
    'unknown'
FROM artifacts a
LEFT JOIN LATERAL (
    SELECT revision_number
    FROM artifact_revisions
    WHERE artifact_id = a.id
    ORDER BY revision_number DESC
    LIMIT 1
) r ON TRUE
ON CONFLICT (artifact_id) DO NOTHING;

-- The stored status reports the last attempt. effective_status additionally
-- detects content drift even when a mutation path has not refreshed the index
-- yet. Revision number remains provenance; title/body hashes decide whether
-- the existing chunks are still valid after metadata-only revisions.
CREATE VIEW artifact_index_health AS
SELECT
    a.id AS artifact_id,
    a.project_id,
    a.slug,
    COALESCE(r.revision_number, 0) AS current_revision_number,
    encode(sha256(convert_to(a.body_markdown, 'UTF8')), 'hex') AS current_body_hash,
    encode(sha256(convert_to(a.title, 'UTF8')), 'hex') AS current_title_hash,
    s.revision_number AS indexed_revision_number,
    s.body_hash AS indexed_body_hash,
    s.title_hash AS indexed_title_hash,
    s.model_name,
    s.model_id,
    s.model_dim,
    s.status AS stored_status,
    CASE
        WHEN s.artifact_id IS NULL THEN 'unknown'
        WHEN s.status = 'failed' THEN 'failed'
        WHEN s.status = 'unknown' THEN 'unknown'
        WHEN s.body_hash <> encode(sha256(convert_to(a.body_markdown, 'UTF8')), 'hex')
          OR s.title_hash <> encode(sha256(convert_to(a.title, 'UTF8')), 'hex') THEN 'stale'
        ELSE 'indexed'
    END AS effective_status,
    COALESCE(s.attempt_count, 0) AS attempt_count,
    s.last_attempt_at,
    s.indexed_at,
    s.last_error,
    s.updated_at
FROM artifacts a
LEFT JOIN LATERAL (
    SELECT revision_number
    FROM artifact_revisions
    WHERE artifact_id = a.id
    ORDER BY revision_number DESC
    LIMIT 1
) r ON TRUE
LEFT JOIN artifact_index_state s ON s.artifact_id = a.id;

-- +goose Down
DROP VIEW IF EXISTS artifact_index_health;
DROP TABLE IF EXISTS artifact_index_state;
