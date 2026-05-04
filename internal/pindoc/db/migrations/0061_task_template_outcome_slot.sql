-- +goose Up
-- Add the Outcome slot to existing _template_task artifacts without
-- clobbering local prose edits. New projects receive the same slot from
-- internal/pindoc/projects/templates.go.

WITH target AS (
    SELECT
        a.id,
        a.title,
        a.body_markdown,
        a.tags,
        a.completeness,
        COALESCE((
            SELECT MAX(r.revision_number)
              FROM artifact_revisions r
             WHERE r.artifact_id = a.id
        ), 0) AS head_rev
      FROM artifacts a
     WHERE a.slug = '_template_task'
       AND a.status <> 'archived'
),
validator_patched AS (
    SELECT
        t.*,
        CASE
            WHEN t.body_markdown ~* $re$<!--\s*validator:[^>]*required_h2=[^;>]*\bOutcome\b$re$ THEN t.body_markdown
            WHEN t.body_markdown ~* $re$<!--\s*validator:[^>]*required_h2=$re$ THEN
                regexp_replace(
                    t.body_markdown,
                    $re$(<!--\s*validator:\s*required_h2=)([^;>]*)(;)$re$,
                    $rep$\1\2,Outcome\3$rep$
                )
            ELSE
                '<!-- validator: required_h2=목적,범위,코드 좌표,TODO,TC / DoD,Outcome; required_keywords=acceptance -->' || E'\n' || t.body_markdown
        END AS validator_body
      FROM target t
),
patched AS (
    SELECT
        v.*,
        CASE
            WHEN v.validator_body ~* $re$(?m)^##\s*(Outcome|결과|완료 결과|산출)(\s|/|\(|$)$re$ THEN v.validator_body
            WHEN v.validator_body ~ $re$(?m)^## Open issues / 남은 질문$re$ THEN
                regexp_replace(
                    v.validator_body,
                    E'\n## Open issues / 남은 질문',
                    E'\n## Outcome\n\n완료 시점에는 핵심 결과, 코드 변경 evidence, 회귀 진술을 한 문단으로 기록한다. 코드 변경 evidence는 commit hash 또는 PR URL이어야 하고, 회귀 진술은 실행한 자동 TC와 기존 동작 호환성을 함께 남긴다. 구현 전에는 이 섹션을 비워두기보다 어떤 evidence를 채워야 claim_done이 가능한지 짧게 남겨둔다.\n\n## Open issues / 남은 질문'
                )
            ELSE
                v.validator_body || E'\n\n## Outcome\n\n완료 시점에는 핵심 결과, 코드 변경 evidence, 회귀 진술을 한 문단으로 기록한다. 코드 변경 evidence는 commit hash 또는 PR URL이어야 하고, 회귀 진술은 실행한 자동 TC와 기존 동작 호환성을 함께 남긴다. 구현 전에는 이 섹션을 비워두기보다 어떤 evidence를 채워야 claim_done이 가능한지 짧게 남겨둔다.'
        END AS next_body
      FROM validator_patched v
),
changed AS (
    SELECT *
      FROM patched
     WHERE next_body IS DISTINCT FROM body_markdown
),
updated AS (
    UPDATE artifacts a
       SET body_markdown  = c.next_body,
           author_id      = 'pindoc-migration',
           author_version = '0061',
           updated_at     = now()
      FROM changed c
     WHERE a.id = c.id
    RETURNING a.id, a.title, a.body_markdown, a.tags, a.completeness
)
INSERT INTO artifact_revisions (
    artifact_id, revision_number, title, body_markdown, body_hash,
    tags, completeness, author_kind, author_id, author_version,
    commit_msg, revision_shape
)
SELECT
    u.id,
    c.head_rev + 1,
    u.title,
    u.body_markdown,
    encode(digest(u.body_markdown, 'sha256'), 'hex'),
    u.tags,
    u.completeness,
    'system',
    'pindoc-migration',
    '0061',
    'migration 0061: add Outcome slot to _template_task',
    'body_patch'
  FROM updated u
  JOIN changed c ON c.id = u.id
 WHERE NOT EXISTS (
       SELECT 1
         FROM artifact_revisions r
        WHERE r.artifact_id = u.id
          AND r.commit_msg = 'migration 0061: add Outcome slot to _template_task'
 );

-- +goose Down
-- Irreversible content migration; existing local template edits are preserved
-- on rollback rather than attempting to delete a semantic section.
SELECT 1;
