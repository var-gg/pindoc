-- +goose Up
-- Phase F scope-defer graph edges.
--
-- Records "acceptance item on artifact A was moved to artifact B with
-- reason R" as a queryable edge so downstream tooling (pindoc.scope.
-- in_flight, Reader UI) can show the disposition of every acceptance
-- item without grep-walking bodies. The complementary revision row with
-- revision_shape='scope_defer' + shape_payload carries the ephemeral
-- state-change record; edges are the normalised join table the
-- "in-flight work" query hits.
--
-- Uniqueness: (from_artifact, from_item_ref, to_artifact) triple. A
-- single acceptance item can be re-routed to multiple successors over
-- time (each row) but the same triple can't be recorded twice.
-- from_item_ref is the agent-facing locator ("acceptance[2]") so the
-- edge survives body edits that renumber subsequent checkboxes — the
-- Reader resolves the locator against the current body at query time.

CREATE TABLE artifact_scope_edges (
    id                 BIGSERIAL PRIMARY KEY,
    from_artifact_id   UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    from_item_ref      TEXT NOT NULL,
    to_artifact_id     UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    reason             TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by_agent   TEXT,
    UNIQUE (from_artifact_id, from_item_ref, to_artifact_id)
);

CREATE INDEX idx_scope_edges_from ON artifact_scope_edges (from_artifact_id);
CREATE INDEX idx_scope_edges_to ON artifact_scope_edges (to_artifact_id);
CREATE INDEX idx_scope_edges_created ON artifact_scope_edges (created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_scope_edges_created;
DROP INDEX IF EXISTS idx_scope_edges_to;
DROP INDEX IF EXISTS idx_scope_edges_from;
DROP TABLE IF EXISTS artifact_scope_edges;
