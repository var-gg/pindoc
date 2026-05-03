-- +goose Up
-- Dynamic Client Registration is closed by default and records minimal
-- request audit metadata when the operator explicitly opens it.

ALTER TABLE server_settings
    ADD COLUMN IF NOT EXISTS dcr_mode TEXT NOT NULL DEFAULT 'closed';

UPDATE server_settings
   SET dcr_mode = 'closed'
 WHERE dcr_mode IS NULL OR dcr_mode NOT IN ('closed', 'open');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'server_settings_dcr_mode_check'
    ) THEN
        ALTER TABLE server_settings
            ADD CONSTRAINT server_settings_dcr_mode_check
            CHECK (dcr_mode IN ('closed', 'open'));
    END IF;
END $$;

ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS created_remote_addr TEXT NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS created_remote_addr;

ALTER TABLE server_settings
    DROP CONSTRAINT IF EXISTS server_settings_dcr_mode_check,
    DROP COLUMN IF EXISTS dcr_mode;
