-- +goose Up
-- Project repository references for workspace detection.
--
-- git_remote_url stores the canonical lookup key
-- (example: github.com/var-gg/pindoc). git_remote_url_original preserves
-- the operator/agent-provided remote string for audit/debugging.

CREATE TABLE project_repos (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id              UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    git_remote_url          TEXT NOT NULL,
    git_remote_url_original TEXT NOT NULL DEFAULT '',
    name                    TEXT,
    default_branch          TEXT NOT NULL DEFAULT 'main',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, git_remote_url)
);

CREATE INDEX idx_project_repos_git_remote_url
    ON project_repos(git_remote_url);

-- +goose Down

DROP INDEX IF EXISTS idx_project_repos_git_remote_url;
DROP TABLE IF EXISTS project_repos;
