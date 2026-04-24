# 19. Area Taxonomy

Pindoc Area는 Artifact가 답하는 **primary concern**을 나타낸다. Area는 문서 형식이나 작업 상태가 아니라
"이 artifact가 어느 질문에 답하는가"를 고정된 shelf로 표현한다. 이 문서는 `strategy`, `context`,
`experience`, `system`, `operations`, `governance`, `cross-cutting`, `misc` 8개 top-level skeleton을
운영하기 위한 판단 기준이다.

## 1. 8 Skeleton Rationale

D0 `area-taxonomy-reform-path-a`는 기존 9개 top-level(`vision`, `architecture`, `data-model`,
`mechanisms`, `ui`, `roadmap`, `decisions`, `cross-cutting`, `misc`)을 8 concern skeleton으로 교체한다.
핵심 이유는 concern, form, time view, implementation detail이 같은 계층에 섞여 있었기 때문이다.

Rosenfeld/Morville/Arango 계열 IA 관점에서는 navigation shelf가 사용자의 질문 구조를 반영해야 한다.
`Decision` 같은 문서 형식이 Area가 되면 사용자는 "무엇에 관한 결정인가"를 다시 추론해야 한다.

Ranganathan facet classification과 Fricke의 비판은 분류축을 섞지 말라고 경고한다. Pindoc에서
Type은 artifact form이고 Area는 concern이다. Type과 Area를 같은 값으로 중복 인코딩하지 않는다.

Katsanos/Petrie의 card sorting 관점에서는 실제 사용자 mental model이 안정적인 상위 범주를 먼저 찾는다.
`context`, `experience`, `system`처럼 문제공간, 경험, 구현을 분리하면 artifact가 늘어도 탐색 비용이
천천히 증가한다.

Guy/Tonkin/Mathes의 folksonomy 논의는 Tag의 역할을 설명한다. 반복성이 아직 증명되지 않은 명칭은 Tag로
시작하고, 여러 artifact에서 안정적으로 재사용될 때만 sub-area로 승격한다.

Larson/Czerwinski/Zaphiris의 depth-breadth 연구는 얕은 계층을 선호한다. Pindoc은 top-level 8개와
depth 1 sub-area까지만 허용한다. `system/mcp/tools`처럼 깊어지는 구조는 탐색보다 관리 비용을 키운다.

ADR, C4, arc42, Diataxis는 모두 form과 subject를 분리한다. ADR은 `Decision` type이고, C4/arc42/Diataxis의
구조는 subject area나 Type 선택을 돕는 참고 프레임일 뿐 Area 자체가 아니다.

## 2. Slug Definitions

### `strategy`

포함: 제품 방향, vision, goals, scope, roadmap, hypothesis, business model, launch criteria.

제외: 실제 구현 구조는 `system`, 운영 절차는 `operations`, 규칙/정책은 `governance`.

혼동 주의: `roadmap`은 top-level이 아니라 `strategy/roadmap`이다. 일정표라도 배포 운영 절차 자체는
`operations/release`가 더 맞다.

### `context`

포함: user research, competitor analysis, literature review, external standard, market signal,
external API 사실 조사.

제외: 외부 정보를 바탕으로 한 선택은 `strategy` 또는 subject area의 `Decision`으로 둔다.

혼동 주의: "왜 이걸 해야 하는가"는 `strategy`, "바깥 세계에서 무엇이 사실인가"는 `context`다.

### `experience`

포함: UI, flow, information architecture, content, onboarding, command palette, developer experience,
external actor가 보는 상태.

제외: 내부 API, database, job worker, embedder, MCP tool implementation은 `system`.

혼동 주의: 화면에 드러나는 에러 처리는 `experience/ui`, 에러를 만드는 backend retry logic은 `system/api`
또는 `system/mechanisms`다.

### `system`

포함: architecture, data model, API, integrations, mechanisms, MCP, embedding, runtime behavior,
internal service boundaries.

제외: launch, incident response, release playbook은 `operations`; policy와 ownership은 `governance`.

