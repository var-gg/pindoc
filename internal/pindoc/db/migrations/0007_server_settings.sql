-- +goose Up
-- Phase 14a: server-level operational settings.
--
-- Single-row table (enforced by CHECK id = 1) that holds instance config
-- an operator may want to tweak via UI/CLI without touching env or
-- restarting. Keep infrastructure config (DB URL, ports, TLS) in env/file
-- where restart is expected; put operator-editable config here.
--
-- env remains the seed source: on first boot the server populates empty
-- rows from PINDOC_PUBLIC_BASE_URL (and future env seeds). After that,
-- DB is the source of truth — env changes are ignored. This matches the
-- Ghost / Plausible pattern: env seeds, Admin UI/CLI updates, no silent
-- UI-override-by-env surprise.
--
-- Expansion: add a column per setting when needed. Typed columns beat a
-- key/value map because they give us migrations + defaults for free.

CREATE TABLE server_settings (
    id               SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    public_base_url  TEXT NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO server_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS server_settings;
