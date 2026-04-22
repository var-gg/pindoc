-- +goose Up
-- Phase 13: seed template artifacts for the pindoc project.
--
-- These artifacts are the "current best practice" structure for Debug /
-- Decision / Analysis / Task bodies. Agents read them before proposing a
-- new artifact so format stays consistent across the corpus. They are
-- themselves normal artifacts — update_of revises them, and the revision
-- chain becomes the evolution record for the recommended structure.
--
-- Slug prefix `_template_` is the filter contract: the HTTP API hides
-- them from default lists, Reader UI reveals them behind a toggle, and
-- search treats them like any other artifact (so agents hit them via
-- context.for_task too).
--
-- This migration seeds the templates for the pindoc project. Projects
-- created later (via pindoc.project.create) get the same four seeded
-- by the create tool itself — keep the two sites in sync.

WITH p AS (SELECT id FROM projects WHERE slug = 'pindoc'),
     a AS (SELECT id FROM areas WHERE slug = 'misc' AND project_id = (SELECT id FROM p))
INSERT INTO artifacts (
    project_id, area_id, slug, type, title, body_markdown, tags,
    completeness, status, review_state,
    author_kind, author_id, author_version, published_at
)
SELECT
    p.id, a.id, v.slug, v.type, v.title, v.body, ARRAY['_template'],
    'partial', 'published', 'auto_published',
    'system', 'pindoc-seed', '0.0.1', now()
