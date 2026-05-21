-- +goose Up
-- Decision taxonomy-change-operation T9: a top-level area can now be born
-- after project creation — through a top_level.add or profile.adopt
-- change-set, not only the project-create seed. origin_profile_slug
-- records which profile the area's spec came from; origin_change_id ties a
-- runtime-added area to the taxonomy_changes row that created it. Both are
-- NULL for areas seeded at project creation and for every pre-existing
-- area, so the columns are pure audit metadata with no behavior change.

ALTER TABLE areas
    ADD COLUMN origin_profile_slug TEXT,
    ADD COLUMN origin_change_id UUID
        REFERENCES taxonomy_changes(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE areas
    DROP COLUMN IF EXISTS origin_change_id,
    DROP COLUMN IF EXISTS origin_profile_slug;
