-- +goose Up
-- Phase M1.x — Task status baseline backfill.
--
-- Context: migration 0013 remapped legacy status strings but intentionally
-- only touched rows that already had task_meta.status. Several dogfood
-- Tasks were proposed after the status-v2 migration and before the
-- artifact.propose create path injected a default status, so they landed
-- with task_meta present but no status key. The Reader Kanban then has to
-- render a "no status" bucket, which violates the Task lifecycle invariant.
--
-- Rule: missing Task status means the Task is still open. Preserve every
-- other task_meta key (assignee, priority, due_at, parent_slug).

UPDATE artifacts
   SET task_meta = COALESCE(task_meta, '{}'::jsonb)
                   || jsonb_build_object('status', 'open')
 WHERE type = 'Task'
   AND (task_meta->>'status') IS NULL;

-- +goose Down
-- No-op by design. The Up migration restores the lifecycle invariant
-- "Task rows have a status"; removing status on rollback would recreate
-- invalid operational data and cannot distinguish backfilled rows from
-- legitimate open Tasks edited after the migration.
SELECT 1;
