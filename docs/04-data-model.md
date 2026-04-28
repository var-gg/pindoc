# 04. Data Model

Pindoc의 데이터 모델. Project · Artifact (Tier A + Tier B) · Area · Permission · Event · Graph.

> V1 기준 논리 모델. 물리 스키마(DDL)는 구현 단계.

## Project

```
Project {
  id: string                         // "proj_xxx"
  name: string                       // "shop-fe"
  slug: string
  description: markdown?
  icon: string?
  
  repos: RepoRef[]
  active_domain_packs: DomainPack[]
  
  settings: {
    pindoc_md_mode: "auto" | "manual" | "off"
    sensitive_ops: "auto" | "confirm"
    dashboard_slots: DashboardSlotConfig
  }
  
  owner: AgentRef | UserRef
  created_at: timestamp
  created_by: AgentRef | UserRef    // Project 생성은 사람도 가능 (에이전트 원칙 예외)
}

RepoRef {
  provider: "github" | "gitlab" | "local"
  identifier: string                 // "org/repo" 또는 local path
  default_branch: string
}

DomainPack {
  name: "web-saas" | "game" | "ml" | "mobile" | "cs-desktop" | "library" | "embedded"
  version: string
  status: "stable" | "skeleton"
}
```

**Project 생성 예외**: Project **생성 자체**는 사람 CLI(`pindoc init`)로도 가능 — 에이전트 경유 원칙의 유일한 예외. Project 내부의 artifact write는 agent-only.

## ProjectMembership / Permission

```
ProjectMembership {
  project_id: ProjectRef
  user_id: UserRef
  role: "owner" | "editor" | "viewer"
  invited_by?: UserRef
  joined_at: timestamp
}
```

- `owner`: 프로젝트 admin. 멤버 초대/제거, role 변경, 프로젝트 삭제
- `editor`: artifact/task write 권한. V1.5 invite flow의 기본 역할
- `viewer`: 읽기 전용

V1 `trusted_local`에서는 권한 체크가 아직 project_members를 사용하지 않고
기존처럼 모든 project를 owner로 해석한다. 다만 schema는 미리 생성되어
server boot 시 env-derived default user가 `pindoc` project owner row를
idempotent하게 받고, `pindoc.project.create`는 caller `Principal.UserID`가
있으면 같은 transaction 안에서 owner row를 생성한다.

---

## Artifact — 공통 필드

```
Artifact {
  id: string
  project_id: ProjectRef             // 필수
  type: ArtifactType                 // Tier A 또는 활성 Tier B
  tier: "core" | "domain"
  title: string
  slug: string
  
  created_at: timestamp
  updated_at: timestamp
  
  // Agent-only write 스키마 보증
  created_by: AgentRef               // User 타입 거부
  last_modified_via: AgentRef
  source_session: SessionRef         // 필수 (아래 정의)
  
  version: int
  body: TypedBody
  
  // 고정 & 리소스 (source of truth — Graph edge는 이것에서 derive)
  pins: Pin[]
  related_resources: ResourceRef[]
  
  // 분류
  area: AreaRef                      // 필수, 단수 (없으면 /misc)
  labels: string[]
  
  // 관계 (source of truth)
  references: ArtifactRef[]
  referenced_by: ArtifactRef[]       // 역참조 — 시스템 관리
  
  // 상태 (3축)
  completeness: "draft" | "partial" | "settled"          // 성숙도
  status: "published" | "stale" | "superseded" | "archived"  // 생애주기
  review_state: "auto_published" | "pending_review" | "approved" | "rejected"
                                                          // 승인 경로
  stale_reason: string?
  superseded_by: ArtifactRef?
  
  // 발행·Review
  promote_intent: Intent?
  reviewed_at: timestamp?
  reviewed_by: UserRef?
}
```

### 3축 상태의 관계

```
Promote propose
    ↓
Pre-flight / Conflict / Schema 통과
    ↓
sensitive_ops 판정
    │
    ├─ "auto" or 일반 op  → review_state: "auto_published"    → status: "published"
    │                                                             completeness: partial/settled
    │                                                             (에이전트가 제안한 대로)
    │
    └─ "confirm" & sensitive op → review_state: "pending_review"
                                        ↓
                              사용자 OK → "approved" → "published"
                              사용자 NO → "rejected" (archive)
```

