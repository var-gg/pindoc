-- +goose Up
-- Canonicalize active users.email by lower(email), with soft-delete room.
--
-- deleted_at is nullable and unused by V1 flows, but the partial lower()
-- index is defined only for active rows so a future soft-deleted account
-- can release its email for a new signup.

ALTER TABLE users
    ADD COLUMN deleted_at TIMESTAMPTZ NULL;

DO $$
DECLARE
    duplicate_groups INTEGER;
BEGIN
    SELECT count(*) INTO duplicate_groups
    FROM (
        SELECT lower(email)
        FROM users
        WHERE email IS NOT NULL
          AND deleted_at IS NULL
        GROUP BY lower(email)
        HAVING count(*) > 1
    ) collisions;

    IF duplicate_groups > 0 THEN
        RAISE EXCEPTION 'USERS_EMAIL_LOWER_DUPLICATE: % active lower(email) collision group(s); clean users.email before applying this migration', duplicate_groups;
    END IF;
END $$;

-- Rebuild the legacy case-sensitive unique index with the same active-row
-- predicate. The new lower(email) index is the canonical guard, but keeping
-- the legacy index name preserves existing error-code mapping and exact
-- email lookup performance.
DROP INDEX IF EXISTS idx_users_email_unique;
CREATE UNIQUE INDEX idx_users_email_unique
    ON users (email)
    WHERE email IS NOT NULL
      AND deleted_at IS NULL;

CREATE UNIQUE INDEX users_email_lower_idx
    ON users (lower(email))
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS users_email_lower_idx;
DROP INDEX IF EXISTS idx_users_email_unique;
CREATE UNIQUE INDEX idx_users_email_unique
    ON users (email)
    WHERE email IS NOT NULL;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
