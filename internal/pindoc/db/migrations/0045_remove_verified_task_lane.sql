-- +goose Up
-- Backup procedure:
--   1. Before applying this migration in production, take a logical backup:
--        pg_dump --format=custom --file=pindoc-before-0045.dump "$DATABASE_URL"
--   2. To inspect the affected rows before apply:
--        SELECT slug, task_meta->>'status' FROM artifacts WHERE type='Task' AND task_meta->>'status'='verified';
--        SELECT slug FROM artifacts WHERE type='VerificationReport' AND status NOT IN ('archived','superseded');
--   3. Rollback cannot reconstruct which claimed_done Tasks used to be
--      verified after the lane is collapsed. Restore the dump if that
--      historical distinction is required.

INSERT INTO events (project_id, kind, subject_id, payload)
SELECT project_id,
       'artifact.task_status_collapsed',
       id,
       jsonb_build_object(
         'migration', '0045_remove_verified_task_lane',
         'from_status', 'verified',
         'to_status', 'claimed_done',
         'collapsed_at', now()
       )
  FROM artifacts
 WHERE type = 'Task'
   AND task_meta->>'status' = 'verified';

UPDATE artifacts
   SET task_meta = jsonb_set(COALESCE(task_meta, '{}'::jsonb), '{status}', '"claimed_done"', true),
       updated_at = now()
 WHERE type = 'Task'
   AND task_meta->>'status' = 'verified';

INSERT INTO events (project_id, kind, subject_id, payload)
SELECT project_id,
       'artifact.type_archived',
       id,
       jsonb_build_object(
         'migration', '0045_remove_verified_task_lane',
         'archived_type', 'VerificationReport',
         'reason', 'VerificationReport type retired with verified task lane',
         'archived_at', now()
       )
  FROM artifacts
 WHERE type = 'VerificationReport'
   AND status NOT IN ('archived', 'superseded');

UPDATE artifacts
   SET status = 'archived',
       updated_at = now()
 WHERE type = 'VerificationReport'
   AND status NOT IN ('archived', 'superseded');

-- +goose Down
-- Irreversible without the pre-migration backup: verified statuses are
-- intentionally collapsed into claimed_done.
