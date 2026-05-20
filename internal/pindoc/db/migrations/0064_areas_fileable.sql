-- +goose Up
-- Decision area-taxonomy-profiled-skeleton T2: a per-area `fileable`
-- flag replaces the hardcoded "only misc/_unsorted top-level may hold
-- artifacts" rule in validateSetAreaTarget. A domain profile's
-- first-class top-level areas (e.g. game-narrative `characters`) are
-- fileable; pure structural shelves (e.g. game-narrative `project`)
-- are not.
--
-- Backfill: every existing area defaults to fileable=true, then
-- non-misc/_unsorted top-level areas (parent_id IS NULL) are set
-- false. This preserves pre-T2 behavior for software-product
-- projects, where artifacts could only land in sub-areas plus the
-- misc/_unsorted top-level areas.

ALTER TABLE areas ADD COLUMN fileable BOOLEAN NOT NULL DEFAULT true;

UPDATE areas SET fileable = false
 WHERE parent_id IS NULL
   AND slug NOT IN ('misc', '_unsorted');

-- +goose Down
ALTER TABLE areas DROP COLUMN IF EXISTS fileable;
