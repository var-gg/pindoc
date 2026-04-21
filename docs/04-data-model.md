# 04. Data Model

Varn의 데이터 모델. Tier A/B Artifact 스키마, Area, Pin/Related Resource, Graph 엣지를 정의합니다.

> 이 문서는 V1 기준 논리 모델입니다. 물리 스키마(테이블 DDL)는 구현 단계에서 별도 정의.

## 공통 필드 (모든 Artifact 공통)

```
Artifact {
  id: string                      // "doc_a3f5e2c" / "task_b1c2d3" 등
  type: ArtifactType              // Tier A core 또는 Tier B pack 중
  tier: "core" | "domain"         // Tier 구분
  title: string
  slug: string
  
  created_at: timestamp
  updated_at: timestamp
  
  // Agent-only write의 스키마 수준 보증
  created_by: AgentRef            // 반드시 agent (user 타입 거부)
  last_modified_via: AgentRef     // 모든 수정은 agent 경유
  source_session: SessionRef      // 세션 없이 write 금지 = agent-only 강제
  
  version: int
  body: TypedBody                 // 타입별 스키마
  
  // 고정 & 리소스
  pins: Pin[]                     // hard pin (stale 감지 대상)
  related_resources: ResourceRef[] // soft link (navigation, M7 주기 검증)
  
  // 분류
  area: AreaRef                   // 필수 (미지정 시 /Misc)
  labels: string[]                // 자유 태그 (scope은 area가 담당)
  
  // 관계
  references: ArtifactRef[]
  referenced_by: ArtifactRef[]
  
  // 상태
  completeness: "draft" | "partial" | "settled"   // 성숙도 (기본 partial)
  status: "published" | "stale" | "superseded" | "archived"
  stale_reason: string?
  superseded_by: ArtifactRef?
  
  // 승인
  promote_intent: Intent?
  approved_at: timestamp?
  approved_by: UserRef?           // 사람은 승인만 (write는 에이전트)
}
```

**Agent-only write의 스키마 강제 포인트**:
- `created_by`와 `last_modified_via`는 `AgentRef` 타입만 허용. User 타입은 스키마 수준 거부.
- `source_session` 필수 — 세션 컨텍스트 없이 write 경로 자체가 막힘.
- `approved_by`만 `UserRef`. 사람의 역할은 승인, 쓰기가 아님.

**completeness 규칙**:
- `draft`: 구조만 있음. UI 링크·Related Resource 전부 disabled.
- `partial`: 의미 있는 내용. 기본값. 링크 활성.
- `settled`: 사람이 "완결" 승인. 편집·supersede 제한.

---

## Artifact Types — Tier A (Core, 강제)

모든 install에 자동 장착. V1에서 완성 제공. 도메인 무관.

### Decision (ADR) — Architecture Decision Record

```
body: {
  decision_status: "proposed" | "accepted" | "deprecated" | "superseded"
  context: markdown              // 왜 이 결정이 필요한가 (필수)
  decision: markdown             // 무엇을 결정했는가 (필수)
  consequences: markdown         // 긍·부 결과 (필수)
  alternatives: Alternative[]    // 고려했던 대안 (Pre-flight: 최소 2개)
  date_decided: date
}

Alternative { name, summary, why_rejected }
```

### Analysis — 분석

```
body: {
  problem: markdown              // 필수
  context: markdown              // 필수
  findings: markdown             // 필수
  diagrams: Mermaid[]
  recommendations: markdown?
  open_questions: string[]
}
```

### Debug — 디버깅

```
body: {
  symptom: markdown              // 필수
  reproduction: markdown         // 필수
  hypotheses_tried: Hypothesis[] // 최소 1개
  root_cause: markdown?          // resolved 시 필수
  resolution: markdown?          // resolved 시 필수
  status: "open" | "resolved" | "workaround"
  related_errors: string[]
}

Hypothesis {
  statement, tested, result: "confirmed" | "rejected" | "inconclusive",
  evidence: markdown
}
```

