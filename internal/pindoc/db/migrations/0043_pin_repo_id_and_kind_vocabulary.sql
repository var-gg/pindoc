-- +goose Up
-- Phase: canonical pin repo coordinates + expanded pin kind vocabulary.

ALTER TABLE project_repos
    ADD COLUMN IF NOT EXISTS local_paths TEXT[] NOT NULL DEFAULT '{}'::text[],
    ADD COLUMN IF NOT EXISTS urls        TEXT[] NOT NULL DEFAULT '{}'::text[];

UPDATE project_repos
   SET urls = ARRAY(
        SELECT DISTINCT u
          FROM unnest(ARRAY[
              NULLIF(git_remote_url, ''),
              NULLIF(git_remote_url_original, '')
          ]::text[]) AS u
         WHERE u IS NOT NULL
   )
 WHERE urls = '{}'::text[];

ALTER TABLE artifact_pins
    ADD COLUMN IF NOT EXISTS repo_id UUID NULL REFERENCES project_repos(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_artifact_pins_repo_id ON artifact_pins(repo_id);

ALTER TABLE artifact_pins
    DROP CONSTRAINT IF EXISTS artifact_pins_kind_check;

ALTER TABLE artifact_pins
    ADD CONSTRAINT artifact_pins_kind_check
    CHECK (kind IN ('code', 'doc', 'config', 'asset', 'resource', 'url'));

WITH inferred AS (
    SELECT
        id,
        CASE
            WHEN lower(path) LIKE 'http://%' OR lower(path) LIKE 'https://%' THEN 'url'
            WHEN regexp_replace(replace(lower(path), E'\\', '/'), '^.*/', '') ~ '^(readme|changelog|license|notice|contributing)' THEN 'doc'
            WHEN lower(path) ~ '\.(md|mdx|markdown|txt|rst|adoc)$' THEN 'doc'
            WHEN regexp_replace(replace(lower(path), E'\\', '/'), '^.*/', '') ~ '^(dockerfile|docker-compose)(\.|$)' THEN 'config'
            WHEN regexp_replace(replace(lower(path), E'\\', '/'), '^.*/', '') IN ('makefile') THEN 'config'
            WHEN regexp_replace(replace(lower(path), E'\\', '/'), '^.*/', '') LIKE '.env%' THEN 'config'
            WHEN regexp_replace(replace(lower(path), E'\\', '/'), '^.*/', '') LIKE '%.config.%' THEN 'config'
            WHEN lower(path) ~ '\.(json|ya?ml|toml|ini|conf)$' THEN 'config'
            WHEN lower(path) ~ '\.(png|jpe?g|gif|svg|webp|pdf|mp4|mp3|woff2?|ttf|ico)$' THEN 'asset'
            ELSE 'code'
        END AS kind
    FROM artifact_pins
    WHERE kind = 'code'
)
UPDATE artifact_pins p
   SET kind = inferred.kind
  FROM inferred
 WHERE p.id = inferred.id
   AND inferred.kind <> 'code';

UPDATE artifact_pins ap
   SET repo_id = pr.id
  FROM artifacts a
  JOIN project_repos pr ON pr.project_id = a.project_id
 WHERE ap.artifact_id = a.id
   AND ap.repo_id IS NULL
   AND (
        lower(ap.repo) = lower(COALESCE(pr.name, ''))
        OR lower(ap.repo) = lower(pr.git_remote_url)
        OR EXISTS (SELECT 1 FROM unnest(pr.urls) AS u WHERE lower(u) = lower(ap.repo))
        OR (
            ap.repo = 'origin'
            AND 1 = (
                SELECT count(*)
                  FROM project_repos pr2
                 WHERE pr2.project_id = a.project_id
            )
        )
   );

-- +goose Down

DROP INDEX IF EXISTS idx_artifact_pins_repo_id;
ALTER TABLE artifact_pins DROP COLUMN IF EXISTS repo_id;

ALTER TABLE artifact_pins
    DROP CONSTRAINT IF EXISTS artifact_pins_kind_check;

UPDATE artifact_pins
   SET kind = 'code'
 WHERE kind IN ('doc', 'config', 'asset');

ALTER TABLE artifact_pins
    ADD CONSTRAINT artifact_pins_kind_check
    CHECK (kind IN ('code', 'resource', 'url'));

ALTER TABLE project_repos
    DROP COLUMN IF EXISTS urls,
    DROP COLUMN IF EXISTS local_paths;
