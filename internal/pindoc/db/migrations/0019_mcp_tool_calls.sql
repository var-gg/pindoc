-- +goose Up
-- Phase J MCP tool-call telemetry.
--
-- Every MCP tool call lands here asynchronously (fire-and-forget channel
-- drained by a background flusher — see internal/pindoc/telemetry) so
-- agents see zero latency cost. The table is ops telemetry, not a domain
-- event stream — deliberately separate from `events` so domain queries
-- ("what happened to this artifact") stay clean while ops queries
-- ("which tool is bleeding tokens") have their own surface.
--
-- Why we store approximate token counts alongside byte/char counts:
-- Claude's tokenizer isn't public. BPE approximation via tiktoken
-- cl100k_base is within ±20% for CJK and ±5% for English — enough to
-- surface "this tool is a token hog" trends without claiming exact
-- parity with Anthropic's billing.

CREATE TABLE mcp_tool_calls (
    id                 BIGSERIAL PRIMARY KEY,
    started_at         TIMESTAMPTZ NOT NULL,
    duration_ms        INT NOT NULL,
    tool_name          TEXT NOT NULL,
    agent_id           TEXT,
    user_id            UUID REFERENCES users(id) ON DELETE SET NULL,
    project_slug       TEXT,
    input_bytes        INT NOT NULL,
    output_bytes       INT NOT NULL,
    input_chars        INT NOT NULL,
    output_chars       INT NOT NULL,
    input_tokens_est   INT NOT NULL,
    output_tokens_est  INT NOT NULL,
    error_code         TEXT,
    toolset_version    TEXT
);

CREATE INDEX idx_tool_calls_started ON mcp_tool_calls (started_at DESC);
CREATE INDEX idx_tool_calls_tool    ON mcp_tool_calls (tool_name, started_at DESC);
CREATE INDEX idx_tool_calls_agent   ON mcp_tool_calls (agent_id, started_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_tool_calls_agent;
DROP INDEX IF EXISTS idx_tool_calls_tool;
DROP INDEX IF EXISTS idx_tool_calls_started;
DROP TABLE IF EXISTS mcp_tool_calls;
