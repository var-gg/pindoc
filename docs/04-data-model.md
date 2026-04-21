# 04. Data Model

Varn의 데이터 모델. Project · Artifact 스키마(Tier A + Tier B) · Area · Permission · Event · Graph 엣지.

> V1 기준 논리 모델입니다. 물리 스키마(DDL)는 구현 단계에서 별도 정의.

## Project

**모든 것의 최상위.** Artifact/Area/Session/Member는 반드시 하나의 Project에 속한다.

```
Project {
  id: string                         // "proj_xxx"
  name: string                       // "shop-fe"
  slug: string                       // URL-safe
  description: markdown?
  icon: string?                      // emoji 또는 URL
  
  repos: RepoRef[]                   // 연결된 Git repo들 (보통 1개)
  active_domain_packs: DomainPack[]  // install 시 선택한 pack들
  
  settings: {
    varn_md_mode: "auto" | "manual" | "off"
    sensitive_ops: "auto" | "confirm"        // Review Queue 사용 여부
    dashboard_slots: DashboardSlotConfig
  }
  
  owner: AgentRef | UserRef
  created_at: timestamp
  created_by: AgentRef | UserRef    // Project 생성은 사람도 가능 (에이전트 경유 예외)
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

**Project 생성 예외**: Project **생성 자체**는 사람 CLI(`varn init` → 새 Project)로도 가능 — 에이전트 경유 원칙의 유일한 예외. Project **내부** artifact 쓰기는 agent-only 엄수.

## Project Membership / Permission

```
ProjectMembership {
  id: string
  project_id: ProjectRef
  principal: AgentRef | UserRef
  role: "admin" | "writer" | "approver" | "reader"
  granted_at: timestamp
  granted_by: UserRef
  revoked_at: timestamp?
}
```

**Role 의미**:
- `admin`: Project 설정 변경, 멤버/Domain Pack 관리, agent token 발급/회수
- `writer`: Artifact write 권한 (주로 Agent)
- `approver`: Review Queue 처리 (사람 전용)
- `reader`: 읽기만 (사람/에이전트)

한 사람/에이전트가 여러 Project에 서로 다른 role.

**매핑 예시**:
```
Alice (User):    shop-fe = admin+approver, shop-be = approver, side-game = admin
Bob (User):      shop-fe = reader,         shop-be = admin+approver
Alice's Claude (Agent): shop-fe = writer,  shop-be = writer (매니지먼트라 양쪽)
Bob's Cursor (Agent):   shop-be = writer
```

---

## Artifact — 공통 필드

```
Artifact {
  id: string
  project_id: ProjectRef             // 필수, 최상위 스코프
  type: ArtifactType                 // Tier A 또는 활성 Tier B
  tier: "core" | "domain"
  title: string
  slug: string
  
  created_at: timestamp
  updated_at: timestamp
  
  // Agent-only write 스키마 보증
  created_by: AgentRef               // User 타입 거부
  last_modified_via: AgentRef        // 모든 수정은 agent 경유
  source_session: SessionRef         // 필수 (agent-only 강제)
  
  version: int
  body: TypedBody
  
  // 고정 & 리소스
  pins: Pin[]
  related_resources: ResourceRef[]
  
  // 분류
  area: AreaRef                      // 필수 (없으면 /Misc)
  labels: string[]
  
  // 관계
  references: ArtifactRef[]
  referenced_by: ArtifactRef[]
  
  // 상태
  completeness: "draft" | "partial" | "settled"   // 기본 "partial"
  status: "published" | "stale" | "superseded" | "archived"
  stale_reason: string?
  superseded_by: ArtifactRef?
  
  // 발행·Review
  promote_intent: Intent?
  review_state: "auto_published" | "pending_review" | "approved" | "rejected"
  reviewed_at: timestamp?
  reviewed_by: UserRef?              // Review Queue 거친 경우만
}
```

**publish 경로 2가지**:
- **일반 propose** → Pre-flight → Conflict → Schema 통과 → `review_state: "auto_published"`
- **Sensitive ops** (`sensitive_ops: confirm` 설정 + 해당 작업) → `review_state: "pending_review"` → 사람 OK 후 `approved`

---

## Artifact Types — Tier A (Core, 강제)

Decision(ADR), Analysis, Debug, Flow, Task, TC, Glossary.

스키마는 [이전 04 data-model]과 동일 (여기서 중복 생략, 구현 시 재사용).

## Artifact Types — Tier B (Domain Pack)

V1: Web SaaS/SI만 stable (Feature/APIEndpoint/Screen/DataModel).
V1.x+: Game/ML/Mobile skeleton 성숙.
V2+: CS Desktop/Library/Embedded, Tier C Custom.

---

## Area (Project 하위 수직 구분)

```
Area {
  id: string
  project_id: ProjectRef             // 필수, Project 하위
  name: string                       // "Payment"
  slug: string
  parent: AreaRef?                   // sub-area
  description: markdown?
  owner: AgentRef | UserRef?
  
  created_at: timestamp
  created_by: AgentRef               // Area 생성도 agent-only
  last_modified_via: AgentRef
}
```

- 모든 Artifact는 하나의 Area에 속함 (미지정 시 `/Misc`)
- Area는 Project 하위 스코프 (Project A의 `/Cart`와 Project B의 `/Cart`는 별개)
- 신규 Area 생성은 Write-Intent Router 통과 + `sensitive_ops: confirm` 이면 Review Queue

---

## Project Tree

```
ProjectTree {
  project_id: ProjectRef
  tier_a_types: ArtifactType[]       // 고정
  active_domain_packs: DomainPack[]  // Project.active_domain_packs 참조
  areas: Area[]
  layout_preference: "type_first" | "area_first"
}
```

---

## Pin (Hard)

```
Pin {
  repo: string
  ref_type: "commit" | "branch" | "pr" | "path_only"
  commit_sha: string?
  branch: string?
  pr_number: int?
  paths: string[]
  pinned_at: timestamp
  pinned_by: AgentRef
}
```

Stale 감지 대상.

---

## Related Resource (Soft)

```
ResourceRef {
  type: "code" | "asset" | "api" | "doc" | "link"
  ref: string
  purpose: string
  added_at: timestamp
  added_by: AgentRef
  last_verified_at: timestamp?
  verified_status: "valid" | "broken" | "stale" | "unverified"
}
```

Stale 감지 아님. Fast Landing + 사이드 패널. **M7 Freshness Re-Check**로 주기 검증.

`completeness == "draft"` artifact의 링크는 UI disabled.

---

## Intent

```
Intent {
  kind: "new" | "modification" | "split" | "supersede"
  target_type: ArtifactType
  project_id: ProjectRef             // 필수
  target_scope: string[]             // Area 경로
  target_id: string?
  source_ids: string[]?
  reason: string
  related_session: SessionRef?
  declared_by: AgentRef
  declared_at: timestamp
}
```

---

## Graph Edge Types

```
EdgeType:
  - references
  - derives_from
  - validates
  - implements
  - supersedes
  - pinned_to
  - related_resource
  - blocked_by
  - relates_to
  - continuation_of
