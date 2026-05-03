-- +goose Up
-- OAuth client administration, Dynamic Client Registration provenance, and
-- consent grant cache.

ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS created_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS created_via TEXT NOT NULL DEFAULT 'env_seed',
    ADD COLUMN IF NOT EXISTS seed_suppressed BOOLEAN NOT NULL DEFAULT false;

UPDATE oauth_clients
   SET display_name = client_id
 WHERE display_name = '';

CREATE TABLE IF NOT EXISTS oauth_consents (
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id      TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    granted_scopes TEXT[] NOT NULL DEFAULT '{}',
    granted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, client_id)
);

-- +goose Down

DROP TABLE IF EXISTS oauth_consents;

ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS seed_suppressed,
    DROP COLUMN IF EXISTS created_via,
    DROP COLUMN IF EXISTS created_by_user_id,
    DROP COLUMN IF EXISTS display_name;
