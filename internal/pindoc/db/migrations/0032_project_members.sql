-- +goose Up
-- Project membership foundation for V1.5 auth.
--
-- V1 trusted_local remains permissive at the auth layer, but every
-- project can now carry explicit user membership rows. Existing single
-- user installs get owner rows from boot-time bootstrap once the server
-- knows the env-derived user id; this migration only creates schema.

CREATE TABLE project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_members_user ON project_members(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_project_members_user;
DROP TABLE IF EXISTS project_members;