시간이 지나면 `status`가 `published → stale → superseded → archived` 로 전이.

**UI 단순화**: 내부 3축은 유지하되 UI 뱃지는 4단계로 축약:
- **draft** (completeness=draft)
- **live** (status=published & completeness≥partial & review_state ∈ {auto_published, approved})
- **stale** (status=stale)
- **archived** (status=archived 또는 superseded)

`pending_review`는 Review Queue 화면에만 나타남 — Wiki에는 노출되지 않음.

---

## Epistemic Axes (`artifact_meta`)

3축 상태(completeness / status / review_state)는 artifact의 **생애주기**를 말하지만, "이 기록을 얼마나 믿어도 되는가 / 왜 여기에 있나 / 다음 세션 context로 들어가나"에는 답하지 못한다. 외부 리뷰(2026-04-23) 흡수 결과로 등장한 **네 번째 축군**이 `artifact_meta` JSONB다. 단일 JSONB 안에 6개의 선택 축이 들어가고, Applicable Rules Mechanism을 위한 선택 rule scope 필드가 같은 JSONB에 붙는다.

Migration: `0012_artifact_meta.sql` — `artifacts.artifact_meta JSONB NOT NULL DEFAULT '{}'` + partial GIN index (`jsonb_path_ops`, skip empty rows).

| 축 | 값 | 의미 |
|---|---|---|
| `source_type` | `code` / `artifact` / `user_chat` / `external` / `mixed` | 진실의 substrate. pins에 code 있으면 기본 `code`로 추론됨. |
| `consent_state` | `not_needed` / `requested` / `granted` / `denied` | user-originated 지식 승격 경계. 서버는 agent 선언에 의존. |
| `confidence` | `low` / `medium` / `high` | agent-declared. 장기적으로 server-computed 축 병기 고려(관찰 기록: `외부-리뷰-후속-관찰-...`). |
| `audience` | `owner_only` / `approvers` / `project_readers` | private/shared 분리. `source_type=user_chat` + PII pattern 감지 시 `owner_only`로 강등. |
| `next_context_policy` | `default` / `opt_in` / `excluded` | 다음 session retrieval에서 `context.for_task`가 `excluded`를 스킵. `opt_in`은 호출 시 surface. |
| `verification_state` | `verified` / `partially_verified` / `unverified` | 검증 전 추론과 코드-grounded 확인 분리. `source_type=code` 기본 `partially_verified`. |

### Applicable Rules fields

정책 wiki(design contract, coding convention, security rule 등)는 `artifact_meta.rule_severity`를 설정하면 `context.for_task`의 `applicable_rules[]`로 자동 surface된다. DB 스키마 변경은 없다. 기존 `artifact_meta` JSONB의 새 optional key다.

| 필드 | 값 | 의미 |
|---|---|---|
| `applies_to_areas` | area_slug 배열, `*`, `ui/*` 같은 wildcard scope | 이 rule이 자동 적용될 area 범위. 생략하면 rule artifact의 own area + sub-area에 적용된다. |
| `applies_to_types` | artifact type 배열 | 적용 type. 생략/빈 배열이면 모든 type에 적용된다. `context.for_task`의 default target type은 `Task`. |
| `rule_severity` | `binding` / `guidance` / `reference` | 존재하면 정책/rule artifact로 marking된다. 정렬은 binding → guidance → reference. |
| `rule_excerpt` | string | `applicable_rules[]`에 들어가는 짧은 요약. 생략 시 서버가 첫 H2 section 본문에서 200자 내외로 추출한다. |

Area inference:

- `applies_to_areas`가 있으면 target area 또는 parent chain에 매칭되는 rule만 적용된다. `experience/*` 같은 wildcard는 해당 parent chain 아래의 sub-area에 매칭된다.
- `applies_to_areas`가 없으면 rule artifact의 own area + sub-area에 적용된다.
- `cross-cutting` 및 그 child area(`security`, `privacy`, `accessibility`, `reliability`, `observability`, `localization`)에 놓인 rule은 area scope를 명시하지 않아도 모든 task에 적용된다.

Default 결정(서버 resolver, `resolveArtifactMeta`):