### Flow — 플로우

```
body: {
  overview: markdown             // 필수
  diagram: Mermaid               // 필수, 1개 이상
  steps: Step[]?
  actors: string[]
  triggers: string[]
  edge_cases: markdown?
}
```

### Task

```
body: {
  description: markdown
  acceptance_criteria: string[]?
  assignee: AgentRef | UserRef?  // 사람 또는 에이전트 할당 가능
  estimated_hours: number?
  priority: "low" | "medium" | "high" | "urgent"
  status: "todo" | "in_progress" | "blocked" | "done" | "cancelled"
  blocked_reason: string?
  
  agent_attempts: AgentAttempt[]  // 1급 필드
}

AgentAttempt {
  id, started_at, ended_at, agent_id, approach, session_ref,
  outcome: "success" | "partial" | "failure" | "abandoned",
  notes
}
```

### TC (TestCase)

```
body: {
  title, description,
  executable_by: "agent" | "human_e2e" | "hybrid",
  
  automation: {                  // executable_by !== "human_e2e"
    type: "unit" | "integration" | "e2e_automated",
    script_path: string?,
    runner: "jest" | "pytest" | "playwright" | ...
  }?,
  
  manual_steps: Step[]?,         // executable_by !== "agent"
  expected_result: markdown?,
  
  runs: TCRun[],
  required_for_close: bool,
  last_status: "pending" | "passing" | "failing" | "blocked" | "stale"
}

TCRun {
  run_at, executed_by, result: "pass" | "fail" | "error" | "skip",
  duration_ms?, output?, commit?
}
```

**TC Gating** ([05 M5](05-mechanisms.md)): Feature close 조건으로 강제 (V1.1).

### Glossary — 용어 정의

```
body: {
  term: string                   // 필수
  definition: markdown           // 필수
  context: markdown?             // 어느 맥락에서 쓰이는지
  aliases: string[]?             // 동의어
  see_also: ArtifactRef[]?
}
```

용어 혼동 방지. 에이전트가 생소 용어 만나면 lookup → 없으면 Glossary artifact 신규 propose.

---

## Artifact Types — Tier B (Domain Pack)

Install 시 1개 이상 선택. V1에서 **Web SaaS/SI만 완성**, 나머지는 **skeleton** (기본 필드만 존재, 스키마 성숙은 V1.x+ 커뮤니티 기여).

### Web SaaS/SI Pack (V1 stable)

**Feature**
```
body: {
  overview: markdown             // 필수
  scope: markdown                // 필수
  acceptance_criteria: string[]  // 필수
  dependencies: ArtifactRef[]
  status: "planned" | "in_progress" | "shipped" | "deprecated"
}
```

**API Endpoint**
```
body: {
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
  path: string                   // 필수
  description: markdown          // 필수
  request_schema: json?
  response_schema: json?
  auth_required: bool
  rate_limit: string?
  deprecation: { since, replacement }?
}
```

**Screen/Page**
```
body: {
  route: string?
  description: markdown          // 필수
  wireframe: Mermaid?
  components: string[]
  states: string[]               // loading, error, empty, success
  linked_endpoints: ArtifactRef[]
}
```

**DataModel**
```
body: {
  entity: string                 // 필수
  fields: Field[]                // 필수
  relations: Relation[]
  storage: "postgres" | "redis" | "..."
  migrations: string[]
}
```

### Game Pack (V1.x+ skeleton)

필드 이름만 고정, 스키마 상세는 커뮤니티 성숙:
- `Feature`, `Mechanic`, `Level`, `Character`, `Asset`

### ML/AI Pack (V1.x+ skeleton)

- `Feature`, `Dataset`, `Model`, `Experiment`
- Hugging Face Model Card 포맷 호환 고려

### Mobile Pack (V1.x+ skeleton)

- `Feature`, `Screen`, `Service`, `NativeModule`

