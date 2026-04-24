-- +goose Up
-- Phase M1.x — Task assignee backfill (Decision
-- task-assignee-default-author-id).
--
-- Context: the propose create path never injected a default assignee,
-- so 24 of 25 existing Tasks sit with task_meta.assignee NULL. UI
-- TaskControls now expects every active Task to land with a
-- legitimate dropdown-visible owner — missing assignee reads as an
-- operational gap, not a signal.
--
-- Rule: rows that are still operationally active (status null or
-- `open`) and have no assignee yet get `agent:codex` assigned.
-- claimed_done / verified / blocked / cancelled are left alone —
-- retroactively renaming the assignee of a completed Task would
-- muddy the audit trail, and those rows don't need a dropdown owner
-- to function.
--
-- Go-forward create rule (preflight): new Task without explicit
-- assignee gets `agent:<author_id>`. This migration only covers the
-- rows that predate that rule.

UPDATE artifacts
   SET task_meta = COALESCE(task_meta, '{}'::jsonb)
                   || jsonb_build_object('assignee', 'agent:codex')
 WHERE type = 'Task'
   AND (task_meta->>'assignee') IS NULL
   AND (
       (task_meta->>'status') IS NULL
       OR (task_meta->>'status') = 'open'
   );

-- +goose Down
-- Only revert the exact sentinel we set. If downstream edits already
-- moved the assignee elsewhere, the Down is a no-op for that row —
-- we don't want to undo a legitimate reassignment because we happen
-- to be rolling this migration back.
UPDATE artifacts
   SET task_meta = task_meta - 'assignee'
 WHERE type = 'Task'
   AND (task_meta->>'assignee') = 'agent:codex'
   AND (
       (task_meta->>'status') IS NULL
       OR (task_meta->>'status') = 'open'
   );