- `source_type` 미지정 + code pin 존재 → `code`, `verification_state=partially_verified`
- `source_type=user_chat` → `next_context_policy=opt_in` (caller가 `excluded` 명시하면 그대로 유지)
- `source_type=user_chat` + body PII 패턴(email / `Authorization:` / `api_key=`) → `audience=owner_only`
- 나머지 축은 agent-declared 우선, 서버 기본값 없음

Update path 규칙: `task_meta`와 동일하게 "send-to-overwrite" — `artifact_meta`를 포함해 propose하면 그 payload가 JSONB를 전체 교체하고, 생략하면 기존 값을 유지한다. 서버 merge 없음.

API 노출:

- `artifact.propose` 응답 `artifact_meta` — 실제로 persist된 resolved meta. 요청 payload와 다를 수 있음(resolver가 덮어쓴 경우).
- `artifact.read(view=full|brief|continuation)` 응답 `artifact_meta` — 저장된 JSONB 그대로.
- `context.for_task` landings · `artifact.search` hits — 3필드 `trust_summary` (`source_type` · `confidence` · `next_context_policy`)만 동반. Reader Trust Card에서 full meta가 필요하면 `artifact.read`로.
- `context.for_task` 응답 `applicable_rules[]` — `rule_severity`로 marking된 정책 wiki 중 target area/type에 적용되는 rule의 compact projection (`slug`, `title`, `severity`, `excerpt`, `agent_ref`, URL fields).

`SOURCE_TYPE_UNCLASSIFIED` warning: `source_type` 미지정 + pins 없음 + body에 인용부호 + chat marker("said", "사용자는" 등) 감지되면 accepted 응답에 포함. Block 아님 — agent에게 classify를 유도하는 advisory.

---

## Artifact Types — Tier A (Core, 강제)

Decision(ADR), Analysis, Debug, Flow, Task, TC, Glossary.

주요 원칙:
- 각 타입별 **필수 필드** 존재 (포맷 드리프트 방지)
- `Hypothesis` (Debug), `Alternative` (ADR) 등 중첩 구조 허용
- Mermaid 다이어그램은 `Flow` 타입에 **필수**

대부분 타입의 body 스키마는 지면 절약을 위해 생략 (구현 시 재사용). **Task는 운영 축이 얽혀있어 명시**:

### Task body 스키마

```
Task.body {
  title: string,                      // 필수, ≤120 chars
  description: string,                // 필수, markdown
  acceptance_criteria: string[],       // 완료 조건 체크리스트

  status: "todo" | "in_progress" | "done" | "archived",
  priority: "p0" | "p1" | "p2" | "p3",   // p0=blocker, p3=nice-to-have
  assignee: AgentRef | UserRef | null,    // 에이전트도 assignee 가능 (자율 에이전트 환경)

  implements: ArtifactRef[],          // Feature/Debug/ADR 등을 구현 (graph edge)
  blocked_by: ArtifactRef[],          // 다른 Task 또는 Debug에 블록 (graph edge)

  estimated_effort: "xs" | "s" | "m" | "l" | "xl" | null,  // T-shirt size, optional
  due_date: date | null,              // ISO date, optional

  // 에이전트 수행 맥락
  agent_attempts: AgentAttempt[],     // 에이전트가 이 Task를 잡았다가 놓은 이력
  resolution_artifact: ArtifactRef?,  // 완료 시 생성된 산출물 (예: 이 Task로 만든 Feature)
}

AgentAttempt {
  agent: AgentRef,                    // claude-code-xxx 등
  started_at, ended_at,
  outcome: "done" | "blocked" | "abandoned",
  note?: string                       // Harness 주도 자동 기록
}
```

### Task 상태 머신 (v3, migration 0045)

```
           ┌────────────────────────────┐
           ▼                            │
open ──▶ claimed_done ──▶ archived ◀────┘
 │
 └──▶ blocked    ──▶ cancelled
```

