# 10. MCP Tools Spec

Pindoc V1 MCP Tool 전체 스펙. Input/Output 스키마 + 예시 + 에러 케이스.

> **⚠️ 스펙과 런타임 구현의 관계**: 이 문서는 V1 완성 시점의 **aspirational 계약**이다.
> 현 시점 (2026-04-26, M1 dogfood 중) 실제 구현 여부는 아래 §Implementation Status 표 참조.
> Tool별 섹션 제목 옆 뱃지로도 표시한다:
> - ✅ **implemented** — 런타임에 등록되어 바로 호출 가능
> - 🟡 **partial** — 일부 동작하나 스펙과 drift 있음 (섹션 하단에 drift 주석)
> - 📋 **planned** — 런타임 미등록. V1.x+에서 도입 예정
> - 🚫 **obsolete** — 후속 Decision으로 폐기. runtime/schema에 맞춰 재작성 필요

## Implementation Status (2026-04-26)

| Tool | 상태 | 비고 |
|---|---|---|
| `pindoc.ping` | ✅ implemented | Phase 1 핸드쉐이크용. §Tool Catalog 외 (handshake-only). |
| `pindoc.harness.install` | ✅ implemented | `pindoc init` CLI 없이 MCP 호출만으로 PINDOC.md body 반환. 파일 쓰기는 에이전트 책임. 출력에는 workspace detection용 YAML frontmatter(project_slug/project_id/locale/schema_version)와 chip-driven Task lifecycle Section 12가 포함된다. |
| `pindoc.project.current` | ✅ implemented | account-level + per-call project scope. stdio와 streamable_http 모두 `scope_mode: "per_call"`, `new_project_requires_reconnect: false`; HTTP 데몬 URL은 단일 `/mcp`이고 각 tool input의 `project_slug`가 scope를 결정한다. `auth_mode`는 V1에서 `"trusted_local"`. Phase 9부터 `capabilities` 블록, Phase 14a에서 `receipt_ttl_sec: 1800`, `requires_expected_version: true`, `public_base_url`(server_settings) 추가, `auth_mode`는 Phase 14a rename from `"none"`. |
| `pindoc.project.create` | ✅ implemented | Phase 8 신규. 프로젝트 row 삽입 + 9개 project-root area seed(8 concern skeleton + `_unsorted`) + starter sub-area seed + 4 template artifact seed (Phase 13). `primary_language`는 명시 필수, default 금지, immutable/recreate-only 경고 포함; 지원 enum은 `en`, `ko`, `ja`. 응답 URL은 `/p/{slug}/wiki`; 새 slug는 같은 MCP 연결의 다음 `project_slug` input에서 바로 사용 가능. |
| `pindoc.project_export` | ✅ implemented | Project/area markdown export. zip/tar archive를 base64로 반환한다. 각 artifact는 `<area>/<slug>.md` + frontmatter(title/type/area/tags/completeness/agent_ref/meta/revision/edges), `include_revisions=true`면 `<slug>.revisions.md`를 추가한다. Reader HTTP API는 `/api/p/{project}/export`에서 binary archive를 내려준다. |
| `pindoc.project.list` | 📋 planned | V1.5 멀티프로젝트 권한 모델과 함께. 지금은 `GET /api/projects` HTTP 엔드포인트로 대체. |
| `pindoc.project.switch` | 🚫 obsolete | account-level `/mcp` + per-call `project_slug` 모델에서 폐기. |
| `pindoc.area.list` | ✅ implemented | 현재 프로젝트의 area 트리 반환. |
| `pindoc.area.propose` | 📋 planned | M1 영역 아님. `misc` fallback + agent 수동 area 생성으로 버티는 중. |
| `pindoc.artifact.read` | 🟡 partial | 현재는 단일 artifact 본문 반환. **스펙의 `include=neighbors\|recent_changes\|…`는 미구현**. Phase 12에서 `view=brief\|full\|continuation` 도입 예정. Phase 9부터 응답에 `agent_ref` (`pindoc://<slug>`) + `human_url` (`/p/:project/wiki/<slug>`) 두 URL을 분리 반환. |
| `pindoc.artifact.translate` | ✅ implemented | Agent-driven on-demand translation helper. 서버는 LLM 번역을 하지 않고 source markdown + source/target locale + `translation_of` 캐시 후보를 반환한다. 캐시는 translated artifact가 `body_locale`와 `translation_of` edge를 갖는 ordinary artifact 방식. |
| `pindoc.artifact.propose` | ✅ implemented | Phase 11에서 create/update/supersede 분기 + `basis.search_receipt` hard enforce + `pins[]` + `supersede_of` + `relates_to[]` + semantic conflict block 완료. Phase 12에서 `Failed[]` stable code + `NextTools[]` + `Related[]` 추가. Phase 14b에서 `expected_version` **update 경로 필수화** (미제공 → `NEED_VER`), 모든 not_ready에 `patchable_fields[]`, accepted path에 `warnings[]`/`warning_severities[]` (`RECOMMEND_READ_BEFORE_CREATE`, `SLUG_VERBOSE`, `SECTION_DUPLICATES_EDGES`, `MISSING_COMMIT_MSG_ON_CREATE` 등)와 일부 `suggested_actions[]`, `human_url_abs` 응답 포함 (public_base_url 설정 시). Create path에서 `commit_msg` 누락은 warning + `[fallback_missing_commit_msg] ...` revision message로 soft-required 처리한다. 공통 envelope은 여전히 `accepted\|not_ready`. |
| `pindoc.artifact.search` | ✅ implemented | Phase 10에서 real embedder (TEI + multilingual-e5-base) 전환. 응답에 `agent_ref` + `human_url` + (Phase 14b부터) `human_url_abs`. **`search_receipt`** (opaque token, TTL **30분** — Phase 14a에서 10→30분 연장) 포함. 같은 세션 내 이후 propose 호출에서 `basis.search_receipt`로 제시해야 create 경로 gate 통과. |
| `pindoc.artifact.revisions` | ✅ implemented | Phase 7 신규. artifact의 모든 revision 메타 + 최신 순. |
| `pindoc.artifact.diff` | ✅ implemented | Phase 7 신규. unified diff + section_deltas (heading 단위 added/removed/modified). |
| `pindoc.artifact.summary_since` | ✅ implemented | Phase 7 신규. `since_rev` 또는 `since_time` 기준 누적 변화 요약. |
| `pindoc.task.queue` | ✅ implemented | Reader Tasks board와 동일한 pending 의미(`task_meta.status` missing 또는 `open`)로 Task 대기열과 status/area/priority count를 반환. `scope.in_flight`와 다름. |
| `pindoc.task.assign` | ✅ implemented | Task assignee 단건 변경 전용 semantic shortcut. 내부적으로 `artifact.propose(shape="meta_patch", task_meta={assignee})` 경로로 수렴하며 search_receipt gate를 우회한다. |
| `pindoc.task.bulk_assign` | ✅ implemented | 여러 Task assignee를 한 번에 변경. `reason` 필수(2-200 runes), 부분 성공 허용, 성공 revision은 shared `bulk_op_id`로 묶는다. |
| `pindoc.context.for_task` | ✅ implemented | top landings + rationale + `search_receipt` + `candidate_updates[]` + stale age hint + `suggested_areas[]` + `recent_change_groups[]`. Change Groups는 body 없이 `{group_id, kind, commit_summary, time, artifact_count, areas, importance}`만 반환하며 `include_change_groups` default true, cap default 5/max 20. |
| `pindoc.graph.neighbors` | 📋 planned | `artifact_edges` 테이블 + `relates_to[]` propose 필드와 함께 (Phase 11). |
| `pindoc.resource.verify` | 📋 planned | M7 Freshness. pins 모델 도입 후. V1.x. |
| `pindoc.tc.register` / `.run_result` | 📋 planned | V1.1. |

