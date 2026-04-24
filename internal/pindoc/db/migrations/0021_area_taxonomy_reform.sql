-- +goose Up
-- Area taxonomy reform (D0 Path A).
--
-- Goal:
--   * Seed the fixed 8 top-level concern skeleton for every existing project:
--     strategy, context, experience, system, operations, governance,
--     cross-cutting, misc.
--   * Move legacy top-level areas into the new hierarchy.
--   * Move legacy decisions-area artifacts to _unsorted for T2c subject-area
--     fine tuning.
--
-- Rollback plan:
--   Keep a DB backup for 7 days after this migration lands. A best-effort
--   reverse migration can recreate the old 9 top-level slugs and move whole
--   subtrees back before T2c runs. After T2c manually rehomes decisions from
--   _unsorted to subject areas, rollback should restore from backup rather
--   than infer intent from current area_id alone.

INSERT INTO areas (project_id, slug, name, description, is_cross_cutting)
SELECT p.id, v.slug, v.name, v.description, v.is_cross_cutting
FROM projects p
CROSS JOIN (VALUES
    ('strategy',      'Strategy',      'Why this exists: vision, goals, scope, hypotheses, roadmap.', FALSE),
    ('context',       'Context',       'External facts: users, competitors, literature, standards, external APIs.', FALSE),
    ('experience',    'Experience',    'What external actors see and do: UI, flows, IA, content, developer experience.', FALSE),
    ('system',        'System',        'How it works internally: architecture, data, API, integrations, mechanisms, MCP, embedding.', FALSE),
    ('operations',    'Operations',    'How it ships, runs, and is supported: delivery, release, launch, incidents, editorial ops.', FALSE),
    ('governance',    'Governance',    'Rules, ownership, compliance, review, and taxonomy policy.', FALSE),
    ('cross-cutting', 'Cross-cutting', 'Reusable named concerns spanning multiple areas: security, privacy, accessibility, reliability, observability, localization.', TRUE),
    ('misc',          'Misc',          'Temporary overflow when no better subject area is clear.', FALSE),
    ('_unsorted',     '_Unsorted',     'Quarantine queue for artifacts that need reclassification.', FALSE)
) AS v(slug, name, description, is_cross_cutting)
ON CONFLICT (project_id, slug) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    is_cross_cutting = EXCLUDED.is_cross_cutting,
    parent_id = NULL,
    updated_at = now();

-- New hierarchy targets needed for the legacy artifact remap. T2b seeds the
-- broader starter catalog; this migration only creates the sub-areas required
-- to preserve existing artifact placement.
INSERT INTO areas (project_id, parent_id, slug, name, description, is_cross_cutting)
SELECT p.id, parent.id, v.slug, v.name, v.description, FALSE
FROM projects p
CROSS JOIN (VALUES
    ('system',     'architecture', 'Architecture', 'System architecture and internal boundaries.'),
    ('system',     'data',         'Data',         'Schema, data model, migrations, and data contracts.'),
    ('system',     'mechanisms',   'Mechanisms',   'Internal mechanisms and runtime behavior.'),
    ('system',     'mcp',          'MCP',          'MCP tool contract and runtime surface.'),
    ('system',     'embedding',    'Embedding',    'Vector provider, chunking, dimensions, and retrieval substrate.'),
    ('experience', 'ui',           'UI',           'Reader UI, Inbox, Graph, Cmd+K, onboarding, and interaction design.'),
    ('strategy',   'roadmap',      'Roadmap',      'V1/V1.x/V2 plan, business model, and launch criteria.')
) AS v(parent_slug, slug, name, description)
JOIN areas parent ON parent.project_id = p.id AND parent.slug = v.parent_slug
ON CONFLICT (project_id, slug) DO UPDATE
SET parent_id = EXCLUDED.parent_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    is_cross_cutting = FALSE,
    updated_at = now();

-- Old top-level vision is folded into top-level strategy for now. T2b may
-- create strategy/vision for future artifacts, but existing vision artifacts
-- land at strategy per D0/T2a.
WITH pairs AS (
    SELECT old.id AS old_id, target.id AS target_id
    FROM areas old
    JOIN areas target ON target.project_id = old.project_id AND target.slug = 'strategy'
    WHERE old.slug = 'vision'
)
UPDATE artifacts a
SET area_id = pairs.target_id,
    updated_at = now()
