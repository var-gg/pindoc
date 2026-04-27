-- +goose Up
-- Artifact body locale metadata for agent-driven on-demand translation
-- (Task `task-artifact-translate-tool`).
--
-- Project `primary_language` remains the default authoring language, but
-- cached translations are ordinary artifacts with body_locale set to the
-- translated view language and a translation_of edge to the source.

ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS body_locale TEXT NOT NULL DEFAULT 'en';

UPDATE artifacts a
   SET body_locale = COALESCE(NULLIF(p.primary_language, ''), 'en')
  FROM projects p
 WHERE a.project_id = p.id
   AND (a.body_locale IS NULL OR a.body_locale = '' OR a.body_locale = 'en');

CREATE INDEX IF NOT EXISTS idx_artifacts_project_body_locale
    ON artifacts(project_id, body_locale);

-- +goose Down
DROP INDEX IF EXISTS idx_artifacts_project_body_locale;
ALTER TABLE artifacts DROP COLUMN IF EXISTS body_locale;