### 공통 drift 주석

- **공통 응답 envelope**: 스펙은 `ok | error | not_ready`를 말하지만 현재 `artifact.propose`는 `accepted | not_ready`. 나머지 tool은 구조체 필드 직접 반환 (status 필드 없음). Phase 12에서 envelope 통일 예정.
- **인증**: 현재 V1 local transport는 stdio와 streamable_http 모두
  `auth_mode="trusted_local"`이다. MCP config에 `Authorization` header를 넣지
  않는다. 자기-호스팅 공개 / SaaS 시점에 GitHub OAuth + agent token을 별도
  Decision으로 도입한다.
- **`request_id` / 공통 `warnings[]`**: `artifact.propose` accepted path는 `warnings[]`/`warning_severities[]`를 반환하지만, tool 공통 envelope의 `request_id`와 전 도구 공통 warnings는 아직 미구현.
- **`draft_id`**: `not_ready` 응답에 포함된다고 스펙에 있으나 현재 미구현 (실패 시 agent가 동일 input으로 재호출). Phase 12에서 도입 검토.

---

## 공통 규약

### 인증

현재 V1 local mode는 loopback `trusted_local` 신뢰 모델이다. HTTP MCP config는
단일 account-level URL만 둔다.

```jsonc
{ "mcpServers": { "pindoc": { "type": "http", "url": "http://127.0.0.1:5830/mcp" } } }
```

