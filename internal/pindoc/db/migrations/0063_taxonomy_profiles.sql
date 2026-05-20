-- +goose Up
-- Decision area-taxonomy-profiled-skeleton: every project records which
-- taxonomy profile seeded its area skeleton. Pre-existing projects were
-- all created under the universal 8-concern skeleton, which the amend
-- decision renamed to the `software-product` profile — so the column
-- DEFAULT backfills every existing row to that profile and version.
--
-- The profile registry itself lives in Go (internal/pindoc/projects
-- areas.go, TaxonomyProfiles); this migration only records the per-
-- project pin. project.create writes the slug/version explicitly, so
-- the DEFAULT only ever applies to the backfill of existing rows.

ALTER TABLE projects
    ADD COLUMN taxonomy_profile_slug    TEXT NOT NULL DEFAULT 'software-product',
    ADD COLUMN taxonomy_profile_version TEXT NOT NULL DEFAULT 'reform-v1';

-- +goose Down
ALTER TABLE projects
    DROP COLUMN IF EXISTS taxonomy_profile_slug,
    DROP COLUMN IF EXISTS taxonomy_profile_version;
