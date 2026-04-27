-- +goose Up
-- Per-tool result attributes for mcp_tool_calls.
--
-- The base schema (migration 0019) tracks bytes / tokens / error codes —
-- enough for "is this tool bleeding tokens?" trends but blind to the
-- per-tool semantics. Decision mcp-dx-외부-리뷰-codex-1차-피드백-6항목
-- 발견 4 calls for a JSONB metadata column so workspace.detect's `via`
-- priority chain, area.list's include_templates flag, propose's shape /
-- type bucket, and search's top_k / hits_count become first-class
-- queryable dimensions without a per-tool side table.
--
-- Default '{}'::jsonb so existing tooling that SELECTs the column gets a
-- uniform shape regardless of whether a per-tool extractor ran. NOT NULL
-- so JSONB queries don't need to coalesce on every read.

ALTER TABLE mcp_tool_calls
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Optional GIN index for "find all rows with metadata.via='pindoc_md_path'"
-- style queries. Cheap on this table because tool calls are append-only
-- and the metadata payload is tiny per row.
CREATE INDEX idx_tool_calls_metadata_gin
    ON mcp_tool_calls USING GIN (metadata jsonb_path_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_tool_calls_metadata_gin;
ALTER TABLE mcp_tool_calls DROP COLUMN IF EXISTS metadata;