FROM p, a, (VALUES
    (
        '_template_debug',
        'Debug',
        'Template — Debug artifact',
        $$> **This artifact is a template.** Read it before proposing a new
> `Debug` artifact so the resulting document matches the house structure.
> The template itself is an ordinary artifact — improvements land as
> revisions (`update_of`) on this slug. Do **not** copy-paste this body
> verbatim into a new Debug; rewrite each section with your specific case.

## 증상 (Symptom)

관측된 잘못된 동작. 한두 문장. 재현 빈도, 영향 범위.

예: *"결제 API 호출이 15% 확률로 504 Gateway Timeout. 피크 트래픽 시간대."*

## 재현 (Reproduction)

- [ ] 재현 단계 1 (명령어/요청/입력)
- [ ] 재현 단계 2
- [ ] 관측되는 에러/로그

재현률: `N / N회`. 재현 안 되는 경우 언제 그런지.

## 가설 (Hypotheses tried)

- **가설 1 — 짧은 제목** — 결과: 맞음/틀림 + 한 줄 근거
- **가설 2** — 결과
- **가설 3** — 결과

## 원인 (Root cause)

실제 원인. 어떤 조건에서 왜 이 증상이 나오는지. 코드 라인/로그/측정치로 증명.

## 해결 (Resolution)

실제로 수정한 내용. 어떤 파일의 어떤 부분을 어떻게 고쳤는지. 관련 PR /
commit 링크는 본 artifact의 `pins[]` 로 연결.

## 검증 (Verification)

- [ ] 재현 단계를 다시 돌려 증상이 사라짐 확인
- [ ] regression test 추가 여부

## Open questions / 남은 미스터리

아직 풀리지 않은 부수적 질문. 다음 세션에서 이어받을 실마리.

## 연관

- 이 fix가 이전 Debug artifact를 `supersede_of` 하는지
- `relates_to` 로 엮을 Decision/Feature
$$
    ),
    (
        '_template_decision',
        'Decision',
        'Template — Decision (ADR-style) artifact',
        $$> **This artifact is a template.** Read before proposing a new
> `Decision` artifact. Template evolves via `update_of`.

## Context

결정이 필요한 배경. 현재 상황, 제약, 이해관계자. "왜 지금 결정해야 하는가".

## Decision

실제 결정 사항. 한두 문장으로 단호하게.

예: *"결제 실패 재시도는 최대 3회, exponential backoff (1s, 3s, 9s)로 한다."*

## Rationale

왜 이 결정인가. Alternatives 각각과의 trade-off.

## Alternatives considered

- **Alt A — 짧은 제목**: 장점 / 단점 / 기각 이유
- **Alt B**: 장점 / 단점 / 기각 이유
- **Alt C** (필요 시): …

최소 2개 이상의 Alternatives. 하나는 "아무것도 안 한다 (현상 유지)"도 유효.

## Consequences

이 결정의 단/중/장기 영향. 긍정/부정 모두.

- **단기**: ...
- **중기**: ...
- **장기**: ...

## Open questions

아직 미해결 세부 사항. 이후 별도 Decision으로 쪼갤 지점.

## 연관

- 영향 받는 Feature/Task는 `relates_to` 로 연결
- 기존 Decision을 뒤집는다면 `supersede_of` 사용
$$
    ),
    (
        '_template_analysis',
        'Analysis',
        'Template — Analysis artifact',
        $$> **This artifact is a template.** Read before proposing a new
> `Analysis`. Template evolves via `update_of`.

## TL;DR

한 문장 결론. 바쁜 사람은 이것만 읽으면 의미가 전달되어야 한다.

## 목적 / Scope

- 이 분석이 답하려는 질문
- 건드리는 영역 / 건드리지 않는 영역

## 조사 시점 (Investigation timestamp)

- 날짜: `YYYY-MM-DD`
- 방법: 어떤 도구 / 쿼리 / 코드 snapshot 으로 확인했는지

## 발견 (Findings)

핵심 발견 사항. H3 하위 섹션으로 나눠 구조화.

### 발견 1

### 발견 2

### 발견 3

## 평가 (Interpretation)

발견에 대한 저자 판단. 강점 / 약점 / 위험 / 기회.

## 재조회 방법 (Re-verification)

같은 조사를 나중에 재실행하려면 어떤 명령 / 쿼리를 돌리면 되는지.
`pins[]` 로 관련 script 연결.

## Open questions / 남은 미스터리

- 답을 내지 못한 부수적 질문
- 후속 분석이 필요한 지점

## 연관

- 이 분석에서 파생된 Decision/Task 는 `relates_to` 로 연결
$$
    ),
    (
        '_template_task',
        'Task',
        'Template — Task artifact',
        $$> **This artifact is a template.** Read before proposing a new
> `Task`. Template evolves via `update_of`.

## 목적 / Purpose

이 task가 해결하는 문제. 왜 지금 하는가.

## 범위 / Scope

- 건드릴 영역
- 건드리지 않을 영역 (명시적으로)

## 분석 요약 (Analysis summary)

작업 전 조사 결과 요약. 이미 관련 Analysis/Decision artifact 가 있다면
`relates_to` 로 연결하고 여기서는 한 문단만.

## TODO

- [ ] acceptance criterion 1 (완료를 판별할 수 있는 구체적 체크)
- [ ] acceptance criterion 2
- [ ] ...

각 항목은 **acceptance** 성격이어야 한다. "어떤 상태가 되면 이 체크가 true인가"
가 분명해야 한다.

## 리소스 경로 (Resources)

작업에 걸리는 파일 / 모듈 / 문서 경로. 정확한 경로는 `pins[]` 로 연결.

## TC / DoD

### 자동 TC

- 추가/수정할 자동 테스트 이름
- 기존 통과 범위 유지

### 수동 QA

- 시나리오별 확인 단계

### 완료 기준 (DoD)

TODO가 모두 체크됐고, TC가 통과했고, 추가로 확인해야 할 비기능 요구 (성능,
로그, 모니터링)가 있는지.

## Open issues / 남은 질문

결정이 필요한 사소한 지점. 별도 Decision으로 올릴 지, 이 task 안에서 풀지.

## 연관

- 관련 Task / Decision / Debug 는 `relates_to` 로 연결
- `blocks` / `implements` 관계도 활용
$$
    )
) AS v(slug, type, title, body);

-- +goose Down
DELETE FROM artifacts
WHERE slug IN ('_template_debug', '_template_decision', '_template_analysis', '_template_task')
  AND project_id = (SELECT id FROM projects WHERE slug = 'pindoc');
