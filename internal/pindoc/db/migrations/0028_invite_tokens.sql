-- +goose Up
-- Team invite tokens for OAuth signup. Raw tokens are returned once to
-- the issuer; only SHA-256 token_hash values are stored.

CREATE TABLE invite_tokens (
    token_hash  TEXT PRIMARY KEY,
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('editor', 'viewer')),
    issued_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    consumed_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_invite_tokens_project_consumed
    ON invite_tokens(project_id, consumed_at);
CREATE INDEX idx_invite_tokens_expires_at
    ON invite_tokens(expires_at);

-- +goose Down

DROP INDEX IF EXISTS idx_invite_tokens_expires_at;
DROP INDEX IF EXISTS idx_invite_tokens_project_consumed;
DROP TABLE IF EXISTS invite_tokens;
