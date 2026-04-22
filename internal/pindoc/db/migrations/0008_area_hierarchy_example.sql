-- +goose Up
-- Phase 15a: minimal demonstration of Area hierarchy.
--
-- DB schema has always had areas.parent_id. UI rendered flat because no
-- seed exercised the tree. This migration adds two sub-areas under
-- `architecture` so the Reader Sidebar's recursive tree renderer has real
-- data to show in dogfood. Real sub-area creation happens per-project via
-- operator CLI (future) or direct INSERT — this seed is illustrative, not
-- load-bearing.

WITH p AS (SELECT id FROM projects WHERE slug = 'pindoc'),
     parent_a AS (SELECT id FROM areas WHERE project_id = (SELECT id FROM p) AND slug = 'architecture')
INSERT INTO areas (project_id, parent_id, slug, name, description, is_cross_cutting)
SELECT
    (SELECT id FROM p),
    (SELECT id FROM parent_a),
    v.slug,
    v.name,
    v.description,
    FALSE
FROM (VALUES
    ('embedding-layer', 'Embedding layer',    'Vector provider, chunking, dimension management. Sub-area of Architecture.'),
    ('mcp-surface',     'MCP surface',        'MCP tool contract and runtime shape. Sub-area of Architecture.')
) AS v(slug, name, description)
ON CONFLICT (project_id, slug) DO NOTHING;

-- +goose Down
DELETE FROM areas
WHERE project_id = (SELECT id FROM projects WHERE slug = 'pindoc')
  AND slug IN ('embedding-layer', 'mcp-surface');