`Authorization: Bearer ...` header는 아직 사용하지 않는다. 공개 self-host와
SaaS에서 OAuth / agent token을 도입할 때 이 섹션을 갱신한다.

### Project Scope

Project-scoped tool은 `project_slug` input을 명시적으로 받는다. 연결 URL은
`/mcp` 하나이고, connection 안에 hidden active project를 두지 않는다.
`pindoc.project.switch`는 현재 V1 scope 모델에 없다. 워크스페이스의 기본
project는 `PINDOC.md` frontmatter(`project_slug`) 또는 agent가 첫 turn에
선택한 세션-local default로 결정하고, 실제 tool call에는 `project_slug`를
넣는다.

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
    hint?: string                      // "pindoc.artifact.search(type=Decision, area=system/api) 호출 권장"
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
| 2 | `pindoc.project.current` | reader+ |
| 3 | `pindoc.project.create` | writer |
| 3a | `pindoc.project_export` | reader+ |
| 4 | `pindoc.area.list` | reader+ |
| 5 | `pindoc.artifact.search` | reader+ |
| 6 | `pindoc.artifact.propose` | writer |
| 7 | `pindoc.artifact.read` | reader+ |
| 8 | `pindoc.artifact.translate` | reader+ |
| 9 | `pindoc.graph.neighbors` | reader+ |
| 10 | `pindoc.context.for_task` | reader+ |
| 11 | `pindoc.resource.verify` | writer |
| 12 | `pindoc.tc.register` | writer |
| 13 | `pindoc.tc.run_result` | writer |
| 14 | `pindoc.task.queue` | reader+ |
| 15 | `pindoc.task.assign` | writer |
| 16 | `pindoc.task.bulk_assign` | writer |

내부 전용 (MCP 공개 X): `artifact.commit`, `artifact.archive`, `area.delete`. Review Queue 승인 시 서버 내부 호출.

---

## 1. `pindoc.harness.install`

> `pindoc init` CLI가 서버와 통신해 PINDOC.md 생성 + CLAUDE.md 주입. 현재
> V1 local mode는 `trusted_local`이라 agent token 없이 호출한다.

Runtime M1 shape is MCP-only: input uses `project_slug`, optional
`language`, optional `locale` (view preference), and optional
`include_section_12` (default true). Output keeps legacy `body` /
`suggested_path` and also returns `pindoc_md_content` / `pindoc_md_path`
aliases. The generated content starts with YAML frontmatter and includes
Section 12 unless explicitly disabled.

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

- `PROJECT_SLUG_REQUIRED` — project scope를 결정할 수 없음

