-- +goose Up
-- Phase 15c: artifact_pins.kind enum for non-code references.
--
-- The original Phase 11a schema assumed every pin points at a code path
-- with commit+line range. Real dogfood immediately produced artifacts
-- (AWS infrastructure snapshots, architecture analyses, spec references)
-- where the useful anchor is an infra resource id or a canonical external
-- URL, not a file path.
--
-- Rather than overloading the `path` column with "aws://vpc-0c6b..."
-- strings we split the pin kinds. `code` keeps the existing repo+
-- commit_sha+path+line-range semantics. `resource` uses path to hold a
-- typed resource ref ("aws:vpc:vpc-0c6bff25", "gcp:cloudrun:my-svc").
-- `url` uses path as the URL string. Existing rows default to `code` —
-- their paths are already file paths, so nothing needs backfilling.

ALTER TABLE artifact_pins
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'code'
    CHECK (kind IN ('code', 'resource', 'url'));

CREATE INDEX idx_artifact_pins_kind ON artifact_pins(kind);

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_pins_kind;
ALTER TABLE artifact_pins DROP COLUMN IF EXISTS kind;
