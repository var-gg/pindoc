# 10. MCP Tools Spec

Pindoc V1 MCP Tool 전체 스펙. Input/Output 스키마 + 예시 + 에러 케이스.

## 공통 규약

### 인증

모든 tool 호출은 **Agent Token** 필요. MCP config의 `Authorization: Bearer pindoc_{token}` 헤더로 전송.

### Project Scope

대부분 tool 은 **현재 활성 Project** 기준으로 동작. `pindoc.project.switch` 로 활성 전환. 명시 시 `project_id` 파라미터로 override 가능 (권한 있을 때만).

### 응답 공통 필드

```typescript
{
  status: "ok" | "error" | "not_ready",
  request_id: string,           // 추적용
  warnings?: string[],          // 비차단 경고
  error?: { code, message, details? }  // status="error"일 때
  // tool별 추가 필드
}
```

### 에러 코드

| Code | 의미 |
|---|---|
| `unauthorized` | Token 없음/만료 |
| `forbidden` | 이 Project에 권한 없음 |
| `not_found` | Artifact/Area/Session 없음 |
| `conflict` | Write-Intent Router 충돌 |
| `schema_invalid` | 필수 필드 누락 |
| `not_ready` | Pre-flight Check 미통과 (아래 별도) |
| `rate_limit` | 요청 제한 초과 |
| `server_error` | 5xx |

### `not_ready` 응답 (Pre-flight Check)

`pindoc.artifact.propose` 등 write tool에서 특화 응답:

```typescript
{
  status: "not_ready",
  request_id: string,
  draft_id: string,                    // 부분 저장된 draft
  checklist: Array<{
    item: string,                      // "alternatives 최소 2개 탐색?"
    passed: boolean,
    hint?: string                      // "pindoc.artifact.search(type=ADR, area=/Payment) 호출 권장"
  }>,
  suggested_next_tools: ToolCallHint[]
}
```

에이전트는 checklist 미통과 항목을 처리한 뒤 `draft_id` 를 포함해 재제출.

---

## Tool Catalog (V1)

| # | Tool | 권한 |
|---|------|------|
| 1 | `pindoc.harness.install` | CLI only (서버 어드민) |
| 2 | `pindoc.project.list` | reader+ |
| 3 | `pindoc.project.switch` | reader+ |
| 4 | `pindoc.artifact.search` | reader+ |
| 5 | `pindoc.artifact.propose` | writer |
| 6 | `pindoc.artifact.read` | reader+ |
| 7 | `pindoc.graph.neighbors` | reader+ |
| 8 | `pindoc.context.for_task` | reader+ |
| 9 | `pindoc.resource.verify` | writer |
| 10 | `pindoc.area.propose` | writer |
| 11 | `pindoc.tc.register` | writer |
| 12 | `pindoc.tc.run_result` | writer |

내부 전용 (MCP 공개 X): `artifact.commit`, `artifact.archive`, `area.delete`. Review Queue 승인 시 서버 내부 호출.

---

## 1. `pindoc.harness.install`

> `pindoc init` CLI가 서버와 통신해 PINDOC.md 생성 + CLAUDE.md 주입. 사용자 Agent Token 발급 이후에 호출.

### Input

```typescript
{
  project_id: string,
  working_directory: string,     // 로컬 프로젝트 루트 (상대 경로 주입용)
  target_files: ["CLAUDE.md", "AGENTS.md", ".cursorrules"],
  mcp_clients: ["claude-code", "cursor", "cline", "codex"]  // 감지된 것
}
```

### Output

```typescript
{
  status: "ok",
  pindoc_md_content: string,     // PINDOC.md 본문
  injection_snippets: Array<{
    file: "CLAUDE.md" | ...,
    snippet: string              // "See ./PINDOC.md for this project's agent protocol."
  }>,
  mcp_configs: Array<{
    client: "claude-code" | ...,
    config_path: string,         // 예: "~/.config/claude-code/mcp.json"
    config_fragment: object      // 주입할 JSON
  }>
}
```

### 에러

- `forbidden` — token이 admin role 아님

---

## 2. `pindoc.project.list`

### Input

```typescript
{}   // token으로 사용자 식별, 접근 가능 project 반환
```

### Output

```typescript
{
  status: "ok",
  projects: Array<{
    id: string,
    slug: string,
    name: string,
    role: "admin" | "writer" | "approver" | "reader",
    active: boolean,             // 현재 활성 여부
    icon?: string,
    active_packs: DomainPack[]
  }>
}
```

---

## 3. `pindoc.project.switch`

### Input

```typescript
{
  project_id: string | project_slug: string
}
```

### Output

```typescript
{
  status: "ok",
  active_project: Project,
  pindoc_md_url: string          // 현재 project의 PINDOC.md URL
}
```

