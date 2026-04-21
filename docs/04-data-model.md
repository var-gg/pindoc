# 04. Data Model

Varn의 데이터 모델. Artifact 타입 스키마, TC 구조, 그래프 엣지를 정의합니다.

> 이 문서는 V1 기준 논리 모델입니다. 물리 스키마(테이블 DDL)는 구현 단계에서 별도 정의.

## 공통 필드 (모든 Artifact 공통)

```
Artifact {
  id: string                  // "doc_a3f5e2c" / "task_b1c2d3" / "tc_e4f5g6"
  type: ArtifactType          // "Document/Analysis" | "Task" | "TC" ...
  title: string
  slug: string                // URL-safe identifier
  created_at: timestamp
  updated_at: timestamp
  created_by: Actor           // user | agent
  version: int                // 수정 시마다 증가
  
  // 타입별 본문
  body: TypedBody             // 타입에 따라 다른 스키마
  
  // 고정(pinning)
  pins: Pin[]
  
  // 관계
  references: ArtifactRef[]   // 이 artifact가 인용하는 것들
  referenced_by: ArtifactRef[] // 이 artifact를 인용하는 것들 (역참조)
  
  // 상태
  status: "draft" | "published" | "stale" | "superseded" | "archived"
  stale_reason: string?       // status가 stale일 때
  superseded_by: ArtifactRef? // status가 superseded일 때
  
  // 출처
  source_session: SessionRef? // 어느 세션에서 promote됐는지
  promote_intent: Intent?     // promote 당시의 intent 스냅샷
  
  // 태깅
  scope: string[]             // 스코프 태그 (예: ["payment", "checkout"])
  labels: string[]            // 자유 태그
}
```

---

## Artifact Types (V1)

### Document/Analysis — 코드/시스템/이슈 분석

```
body: {
  problem: markdown           // 무엇이 문제인가 (필수)
  context: markdown           // 배경, 현재 상황 (필수)
  findings: markdown          // 분석 결과 (필수)
  diagrams: Mermaid[]         // 0개 이상
  recommendations: markdown?  // 선택
  open_questions: string[]    // 남은 질문들
}
```

**필수 필드 검증**: `problem`, `context`, `findings`가 비어있으면 publish 실패.

---

### Document/ADR — Architecture Decision Record

```
body: {
  decision_status: "proposed" | "accepted" | "deprecated" | "superseded"
  context: markdown           // 왜 이 결정이 필요한가 (필수)
  decision: markdown          // 무엇을 결정했는가 (필수)
  consequences: markdown      // 긍정/부정적 결과 (필수)
  alternatives: markdown?     // 고려했던 대안들
  date_decided: date
}
```

**필수**: `context`, `decision`, `consequences`. ADR인데 consequences 비어있으면 publish 실패.

---

### Document/Flow — 플로우 문서

```
body: {
  overview: markdown          // 흐름의 개요 (필수)
  diagram: Mermaid            // 플로우 다이어그램 (필수, 1개 이상)
  steps: Step[]?              // 선택: 단계별 설명
  actors: string[]            // 참여 주체 (예: ["User", "PaymentGateway", "DB"])
  triggers: string[]          // 어떤 이벤트로 이 흐름이 시작되는가
  edge_cases: markdown?       // 예외 상황
}
```

**특이점**: Flow 타입은 **mermaid 다이어그램이 필수**. 텍스트만으로는 publish 불가. 이것이 Flow의 정체성.

---

### Document/Debug — 디버깅 세션 요약

```
body: {
  symptom: markdown           // 증상 (필수)
  reproduction: markdown      // 재현 단계 (필수)
  hypotheses_tried: Hypothesis[]  // 시도한 가설들 (최소 1개)
  root_cause: markdown?       // 근본 원인 (해결된 경우)
  resolution: markdown?       // 해결책 (해결된 경우)
  status: "open" | "resolved" | "workaround"
  related_errors: string[]    // 에러 메시지, 로그 스니펫
}

Hypothesis {
  statement: string
  tested: bool
  result: "confirmed" | "rejected" | "inconclusive"
  evidence: markdown
}
```