### CS Desktop / Library / Embedded Pack (V2+)

스켈레톤 정의만. 활성 기여자 등장 시 stable.

---

## Area (Project Tree Node)

```
Area {
  id: string
  name: string                   // "Payment", "Cart", "Auth"
  slug: string
  parent: AreaRef?               // sub-area 가능
  description: markdown?
  owner: AgentRef | UserRef?
  
  created_at: timestamp
  created_by: AgentRef           // Area도 에이전트가 만듬
  last_modified_via: AgentRef
}
```

- **모든 Artifact는 하나의 Area에 속함**. 미지정 시 시스템이 `/Misc`로 분류.
- Area 자체도 agent-only write. 신규 Area 생성은 Write-Intent Router 통과 (중복 이름 거부).
- UI 네비게이션 2축:
  - Type 축: `/Decision`, `/Debug`, `/Feature` 등
  - Area 축: `/Payment`, `/Cart` 아래에 모든 타입 노출
- Scope 거버넌스는 Area로 해결됨 — 기존 자유 `scope: string[]`가 Area 경로로 승격.

## Project Tree

```
ProjectTree {
  tier_a_types: ArtifactType[]   // 항상 고정
  active_domain_packs: DomainPack[]
  areas: Area[]
  layout: "type_first" | "area_first"   // UI 기본
}

DomainPack {
  name: "web-saas" | "game" | "ml" | "mobile" | "cs-desktop" | "library" | "embedded"
  version: string
  types: ArtifactType[]
  status: "stable" | "skeleton"
}
```

V1: Web SaaS Pack `stable`, 나머지 `skeleton`.

---

## Pin — Hard (stale 감지 대상)

```
Pin {
  repo: string                   // "company/main-app"
  ref_type: "commit" | "branch" | "pr" | "path_only"
  commit_sha: string?
  branch: string?
  pr_number: int?
  paths: string[]                // glob 가능
  pinned_at: timestamp
  pinned_by: AgentRef
}
```

- 해당 repo push 감지 → paths diff 확인 → 의미 변경이면 `stale` 플래그
- stale 판정 기준: [05 M3](05-mechanisms.md)

---

## Related Resource — Soft Link (NEW)

```
ResourceRef {
  type: "code" | "asset" | "api" | "doc" | "link"
  ref: string                    // path 또는 URL
  purpose: string                // 왜 관련인지 한 줄
  added_at: timestamp
  added_by: AgentRef
  
  // M7 Freshness Re-Check 결과
  last_verified_at: timestamp?
  verified_status: "valid" | "broken" | "stale" | "unverified"
}
```

**Pin과의 차이**:

| | Pin | ResourceRef |
|---|-----|-------------|
| 의미 | Hard pin (정합 필수) | Soft link (맥락 navigation) |
| Stale 감지 | ✅ 자동 | ❌ (M7으로 주기 검증) |
| 주 용도 | 문서-코드 정합 보증 | Fast Landing, UI 패널 |
| UI 표현 | 본문 헤더 메타 | 사이드 패널 "Related Resources" |

**GitHub Outbound 링크**: `type: "code"` + repo/commit/path → UI 클릭 시 `https://github.com/.../blob/COMMIT/PATH#L10-L30`.

**Disabled 규칙**: `completeness == "draft"` artifact의 ResourceRef 링크는 UI에서 disabled (아직 신뢰 불가).

**본문 vs 메타 분리**: ResourceRef는 markdown 본문에 섞지 않고 별도 필드. UI에서도 별도 섹션 렌더.

---

## Intent

```
Intent {
  kind: "new" | "modification" | "split" | "supersede"
  target_type: ArtifactType
  target_scope: string[]         // Area 경로 (필수)
  target_id: string?             // modification/supersede 시
  source_ids: string[]?          // split 시
  reason: string                 // 자연어 설명
  related_session: SessionRef?
  declared_by: AgentRef
  declared_at: timestamp
}
```

