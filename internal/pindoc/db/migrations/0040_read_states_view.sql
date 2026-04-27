-- +goose Up
-- read_events에 user_key 컬럼을 추가해 reader_watermarks와 정합되는 사용자 식별자를
-- raw read 신호 위에도 박는다. 기존 INSERT는 user_id를 항상 NULL로 적재했고
-- V1.5 auth 전까진 user_id가 채워지지 않는다. user_key는 trusted_local 모드에서
-- 기본 'local', 도메인 모드에선 OAuth principal의 stable handle을 받는다.
--
-- artifact_read_states view는 read_events의 raw aggregation per (artifact, user_key).
-- read_state classification (unseen/glanced/read/deeply_read) 와 completion_pct는
-- 본문 길이 + locale 기반 expected_seconds가 필요해 application layer (Go) 에서
-- 계산한다. 이 view는 raw 신호만 모아 application에 넘긴다.

ALTER TABLE read_events ADD COLUMN user_key TEXT;

UPDATE read_events SET user_key = 'local' WHERE user_key IS NULL;

ALTER TABLE read_events ALTER COLUMN user_key SET NOT NULL;

CREATE INDEX idx_read_events_user_artifact
    ON read_events(user_key, artifact_id, started_at);

CREATE OR REPLACE VIEW artifact_read_states AS
SELECT
    re.artifact_id,
    re.user_key,
    MIN(re.started_at)        AS first_seen_at,
    MAX(re.ended_at)          AS last_seen_at,
    SUM(re.active_seconds)    AS total_active_seconds,
    SUM(re.idle_seconds)      AS total_idle_seconds,
    MAX(re.scroll_max_pct)    AS max_scroll_pct,
    COUNT(*)                  AS event_count
FROM read_events re
GROUP BY re.artifact_id, re.user_key;

-- +goose Down
DROP VIEW IF EXISTS artifact_read_states;
DROP INDEX IF EXISTS idx_read_events_user_artifact;
ALTER TABLE read_events DROP COLUMN IF EXISTS user_key;
