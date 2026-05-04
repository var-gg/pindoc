-- +goose Up
-- DCR-created OAuth clients are intentionally ephemeral. Track their
-- absolute secret expiry and last successful use so automated cleanup can
-- keep the public registration cap from becoming a permanent lock.

ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;

UPDATE oauth_clients
   SET expires_at = created_at + interval '90 days'
 WHERE created_via = 'dcr'
   AND expires_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_oauth_clients_dcr_expires_at
    ON oauth_clients(expires_at)
 WHERE deleted_at IS NULL AND created_via = 'dcr';

CREATE INDEX IF NOT EXISTS idx_oauth_clients_dcr_last_used_at
    ON oauth_clients(last_used_at)
 WHERE deleted_at IS NULL AND created_via = 'dcr';

-- +goose Down

DROP INDEX IF EXISTS idx_oauth_clients_dcr_last_used_at;
DROP INDEX IF EXISTS idx_oauth_clients_dcr_expires_at;

ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS last_used_at;