AI-agent 운영 모델로 재설계된 상태머신. 기존 `todo | in_progress | done`는 한 사람이 며칠 단위로 Task를 잡고 있는 가정에서 나왔지만, agent는 수 분 만에 전 사이클을 돈다. 'in_progress'는 깜빡이다 사라지는 상태라 의미를 잃고, 별도 `verified` lane은 검증 report와 Task queue를 이중화했다. v3는 Task lifecycle을 `claimed_done`에서 정착시키고, 검증 성격의 근거는 `artifact_meta.verification_state`와 pins로 표현한다.

1. **Task queue는 작업 상태만 표현**: 열린 일, 완료 주장, 차단, 취소만 Task board가 다룬다.
2. **검증은 evidence axis**: 코드 pin, TC, `artifact_meta.verification_state`가 검증 신호를 담당하고 Task status enum을 늘리지 않는다.

**전이 규칙**:

- `open → claimed_done`: implementer agent가 `pindoc.task.claim_done`을 호출한다. 서버는 본문의 unchecked acceptance checkbox를 `[x]`로 바꾸고 `task_meta.status='claimed_done'`을 같은 revision에 기록한다. `commit_sha`만 넘기면 commit diff에서 references pin을 자동 생성한다.
- `artifact.propose`로 `task_meta.status='verified'` 직접 전이 시도 → `TASK_STATUS_INVALID` reject.
- `* → blocked / cancelled`: 어느 agent나 이유와 함께 전이 가능.
- `* → archived`: sensitive_op (기존 규칙 유지).

**운영 guardrail**: Reader의 Task 대기열은 `task_meta.status`가 없거나 `open`인
row다. Agent가 "열린 Task가 없다"고 말하기 전에는 `pindoc.task.queue`
기본 호출의 `pending_count == 0`을 확인해야 한다. 새 클라이언트는
`assignee_filtered_count`와 `project_total_count`를 구분해 읽는다. `pindoc.scope.in_flight`는
acceptance checkbox(`[ ]` / `[~]`) 조회 도구라 이 lifecycle queue를 대체하지
않는다.

**남은 질문** (후속):
- trivial Task의 `self_attested` opt-out은 도입 보류(M1.x 범위 밖).

### Task 전용 Pre-flight

`varn.artifact.propose(type=Task, ...)` 호출 시 서버 체크:
- `acceptance_criteria.length ≥ 1` (모호한 Task 방지)
- `implements[]` 또는 `area` 중 최소 하나 존재 (고아 Task 방지)
- `priority` 명시 (p0~p3, default=p2)
- `assignee` 미지정 허용 (백로그 성격)

### V1 Scope 제약

- **칸반 보드는 V1 out-of-scope** ([08-non-goals.md](08-non-goals.md)). V1 UI는 리스트 + 필터(status/priority/assignee/area)로만.
- Sprint / burndown / velocity 없음. Jira/Linear 대체 아님.
- Task는 **Artifact로서의 1급 시민** — 모든 edge / Fast Landing / Search가 동일하게 작동.

### Template ↔ Validator 관계

Pre-flight의 type별 키워드 가드(`TASK_NO_ACCEPTANCE`, `DEC_NO_SECTIONS`, `DBG_NO_REPRO`, `DBG_NO_RESOLUTION`)와 `MISSING_H2` warning은 과거 Go 코드에 하드코딩된 문자열 목록을 참조했다. 그 결과 `_template_*` 본문이 revision으로 진화할 때 validator 규칙이 따라오지 않아, 템플릿을 성실히 따라 쓴 agent가 오히려 reject를 맞는 drift가 발생했다(Task `preflight-template-drift-통합`).

V1.x부터는 각 `_template_<type>` artifact body 최상단에 validator 메타 주석이 source-of-truth다:

```markdown
<!-- validator: required_h2=Purpose,Scope,Acceptance criteria; required_keywords=acceptance -->
> **This artifact is a template.** ...
```

서버가 해당 type의 propose를 받을 때 `_template_<type-lowercase>` 의 body를 읽어 `required_h2` · `required_keywords` 를 추출하고 preflight 가드에 적용한다. 메타 주석이 없거나 DB 조회가 실패하면 과거 하드코딩 fallback 이 그대로 동작(backward compat). 템플릿이 `update_of` 로 수정되면 `_template_*` slug 기반 cache invalidation 이 자동 트리거되어 다음 propose 부터 새 규칙이 반영된다.

