-- +goose Up
-- Normalize legacy Task assignees that stored raw user UUIDs. The canonical
-- display value is @github_handle when available, otherwise user:display_name.
-- Rows whose UUID no longer resolves are left untouched so operators can audit
-- them instead of silently inventing a label.

WITH resolved AS (
    SELECT a.id,
           CASE
               WHEN NULLIF(trim(u.github_handle), '') IS NOT NULL
                   THEN '@' || ltrim(trim(u.github_handle), '@')
               ELSE 'user:' || trim(u.display_name)
           END AS normalized_assignee
      FROM artifacts a
      JOIN users u
        ON u.id::text = CASE
            WHEN a.task_meta->>'assignee' LIKE 'user:%'
                THEN substring(a.task_meta->>'assignee' FROM 6)
            ELSE a.task_meta->>'assignee'
        END
     WHERE a.type = 'Task'
       AND u.deleted_at IS NULL
       AND (
           a.task_meta->>'assignee' ~* '^user:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
           OR a.task_meta->>'assignee' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
       )
)
UPDATE artifacts a
   SET task_meta = jsonb_set(
       COALESCE(a.task_meta, '{}'::jsonb),
       '{assignee}',
       to_jsonb(r.normalized_assignee),
       true
   )
  FROM resolved r
 WHERE a.id = r.id;

-- +goose Down
-- Irreversible data cleanup: the original UUID can be recovered from backups
-- or artifact revisions when needed. Do not rewrite human-readable assignees
-- back to raw identifiers on rollback.
SELECT 1;