### 에러

- `not_found` / `forbidden`

---

## 4. `pindoc.artifact.search`

### Input

```typescript
{
  query: string,                 // 자연어
  filters?: {
    type?: ArtifactType[],
    area?: AreaRef[],
    completeness?: Array<"draft" | "partial" | "settled">,
    status?: Array<"published" | "stale" | "superseded" | "archived">,
    created_after?: timestamp,
    created_before?: timestamp
  },
  semantic: boolean,             // true = pgvector 의미 검색, false = 키워드만
  limit?: number,                // 기본 10
  scope?: "current_project" | "cross_project"
}
```

### Output

```typescript
{
  status: "ok",
  results: Array<{
    artifact_id: string,
    url: string,
    title: string,
    type: ArtifactType,
    area: AreaRef,
    completeness: Completeness,
    status: Status,
    relevance_score: number,     // 0~1
    snippet: string              // 매치된 컨텍스트
  }>,
  total: number
}
```

### 예시

```typescript
// Request
{
  query: "PG 타임아웃 재시도",
  filters: { type: ["Debug", "Analysis", "ADR"], area: ["/Payment"] },
  semantic: true,
  limit: 5
}

// Response
{
  status: "ok",
  results: [
    {
      artifact_id: "doc_debug_abc",
      url: "https://pindoc.example.com/a/doc_debug_abc",
      title: "PG사 API 타임아웃 재시도 오류",
      type: "Debug",
      area: "/Payment",
      completeness: "partial",
      status: "published",
      relevance_score: 0.92,
      snippet: "...결제 요청 중 3%가 504 Gateway Timeout..."
    },
    ...
  ],
  total: 3
}
```

---

## 5. `pindoc.artifact.propose`

> Promote의 엔트리 포인트. Pre-flight Check가 NOT_READY를 반환할 수 있음.

### Input

```typescript
{
  intent: {
    kind: "new" | "modification" | "split" | "supersede",
    target_type: ArtifactType,
    target_area: AreaRef,
    target_id?: string,               // modification/supersede
    source_ids?: string[],            // split
    reason: string,
    related_session: SessionRef
  },
  body: TypedBody,                    // 타입별 스키마
  pins?: Pin[],
  related_resources?: ResourceRef[],
  completeness: "draft" | "partial" | "settled",   // settled는 sensitive
  draft_id?: string                   // 재제출 시
}
```

### Output — READY (성공 + auto-publish)

```typescript
{
  status: "ok",
  artifact_id: string,
  url: string,                        // 발행된 artifact URL
  review_state: "auto_published",
  graph_updates: {
    edges_created: Edge[],
    stale_triggered: ArtifactRef[]    // 영향 전파
  },
  derived_suggestions?: Array<{       // 파생 제안 (TC, Task)
    type: ArtifactType,
    title_hint: string,
    area: AreaRef
  }>
}
```

### Output — Sensitive Op + `confirm` 모드

```typescript
{
  status: "ok",
  draft_id: string,
  url: string,                        // Review Queue preview URL
  review_state: "pending_review",
  estimated_reviewers: UserRef[]      // approver role 사용자
}
```

### Output — NOT_READY (Pre-flight Check)

```typescript
{
  status: "not_ready",
  draft_id: string,
  checklist: Array<{ item, passed, hint? }>,
  suggested_next_tools: [...]
}
```

### Output — Conflict

```typescript
{
  status: "error",
  error: {
    code: "conflict",
    message: "관련 artifact 2개 발견 (유사도 0.85+)",
    details: {
      conflicts: Array<{
        artifact_id, url, title, similarity,
        suggested_actions: ["update_existing", "prove_distinct"]
      }>
    }
  }
}
```

에이전트 대응: conflict 해결 (update_existing 으로 전환 또는 prove_distinct에 reason 첨부 후 재제출).

---

## 6. `pindoc.artifact.read`

> URL 또는 ID로 artifact + Continuation Context fetch.

### Input

```typescript
{
  url_or_id: string,             // "https://pindoc.example.com/a/xxx" 또는 "doc_xxx"
  include?: {                    // 기본 모두 포함
    neighbors: boolean,
    recent_changes: boolean,
    related_resources: boolean,
    source_session: boolean,
    area_context: boolean
  },
  neighbor_depth?: number        // 기본 1, 최대 3
}
```

### Output

```typescript
{
  status: "ok",
  artifact: Artifact,
  context: ContinuationContext
}
```

### 에러

- `not_found` — artifact 없음
- `forbidden` — cross-project인데 read 권한 없음

---

## 7. `pindoc.graph.neighbors`

### Input

