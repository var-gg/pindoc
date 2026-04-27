-- +goose Up
-- Instance-level identity provider registry.
--
-- Decision decision-auth-model-loopback-and-providers § 3 puts the
-- active IdP list on PINDOC_AUTH_PROVIDERS env. task-providers-admin-
-- ui completes the envelope: env seeds defaults, the admin UI mutates
-- this DB row at runtime so credential rotation / IdP toggling does
-- not require a daemon restart.
--
-- One row per (provider_name) pair. credential_secret_encrypted holds
-- AES-256-GCM ciphertext keyed by PINDOC_INSTANCE_KEY (32-byte base64
-- env). Plain client_id stays unencrypted because it appears in the
-- /.well-known/oauth-authorization-server metadata anyway. Daemon
-- refuses to start when an encrypted row exists but PINDOC_INSTANCE_KEY
-- is unset (fail loud rather than silently lose decryption capability).

CREATE TABLE instance_providers (
    id                            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- provider_name is the wire identifier the auth chain switches on
    -- ("github", "google", "local-password", ...). Lowercase canonical.
    provider_name                 TEXT NOT NULL UNIQUE,
    -- Display copy for the admin UI. Generated server-side from
    -- provider_name when the operator does not override.
    display_name                  TEXT NOT NULL DEFAULT '',
    -- Plain client_id — public-facing per OAuth 2.1 (visible in
    -- authorization redirects).
    client_id                     TEXT NOT NULL,
    -- AES-256-GCM ciphertext: nonce(12) || ciphertext || tag(16). Empty
    -- when the IdP doesn't need a secret (passkey / WebAuthn future).
    credential_secret_encrypted   BYTEA NOT NULL DEFAULT ''::bytea,
    -- Free-form provider config (auth_url overrides, scopes, etc.) —
    -- non-secret. Encrypt it instead if a future provider stores
    -- secrets here.
    config_json                   JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled                       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by_user_id            UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX instance_providers_enabled_idx
    ON instance_providers (enabled)
 WHERE enabled = TRUE;

-- +goose Down
DROP TABLE IF EXISTS instance_providers;
