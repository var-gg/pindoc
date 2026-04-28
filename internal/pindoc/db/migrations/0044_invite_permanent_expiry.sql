-- +goose Up
-- Permanent invite links store NULL expires_at. Non-permanent rows keep the
-- existing timestamp semantics and active queries treat NULL as never expiring.

ALTER TABLE invite_tokens
    ALTER COLUMN expires_at DROP NOT NULL;

-- +goose Down

UPDATE invite_tokens
   SET expires_at = issued_at + interval '30 days'
 WHERE expires_at IS NULL;

ALTER TABLE invite_tokens
    ALTER COLUMN expires_at SET NOT NULL;