---

## 2. `pindoc.project.current`

### Input

```typescript
{
  project_slug?: string
}
```

### Output

```typescript
{
  id: string,
  slug: string,
  name: string,
  primary_language: "en" | "ko" | "ja",
  capabilities: {
    scope_mode: "per_call",
    transport: "stdio" | "streamable_http",
    auth_mode: "trusted_local",
    new_project_requires_reconnect: false
  }
}
```

---

## 3. `pindoc.project.create`

### Input

```typescript
{
  slug: string,
  name: string,
  primary_language: "en" | "ko" | "ja",
  color?: string,
  description?: string
}
```

### Output

```typescript
{
  slug: string,
  wiki_url: string
}
```

### 에러

- `PROJECT_SLUG_TAKEN`
- `PRIMARY_LANGUAGE_REQUIRED`

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
  filters: { type: ["Debug", "Analysis", "Decision"], area: ["system/api"] },
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
      area: "system/api",
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

### Body patch alternative (update_of only)

update 경로에서 `body_markdown` 전체 대신 `body_patch` 객체를 보내면 서버가 기존 body에 patch 를 적용한 결과를 새 revision body 로 저장한다. `body_markdown` 과 `body_patch` 는 상호 배타(`PATCH_EXCLUSIVE`)이고 create/supersede path 는 거부한다(`PATCH_UPDATE_ONLY`). Task `artifact-propose-본문-patch-입력-도입` 참조.

세 mode:

**section_replace** — `## heading` 한 섹션만 본문 교체. heading은 `## 목적 / Purpose` 같은 ko/en 혼합 슬롯도 fuzzy 매치.

```json
{
  "update_of": "task-reader-ia-refactor",
  "expected_version": 2,
  "commit_msg": "wording cleanup in Acceptance criteria",
  "body_patch": {
    "mode": "section_replace",
    "section_heading": "Acceptance criteria",
    "replacement": "- [x] Surface state가 URL segment 기반\n- [x] Task Surface kanban\n..."
  }
}
```

**checkbox_toggle** — Task TODO 체크박스 atomic op. `checkbox_index` 는 body 전체에서 `- [ ]` / `- [x]` 항목을 0부터 센다. 이미 target state 이면 accepted + `PATCH_NOOP` warning.

```json
{
  "update_of": "task-reader-width-modes",
  "expected_version": 2,
  "commit_msg": "mark first acceptance done",
  "body_patch": {
    "mode": "checkbox_toggle",
    "checkbox_index": 0,
    "checkbox_state": true
  }
}
```

**append** — 본문 끝에 텍스트 추가(빈 줄 구분).

```json
{
  "update_of": "decision-task-status-v2",
  "expected_version": 1,
  "commit_msg": "log retroactive note in Consequences",
  "body_patch": {
    "mode": "append",
    "append_text": "후속 관찰(2026-04-23): claimed_done 컬럼에 7건 누적, verified 0건."
  }
}
```

에러 코드는 `PATCH_MODE_INVALID` · `PATCH_HEADING_EMPTY` · `PATCH_SECTION_NOT_FOUND` · `PATCH_CHECKBOX_INDEX_REQUIRED` · `PATCH_CHECKBOX_STATE_REQUIRED` · `PATCH_CHECKBOX_OUT_OF_RANGE` · `PATCH_APPEND_EMPTY` 중 하나가 `failed[]` 에 담긴다.

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

> Fast Landing. 자연어 task 설명으로 관련 artifact, update 후보, stale hint, 최근 Change Group 요약을 반환한다.

### Input

```typescript
{
  project_slug: string,
  task_description: string,       // "장바구니 재시도 로직"
  top_k?: number,                 // 기본 3, max 10
  areas?: string[],
  include_templates?: boolean,    // default false
  include_superseded?: boolean,   // default false
  include_change_groups?: boolean,// default true
  change_group_limit?: number,    // default 5, max 20
  since_revision_id?: number
}
```

