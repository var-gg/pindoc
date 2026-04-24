-- +goose Up
-- Starter sub-area seed for the 8 concern skeleton.
--
-- T2a creates only the top-level skeleton and the minimum sub-areas required
-- to preserve existing artifact placement. This migration adds the starter
-- catalog used by agents and the Reader area picker for new artifacts.
--
-- Rollback procedure:
--   goose down removes only these starter catalog rows. If any artifacts have
--   already landed in one of the starter sub-areas, PostgreSQL's FK restrict
--   will block deletion; rehome those artifacts first or restore from backup.

INSERT INTO areas (project_id, parent_id, slug, name, description, is_cross_cutting)
SELECT
    p.id,
    parent.id,
    v.slug,
    v.name,
    v.description,
    (v.parent_slug = 'cross-cutting')
FROM projects p
CROSS JOIN (VALUES
    ('context',       'users',                    'Users',                    'User research, personas, jobs, and needs.'),
    ('context',       'competitors',              'Competitors',              'Competitive analysis and adjacent products.'),
    ('context',       'literature',               'Literature',               'Literature review and external research.'),
    ('context',       'external-apis',            'External APIs',            'Third-party API facts, limits, contracts, and behavior.'),
    ('context',       'standards',                'Standards',                'External standards and protocol references.'),
    ('context',       'glossary',                 'Glossary',                 'Domain vocabulary and terminology context.'),

    ('experience',    'flows',                    'Flows',                    'User, agent, and developer-facing flows.'),
    ('experience',    'information-architecture', 'Information architecture', 'Navigation, hierarchy, and wayfinding.'),
    ('experience',    'content',                  'Content',                  'Reader copy, documentation content, and message structure.'),
    ('experience',    'developer-experience',     'Developer experience',     'Developer-facing setup, guidance, and ergonomics.'),
    ('experience',    'campaigns',                'Campaigns',                'Marketing or launch campaign experience.'),

    ('system',        'api',                      'API',                      'Internal and external API contracts.'),
    ('system',        'integrations',             'Integrations',             'Integration boundaries and adapters.'),

    ('operations',    'delivery',                 'Delivery',                 'Delivery flow and handoff.'),
    ('operations',    'release',                  'Release',                  'Release process and notes.'),
    ('operations',    'launch',                   'Launch',                   'Launch operations and readiness.'),
    ('operations',    'incidents',                'Incidents',                'Incident response and postmortems.'),
    ('operations',    'editorial-ops',            'Editorial ops',            'Documentation and content operations.'),
    ('operations',    'community-ops',            'Community ops',            'Community support and moderation operations.'),

    ('governance',    'policies',                 'Policies',                 'Product and project policies.'),
    ('governance',    'compliance',               'Compliance',               'Compliance requirements and constraints.'),
    ('governance',    'ownership',                'Ownership',                'Ownership, accountability, and review boundaries.'),
    ('governance',    'review',                   'Review',                   'Review rules and approval gates.'),
    ('governance',    'taxonomy-policy',          'Taxonomy policy',          'Area taxonomy and classification governance.'),

    ('cross-cutting', 'security',                 'Security',                 'Security concern spanning multiple areas.'),
    ('cross-cutting', 'privacy',                  'Privacy',                  'Privacy concern spanning multiple areas.'),
    ('cross-cutting', 'accessibility',            'Accessibility',            'Accessibility concern spanning multiple areas.'),
    ('cross-cutting', 'reliability',              'Reliability',              'Reliability concern spanning multiple areas.'),
    ('cross-cutting', 'observability',            'Observability',            'Observability concern spanning multiple areas.'),
    ('cross-cutting', 'localization',             'Localization',             'Localization concern spanning multiple areas.')
) AS v(parent_slug, slug, name, description)
JOIN areas parent ON parent.project_id = p.id AND parent.slug = v.parent_slug
ON CONFLICT (project_id, slug) DO NOTHING;

-- +goose Down
DELETE FROM areas
WHERE slug IN (
    'users', 'competitors', 'literature', 'external-apis', 'standards', 'glossary',
    'flows', 'information-architecture', 'content', 'developer-experience', 'campaigns',
    'api', 'integrations',
    'delivery', 'release', 'launch', 'incidents', 'editorial-ops', 'community-ops',
    'policies', 'compliance', 'ownership', 'review', 'taxonomy-policy',
    'security', 'privacy', 'accessibility', 'reliability', 'observability', 'localization'
);