`MISSING_H2` fuzzy 매치도 `/` · `·` · em-dash 분할을 지원해 `## 목적 / Purpose` 같은 ko/en 혼합 heading과 `## TODO — Acceptance criteria` 같은 subtitle 형태가 각 slot과 정상 매치된다.

## Artifact Types — Tier B (Domain Pack)

### Web SaaS/SI Pack (V1 stable)

```
Feature      { overview, scope, acceptance_criteria[], dependencies[], status }
APIEndpoint  { method, path, description, request_schema?, response_schema?, auth_required, rate_limit? }
Screen       { route?, description, wireframe?, components[], states[], linked_endpoints[] }
DataModel    { entity, fields[], relations[], storage, migrations[] }
```

### Game Pack (V1.x+ skeleton)

필드 이름만 고정, 상세는 커뮤니티 성숙:
- `Feature`, `Mechanic`, `Level`, `Character`, `Asset`

### ML/AI Pack (V1.x+ skeleton)

- `Feature`, `Dataset`, `Model`, `Experiment`
- Hugging Face Model Card 포맷 호환 고려

### Mobile / CS Desktop / Library / Embedded (V1.x~V2+)

스켈레톤만. 기여자 등장 시 stable.

---

## Area

Area는 Artifact가 속한 **primary concern**이다. 문서 형식, 작업 상태, 작성자, 임시 initiative가 아니라
"이 artifact가 답하는 핵심 질문"을 기준으로 하나의 shelf를 고른다.

```
Area {
  id, project_id, name, slug,
  parent: AreaRef?,
  description?,
  owner: AgentRef | UserRef?,
  
  created_at, created_by: AgentRef, last_modified_via: AgentRef
}
```

### Top-level 8 concern skeleton

Top-level Area slug는 프로젝트마다 자유 생성하지 않는다. Pindoc V1의 고정 skeleton은 다음 8개다.

| slug | 핵심 질문 | starter sub-area |
|---|---|---|
| `strategy` | 왜 하는가, 무엇을 선택하는가 | `vision`, `goals`, `scope`, `hypotheses`, `roadmap` |
| `context` | 바깥 세계에서 무엇이 사실인가 | `users`, `competitors`, `literature`, `external-apis`, `standards` |
| `experience` | 외부 actor가 무엇을 보고 겪는가 | `ui`, `flows`, `information-architecture`, `content`, `developer-experience` |
| `system` | 내부적으로 어떻게 실현되는가 | `architecture`, `data`, `api`, `integrations`, `mechanisms`, `mcp`, `embedding` |
| `operations` | 어떻게 ship/run/support 하는가 | `delivery`, `release`, `launch`, `incidents`, `editorial-ops` |
| `governance` | 어떤 규칙/ownership/제약이 있는가 | `policies`, `compliance`, `ownership`, `review`, `taxonomy-policy` |
| `cross-cutting` | 여러 Area를 가로지르는 reusable named concern은 무엇인가 | `security`, `privacy`, `accessibility`, `reliability`, `observability`, `localization` |
| `misc` | 아직 분류가 확정되지 않은 temporary overflow인가 | 없음 |

### 소속 규칙

- 모든 Artifact는 **하나의 Area**에 속한다. `Artifact.area`는 필수 단수 필드이며, 미지정 시 `/misc`로 들어간다.
- `labels`/Tag는 보조 분류다. 단수 Area 원칙을 회피하기 위해 여러 Area 이름을 label로 중복 저장하지 않는다.
- Area는 Project 하위 스코프다. Project A의 `/system/api`와 Project B의 `/system/api`는 별개다.
- 신규 Area 생성은 Write-Intent Router 통과 + `sensitive_ops: confirm` 이면 Review Queue.

### Sub-area promotion

Sub-area는 **depth 1 only**다. 즉 `system/mcp`는 가능하지만 `system/mcp/tools` 같은 depth 2+는 만들지 않는다.
각 sub-area는 정확히 하나의 top-level parent만 가진다.

Sub-area로 승격하려면 다음 조건을 만족해야 한다.

- 여러 artifact에서 반복적으로 등장하는 stable recurring noun이다.
- 문서 형식이 아니라 concern을 가리킨다.
- owner 또는 운영 규칙을 붙일 만큼 장기적으로 남는다.
- 단일 artifact의 임시 작업명이 아니라 이후 artifact도 같은 shelf를 공유할 가능성이 높다.