혼동 주의: `data-model`, `mechanisms`, `architecture`는 top-level이 아니라 `system/data`,
`system/mechanisms`, `system/architecture`다.

### `operations`

포함: delivery, release, deployment, incident, support, editorial ops, migration runbook,
environment setup.

제외: 정책 자체는 `governance`, 제품 방향은 `strategy`, 구현 설계는 `system`.

혼동 주의: "어떻게 만들어져 있는가"는 `system`, "어떻게 ship/run/support 하는가"는 `operations`다.

### `governance`

포함: policy, compliance, ownership, review, taxonomy policy, permission, consent, write rules.

제외: 정책을 구현하는 코드 설계는 `system`, 운영 체크리스트는 `operations`.

혼동 주의: "누가 어떤 규칙으로 허용하는가"가 핵심이면 `governance`다.

### `cross-cutting`

포함: 여러 top-level area에 반복적으로 걸치는 reusable named concern. 예: security, privacy,
accessibility, reliability, observability, localization.

제외: 특정 primary area의 단일 instance. Reader UI 접근성 수정은 `experience/ui` + `accessibility` Tag다.

혼동 주의: cross-cutting은 "여러 곳에 걸친다"가 아니라 "여러 곳에 반복 적용되는 named concern"일 때만 쓴다.
입장/해제 절차는 [21. Cross-cutting Admission Rule](21-cross-cutting-admission-rule.md)을 따른다.

### `misc`

포함: 분류가 아직 확정되지 않은 temporary overflow.

제외: 귀찮아서 넣는 임시 휴지통, 영구 보관 shelf.

혼동 주의: `misc` artifact는 30일 안에 rehome 대상이다. `misc` 사용은 실패가 아니라 미분류를 드러내는 신호다.

## 3. Pairwise Guide

| 구분 | `A`에 둔다 | `B`에 둔다 |
|---|---|---|
| `strategy` vs `context` | 선택, 방향, 목표, 우선순위 | 외부 사실, 리서치, 비교, 표준 |
| `experience` vs `system` | 사용자/에이전트/개발자가 보는 흐름과 화면 | 내부 구조, 데이터, API, runtime mechanism |
| `operations` vs `governance` | release, deploy, incident, support 절차 | policy, permission, ownership, review rule |
| `cross-cutting` vs primary area | 여러 area에 재사용되는 named concern 자체 | 특정 subject area 안의 단일 사례 + Tag |

판단이 애매하면 Artifact의 title을 질문으로 바꿔본다. "왜/무엇을 선택하는가"는 `strategy`,
"무엇이 사실인가"는 `context`, "무엇을 보고 겪는가"는 `experience`, "어떻게 실현되는가"는 `system`,
"어떻게 운영하는가"는 `operations`, "어떤 규칙으로 허용되는가"는 `governance`다.

## 4. Sub-area Promotion

Sub-area는 depth 1 only다. `system/mcp`는 허용하고 `system/mcp/tools`는 만들지 않는다.
각 sub-area는 single parent를 가진다.

정식 승격, rename, merge, remove 절차는 [20. Sub-area Promotion Policy](20-sub-area-promotion-policy.md)를
source of truth로 둔다. 이 섹션은 빠른 판단 기준만 요약한다.

승격 조건:

- 여러 artifact에서 반복적으로 등장하는 stable recurring noun이다.
- Type, status, 사람, team, agent, 일회성 project code가 아니다.
- owner, description, review rule을 붙여도 어색하지 않다.
- 같은 이름이 다음 artifact에서도 재사용될 가능성이 높다.

금지 대상:

- 문서 형식: `decision`, `task`, `analysis`, `debug`, `apiendpoint`, `screen`
- workflow 상태: `todo`, `in-progress`, `review`, `done`, `blocked`
- 사람/팀/agent 이름: `alice`, `backend-team`, `codex`, `claude`
- one-off initiative: `april-patch`, `mvp-week`, `phase-3`, `launch-day-fix`

Tag -> sub-area 승격 절차:

