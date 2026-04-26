-- +goose Up
-- Standardize audit/history references to users(id). Deleting a user must
-- not delete the artifacts, telemetry, read history, or invitation audit
-- trail they touched; only the user reference is nulled.

ALTER TABLE artifacts
    DROP CONSTRAINT IF EXISTS artifacts_author_user_id_fkey;
ALTER TABLE artifacts
    ADD CONSTRAINT artifacts_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE artifact_scope_edges
    DROP CONSTRAINT IF EXISTS artifact_scope_edges_created_by_user_id_fkey;
ALTER TABLE artifact_scope_edges
    ADD CONSTRAINT artifact_scope_edges_created_by_user_id_fkey
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE mcp_tool_calls
    DROP CONSTRAINT IF EXISTS mcp_tool_calls_user_id_fkey;
ALTER TABLE mcp_tool_calls
    ADD CONSTRAINT mcp_tool_calls_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE project_members
    DROP CONSTRAINT IF EXISTS project_members_invited_by_fkey;
ALTER TABLE project_members
    ADD CONSTRAINT project_members_invited_by_fkey
    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE read_events
    DROP CONSTRAINT IF EXISTS read_events_user_id_fkey;
ALTER TABLE read_events
    ADD CONSTRAINT read_events_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
-- The pre-standardization schema already intended these audit references
-- to be SET NULL. Down reasserts that valid FK shape rather than weakening
-- retention guarantees.

ALTER TABLE read_events
    DROP CONSTRAINT IF EXISTS read_events_user_id_fkey;
ALTER TABLE read_events
    ADD CONSTRAINT read_events_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE project_members
    DROP CONSTRAINT IF EXISTS project_members_invited_by_fkey;
ALTER TABLE project_members
    ADD CONSTRAINT project_members_invited_by_fkey
    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE mcp_tool_calls
    DROP CONSTRAINT IF EXISTS mcp_tool_calls_user_id_fkey;
ALTER TABLE mcp_tool_calls
    ADD CONSTRAINT mcp_tool_calls_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE artifact_scope_edges
    DROP CONSTRAINT IF EXISTS artifact_scope_edges_created_by_user_id_fkey;
ALTER TABLE artifact_scope_edges
    ADD CONSTRAINT artifact_scope_edges_created_by_user_id_fkey
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE artifacts
    DROP CONSTRAINT IF EXISTS artifacts_author_user_id_fkey;
ALTER TABLE artifacts
    ADD CONSTRAINT artifacts_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL;
