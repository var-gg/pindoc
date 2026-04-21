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
  id, project_id, principal: AgentRef | UserRef,
  role: "admin" | "writer" | "approver" | "reader",
  granted_at, granted_by, revoked_at?
}
```

- `admin`: 설정, 멤버, Domain Pack, agent token 발급
- `writer` (주로 Agent): Artifact write
- `approver` (사람): Review Queue 처리
- `reader`: 읽기

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
  area: AreaRef                      // 필수, 단수 (없으면 /Misc)
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

### Task 상태 머신

```
todo ─▶ in_progress ─▶ done
  │         │            │
  └─────────┴──▶ archived ◀──┘
```

전이 규칙:
- `todo → in_progress`: 에이전트가 assignee로 잡을 때 자동
- `in_progress → done`: `acceptance_criteria` 전부 체크 + (선택) `resolution_artifact` 연결
- `* → archived`: 명시적 요청 (sensitive_op, Review Queue)
- `done → archived`: 허용 (히스토리 정리)
- `archived → *`: 금지 (새 Task로 재생성)

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

```
Area {
  id, project_id, name, slug,
  parent: AreaRef?,
  description?,
  owner: AgentRef | UserRef?,
  
  created_at, created_by: AgentRef, last_modified_via: AgentRef
}
```

- 모든 Artifact는 **하나의 Area**에 속함 (단수). 미지정 시 `/Misc`.
- Project 하위 스코프 — Project A의 `/Cart`와 Project B의 `/Cart`는 별개.
- 신규 Area 생성은 Write-Intent Router 통과 + `sensitive_ops: confirm` 이면 Review Queue.

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

**Cross-area**: Artifact는 1개 Area에만 속함. "여러 area에 걸친 관심사"는 **상위 Area** (`/Cross-cutting/Observability`) 또는 **별도 Artifact 여러 개** + Graph `relates_to` edge로.

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
├─ Areas: /Cart, /Payment, /Auth, /Misc
├─ Members: Alice(admin+approver), Bob(reader),
│           Alice-claude(writer), Bob-cursor(reader)
└─ Artifacts:
    └─ Feature "장바구니 재시도 UI" [Area: /Cart, completeness: partial]
        ├─ references → shop-be:APIEndpoint "POST /cart/retry"  (cross-project)
        ├─ pins: [retry-ui.tsx @ commit-abc]
        └─ related_resources: [useCart.ts, cart-store.ts]

Project: shop-be
└─ APIEndpoint "POST /cart/retry" [Area: /Cart]
    └─ referenced_by ← shop-fe:Feature "장바구니 재시도 UI"
```

Event 예: Bob이 BE API schema 변경 → `pin.changed` → Graph cross-edge 따라 shop-fe Feature에 `artifact.stale_detected` 전파 → Alice UI Inbox "FE 확인 필요".