**가치**: 버려진 가설까지 보존해서 **"왜 다른 접근은 안 됐는지"** 가 남음. 같은 버그 재발 시 시간 절약.

---

### Document/Feature — 피쳐 개요

```
body: {
  overview: markdown          // 피쳐 개요 (필수)
  scope: markdown             // 무엇을 포함하고 무엇을 제외하는가 (필수)
  acceptance_criteria: string[]  // 완료 조건 (필수, 1개 이상)
  dependencies: ArtifactRef[]    // 의존하는 다른 artifact
  status: "planned" | "in_progress" | "shipped" | "deprecated"
}
```

Feature는 Task와 TC의 허브 역할. Feature에 연결된 TC가 전부 pass여야 Feature가 "shipped" 가능.

---

### Task

```
body: {
  description: markdown
  acceptance_criteria: string[]?
  assignee: User?
  estimated_hours: number?
  priority: "low" | "medium" | "high" | "urgent"
  status: "todo" | "in_progress" | "blocked" | "done" | "cancelled"
  blocked_reason: string?     // blocked일 때
  
  // 에이전트 작업 로그 (1급 필드)
  agent_attempts: AgentAttempt[]
}

AgentAttempt {
  id: string
  started_at: timestamp
  ended_at: timestamp?
  agent_id: string            // "claude-code-user@foo" 등
  approach: markdown          // 어떤 접근을 했는지
  session_ref: SessionRef     // 어느 세션에서 작업했는지
  outcome: "success" | "partial" | "failure" | "abandoned"
  notes: markdown?
}
```

**특이점**: `agent_attempts`가 1급 필드. 기존 task manager들은 이걸 댓글로 처리하지만 Varn은 구조화. 여러 에이전트가 병렬로 같은 태스크에 붙은 경우 각 시도가 분리돼 기록됨.

---

### TestCase (TC)

```
body: {
  title: string
  description: markdown
  
  // 실행 주체
  executable_by: "agent" | "human_e2e" | "hybrid"
  
  // 자동화 정보 (executable_by !== "human_e2e"인 경우)
  automation: {
    type: "unit" | "integration" | "e2e_automated"
    script_path: string?      // 저장소 내 경로
    runner: string?           // "jest" | "pytest" | "playwright" 등
  }?
  
  // 사람 실행 정보 (executable_by !== "agent"인 경우)
  manual_steps: Step[]?
  expected_result: markdown?
  
  // 실행 이력
  runs: TCRun[]
  
  // 필수 여부
  required_for_close: bool    // 연결된 Feature/Task close 조건으로 강제할지
  
  // 상태 (마지막 run 기준)
  last_status: "pending" | "passing" | "failing" | "blocked" | "stale"
}

TCRun {
  run_at: timestamp
  executed_by: Actor
  result: "pass" | "fail" | "error" | "skip"
  duration_ms: number?
  output: markdown?
  commit: string?             // 어느 커밋에서 실행됐는지
}
```

**핵심 제약**: Feature의 `status`를 `shipped`로 바꾸려면 → 연결된 TC 중 `required_for_close=true`인 것들이 전부 `last_status=passing`이어야 함. 그렇지 않으면 API/UI에서 거부.

---

## Pin 구조

```
Pin {
  repo: string                // "company/main-app"
  ref_type: "commit" | "branch" | "pr" | "path_only"
  commit_sha: string?         // ref_type이 commit일 때
  branch: string?
  pr_number: int?
  paths: string[]             // glob 패턴 가능
  pinned_at: timestamp
  pinned_by: Actor
}
```