1. Tag로 시작한다.
2. 30일 동안 3개 이상 artifact에서 같은 noun이 반복되는지 본다.
3. 같은 top-level parent에 계속 landing하는지 확인한다.
4. owner와 description을 쓸 수 있으면 `pindoc.area.create`로 depth 1 sub-area를 만든다.
5. 기존 artifact를 새 sub-area로 rehome하고, Tag는 필요할 때만 유지한다.

## 5. Starter Sub-area Catalog

### SW Product Profile

| top-level | starter sub-area examples |
|---|---|
| `strategy` | `vision`, `goals`, `scope`, `roadmap` |
| `context` | `users`, `competitors`, `external-apis` |
| `experience` | `ui`, `flows`, `information-architecture`, `developer-experience` |
| `system` | `architecture`, `data`, `api`, `mcp`, `embedding` |
| `operations` | `release`, `delivery`, `incidents`, `migration` |
| `governance` | `ownership`, `review`, `taxonomy-policy`, `permissions` |
| `cross-cutting` | `security`, `privacy`, `accessibility`, `observability` |

최소 starter set 15: `vision`, `goals`, `roadmap`, `users`, `competitors`, `ui`, `flows`,
`information-architecture`, `architecture`, `data`, `api`, `mcp`, `release`, `ownership`, `security`.

### Research / Academic Profile

| top-level | starter sub-area examples |
|---|---|
| `strategy` | `research-questions`, `scope`, `hypotheses` |
| `context` | `literature`, `datasets`, `benchmarks`, `standards`, `prior-art` |
| `experience` | `publication`, `figures`, `reviewer-experience` |
| `system` | `methods`, `models`, `pipelines`, `evaluation`, `reproducibility` |
| `operations` | `experiment-runs`, `artifact-release`, `submission` |
| `governance` | `ethics`, `consent`, `data-use`, `authorship` |
| `cross-cutting` | `privacy`, `bias`, `reliability`, `observability` |

최소 starter set 15: `research-questions`, `hypotheses`, `literature`, `datasets`, `benchmarks`,
`prior-art`, `publication`, `methods`, `models`, `pipelines`, `evaluation`, `reproducibility`,
`experiment-runs`, `ethics`, `privacy`.

### Marketing / Content Profile

| top-level | starter sub-area examples |
|---|---|
| `strategy` | `positioning`, `goals`, `segments`, `campaign-roadmap` |
| `context` | `audience`, `competitors`, `market-research`, `channels` |
| `experience` | `landing-pages`, `content`, `copy`, `email`, `social` |
| `system` | `analytics`, `cms`, `tracking`, `integrations` |
| `operations` | `calendar`, `publishing`, `launch`, `handoff` |
| `governance` | `brand-policy`, `legal-review`, `approval`, `ownership` |
| `cross-cutting` | `accessibility`, `localization`, `privacy`, `measurement` |

최소 starter set 15: `positioning`, `segments`, `campaign-roadmap`, `audience`, `competitors`,
`market-research`, `landing-pages`, `content`, `copy`, `email`, `analytics`, `cms`, `calendar`,
`brand-policy`, `localization`.

### DevRel Profile

| top-level | starter sub-area examples |
|---|---|
| `strategy` | `community-goals`, `programs`, `roadmap` |
| `context` | `developers`, `ecosystem`, `standards`, `feedback` |
| `experience` | `docs`, `tutorials`, `sdk-experience`, `samples`, `events` |
| `system` | `sdk`, `api`, `tooling`, `sandbox`, `examples` |
| `operations` | `release-notes`, `community-support`, `event-ops`, `triage` |
| `governance` | `contribution-policy`, `moderation`, `license`, `review` |
| `cross-cutting` | `accessibility`, `localization`, `security`, `reliability` |

최소 starter set 15: `community-goals`, `programs`, `developers`, `ecosystem`, `feedback`, `docs`,
`tutorials`, `sdk-experience`, `samples`, `sdk`, `api`, `tooling`, `release-notes`,
`contribution-policy`, `moderation`.

### Hybrid Service Profile

