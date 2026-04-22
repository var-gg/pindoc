-- +goose Up
-- Phase 11a: code pins + artifact-to-artifact edges + _unsorted quarantine area.
--
-- artifact_pins captures "this artifact is about this code location" so stale
-- detection (future phase) and Referenced Confirmation can surface concrete
-- file/line refs instead of prose. Loose schema on purpose: repo is optional
-- (default "origin"), commit_sha is optional (agents often don't know), only
-- path is required. Line range is a tuple of optional ints; (null, null) = whole
-- file.
--
-- artifact_edges captures typed relations between artifacts that
-- complement but don't duplicate the artifacts.superseded_by column. Kept as
-- a separate table so one artifact can have many edges and we can index by
-- relation type without touching the artifacts row.
--
-- _unsorted area is seeded per project as a quarantine queue: propose can
-- target area_slug='_unsorted' when the agent can't classify. The Reader UI
-- lists these with a "needs reclassification" widget. Distinct from 'misc'
-- which is an intentional catchall for genuinely-cross-cutting notes.

CREATE TABLE artifact_pins (
    id           BIGSERIAL PRIMARY KEY,
    artifact_id  UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    repo         TEXT NOT NULL DEFAULT 'origin',
    commit_sha   TEXT,
    path         TEXT NOT NULL,
    lines_start  INT,
    lines_end    INT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (path <> ''),
    CHECK (lines_start IS NULL OR lines_start >= 1),
    CHECK (lines_end IS NULL OR lines_end >= 1),
    CHECK (lines_start IS NULL OR lines_end IS NULL OR lines_end >= lines_start)
);

CREATE INDEX idx_artifact_pins_artifact ON artifact_pins(artifact_id);
CREATE INDEX idx_artifact_pins_path     ON artifact_pins(path);

CREATE TABLE artifact_edges (
    id          BIGSERIAL PRIMARY KEY,
    source_id   UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    target_id   UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    relation    TEXT NOT NULL CHECK (relation IN (
        'implements',
        'references',
        'blocks',
        'relates_to'
    )),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (source_id <> target_id),
    UNIQUE (source_id, target_id, relation)
);

CREATE INDEX idx_artifact_edges_source   ON artifact_edges(source_id);
CREATE INDEX idx_artifact_edges_target   ON artifact_edges(target_id);
CREATE INDEX idx_artifact_edges_relation ON artifact_edges(relation);

-- _unsorted area for every existing project. Kept idempotent so re-running
-- is safe. pindoc.project.create tool inserts this alongside 'misc' for
-- new projects going forward.
INSERT INTO areas (project_id, slug, name, description, is_cross_cutting)
SELECT
    p.id,
    '_unsorted',
    '_Unsorted',
    'Quarantine queue — artifacts the agent couldn''t classify. Reader UI surfaces them for reclassification. Distinct from ''misc'' which is intentional.',
    FALSE
FROM projects p
ON CONFLICT (project_id, slug) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS artifact_edges;
DROP TABLE IF EXISTS artifact_pins;
DELETE FROM areas WHERE slug = '_unsorted';
