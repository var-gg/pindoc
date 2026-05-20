-- +goose Up
-- Decision area-taxonomy-profiled-skeleton T5: a per-top-level
-- max_depth replaces the hardcoded depth-1 cap that area.create and
-- artifact.set_area enforced in Go. Most top-level areas keep depth-1
-- sub-areas (max_depth = 1); a high-cardinality domain (e.g. the
-- game-narrative characters / atlas / narrative / combat areas) may
-- allow depth-2 sub-areas (max_depth = 2).
--
-- max_depth is meaningful on top-level rows (parent_id IS NULL); a
-- sub-area inherits its root top-level's policy. The column DEFAULT 1
-- backfills every existing area to the prior depth-1 behavior, so
-- software-product projects are unchanged.

ALTER TABLE areas ADD COLUMN max_depth SMALLINT NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE areas DROP COLUMN IF EXISTS max_depth;
