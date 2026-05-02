-- +goose Up
-- Revisions predate the OAuth/local-user identity split and only carried
-- author_id, a client/agent label. Add the same nullable user FK that
-- artifact heads already use so history and diff views can render the
-- human author without exposing server-generated ag_* process ids.

ALTER TABLE artifact_revisions
    ADD COLUMN IF NOT EXISTS author_user_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'artifact_revisions_author_user_id_fkey'
    ) THEN
        ALTER TABLE artifact_revisions
            ADD CONSTRAINT artifact_revisions_author_user_id_fkey
            FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_artifact_revisions_author_user
    ON artifact_revisions(author_user_id);

-- Single-user/local installs that were created before OAuth often have
-- legacy agent-authored artifact heads with no author_user_id. If the
-- instance has a default loopback/OAuth user, bind those legacy agent
-- rows to that user; system seed rows intentionally stay anonymous.
WITH default_user AS (
    SELECT default_loopback_user_id AS id
      FROM server_settings
     WHERE default_loopback_user_id IS NOT NULL
     LIMIT 1
)
UPDATE artifacts a
   SET author_user_id = d.id
  FROM default_user d
 WHERE a.author_user_id IS NULL
   AND a.author_kind = 'agent';

UPDATE artifact_revisions r
   SET author_user_id = a.author_user_id
  FROM artifacts a
 WHERE r.artifact_id = a.id
   AND r.author_user_id IS NULL
   AND a.author_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_revisions_author_user;
ALTER TABLE artifact_revisions
    DROP CONSTRAINT IF EXISTS artifact_revisions_author_user_id_fkey;
ALTER TABLE artifact_revisions
    DROP COLUMN IF EXISTS author_user_id;
