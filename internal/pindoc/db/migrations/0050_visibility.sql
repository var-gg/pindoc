-- +goose Up
-- Visibility: cross-Org/external exposure tier on projects and artifacts.
--
-- Pindoc dogfoods itself by exposing the owner's wiki publicly while
-- keeping owner-only strategic content (SaaS roadmap, pricing memos,
-- hiring notes) hidden in the same Project. Visibility is the per-row
-- attribute that drives that filter — distinct from the existing
-- artifact_meta.audience field, which scopes within-project workflow
-- access (reviewer / approver gates), not external exposure.
--
-- Tiers:
--   'public'  — accessible to anyone, including unauthenticated visitors
--               at /pindoc.org/{org}/p/{proj}/...
--   'org'     — only members of the owning Organization see it. Default.
--   'private' — only the artifact's author + explicit ACL members. Future
--               ACL table is a separate migration; for now 'private'
--               means "creator only" which suffices for the solo-dogfood
--               + SaaS-strategy-doc scenario.
--
-- Default 'org' is the safe-by-default choice: nothing leaks to public
-- viewers without an explicit opt-in. Existing dogfood content stays
-- visible to the bootstrap user (as Org member) but is not exposed
-- until the owner bulk-marks it 'public' for OSS publishing.
--
-- Listing queries filter on visibility based on caller role:
--   - unauthenticated viewer at /{org}/p/{proj}: WHERE visibility='public'
--   - Org member: WHERE visibility IN ('public','org')
--   - artifact author: all (public/org/private of own)

ALTER TABLE projects
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'org'
        CHECK (visibility IN ('public', 'org', 'private'));

ALTER TABLE artifacts
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'org'
        CHECK (visibility IN ('public', 'org', 'private'));

CREATE INDEX idx_projects_visibility ON projects(visibility);
CREATE INDEX idx_artifacts_visibility ON artifacts(visibility);


-- +goose Down
DROP INDEX IF EXISTS idx_artifacts_visibility;
DROP INDEX IF EXISTS idx_projects_visibility;
ALTER TABLE artifacts DROP COLUMN IF EXISTS visibility;
ALTER TABLE projects DROP COLUMN IF EXISTS visibility;
