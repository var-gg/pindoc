-- +goose Up
-- Drop the unconditional `pindoc` bootstrap project on fresh OSS installs.
--
-- 0002 unconditionally seeds a `pindoc` project. 0006 / 0008 then add
-- `_template_` artifacts and example sub-areas under it. The combination
-- occupies the `pindoc` slug on a fresh OSS clone, contradicts README's
-- promise that `/` redirects to the new-project wizard, and surfaces an
-- empty default project to first-time users.
--
-- This migration removes the bootstrap project iff it carries only the
-- `_template_` scaffolding (no operator-authored artifact). Operator
-- dogfood instances retain their `pindoc` project because they hold
-- non-template artifacts. Idempotent: silent no-op when the project is
-- already gone or has been written into.
--
-- ON DELETE CASCADE on projects(id) handles areas, artifacts, chunks,
-- revisions, edges, pins, events, invite_tokens, members, assets and
-- other downstream rows automatically.
DELETE FROM projects p
 WHERE p.slug = 'pindoc'
   AND NOT EXISTS (
       SELECT 1 FROM artifacts a
        WHERE a.project_id = p.id
          AND NOT (a.tags @> ARRAY['_template'])
   );

-- +goose Down
-- 0002 / 0006 / 0008 cannot be re-run from this migration; treat as
-- forward-only cleanup.
