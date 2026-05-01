-- +goose Up
-- Bind the "loopback principal" identity to a real users.id row at
-- runtime so the operator does not have to edit env to attribute
-- their work. Decision agent-only-write-분할 + Decision decision-
-- auth-model-loopback-and-providers § 2 already give the loopback
-- principal owner role; this column closes the attribution gap.
--
-- Boot logic (cmd/pindoc-server/main.go):
--   1. If env PINDOC_USER_NAME / PINDOC_USER_EMAIL are set and this
--      column is NULL, upsert a users row from env and seed.
--   2. Else if exactly one non-test users row exists (email NOT LIKE
--      '%@example.invalid'), bind it (legacy loopback backfill case).
--   3. Else leave NULL — Reader UI shows the onboarding identity
--      form (clean install: users table is empty).
--
-- ON DELETE SET NULL because losing the bound user is a recoverable
-- state — Reader falls back to the onboarding form so the operator
-- can re-bind without dropping into psql.

ALTER TABLE server_settings
    ADD COLUMN default_loopback_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE server_settings
    DROP COLUMN IF EXISTS default_loopback_user_id;
