-- +goose Up
-- Phase 19 (revision shapes) foundation — "legal zero 제거".
--
-- 1. artifact_revisions gains revision_shape + shape_payload so later
--    phases (MetaPatch, AcceptanceTransition, ScopeDefer) can land typed
--    revisions without re-encoding body_markdown. Existing rows default
--    to 'body_patch' — the only shape that existed pre-0017.
--
-- 2. body_markdown becomes nullable. MetaPatch revisions will reference
--    the previous revision's body by leaving body_markdown NULL; the
--    read path keeps using artifacts.body_markdown as head so the
--    reader loop is unaffected.
--
-- 3. Backfill revision 1 for any artifact that's missing revisions.
--    Raw-INSERT seeds (migration 0006 _template_*; any future
--    project_create templates pre-dating the companion code fix in
--    Phase A) left artifacts without an initial revision, which made
--    head() = 0 a legal state. With this backfill every artifact is
--    guaranteed to have revision >= 1, so expected_version = 0 is no
--    longer a valid value — the MCP tool rejects it as
--    FIELD_VALUE_RESERVED.

ALTER TABLE artifact_revisions
    ADD COLUMN revision_shape TEXT NOT NULL DEFAULT 'body_patch'
        CHECK (revision_shape IN ('body_patch', 'meta_patch', 'acceptance_transition', 'scope_defer'));

ALTER TABLE artifact_revisions
    ADD COLUMN shape_payload JSONB;

ALTER TABLE artifact_revisions
    ALTER COLUMN body_markdown DROP NOT NULL;

CREATE INDEX idx_artifact_revisions_shape
    ON artifact_revisions (artifact_id, revision_shape);

-- Mirrors the 0004 backfill: synthesize revision 1 for artifacts that
-- landed via raw INSERT (seed migrations, any pre-fix project_create).
INSERT INTO artifact_revisions (
    artifact_id, revision_number, title, body_markdown, body_hash, tags,
    completeness, author_kind, author_id, author_version,
    source_session_ref, commit_msg, revision_shape, shape_payload, created_at
)
SELECT
    id,
    1,
    title,
    body_markdown,
    encode(sha256(convert_to(body_markdown, 'UTF8')), 'hex'),
    tags,
    completeness,
    author_kind,
    author_id,
    author_version,
    source_session_ref,
    'backfill: rev 1 synthesised by migration 0017 (legal-zero elimination)',
    'body_patch',
    NULL,
    COALESCE(published_at, created_at)
FROM artifacts
WHERE NOT EXISTS (
    SELECT 1 FROM artifact_revisions r WHERE r.artifact_id = artifacts.id
);

-- +goose Down
-- Backfilled revision rows are not removed on down: they carry real body
-- content that subsequent update_of calls may depend on. Operators that
-- need a clean rollback can identify them by
-- commit_msg LIKE 'backfill: rev 1 synthesised by migration 0017%'.
DROP INDEX IF EXISTS idx_artifact_revisions_shape;
ALTER TABLE artifact_revisions ALTER COLUMN body_markdown SET NOT NULL;
ALTER TABLE artifact_revisions DROP COLUMN IF EXISTS shape_payload;
ALTER TABLE artifact_revisions DROP COLUMN IF EXISTS revision_shape;