### Output

```typescript
{
  task_description: string,
  landings: Array<{
    artifact_id: string,
    slug: string,
    type: string,
    title: string,
    area_slug: string,
    rationale: string,
    agent_ref: string,             // pindoc://<slug>
    human_url: string,             // /p/:project/wiki/:slug
    distance: number,
    trust_summary: {
      source_type?: string,
      confidence?: string,
      next_context_policy?: string
    }
  }>,
  search_receipt?: string,
  candidate_updates?: Array<{ slug: string, reason: string, distance: number }>,
  stale?: Array<{ slug: string, reason: string, days_old: number }>,
  suggested_areas: Array<{ area_slug: string, score: number, reason: string }>,
  recent_change_groups?: Array<{
    group_id: string,
    kind: "human_trigger" | "auto_sync" | "maintenance" | "system",
    commit_summary: string,
    time: string,
    artifact_count: number,
    areas: string[],
    importance: { score: number, level: "low" | "medium" | "high", reasons?: string[] }
  }>,
  embedder_used: { name: string, model_id?: string, dimension: number }
}
```

`recent_change_groups`는 Today 화면과 같은 backend grouping query를 쓰지만, MCP context에서는 body를 절대 포함하지 않는다. 목적은 “이 task를 시작하기 직전에 같은 영역에서 무슨 묶음 변경이 있었는가”만 빠르게 보여 주는 것이다.

---

## 8a. `pindoc.project_export`

> Project/area 단위 Markdown archive export. Reader UI export 버튼과 같은 builder를 사용한다.

### Input

```typescript
{
  project_slug: string,
  areas?: string[],              // optional area_slug filters
  slugs?: string[],              // optional artifact slug filters
  include_revisions?: boolean,   // default false
  format?: "zip" | "tar"         // default zip
}
```

### Output

```typescript
{
  filename: string,
  mime_type: "application/zip" | "application/x-tar",
  encoding: "base64",
  bytes: number,
  artifact_count: number,
  file_count: number,
  content_base64: string
}
```

Archive layout:

```text
<area>/<slug>.md
<area>/<slug>.revisions.md       # include_revisions=true only
```

Frontmatter includes `title`, `type`, `area`, `tags`, `completeness`, `slug`, `agent_ref`, `artifact_meta` axes, `created_at`, `updated_at`, `revision_number`, and typed `relates_to` edges. Import is intentionally a follow-up path; export preserves enough metadata for a later importer to reconstruct ordinary artifacts and revision notes.

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

## 13. `pindoc.task.queue`

> Reader Tasks board와 같은 의미로 Task 대기열을 조회한다. 기본 `status="pending"`은 `task_meta.status`가 없거나 `open`인 row를 뜻한다. acceptance checkbox 기반의 "남은 항목" 조회는 `pindoc.scope.in_flight`가 담당하며, 두 도구는 서로 대체재가 아니다.

### Input

```typescript
{
  status?: "pending" | "all" | "open" | "missing_status" | "missing" |
           "claimed_done" | "verified" | "blocked" | "cancelled",
  area_slug?: string,
  priority?: "p0" | "p1" | "p2" | "p3",
  assignee?: string,
  limit?: number              // default 50, max 500
}
```

### Output

```typescript
{
  source_semantics: "reader_tasks_queue_v1",
  status_filter: string,
  total_count: number,
  pending_count: number,       // status missing + open
  status_counts: {
    open: number,
    missing_status: number,
    claimed_done: number,
    verified: number,
    blocked: number,
    cancelled: number,
    other: number
  },
  area_counts: Record<string, number>,
  priority_counts: Record<string, number>,
  warning_counts: {
    TASK_STATUS_MISSING?: number,
    TASK_ACCEPTANCE_DONE_RECONCILE_PENDING?: number
  },
  items: Array<{
    artifact_id: string,
    slug: string,
    title: string,
    area_slug: string,
    status: string,
    missing_status?: boolean,
    priority?: string,
    assignee?: string,
    due_at?: string,
    parent_slug?: string,
    updated_at: string,
    warnings?: Array<"TASK_STATUS_MISSING" | "TASK_ACCEPTANCE_DONE_RECONCILE_PENDING">,
    agent_ref: string,
    human_url: string,
    human_url_abs?: string
  }>,
  truncated?: boolean,
  notice: string
}
```

