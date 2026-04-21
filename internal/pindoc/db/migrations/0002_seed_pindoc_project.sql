-- +goose Up
-- Seed the Pindoc project itself. Idempotent via ON CONFLICT DO NOTHING.

INSERT INTO projects (slug, name, description, color, primary_language) VALUES
    ('pindoc', 'Pindoc',
     'The wiki you never type into. Agent-writable project wiki. This is the meta-dogfood project.',
     '#7c5cff', 'ko')
ON CONFLICT (slug) DO NOTHING;

WITH p AS (SELECT id FROM projects WHERE slug = 'pindoc')
INSERT INTO areas (project_id, slug, name, description, is_cross_cutting)
SELECT p.id, slug, name, description, is_cross_cutting FROM p, (VALUES
    ('vision',        'Vision',        'Product north star, principles, positioning', FALSE),
    ('architecture',  'Architecture',  'System design, MCP layer, embedding layer, deployment', FALSE),
    ('data-model',    'Data model',    'Schema, Tier A/B, Area, Pin/ResourceRef, 3-axis state', FALSE),
    ('mechanisms',    'Mechanisms',    'M0-M7 — Harness Reversal, Pre-flight, Fast Landing', FALSE),
    ('ui',            'UI',            'Wiki Reader, Inbox, Graph, Cmd+K, Onboarding', FALSE),
    ('roadmap',       'Roadmap',       'V1/V1.x/V2, BM, pindoc.org', FALSE),
    ('decisions',     'Decisions',     'Resolved + open questions', FALSE),
    ('misc',          'Misc',          'Uncategorized', FALSE),
    ('cross-cutting', 'Cross-cutting', 'Observability, security, i18n spanning Areas', TRUE)
) AS a(slug, name, description, is_cross_cutting)
ON CONFLICT (project_id, slug) DO NOTHING;

-- +goose Down
DELETE FROM areas    WHERE project_id = (SELECT id FROM projects WHERE slug = 'pindoc');
DELETE FROM projects WHERE slug = 'pindoc';
