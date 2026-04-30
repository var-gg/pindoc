-- +goose Up
-- Seed the optional SessionHandoff convention template for existing projects.
-- New projects receive the same template through projects.TemplateSeeds.

WITH misc AS (
    SELECT p.id AS project_id, a.id AS area_id
      FROM projects p
      JOIN areas a ON a.project_id = p.id AND a.slug = 'misc'
),
inserted AS (
    INSERT INTO artifacts (
        project_id, area_id, slug, type, title, body_markdown, tags,
        completeness, status, review_state,
        author_kind, author_id, author_version, published_at
    )
    SELECT
        m.project_id, m.area_id,
        '_template_session_handoff',
        'Flow',
        'Template — Session handoff artifact',
        $$<!-- validator: required_h2=Current task,Completed work,Pending checks,Evidence,Next MCP calls; required_keywords=handoff,task,next -->
> **This artifact is a template.** Read before creating a session handoff.
> SessionHandoff is a convention, not a new artifact type; create it as a
> Flow artifact and link it to the active Task with `relates_to`.

## Current task

Name the active Task slug, assignee, and why the session is being handed off.
Keep this to one short paragraph so a continuation agent can identify the
work without reading chat history.

## Completed work

Summarize what changed or what artifact decisions were published. Include
commit SHA, artifact slugs, or branch names when available, but keep durable
coordinates in `pins[]` or `relates_to` rather than only prose.

## Pending checks

List the checks that still need to run or the blockers that prevented
completion. Use checklist items only when each item can be independently
closed by the next agent.

- [ ] pending check or blocker

## Evidence

Name evidence artifacts, verification receipts, relevant pins, or manual QA
notes that the next agent should inspect before proceeding.

## Next MCP calls

State the exact next Pindoc MCP calls expected, such as
`pindoc.context.for_task`, `pindoc.artifact.read(view="continuation")`,
`pindoc.task.queue`, or `pindoc.task.done_check`. This section is the durable
replacement for implicit chat-memory handoff.
$$,
        ARRAY['_template'],
        'partial', 'published', 'auto_published',
        'system', 'pindoc-seed', '0.0.1', now()
    FROM misc m
    WHERE NOT EXISTS (
        SELECT 1
          FROM artifacts a
         WHERE a.project_id = m.project_id
           AND a.slug = '_template_session_handoff'
    )
    RETURNING id, title, body_markdown, tags, completeness, author_kind, author_id, author_version
)
INSERT INTO artifact_revisions (
    artifact_id, revision_number, title, body_markdown, body_hash,
    tags, completeness, author_kind, author_id, author_version,
    commit_msg, revision_shape
)
SELECT
    id, 1, title, body_markdown, encode(digest(body_markdown, 'sha256'), 'hex'),
    tags, completeness, author_kind, author_id, author_version,
    'seed: session handoff template', 'body_patch'
FROM inserted;

-- +goose Down

DELETE FROM artifact_revisions
 WHERE artifact_id IN (
       SELECT id FROM artifacts WHERE slug = '_template_session_handoff'
 );
DELETE FROM artifacts
 WHERE slug = '_template_session_handoff'
   AND author_kind = 'system'
   AND author_id = 'pindoc-seed';
