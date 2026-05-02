-- +goose Up
-- Project-level default for artifacts.visibility.
--
-- Migration 0050 set the per-artifact column with a global safe default
-- of 'org'. That works for projects whose contents are private-by-
-- default (the typical SaaS workspace), but the Pindoc dogfood project
-- itself is meant to be the public OSS sample — flipping every new
-- artifact through an explicit visibility='public' on every propose
-- call is busywork and prone to omission.
--
-- Per-project default solves it: each project declares "my artifacts
-- default to X". artifact.propose cascades:
--   explicit_visibility (param)
--   ?? projects.default_artifact_visibility   -- this column
--   ?? 'org'                                   -- global safe default
--
-- The column carries the same CHECK as artifacts.visibility so the
-- two stay in lockstep. New rows default to 'org' so existing project
-- create flows that don't pass the field keep their conservative
-- behavior; the Pindoc dogfood project flips its row to 'public' via
-- the project settings PATCH endpoint once this lands.

ALTER TABLE projects
    ADD COLUMN default_artifact_visibility TEXT NOT NULL DEFAULT 'org'
        CHECK (default_artifact_visibility IN ('public', 'org', 'private'));


-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS default_artifact_visibility;