### 운영 규칙

- Agent가 "열린 Task를 다 처리했다"고 말하기 전에는 `pindoc.task.queue(status="pending")`의 `pending_count == 0`을 확인해야 한다.
- `missing_status`는 Reader의 `no_status` 컬럼과 같은 의미이며 pending count에 포함된다.
- `TASK_STATUS_MISSING`은 Task lifecycle metadata가 없는 row를 놓치지 않게 하는 guardrail이다.
- `TASK_ACCEPTANCE_DONE_RECONCILE_PENDING`은 acceptance가 100% 해결됐지만 아직 `claimed_done`으로 reconcile되지 않은 transient row를 뜻한다. `pindoc.ping`은 이런 row를 자동으로 `claimed_done`으로 전이한다.
- `pindoc.scope.in_flight`는 `[ ]` / `[~]` acceptance item 조회용이다. Task row의 lifecycle status와 혼동하지 않는다.

### 구현 상태

- ✅ registered in MCP server and toolset catalog
- ✅ default pending semantics match Reader (`missing_status` + `open`)
- ✅ counts are computed before returned item `limit`

---

## 14. `pindoc.task.assign`

> Task assignee 단건 변경용 semantic shortcut. 본문/acceptance는 건드리지 않고 `task_meta.assignee`만 meta_patch revision으로 남긴다.

### Input

```typescript
{
  slug_or_id: string,        // UUID, slug, or pindoc:// URL
  assignee: string,          // agent:<id> | user:<id> | @<handle> | "" clear
  reason?: string,           // revision commit_msg; omit = auto message
  author_id?: string,
  author_version?: string
}
```

### Output

```typescript
{
  status: "accepted" | "not_ready",
  artifact_id?: string,
  slug?: string,
  revision_number?: number,
  human_url?: string,
  human_url_abs?: string,
  new_assignee?: string,
  error_code?: "ASSIGN_MISSING_REF" | "ASSIGN_TARGET_NOT_FOUND" |
               "ASSIGN_NOT_A_TASK" | "ASSIGNEE_FORMAT_INVALID",
  failed?: string[],
  checklist?: string[]
}
```

### 구현 상태

- ✅ registered in MCP server and toolset catalog
- ✅ validates assignee shape
- ✅ delegates to the `meta_patch` operational metadata lane
- ✅ does not issue `bulk_op_id`

---

## 15. `pindoc.task.bulk_assign`

> 여러 Task assignee를 한 번에 변경한다. 하나의 이유로 묶이는 운영상 재배치에만 사용한다.

### Input

```typescript
{
  slugs: string[],            // UUID, slug, or pindoc:// URL
  assignee: string,           // same format as task.assign
  reason: string,             // required, 2-200 runes
  author_id?: string,
  author_version?: string
}
```

### Output

```typescript
{
  status: "accepted" | "partial" | "not_ready",
  bulk_op_id?: string,
  results?: Array<{
    slug: string,
    artifact_id?: string,
    status: "accepted" | "not_ready",
    error_code?: string,
    revision_number?: number,
    human_url?: string
  }>,
  success_count: number,
  fail_count: number,
  new_assignee?: string,
  error_code?: "BULK_REASON_EMPTY" | "REASON_LENGTH_INVALID" |
               "BULK_NO_SLUGS" | "ASSIGNEE_FORMAT_INVALID"
}
```

### 구현 상태

- ✅ `reason` empty / length validation
- ✅ per-slug partial success
- ✅ shared `bulk_op_id` emitted for accepted batch calls
- ✅ each successful row converges on the same `meta_patch` write lane as `task.assign`

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
