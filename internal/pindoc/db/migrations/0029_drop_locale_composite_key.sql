-- +goose Up
-- Canonical-only project identity (Decision
-- `canonical-only-on-demand-translation`, Task
-- `task-canonical-locale-migration`).
--
-- Phase 18 briefly made locale part of project identity:
--   UNIQUE (owner_id, slug, locale)
--   Reader URLs: /p/{slug}/{locale}/wiki/...
--
-- The canonical-only model makes one project slug the single source of
-- truth. `primary_language` remains as metadata describing the canonical
-- language; translated views are on-demand or explicit artifacts, not
-- sibling project rows.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM projects
        GROUP BY slug
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION
            'cannot migrate to canonical-only project identity: duplicate project slug rows exist';
    END IF;
END $$;

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_locale_unique;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_key;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_slug_key;

ALTER TABLE projects ADD CONSTRAINT projects_slug_key UNIQUE (slug);

-- `primary_language` is the canonical language metadata. `locale` was only
-- needed while URL/identity carried a language segment.
ALTER TABLE projects DROP COLUMN IF EXISTS locale;

-- +goose Down
ALTER TABLE projects ADD COLUMN IF NOT EXISTS locale TEXT;
UPDATE projects
   SET locale = COALESCE(NULLIF(primary_language, ''), 'en')
 WHERE locale IS NULL OR locale = '';
ALTER TABLE projects ALTER COLUMN locale SET NOT NULL;

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_slug_key;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_key;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_owner_slug_locale_unique;

ALTER TABLE projects ADD CONSTRAINT projects_owner_slug_locale_unique
    UNIQUE (owner_id, slug, locale);
