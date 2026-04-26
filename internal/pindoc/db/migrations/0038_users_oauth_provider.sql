-- +goose Up
-- GitHub OAuth identity link fields. Email remains the account-linking
-- anchor for existing trusted_local rows; provider/provider_uid preserve
-- the stable IdP identity after the first successful callback.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS provider TEXT NULL,
    ADD COLUMN IF NOT EXISTS provider_uid TEXT NULL;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_provider_check,
    ADD CONSTRAINT users_provider_check
        CHECK (provider IS NULL OR provider IN ('github'));

CREATE UNIQUE INDEX IF NOT EXISTS users_provider_uid_unique
    ON users (provider, provider_uid)
    WHERE provider IS NOT NULL
      AND provider_uid IS NOT NULL
      AND deleted_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS users_provider_uid_unique;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_provider_check,
    DROP COLUMN IF EXISTS provider_uid,
    DROP COLUMN IF EXISTS provider;
