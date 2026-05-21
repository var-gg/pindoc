-- +goose Up
-- Decision taxonomy-change-operation T8: areas gain a lifecycle so a
-- profile.adopt can retire an old top-level area without deleting it.
--
--   active   — normal area; accepts new artifacts when fileable
--   retiring — legacy area; holds existing artifacts, refuses new filing
--   archived — emptied legacy area; hidden from default listings
--
-- Every pre-existing area backfills to 'active' via the column DEFAULT,
-- so taxonomy behavior before T8 is unchanged. retired_by_change_id ties
-- a retiring/archived area to the taxonomy_changes row that moved it; the
-- FK is ON DELETE SET NULL so pruning an old change-set never cascades
-- into areas.

ALTER TABLE areas
    ADD COLUMN lifecycle TEXT NOT NULL DEFAULT 'active'
        CHECK (lifecycle IN ('active', 'retiring', 'archived')),
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD COLUMN retired_by_change_id UUID
        REFERENCES taxonomy_changes(id) ON DELETE SET NULL;

CREATE INDEX idx_areas_project_lifecycle
    ON areas (project_id, lifecycle);

-- +goose Down
DROP INDEX IF EXISTS idx_areas_project_lifecycle;
ALTER TABLE areas
    DROP COLUMN IF EXISTS retired_by_change_id,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS lifecycle;