| top-level | starter sub-area examples |
|---|---|
| `strategy` | `service-model`, `goals`, `scope`, `pricing` |
| `context` | `customers`, `partners`, `regulation`, `contracts` |
| `experience` | `customer-journey`, `operator-console`, `support-flow`, `content` |
| `system` | `backend`, `data`, `integrations`, `automation`, `billing` |
| `operations` | `fulfillment`, `support`, `sla`, `incident`, `handover` |
| `governance` | `compliance`, `ownership`, `review`, `risk` |
| `cross-cutting` | `privacy`, `security`, `reliability`, `localization` |

최소 starter set 15: `service-model`, `pricing`, `customers`, `partners`, `regulation`,
`customer-journey`, `operator-console`, `support-flow`, `backend`, `data`, `integrations`,
`automation`, `fulfillment`, `compliance`, `security`.

## 6. Type vs Area Boundary

| Type | Area 선택 기준 | 혼동 포인트 |
|---|---|---|
| `Decision` | 결정의 subject area | `decisions` Area를 만들지 않는다. |
| `Analysis` | 분석 질문의 subject area | 분석 방법론이 아니라 답하려는 질문 기준. |
| `Debug` | 증상/원인이 속한 subject area | bug 상태가 아니라 원인 도메인 기준. |
| `Flow` | actor가 겪는 흐름이면 `experience/flows`, 내부 process면 `system` 또는 `operations` | Mermaid diagram이 있다고 항상 `experience`는 아니다. |
| `Task` | 작업이 바꾸는 primary surface | assignee나 phase 이름으로 Area를 만들지 않는다. |
| `TC` | 검증하는 behavior의 subject area | test framework 자체는 `system`, 검증 대상은 subject area. |
| `Glossary` | 용어가 주로 쓰이는 subject area | 공통 용어집이면 `governance/taxonomy-policy` 또는 `context`. |
| `Feature` | 사용자가 얻는 capability의 subject area | UI feature면 `experience`, internal capability면 `system`. |
| `APIEndpoint` | API contract와 runtime behavior 기준 `system/api` | API 문서라는 형식 때문에 `experience`로 보내지 않는다. |
| `Screen` | 화면/상태/interaction 기준 `experience/ui` | screen이 호출하는 backend 구조는 별도 `system` artifact. |
| `DataModel` | schema/entity/migration 기준 `system/data` | 데이터 거버넌스 정책은 `governance`. |
| `Mechanic` (V1.x candidate) | 게임/interactive rule의 player experience면 `experience`, engine rule 구현이면 `system/mechanisms` | 아직 V1 accepted type은 아니며 Game Pack 후보로만 취급한다. |

## 7. Operating Metrics

| metric | formula | cadence | target | alert |
|---|---|---|---|---|
| `misc_ratio` | `count(artifacts where area = misc) / count(all published artifacts)` | weekly | `<= 0.05` | `>= 0.10` for 2 weeks |
| `cross_cutting_ratio` | `count(artifacts under cross-cutting) / count(all published artifacts)` | weekly | `0.03..0.15` | `> 0.20` or sudden 2x jump |
| `rehome_rate_30d` | `count(misc artifacts moved to non-misc within 30d) / count(artifacts that entered misc 30d ago)` | weekly | `>= 0.80` | `< 0.60` |
| `agent_suggestion_accuracy` | `count(agent suggested area == final area) / count(agent area suggestions)` | weekly | `>= 0.85` | `< 0.70` |

`misc_ratio`가 높으면 skeleton이 부족하거나 agent guidance가 약하다. `cross_cutting_ratio`가 높으면 primary
area 대신 umbrella shelf를 과사용하고 있을 가능성이 있다. `rehome_rate_30d`는 `misc`를 임시 overflow로
유지하는지 본다. `agent_suggestion_accuracy`는 context retrieval과 area picker guidance 품질을 본다.

Phase 5는 이 정의를 telemetry로 구현한다. 이 문서는 지표 이름, 수식, 목표치, alert 기준의 source of truth다.
