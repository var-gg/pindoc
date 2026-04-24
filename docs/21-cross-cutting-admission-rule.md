# 21. Cross-cutting Admission Rule

`cross-cutting`은 여러 Area를 가로지르는 reusable named concern을 위한 shelf다. "여러 곳에 언급된다"만으로는
부족하다. 같은 기준, 점검표, policy, failure mode가 여러 subject area에 반복 적용될 때만 들어온다.

## 1. Admission Criteria

`cross-cutting/*` sub-area로 승격하려면 다음 조건을 모두 만족해야 한다.

- **3개 이상 top-level Area에서 반복**: 예를 들어 `experience`, `system`, `operations`에 모두 같은 concern이
  나타난다.
- **공통 framework 존재**: checklist, policy, rubric, test rule, telemetry definition처럼 재사용 가능한 기준이 있다.
- **단일 primary area로 축소 불가**: 특정 화면, API, 운영 절차 하나의 문제가 아니라 여러 subject에 걸친다.
- **owner 지정 가능**: concern 자체를 관리할 책임자가 있다.
- **primary area + Tag로 부족함**: Tag만으로는 검색/운영/정책 적용이 약해진다는 근거가 있다.

기본 후보는 `security`, `privacy`, `accessibility`, `reliability`, `observability`, `localization`이다.

## 2. Rejection Criteria

다음은 `cross-cutting`에 넣지 않는다.

- 단일 UI 접근성 수정: `experience/ui` + `accessibility` Tag
- 특정 API의 보안 버그: 원인에 따라 `system/api` 또는 `system/security`가 아니라 `system/api` + `security` Tag
- release checklist 한 항목: `operations/release` + 관련 Tag
- "중요하다"는 이유만 있는 umbrella shelf
- owner나 reusable framework 없이 반복되는 잡다한 문제 묶음

`cross-cutting`이 커질수록 실제 subject area 탐색성이 떨어진다. 모호하면 primary area를 먼저 고른다.

## 3. Promotion Path

1. 후보 concern을 Tag로 운영한다.
2. 30일 이상, 3개 이상 top-level Area에서 반복되는지 본다.
3. 각 Area에서 같은 framework가 재사용되는지 확인한다.
4. Decision artifact에 admission 근거, owner, examples, rejection examples를 쓴다.
5. 승인되면 `cross-cutting/{slug}` depth 1 sub-area를 만든다.
6. concern 자체를 다루는 artifact만 `cross-cutting/*`으로 이동한다. 개별 사례 artifact는 primary area에 둔다.

## 4. Retirement Path

`cross-cutting/*` sub-area가 90일 동안 새 artifact를 받지 않거나, 실제로는 단일 primary area에만 남았으면
retirement review를 연다.

Retirement는 다음 순서로 한다.

1. 현재 artifact 목록과 최근 90일 사용량을 확인한다.
2. 개별 사례는 primary area로 relabel한다.
3. framework artifact가 여전히 유효하면 `governance/policies`나 해당 primary area로 이동한다.
4. 빈 sub-area가 되면 migration 또는 admin flow로 제거한다.
5. docs/19와 관련 Decision에 deprecated 기록을 남긴다.

## 5. Examples

### Admit: `accessibility`

Reader UI, command palette, generated markdown rendering, docs authoring guidance에 같은 accessibility 기준이
반복 적용된다. 단일 UI artifact가 아니라 제품 전반의 점검 framework이므로 `cross-cutting/accessibility`가 맞다.

### Admit: `observability`

MCP telemetry, area taxonomy metrics, warning events, production health checks가 모두 관측 기준을 공유한다.
관측 framework 자체를 다루는 문서는 `cross-cutting/observability`에 둔다. 특정 endpoint의 log bug는
`system/api` + `observability` Tag로 둔다.

### Reject: one-off privacy copy

한 화면의 privacy 문구 수정은 `experience/content` 또는 `experience/ui`가 primary area다. Privacy framework나
policy를 새로 정의하지 않는다면 `cross-cutting/privacy`로 보내지 않는다.
