-- +goose Up
-- Project locale composite key (Decision `decision-project-locale-composite-
-- key`, Task `task-phase-18-project-locale-implementation`). Same slug may
-- exist across locales; URLs carry both segments (/p/{slug}/{locale}/...).
-- V1 self-host single-user starts with `pindoc` → `pindoc/ko`; new projects
-- pick their own locale at create time.
--
-- Schema shape:
--   projects.locale TEXT NOT NULL  — 'en' | 'ko' for M1; more locales land
--                                    once translation_of edges stabilise
--   UNIQUE (owner_id, slug, locale) — replaces the old (owner_id, slug)
--                                     and single-column slug constraints

ALTER TABLE projects ADD COLUMN locale TEXT;
UPDATE projects SET locale = COALESCE(NULLIF(primary_language, ''), 'en');
ALTER TABLE projects ALTER COLUMN locale SET NOT NULL;

-- Drop both single-column slug unique and the Phase 16 (owner_id, slug)
-- composite — locale becomes part of the canonical identity.
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_slug_key;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_key;

ALTER TABLE projects ADD CONSTRAINT projects_owner_slug_locale_unique
    UNIQUE (owner_id, slug, locale);

-- Keep primary_language around as a soft back-compat field for now. Legacy
-- code paths (reembed CLI, harness_install templates) still read it while
-- new code reads `locale`. Decommission lands in a follow-up migration once
-- all call sites are migrated.

-- +goose Down
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_locale_unique;
ALTER TABLE projects ADD CONSTRAINT projects_slug_key UNIQUE (slug);
ALTER TABLE projects ADD CONSTRAINT projects_owner_slug_key UNIQUE (owner_id, slug);
ALTER TABLE projects DROP COLUMN IF EXISTS locale;
