-- +goose Up
-- Runtime MCP server state that should survive daemon restarts but is not
-- operator-editable configuration. The first key tracks the last toolset
-- version observed by this installation so a restarted server can emit a
-- tools/list_changed notification only when the catalog contract changed.

CREATE TABLE mcp_runtime_state (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS mcp_runtime_state;