다음은 sub-area로 만들지 않는다.

- 문서 형식: `decision`, `task`, `analysis`, `debug`, `apiendpoint`, `screen`
- 워크플로우 상태: `todo`, `in-progress`, `review`, `done`, `blocked`
- 사람/팀/에이전트 이름: `alice`, `codex`, `claude`, `backend-team`
- one-off initiative 또는 릴리스 코드명: `april-patch`, `mvp-week`, `phase-3`

반복성은 있으나 아직 shelf로 고정하기 이르면 Tag로 시작하고, 충분히 재사용된 뒤 sub-area로 승격한다.

### Cross-cutting admission rule

`cross-cutting`은 여러 top-level Area를 가로지르는 **reusable named concern**만 받는다.
예: `security`, `privacy`, `accessibility`, `reliability`, `observability`, `localization`.

특정 Area 안의 단일 instance는 `cross-cutting`으로 보내지 않는다. 예를 들어 Reader UI의 접근성 수정은
`experience/ui`에 두고 `accessibility` Tag를 붙인다. 여러 Area에 공통 정책, 점검 기준, telemetry,
governance가 생긴 named concern만 `cross-cutting/<concern>`으로 승격한다.

### Decision landing

`Type=Decision`은 artifact form이고 Area가 아니다. Decision artifact도 주제에 맞는 subject area에 둔다.
예: MCP tool shape 결정은 `system/mcp`, Reader IA 결정은 `experience/information-architecture`,
taxonomy 정책 결정은 `governance/taxonomy-policy`.

`decisions` Area는 금지된다. Decision 여부는 `Artifact.type == "Decision"`으로 표현하고,
Area로 한 번 더 인코딩하지 않는다.

### 기존 9 -> 8 매핑 부록

| 기존 slug | 새 landing | 비고 |
|---|---|---|
| `vision` | `strategy` | vision은 `strategy/vision` starter sub-area로 유지 |
| `architecture` | `system/architecture` | 내부 구조 concern |
| `data-model` | `system/data` | schema/data model concern |
| `mechanisms` | `system/mechanisms` | 내부 동작 원리 |
| `ui` | `experience/ui` | 외부 actor 경험 concern |
| `roadmap` | `strategy/roadmap` | time view는 strategy 하위 |
| `decisions` | subject area + `Type=Decision` | Area는 drop, artifact type만 유지 |
| `cross-cutting` | `cross-cutting` | reusable named concern만 입장 |
| `misc` | `misc` | temporary overflow only |
| `mcp-surface` | `system/mcp` | 기존 architecture sub-area |
| `embedding-layer` | `system/embedding` | 기존 architecture sub-area |

## Project Tree

```
ProjectTree {
  project_id,
  tier_a_types: ArtifactType[],      // 고정
  active_domain_packs: DomainPack[],
  areas: Area[],
  layout_preference: "type_first" | "area_first"
}
```

---

## Pin (Hard) vs Related Resource (Soft)

### Pin

```
Pin {
  repo, ref_type: "commit"|"branch"|"pr"|"path_only",
  commit_sha?, branch?, pr_number?,
  paths: string[],
  pinned_at, pinned_by: AgentRef
}
```

Stale 감지 대상.

### ResourceRef

```
ResourceRef {
  type: "code"|"asset"|"api"|"doc"|"link",
  ref: string,
  purpose: string,
  added_at, added_by: AgentRef,
  last_verified_at?, verified_status: "valid"|"broken"|"stale"|"unverified"
}
```

Stale 감지 아님. Fast Landing + 사이드 패널. M7 Freshness Re-Check로 주기 검증.

**Pin vs ResourceRef 관계**:

| | Pin | ResourceRef |
|---|-----|-----|
| 의미 | 정합 필수 | 맥락 navigation |
| Stale 감지 | ✅ 자동 | ❌ (M7 주기) |
| 저장 | `Artifact.pins[]` | `Artifact.related_resources[]` |
| Graph edge | `pinned_to` (derived) | `related_resource` (derived) |
| UI | 본문 헤더 메타 | 사이드 패널 "Related Resources" |

