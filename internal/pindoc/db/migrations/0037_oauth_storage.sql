-- +goose Up
-- OAuth 2.1 authorization-server storage for oauth_github mode.
-- Token/code columns store fosite signatures only; raw bearer secrets are
-- returned once to the client and never persisted.

CREATE TABLE oauth_clients (
    client_id      TEXT PRIMARY KEY,
    secret_hash    BYTEA NULL,
    redirect_uris  TEXT[] NOT NULL,
    grant_types    TEXT[] NOT NULL,
    response_types TEXT[] NOT NULL DEFAULT ARRAY['code'],
    scopes         TEXT[] NOT NULL,
    public         BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ NULL
);

CREATE TABLE oauth_authorize_codes (
    code_hash        TEXT PRIMARY KEY,
    request_id       TEXT NOT NULL,
    client_id        TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    scopes           TEXT[] NOT NULL,
    requested_scopes TEXT[] NOT NULL DEFAULT '{}',
    requested_at     TIMESTAMPTZ NOT NULL,
    form_data        JSONB NOT NULL DEFAULT '{}'::jsonb,
    session          JSONB NOT NULL DEFAULT '{}'::jsonb,
    active           BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_authorize_codes_request_id
    ON oauth_authorize_codes(request_id);

CREATE TABLE oauth_access_tokens (
    token_hash       TEXT PRIMARY KEY,
    request_id       TEXT NOT NULL,
    client_id        TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    scopes           TEXT[] NOT NULL,
    requested_scopes TEXT[] NOT NULL DEFAULT '{}',
    expires_at       TIMESTAMPTZ NOT NULL,
    form_data        JSONB NOT NULL DEFAULT '{}'::jsonb,
    session          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_access_tokens_request_id
    ON oauth_access_tokens(request_id);
CREATE INDEX idx_oauth_access_tokens_expires_at
    ON oauth_access_tokens(expires_at);

CREATE TABLE oauth_refresh_tokens (
    token_hash        TEXT PRIMARY KEY,
    request_id        TEXT NOT NULL,
    client_id         TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    access_token_hash TEXT NULL,
    expires_at        TIMESTAMPTZ NOT NULL,
    form_data         JSONB NOT NULL DEFAULT '{}'::jsonb,
    session           JSONB NOT NULL DEFAULT '{}'::jsonb,
    rotated_from      TEXT NULL REFERENCES oauth_refresh_tokens(token_hash) ON DELETE SET NULL,
    active            BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_refresh_tokens_request_id
    ON oauth_refresh_tokens(request_id);
CREATE INDEX idx_oauth_refresh_tokens_expires_at
    ON oauth_refresh_tokens(expires_at);

CREATE TABLE oauth_pkce_requests (
    request_id            TEXT PRIMARY KEY,
    client_id             TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    code_challenge        TEXT NOT NULL,
    code_challenge_method TEXT NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    form_data             JSONB NOT NULL DEFAULT '{}'::jsonb,
    session               JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_pkce_requests_expires_at
    ON oauth_pkce_requests(expires_at);

-- +goose Down

DROP INDEX IF EXISTS idx_oauth_pkce_requests_expires_at;
DROP TABLE IF EXISTS oauth_pkce_requests;

DROP INDEX IF EXISTS idx_oauth_refresh_tokens_expires_at;
DROP INDEX IF EXISTS idx_oauth_refresh_tokens_request_id;
DROP TABLE IF EXISTS oauth_refresh_tokens;

DROP INDEX IF EXISTS idx_oauth_access_tokens_expires_at;
DROP INDEX IF EXISTS idx_oauth_access_tokens_request_id;
DROP TABLE IF EXISTS oauth_access_tokens;

DROP INDEX IF EXISTS idx_oauth_authorize_codes_request_id;
DROP TABLE IF EXISTS oauth_authorize_codes;

DROP TABLE IF EXISTS oauth_clients;
