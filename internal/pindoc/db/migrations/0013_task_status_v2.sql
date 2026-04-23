-- +goose Up
-- Task lifecycle v2: redesign status enum around agent/verifier two-phase
-- completion instead of single-seat human workflow (Analysis `mcp-dog-food-
-- 1차-관찰-...` + discussion 2026-04-23).
--
-- Old enum (Jira-style, single-seat):
--   todo | in_progress | blocked | done | cancelled
--
-- New enum:
--   open         — default; agent has not claimed completion
--   claimed_done — implementation agent self-attests done (server requires
--                  minimum evidence: acceptance checkboxes 100%, optional
--                  pin-path git sha newer than task published_at)
--   verified     — a DIFFERENT agent attached a VerificationReport artifact
--                  via pindoc.artifact.verify. propose path rejects direct
--                  transition to this value (VER_VIA_VERIFY_TOOL_ONLY).
--   blocked      — carried over; external dependency blocks progress
--   cancelled    — carried over; Task is abandoned
--
-- Migration policy: data migrate (JSONB value update) only — no column
-- shape change. `todo` and `in_progress` both collapse into `open` because
-- (a) `todo` was the default and (b) `in_progress` was rarely set in the
-- dog-food corpus and is semantically absorbed by "agent is actively
-- working, no separate state needed at human timescales" (see discussion).
-- Existing `done` becomes `claimed_done` because the old `done` was an
-- agent self-assertion without the new verifier step.
--
-- Rollback: reverse mapping when possible (claimed_done → done, open →
-- todo). `verified` cannot round-trip safely — maps back to `done`.

UPDATE artifacts
   SET task_meta = jsonb_set(task_meta, '{status}', '"open"')
 WHERE type = 'Task'
   AND task_meta IS NOT NULL
   AND task_meta ? 'status'
   AND task_meta->>'status' IN ('todo', 'in_progress');

UPDATE artifacts
   SET task_meta = jsonb_set(task_meta, '{status}', '"claimed_done"')
 WHERE type = 'Task'
   AND task_meta IS NOT NULL
   AND task_meta ? 'status'
   AND task_meta->>'status' = 'done';

-- +goose Down
UPDATE artifacts
   SET task_meta = jsonb_set(task_meta, '{status}', '"todo"')
 WHERE type = 'Task'
   AND task_meta IS NOT NULL
   AND task_meta ? 'status'
   AND task_meta->>'status' = 'open';

UPDATE artifacts
   SET task_meta = jsonb_set(task_meta, '{status}', '"done"')
 WHERE type = 'Task'
   AND task_meta IS NOT NULL
   AND task_meta ? 'status'
   AND task_meta->>'status' IN ('claimed_done', 'verified');