```

**Cross-project edges 허용** — FE Feature가 BE API를 `references` 하는 경우. 단 에이전트는 token이 양쪽 Project에 read 권한 있어야 선언 가능.

---

## Event / Notification 모델

Event Bus가 발행하는 시스템 이벤트.

```
Event {
  id: string
  timestamp: timestamp
  type: EventType
  project_id: ProjectRef
  source_ref: ArtifactRef | PinRef | SessionRef | ResourceRef
  payload: object                    // type별 상세
  severity: "info" | "low" | "medium" | "high"
  subscribers_notified: SubscriberRef[]
}

EventType (V1):
  # Artifact 라이프사이클
  - artifact.published            # auto_published 완료
  - artifact.stale_detected       # pin된 코드 변경 감지
  - artifact.superseded
  - artifact.archived
  
  # Pin / Git
  - pin.changed                   # 의미 변경 판정 통과
  - git.push_received             # webhook 수신
  
  # TC
  - tc.failed
  - tc.run_completed
  
  # Resource
  - resource.verified             # M7 결과
  - resource.broken               # ref 파일 없음 감지
  
  # Review
  - review.required               # sensitive op이 Review Queue로
  - review.approved
  - review.rejected
  
  # Project / Area
  - project.area_created
  - project.member_added
  
  # Session
  - session.started
  - session.ended
  - session.promoted_artifact
}
```

**Subscriber 모델** (V1 기본, V1.1에서 UI 확장):

```
EventSubscription {
  id: string
  project_id: ProjectRef
  principal: UserRef | AgentRef | WebhookRef
  event_types: EventType[]           // 구독할 타입들
  filter: JsonLogic?                 // 예: severity="high"
  channel: "ui_inbox" | "webhook" | "email"
  created_at: timestamp
}
```

**V1 구현 범위**:
- Event Bus 인프라 (Postgres LISTEN/NOTIFY 또는 outbox 패턴)
- UI Inbox 채널 (Stale Dashboard, Review Queue가 구독)
- Webhook 채널 (간단한 HTTP POST)

**V1.1**:
- Email 채널
- Slack/Discord 봇 채널
- Smart filter UI

---

## Session

```
Session {
  id: string
  project_id: ProjectRef             // 필수
  agent_id: string
  started_at: timestamp
  ended_at: timestamp?
  user: UserRef
  
  turns: Turn[]
  
  working_directory: string?
  git_context: { repo, branch, commit }?
  
  promoted_artifacts: ArtifactRef[]
  
  // F6 검색 강화
  auto_tags: string[]
  topics: string[]
  embeddings: VectorRef?
  
  retention_days: int                // 기본 90
}
```

---

## Continuation Context

```
ContinuationContext {
  artifact: Artifact
  project: Project                   // 소속 Project 정보
  neighbors: Artifact[]
  recent_changes: Event[]            // 최근 관련 이벤트
  open_questions: string[]
  source_session: SessionRef?
  related_resources: ResourceRef[]   // Fast Landing 번들
  area_context: {
    area: Area,
    sibling_artifacts: ArtifactRef[]
  }
}
```

---

## 예시: Multi-project 시나리오

**shop-fe** Project와 **shop-be** Project가 공존, Alice/Bob 팀:

```
Project: shop-fe
├─ active_domain_packs: [web-saas]
├─ Areas: /Cart, /Payment, /Auth, /Misc
├─ Members: Alice(admin+approver), Bob(reader),
│           Alice-claude(writer), Bob-cursor(reader)
└─ Artifacts:
    └─ Feature "장바구니 재시도 UI" [Area: /Cart]
        └─ references → shop-be:API "POST /cart/retry"  (cross-project)

Project: shop-be
├─ active_domain_packs: [web-saas]
├─ Areas: /Cart, /Payment, /Auth
├─ Members: Alice(approver), Bob(admin+approver+writer(Bob-cursor))
└─ Artifacts:
    └─ APIEndpoint "POST /cart/retry" [Area: /Cart]
        └─ referenced_by ← shop-fe:Feature "장바구니 재시도 UI"
```

Event 흐름 예:
- Bob이 shop-be `POST /cart/retry`의 스키마 변경 → `pin.changed` 이벤트
- Propagation Ledger가 cross-project edge 따라 shop-fe의 관련 Feature에 `artifact.stale_detected` 전파
- Alice의 UI Inbox에 알림 → "FE쪽 확인 필요"