심사: [05 M1](05-mechanisms.md).

---

## Graph Edge Types

```
EdgeType:
  - references              // 이 artifact가 저 것을 인용
  - derives_from            // 파생 (artifact or session)
  - validates               // TC가 Feature 검증
  - implements              // Task가 Feature의 일부
  - supersedes              // 대체
  - pinned_to               // Hard pin
  - related_resource        // Soft link (NEW)
  - blocked_by              // Task-level
  - relates_to              // 약한 관련성
  - continuation_of         // 이 세션이 저 artifact에서 이어짐
```

**질의 예시**:
- "이 Feature의 모든 구현 Task": `Feature -implements-reverse-> Tasks`
- "이 파일 경로와 연결된 최신 artifact 5개": `File -pinned_to|related_resource-reverse-> Artifacts`
- "이 artifact의 continuation 세션들": `Artifact -continuation_of-reverse-> Sessions`

---

## Session

```
Session {
  id: string
  agent_id: string               // "claude-code@alice" 등
  started_at: timestamp
  ended_at: timestamp?
  user: UserRef
  
  turns: Turn[]
  
  working_directory: string?
  git_context: { repo, branch, commit }?
  
  // Promote 결과
  promoted_artifacts: ArtifactRef[]
  
  // 검색 강화 (F6)
  auto_tags: string[]            // 자동 추출 태그
  topics: string[]               // 의미 클러스터링 결과
  embeddings: VectorRef?         // 의미 검색용
  
  retention_days: int            // 기본 90
}

Turn {
  role: "user" | "agent" | "tool"
  content: markdown
  timestamp: timestamp
  tool_calls: ToolCall[]?
}
```

---

## Continuation Context

```
ContinuationContext {
  artifact: Artifact
  neighbors: Artifact[]          // graph 직접 이웃 N개
  recent_changes: Event[]
  open_questions: string[]       // body.open_questions 종합
  source_session: SessionRef?
  related_resources: ResourceRef[]   // Fast Landing 번들
  area_context: {
    area: Area,
    sibling_artifacts: ArtifactRef[]
  }
}
```

사용자가 URL을 에이전트 채팅에 던지면 에이전트가 `varn.wiki.read(url)` → 이 번들 수령 → 대화 재개. 딥링크 불필요.

---

## 예시: Feature 전체 그림

"결제 재시도 로직 개선" Feature 완성 후:

```
Feature: "Payment Retry Logic"     [completeness: settled, status: shipped]
├── tier: domain (web-saas)
├── area: /Payment
├── references: ADR-042 (Retry policy)
├── pins:                           # hard (stale 감지)
│   └── commit a3f5e2c : src/payment/retry.ts
├── related_resources:              # soft (Fast Landing, M7 주기 검증)
│   ├── code: src/payment/gateway.ts (purpose: "PG API wrapper")
│   ├── code: src/api/payment.ts (purpose: "public endpoint")
│   ├── asset: docs/img/retry-flow.png
│   └── doc: https://pg-provider.example/docs/retry
├── implements 관계의 Tasks:
│   ├── Task: "Exponential backoff 구현" [done]
│   ├── Task: "Idempotency key 도입"    [done]
│   └── Task: "Dead letter queue 연결"  [done]
├── validates 관계의 TCs:
│   ├── TC: "Retry 3회 후 실패" [required, passing]
│   ├── TC: "네트워크 타임아웃 재시도" [required, passing]
│   └── TC: "중복 결제 방지 E2E" [required, human_e2e, passing]
└── derives_from:
    ├── Analysis: "결제 실패율 분석"
    └── Debug: "PG사 타임아웃 디버깅"
```

이 그래프가 곧 **"이 팀(또는 이 사용자)이 결제 재시도에 대해 축적한 기억 전체"** 이며, 새 에이전트 세션은 이 번들을 Continuation Context로 자동 수령합니다.
