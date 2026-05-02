-- +goose Up
-- Asset v1 stores immutable blobs separately from artifact markdown.
-- artifact_assets links a blob to one concrete artifact revision so old
-- revisions keep pointing at the file they were authored with.

CREATE TABLE assets (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sha256            TEXT NOT NULL,
    mime_type         TEXT NOT NULL,
    size_bytes        BIGINT NOT NULL,
    original_filename TEXT NOT NULL DEFAULT '',
    storage_driver    TEXT NOT NULL,
    storage_key       TEXT NOT NULL,
    created_by        TEXT NOT NULL DEFAULT '',
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CHECK (mime_type <> ''),
    CHECK (size_bytes >= 0),
    CHECK (storage_driver IN ('localfs')),
    CHECK (storage_key <> ''),
    UNIQUE (project_id, sha256)
);

CREATE INDEX idx_assets_project_created ON assets(project_id, created_at DESC);
CREATE INDEX idx_assets_project_mime ON assets(project_id, mime_type);

CREATE TABLE artifact_assets (
    id                   BIGSERIAL PRIMARY KEY,
    artifact_id          UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    artifact_revision_id UUID NOT NULL REFERENCES artifact_revisions(id) ON DELETE CASCADE,
    asset_id             UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    role                 TEXT NOT NULL CHECK (role IN ('inline_image', 'attachment', 'evidence', 'generated_output')),
    display_order        INT NOT NULL DEFAULT 0,
    created_by           TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (display_order >= 0),
    UNIQUE (artifact_revision_id, asset_id, role)
);

CREATE INDEX idx_artifact_assets_artifact ON artifact_assets(artifact_id);
CREATE INDEX idx_artifact_assets_revision ON artifact_assets(artifact_revision_id);
CREATE INDEX idx_artifact_assets_asset ON artifact_assets(asset_id);
CREATE INDEX idx_artifact_assets_role ON artifact_assets(role);

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_assets_role;
DROP INDEX IF EXISTS idx_artifact_assets_asset;
DROP INDEX IF EXISTS idx_artifact_assets_revision;
DROP INDEX IF EXISTS idx_artifact_assets_artifact;
DROP TABLE IF EXISTS artifact_assets;
DROP INDEX IF EXISTS idx_assets_project_mime;
DROP INDEX IF EXISTS idx_assets_project_created;
DROP TABLE IF EXISTS assets;