**동작**:
- 해당 repo에 push가 감지되면 paths의 diff 확인
- 의미 있는 변경이면 → artifact에 `stale` 플래그
- "의미 있는"의 기준은 [05 Mechanisms](05-mechanisms.md#stale-detection)에서

---

## Intent 구조

```
Intent {
  kind: "new" | "modification" | "split" | "supersede"
  target_type: ArtifactType
  target_scope: string[]      // 이번 write의 영역
  
  // kind별 추가 필드
  target_id: string?          // modification/supersede일 때 대상 artifact ID
  source_ids: string[]?       // split일 때 원본 artifact들
  
  reason: string              // 왜 이렇게 분류했는지 자연어 설명
  related_session: SessionRef?
  declared_by: Actor
  declared_at: timestamp
}
```

**심사 흐름**: [05 Mechanisms - Write-Intent Router](05-mechanisms.md#write-intent-router) 참조.

---

## Graph Edge Types

```
Edge {
  from: ArtifactRef
  to: ArtifactRef
  type: EdgeType
  created_at: timestamp
  created_by: Actor
  metadata: object?
}

EdgeType:
  - references         // 이 artifact가 저 artifact를 인용
  - derives_from       // 이 artifact가 저 artifact/session에서 파생
  - validates          // 이 TC가 저 Feature/Task를 검증
  - implements         // 이 Task가 저 Feature의 일부
  - supersedes         // 이 artifact가 저 artifact를 대체
  - pinned_to          // 이 artifact가 저 코드 경로에 고정
  - blocked_by         // 이 Task가 저 Task에 블록됨
  - relates_to         // 약한 관련성
```

**질의 예시**:

- "이 Feature의 모든 구현 Task": `Feature -implements-reverse-> Tasks`
- "이 파일 경로 관련된 최신 artifact 5개": `File -pinned_to-reverse-> Artifacts ORDER BY recency`
- "이 버그의 뿌리": `Debug -supersedes-ancestors-> older Debugs`

---

## Session 구조

```
Session {
  id: string
  agent_id: string            // "claude-code-user@alice"
  started_at: timestamp
  ended_at: timestamp?
  user: User
  
  // 원본 로그
  turns: Turn[]               // 대화 턴들
  
  // 메타
  working_directory: string?
  git_context: {
    repo: string?
    branch: string?
    commit: string?
  }?
  
  // Promote 결과
  promoted_artifacts: ArtifactRef[]
  
  // 보존
  retention_days: int         // 기본 90, 설정 가능
}

Turn {
  role: "user" | "agent" | "tool"
  content: markdown
  timestamp: timestamp
  tool_calls: ToolCall[]?
}
```

---

## 스키마 확장성

V1 타입 외에 커스텀 타입 추가는 **V2 이후**.

V1에서는 위 6개 Document 타입 + Task + TC만 제공. 스코프 확장보다 **있는 타입을 깊게 만드는 것**이 우선.

V2에서 커스텀 타입을 허용하더라도:
- 타입 정의는 YAML/JSON 스키마로
- 필수 필드 validator 의무
- Graph edge type 등록 필요

"자유롭게 뭐든 쓰는 Notion"이 되면 안 되기 때문에 스키마 확장은 신중히.

---

## 예시: 하나의 Feature 전체 그림

"결제 재시도 로직 개선" Feature가 완성된 후의 상태:

```
Feature: "Payment Retry Logic"  [status: shipped]
├── references: ADR-042 (Retry policy decision)
├── pinned_to: src/payment/retry.ts @ commit a3f5e2c
├── implements 관계의 Tasks:
│   ├── Task: "Exponential backoff 구현"  [done]
│   ├── Task: "Idempotency key 도입"      [done]
│   └── Task: "Dead letter queue 연결"    [done]
├── validates 관계의 TCs:
│   ├── TC: "Retry 3회 후 실패" (required, passing)
│   ├── TC: "네트워크 타임아웃 재시도" (required, passing)
│   └── TC: "중복 결제 방지 E2E" (required, human_e2e, passing)
└── derives_from:
    ├── Analysis: "결제 실패율 분석 리포트"
    └── Debug: "PG사 타임아웃 디버깅 세션"
```

이 그래프가 곧 **"이 팀이 이 피쳐에 대해 축적한 기억 전체"** 입니다.
새 에이전트가 결제 관련 작업을 시작하면, 이 번들이 컨텍스트로 주입됩니다.