FROM pairs
WHERE a.area_id = pairs.old_id;

DELETE FROM areas old
USING areas target
WHERE old.project_id = target.project_id
  AND old.slug = 'vision'
  AND target.slug = 'strategy';

-- Old data-model, mcp-surface, and embedding-layer slugs are replaced by the
-- new child slugs data, mcp, and embedding under system.
WITH pairs AS (
    SELECT old.id AS old_id, target.id AS target_id
    FROM areas old
    JOIN areas target ON target.project_id = old.project_id AND target.slug = 'data'
    WHERE old.slug = 'data-model'
)
UPDATE artifacts a
SET area_id = pairs.target_id,
    updated_at = now()
FROM pairs
WHERE a.area_id = pairs.old_id;

DELETE FROM areas old
USING areas target
WHERE old.project_id = target.project_id
  AND old.slug = 'data-model'
  AND target.slug = 'data';

WITH pairs AS (
    SELECT old.id AS old_id, target.id AS target_id
    FROM areas old
    JOIN areas target ON target.project_id = old.project_id AND target.slug = 'mcp'
    WHERE old.slug = 'mcp-surface'
)
UPDATE artifacts a
SET area_id = pairs.target_id,
    updated_at = now()
FROM pairs
WHERE a.area_id = pairs.old_id;

DELETE FROM areas old
USING areas target
WHERE old.project_id = target.project_id
  AND old.slug = 'mcp-surface'
  AND target.slug = 'mcp';

WITH pairs AS (
    SELECT old.id AS old_id, target.id AS target_id
    FROM areas old
    JOIN areas target ON target.project_id = old.project_id AND target.slug = 'embedding'
    WHERE old.slug = 'embedding-layer'
)
UPDATE artifacts a
SET area_id = pairs.target_id,
    updated_at = now()
FROM pairs
WHERE a.area_id = pairs.old_id;

DELETE FROM areas old
USING areas target
WHERE old.project_id = target.project_id
  AND old.slug = 'embedding-layer'
  AND target.slug = 'embedding';

-- Decisions are a Type, not an Area. Keep the artifacts visible in the
-- quarantine queue until T2c assigns each one to its subject area.
WITH pairs AS (
    SELECT old.id AS old_id, target.id AS target_id
    FROM areas old
    JOIN areas target ON target.project_id = old.project_id AND target.slug = '_unsorted'
    WHERE old.slug = 'decisions'
)
UPDATE artifacts a
SET area_id = pairs.target_id,
    updated_at = now()
FROM pairs
WHERE a.area_id = pairs.old_id;

DELETE FROM areas old
USING areas target
WHERE old.project_id = target.project_id
  AND old.slug = 'decisions'
  AND target.slug = '_unsorted';

-- Keep old same-slug areas but move them below their new parents.
UPDATE areas child
SET parent_id = parent.id,
    name = 'Architecture',
    description = 'System architecture and internal boundaries.',
    is_cross_cutting = FALSE,
    updated_at = now()
FROM areas parent
WHERE child.project_id = parent.project_id
  AND child.slug = 'architecture'
  AND parent.slug = 'system';

UPDATE areas child
SET parent_id = parent.id,
    name = 'Mechanisms',
    description = 'Internal mechanisms and runtime behavior.',
    is_cross_cutting = FALSE,
    updated_at = now()
FROM areas parent
WHERE child.project_id = parent.project_id
  AND child.slug = 'mechanisms'
  AND parent.slug = 'system';

UPDATE areas child
SET parent_id = parent.id,
    name = 'UI',
    description = 'Reader UI, Inbox, Graph, Cmd+K, onboarding, and interaction design.',
    is_cross_cutting = FALSE,
    updated_at = now()
FROM areas parent
WHERE child.project_id = parent.project_id
  AND child.slug = 'ui'
  AND parent.slug = 'experience';

UPDATE areas child
SET parent_id = parent.id,
    name = 'Roadmap',
    description = 'V1/V1.x/V2 plan, business model, and launch criteria.',
    is_cross_cutting = FALSE,
    updated_at = now()
FROM areas parent
WHERE child.project_id = parent.project_id
  AND child.slug = 'roadmap'
  AND parent.slug = 'strategy';

-- +goose Down
-- No automatic rollback. See the 7-day backup plan in the Up comment.
