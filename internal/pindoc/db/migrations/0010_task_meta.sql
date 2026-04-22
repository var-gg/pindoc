-- +goose Up
-- Phase 15b: typed Task metadata.
--
-- Task body markdown carries the TODO/acceptance/DoD structure (see
-- _template_task). The tracker dimensions — status, priority, assignee,
-- due date, parent — are operational data the Reader UI renders as a
-- kanban-lite; burying them in markdown makes the Reader hallucinate or
-- skip fields. Keep them in a typed JSONB blob so schema can evolve
-- without another migration per field.
--
-- Pindoc is NOT becoming Jira. The column exists so the Task artifact
-- lifecycle (promote → assign → transition → close) can be regulated by
-- the same artifact.propose pipeline as Debug/Decision/Analysis; it is
-- not the start of a sprint/board/burndown feature tree.
--
-- Shape (keys are all optional — null = unset):
--   {
--     "status":      "todo" | "in_progress" | "blocked" | "done" | "cancelled",
--     "priority":    "p0" | "p1" | "p2" | "p3",
--     "assignee":    "@handle" | "agent:claude-code" | "user:alice",
--     "due_at":      "2026-04-30T00:00:00Z",
--     "parent_slug": "<parent-task-slug>"
--   }
--
-- Only applies to artifacts with type='Task'. Other types leave it null;
-- the column is nullable on purpose.

ALTER TABLE artifacts
    ADD COLUMN task_meta JSONB;

-- Index lets us scan "all tasks by status" without a JSONB-wide seqscan.
-- Path op class because we only query the status subkey today.
CREATE INDEX idx_artifacts_task_status
    ON artifacts ((task_meta->>'status'))
    WHERE type = 'Task' AND task_meta IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_artifacts_task_status;
ALTER TABLE artifacts DROP COLUMN IF EXISTS task_meta;