**이중 저장 아님**: Artifact 필드가 **source of truth**, Graph edge는 그것에서 derive한 view (쿼리용).

`completeness == "draft"` 인 artifact의 ResourceRef 링크는 UI에서 disabled.

`type: "code"` + repo/commit → UI에서 `https://github.com/.../blob/COMMIT/PATH#L10-L30` outbound 링크.

---

## Intent

```
Intent {
  kind: "new" | "modification" | "split" | "supersede",
  target_type: ArtifactType,
  project_id: ProjectRef,
  target_area: AreaRef,             // 단수
  target_id: string?,
  source_ids: string[]?,
  reason: string,
  related_session: SessionRef?,
  declared_by: AgentRef,
  declared_at: timestamp
}
```

**Cross-area**: Artifact는 1개 Area에만 속함. 여러 area에 걸친 reusable named concern은
`cross-cutting/<concern>`에 둔다. 특정 primary area의 단일 instance는 subject area + Tag로 표현한다.
한 문서가 실제로 여러 subject를 독립적으로 다루면 별도 Artifact 여러 개 + Graph `relates_to` edge로 분리한다.

---

## Graph Edge Types (Derived View)

```
EdgeType:
  - references         // Artifact.references[] 에서 derive
  - derives_from       // Artifact.source_session 또는 source_ids 에서
  - validates          // TC.body.validates 에서
  - implements         // Task.body.implements 에서
  - supersedes         // Artifact.superseded_by 역방향
  - pinned_to          // Artifact.pins[] 에서 derive
  - related_resource   // Artifact.related_resources[] 에서 derive
  - blocked_by         // Task.body.blocked_by 에서
  - relates_to         // 약한 관련성 (명시 선언)
  - continuation_of    // Artifact.source_session 기반
```

**중요**: Edge는 **derived view**. Source of truth는 Artifact 필드. 구현상 materialized view 또는 런타임 쿼리.

**Cross-project edge 허용**: 예 — FE Feature `references` BE APIEndpoint. 선언 에이전트는 양쪽 Project에 read 권한 필요.

---

## SessionRef (Session 대체)

**Pindoc은 raw 세션을 저장하지 않습니다.** 대신 외부 레퍼런스만 유지:

```
SessionRef {
  agent: "claude-code" | "cursor" | "cline" | "codex" | ...
  session_id: string               // 해당 클라이언트 내부 ID
  timestamp: timestamp
  user: UserRef
  title_hint: string?              // Promote 시 에이전트가 제공한 1줄
}
```

**사용**:
- `Artifact.source_session: SessionRef` — "이 artifact가 나온 세션"
- UI에 "원본 세션: Claude Code @ 2026-04-20 14:30" 표시
- 사용자가 원하면 해당 클라이언트에서 session_id로 open (클라이언트 지원 시)

**Pindoc은 보관·검색·전파하지 않음**. Raw 세션의 운명은 해당 에이전트 클라이언트의 책임.

---

## Event / Notification 모델

```
Event {
  id, timestamp,
  type: EventType,
  project_id: ProjectRef,
  source_ref: ArtifactRef | PinRef | ResourceRef | AreaRef,
  payload: object,
  severity: "info" | "low" | "medium" | "high",
  subscribers_notified: SubscriberRef[]
}

EventType (V1):
  # Artifact 라이프사이클
  - artifact.published
  - artifact.stale_detected
  - artifact.superseded
  - artifact.archived
  - artifact.warning_raised    # Task propose-경로-warning-영속화
  
  # Pin / Git
  - pin.changed
  - git.push_received
  
  # TC (V1.1)
  - tc.failed
  - tc.run_completed
  
  # Resource
  - resource.verified          # M7 결과
  - resource.broken
  
  # Review
  - review.required            # sensitive op이 Review Queue로
  - review.approved
  - review.rejected
  
  # Project / Area
  - project.area_created
  - project.member_added
```

```
EventSubscription {
  id, project_id, principal: UserRef | AgentRef | WebhookRef,
  event_types: EventType[],
  filter: JsonLogic?,
  channel: "ui_inbox" | "webhook" | "email",
  created_at
}
```

