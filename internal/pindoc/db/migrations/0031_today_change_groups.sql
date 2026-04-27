-- +goose Up
-- Today surface read state + summary cache. Change Groups are a query model
-- over artifact_revisions, so they do not need their own table.

CREATE TABLE reader_watermarks (
    user_key           TEXT NOT NULL,
    project_id         UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    revision_watermark INT NOT NULL DEFAULT 0,
    seen_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_key, project_id)
);

CREATE INDEX idx_reader_watermarks_project_seen
    ON reader_watermarks(project_id, seen_at DESC);

CREATE TABLE summary_cache (
    cache_key            TEXT PRIMARY KEY,
    project_id           UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_key             TEXT NOT NULL,
    locale               TEXT NOT NULL,
    filter_hash          TEXT NOT NULL,
    baseline_revision_id INT NOT NULL,
    max_revision_id      INT NOT NULL,
    headline             TEXT NOT NULL,
    bullets              TEXT[] NOT NULL DEFAULT '{}',
    source               TEXT NOT NULL CHECK (source IN ('llm', 'rule_based')),
    input_hash           TEXT NOT NULL,
    token_estimate       INT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at           TIMESTAMPTZ
);

CREATE INDEX idx_summary_cache_project_created
    ON summary_cache(project_id, created_at DESC);

CREATE TABLE summary_usage_daily (
    user_key    TEXT NOT NULL,
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    tokens_used INT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_key, project_id, day)
);

-- +goose Down
DROP TABLE IF EXISTS summary_usage_daily;
DROP INDEX IF EXISTS idx_summary_cache_project_created;
DROP TABLE IF EXISTS summary_cache;
DROP INDEX IF EXISTS idx_reader_watermarks_project_seen;
DROP TABLE IF EXISTS reader_watermarks;
