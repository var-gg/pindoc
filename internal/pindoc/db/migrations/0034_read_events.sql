-- +goose Up
-- Artifact read tracking raw events. Aggregation/statistics surfaces are
-- intentionally deferred; this table keeps the per-session facts needed
-- for later rollups without guessing a dashboard shape up front.

CREATE TABLE read_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id    UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    started_at     TIMESTAMPTZ NOT NULL,
    ended_at       TIMESTAMPTZ NOT NULL,
    active_seconds DOUBLE PRECISION NOT NULL DEFAULT 0
                   CHECK (active_seconds >= 0),
    scroll_max_pct DOUBLE PRECISION NOT NULL DEFAULT 0
                   CHECK (scroll_max_pct >= 0 AND scroll_max_pct <= 1),
    idle_seconds   DOUBLE PRECISION NOT NULL DEFAULT 0
                   CHECK (idle_seconds >= 0),
    locale         TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ended_at >= started_at),
    CHECK (active_seconds <= EXTRACT(EPOCH FROM (ended_at - started_at)))
);

CREATE INDEX idx_read_events_artifact_started
    ON read_events(artifact_id, started_at);
CREATE INDEX idx_read_events_user_started
    ON read_events(user_id, started_at);

-- +goose Down
DROP INDEX IF EXISTS idx_read_events_user_started;
DROP INDEX IF EXISTS idx_read_events_artifact_started;
DROP TABLE IF EXISTS read_events;