```typescript
{
  artifact_id: string,
  edge_types?: EdgeType[],       // 필터링
  depth?: number,                // 기본 1
  direction?: "outgoing" | "incoming" | "both"
}
```

### Output

```typescript
{
  status: "ok",
  neighbors: Array<{
    artifact: ArtifactRef,
    edge_type: EdgeType,
    direction: "outgoing" | "incoming",
    distance: number             // hop count
  }>
}
```

---

## 8. `pindoc.context.for_task`

> Fast Landing. 자연어 task 설명으로 관련 artifact + 리소스 번들 반환.

### Input

```typescript
{
  task_description: string,      // "장바구니 재시도 로직"
  scope?: "current_project" | "cross_project",
  max_artifacts?: number,        // 기본 3
  max_resources?: number         // 기본 10
}
```

### Output

```typescript
{
  status: "ok",
  artifacts: Array<{
    artifact_id, url, title, type, area, relevance_score
  }>,
  resources: Array<{
    type: "code" | "asset" | "api" | "doc" | "link",
    ref: string,
    purpose: string,
    source_artifact: ArtifactRef,  // 어느 artifact의 related_resources에서 왔는지
    github_url?: string            // type=code + commit 알면 자동 생성
  }>,
  related_areas: AreaRef[]
}
```

---

## 9. `pindoc.resource.verify`

> M7 Freshness Re-Check 명시 트리거 (V1). V1.1에서 자동화.

### Input

```typescript
{
  artifact_id: string,
  mode: "verify_only" | "propose_updates"   // 기본 propose_updates
}
```

### Output

```typescript
{
  status: "ok",
  verified: Array<{
    resource_ref: ResourceRef,
    status: "valid" | "broken" | "renamed" | "stale",
    new_ref?: string,              // renamed일 때 새 경로
    diff_summary?: string          // stale일 때 변화 요약
  }>,
  proposed_updates?: Array<{
    action: "update_ref" | "remove" | "add_new",
    details: ResourceRef | { remove: ResourceRef } | { add: ResourceRef },
    requires_user_ok: boolean      // Referenced Confirmation 필요 여부
  }>
}
```

---

## 10. `pindoc.area.propose`

> 신규 Area 생성 요청. Write-Intent Router 통과 필수.

### Input

```typescript
{
  name: string,                  // "Observability"
  slug: string,                  // "observability"
  parent?: AreaRef,              // sub-area
  description?: string,
  reason: string                 // 왜 필요한지
}
```

### Output

- `pindoc.artifact.propose` 와 동일 구조 (ok / not_ready / conflict / pending_review).
- Conflict: 이름·slug 중복, 유사도 높은 Area 존재 시.

---

## 11. `pindoc.tc.register`

> 새 TC 등록.

### Input

```typescript
{
  linked_feature_id: string,
  body: TCBody,                  // executable_by, automation, manual_steps 등
  required_for_close: boolean
}
```

### Output

```typescript
{
  status: "ok",
  tc_id: string,
  url: string
}
```

---

## 12. `pindoc.tc.run_result`

> TC 실행 결과 보고.

### Input

```typescript
{
  tc_id: string,
  run_at: timestamp,
  executed_by: AgentRef | UserRef,
  result: "pass" | "fail" | "error" | "skip",
  duration_ms?: number,
  output?: string,
  commit?: string
}
```

### Output

```typescript
{
  status: "ok",
  tc_last_status: TCStatus,
  feature_close_eligibility?: {
    feature_id: string,
    closable: boolean,
    pending_tcs: TCRef[]
  }
}
```

---

## Rate Limiting (V1)

| 영역 | 기본 제한 |
|------|---------|
| Read tools (search, read, graph.neighbors, context, project.list) | 600/min per token |
| Write tools (propose, tc.register, resource.verify) | 60/min per token |
| `pindoc.harness.install` | 10/min per token |

초과 시 `429 rate_limit` 응답 + `Retry-After` 헤더.

---

## Versioning

- Tool 네임스페이스에 버전 미포함 (path 단순화 위해)
- 서버 응답 헤더 `X-Pindoc-Version: 1.0.0`
- Breaking change 시 새 tool 추가 (예: `pindoc.artifact.propose_v2`) + 구 tool deprecation 표시
- PINDOC.md 의 `pindoc_version` 과 서버 버전 불일치 시 경고 (호환 범위 내에서만)

---

## 관련 문서

- Harness 스펙: [09 PINDOC.md Spec](09-pindoc-md-spec.md)
- 아키텍처 전반: [03 Architecture](03-architecture.md)
- 데이터 모델: [04 Data Model](04-data-model.md)
- 메커니즘: [05 Mechanisms](05-mechanisms.md)
- 용어집: [Glossary](glossary.md)