**V1**: Event Bus (Postgres LISTEN/NOTIFY 또는 outbox) + UI Inbox + 간단 Webhook.
**V1.1+**: Email, Slack/Discord, smart filter UI.

### `artifact.warning_raised` payload

Accepted-path warnings(`CANONICAL_REWRITE_WITHOUT_EVIDENCE`, `CONSENT_REQUIRED_FOR_USER_CHAT`, `SOURCE_TYPE_UNCLASSIFIED`, `RECOMMEND_READ_BEFORE_CREATE` 등)는 propose 응답에만 존재했다. Reader Trust Card와 미래 agent 세션이 소급 인지 가능하도록 `artifact.warning_raised` kind 이벤트에 기록한다(Task `propose-경로-warning-영속화`).

```jsonc
{
  "project_id": "<uuid>",
  "kind": "artifact.warning_raised",
  "subject_id": "<artifact uuid>",
  "payload": {
    "codes": ["CANONICAL_REWRITE_WITHOUT_EVIDENCE", "SOURCE_TYPE_UNCLASSIFIED"],
    "revision_number": 3,
    "author_id": "claude-code",
    "canonical_rewrite_without_evidence": true  // 선택, canonical guard 발동 시
  },
  "created_at": "..."
}
```

삽입은 create / update accepted 반환 직전 best-effort(실패 시 warn log + artifact 저장은 유지). Reader는 `/api/p/:project/artifacts/:slug` 응답의 `recent_warnings[]` (최근 5 row) 중 **최신 revision** 값만 Trust Card 뱃지로 렌더하고 이전 revision warning은 revision history에서 열람.

---

## Today / Change Group Read Model

Change Group은 별도 canonical table이 아니라 `artifact_revisions` 위의 query model이다. grouping key 우선순위는 `bulk_op_id` → `source_session+turn/run` → task/agent run id → `source_session+time window` → author/time fallback이며, synthetic `group_id`는 `hash(scope + key_kind + key_value + window_start)`로 만든다.

Today 화면의 상태 저장은 두 테이블만 가진다:

```
reader_watermarks(user_key, project_id, revision_watermark, seen_at)
summary_cache(cache_key, project_id, user_key, locale, filter_hash,
              baseline_revision_id, max_revision_id, headline, bullets,
              source, input_hash, token_estimate, created_at, expires_at)
summary_usage_daily(user_key, project_id, day, tokens_used)
```

`summary_cache.cache_key`는 `hash(account_id, project_slug, user_id, baseline_revision_id, max_revision_id, locale, filter_hash)`다. LLM summary input은 Change Group compact metadata만 포함하고 artifact body는 넣지 않는다. LLM endpoint 미설정, 실패, daily cap 초과 시 deterministic template로 fallback한다.

---

## Continuation Context

```
ContinuationContext {
  artifact: Artifact,
  project: Project,
  neighbors: Artifact[],
  recent_changes: Event[],
  open_questions: string[],
  source_session: SessionRef?,
  related_resources: ResourceRef[],
  area_context: { area: Area, sibling_artifacts: ArtifactRef[] }
}
```

`pindoc.artifact.read(url_or_id)` 응답 번들. URL → 에이전트 fetch로 대화 재개.

---

## 예시: Multi-project 시나리오

```
Project: shop-fe
├─ active_domain_packs: [web-saas]
├─ Areas: /strategy, /experience/ui, /system/api, /misc
├─ Members: Alice(admin+approver), Bob(reader),
│           Alice-claude(writer), Bob-cursor(reader)
└─ Artifacts:
    └─ Feature "장바구니 재시도 UI" [Area: experience/ui, completeness: partial]
        ├─ references → shop-be:APIEndpoint "POST /cart/retry"  (cross-project)
        ├─ pins: [retry-ui.tsx @ commit-abc]
        └─ related_resources: [useCart.ts, cart-store.ts]

Project: shop-be
└─ APIEndpoint "POST /cart/retry" [Area: system/api]
    └─ referenced_by ← shop-fe:Feature "장바구니 재시도 UI"
```

Event 예: Bob이 BE API schema 변경 → `pin.changed` → Graph cross-edge 따라 shop-fe Feature에 `artifact.stale_detected` 전파 → Alice UI Inbox "FE 확인 필요".
