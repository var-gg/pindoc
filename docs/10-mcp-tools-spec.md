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
| `pindoc.ping` | ✅ implemented | Phase 1 핸드쉐이크용. `working_directory`를 넘기면 `PINDOC.md` frontmatter drift를 검사해 `harness_drift_hint`를 반환한다. |
| `pindoc.harness.install` | ✅ implemented | `pindoc init` CLI 없이 MCP 호출만으로 PINDOC.md body 반환. 파일 쓰기는 에이전트 책임. 출력에는 workspace detection용 YAML frontmatter(project_slug/project_id/locale/schema_version), Applicable Rules Section X, chip-driven Task lifecycle Section 12가 포함된다. 기본 `response_format=full`, `file_only`/etag match 시 대형 body와 style snippet을 생략하고 etag만 반환한다. |
| `pindoc.project.current` | ✅ implemented | account-level + per-call project scope. stdio와 streamable_http 모두 `scope_mode: "per_call"`, `new_project_requires_reconnect: false`; HTTP 데몬 URL은 단일 `/mcp`이고 각 tool input의 `project_slug`가 scope를 결정한다. capabilities는 `providers: string[]` (활성 IdP CSV) + `bind_addr` (loopback인지 외부 노출인지)로 인증 정책을 advertise한다 — Decision `decision-auth-model-loopback-and-providers`가 이전 `auth_mode` enum을 폐기. Phase 9부터 `capabilities` 블록, Phase 14a에서 `receipt_ttl_sec: 1800`, `requires_expected_version: true`, `public_base_url`(server_settings) 추가. `receipt_exemption_limit`은 receipt-less bootstrap create 허용 한도(default 5)를 노출한다. |
| `pindoc.project.create` | ✅ implemented | Phase 8 신규. 프로젝트 row 삽입 + 9개 project-root area seed(8 concern skeleton + `_unsorted`) + starter sub-area seed + 4 template artifact seed (Phase 13). `primary_language`는 명시 필수, default 금지, immutable/recreate-only 경고 포함; 지원 enum은 `en`, `ko`, `ja`. 응답 URL은 `/p/{slug}/wiki`; 새 slug는 같은 MCP 연결의 다음 `project_slug` input에서 바로 사용 가능. `next_steps[0]`는 `pindoc.harness.install` 호출과 PINDOC.md 선설치 안내를 담고, 첫 propose용 one-use `bootstrap_receipt`/`search_receipt`를 동봉한다. |
| `pindoc.project_export` | ✅ implemented | Project/area markdown export. zip/tar archive를 base64로 반환한다. 각 artifact는 `<area>/<slug>.md` + frontmatter(title/type/area/tags/completeness/agent_ref/meta/revision/edges), `include_revisions=true`면 `<slug>.revisions.md`를 추가한다. Reader HTTP API는 `/api/p/{project}/export`에서 binary archive를 내려준다. |
| `pindoc.project.list` | 📋 planned | V1.5 멀티프로젝트 권한 모델과 함께. 지금은 `GET /api/projects` HTTP 엔드포인트로 대체. |
| `pindoc.project.switch` | 🚫 obsolete | account-level `/mcp` + per-call `project_slug` 모델에서 폐기. |
| `pindoc.area.list` | ✅ implemented | 현재 프로젝트의 area 트리 반환. |
| `pindoc.area.propose` | 📋 planned | M1 영역 아님. `misc` fallback + agent 수동 area 생성으로 버티는 중. |
| `pindoc.artifact.read` | 🟡 partial | 현재는 단일 artifact 본문 반환. **스펙의 `include=neighbors\|recent_changes\|…`는 미구현**. Phase 12에서 `view=brief\|full\|continuation` 도입 예정. Phase 9부터 응답에 `agent_ref` (`pindoc://<slug>`) + `human_url` (`/p/:project/wiki/<slug>`) 두 URL을 분리 반환. |
| `pindoc.artifact.translate` | ✅ implemented | Agent-driven on-demand translation helper. 서버는 LLM 번역을 하지 않고 source markdown + source/target locale + `translation_of` 캐시 후보를 반환한다. 캐시는 translated artifact가 `body_locale`와 `translation_of` edge를 갖는 ordinary artifact 방식. |
| `pindoc.artifact.propose` | ✅ implemented | Phase 11에서 create/update/supersede 분기 + `basis.search_receipt` hard enforce + `pins[]` + `supersede_of` + `relates_to[]` + semantic conflict block 완료. Empty/same-author area의 첫 N건은 receipt 미제시 create도 `receipt_exempted` 신호와 함께 accepted(default N=5, `PINDOC_RECEIPT_EXEMPTION_LIMIT`). Phase 12에서 `failed[]` stable code + structured `next_tools[]` + `related[]` 추가. Phase 14b에서 `expected_version` **update 경로 필수화** (미제공 → `NEED_VER`), 모든 not_ready에 `patchable_fields[]`, accepted path에 `warnings[]`/`warning_severities[]` (`RECOMMEND_READ_BEFORE_CREATE`, `SLUG_VERBOSE`, `SECTION_DUPLICATES_EDGES`, `MISSING_COMMIT_MSG_ON_CREATE` 등)와 일부 `suggested_actions[]`, `human_url_abs` 응답 포함 (public_base_url 설정 시). H2/structure not_ready는 `expected.required_h2[]`와 `_template_<type>` read hint를 `next_tools[0]`에 싣는다. Create path에서 `commit_msg` 누락은 warning + `[fallback_missing_commit_msg] ...` revision message로 soft-required 처리한다. 공통 envelope은 여전히 `accepted\|not_ready`. |
| `pindoc.artifact.search` | ✅ implemented | Phase 10에서 real embedder (TEI + multilingual-e5-base) 전환. 응답에 `agent_ref` + `human_url` + (Phase 14b부터) `human_url_abs`. **`search_receipt`** (opaque token, TTL **30분** — Phase 14a에서 10→30분 연장) 포함. 같은 세션 내 이후 propose 호출에서 `basis.search_receipt`로 제시해야 create 경로 gate 통과. |
| `pindoc.artifact.revisions` | ✅ implemented | Phase 7 신규. artifact의 모든 revision 메타 + 최신 순. |
| `pindoc.artifact.diff` | ✅ implemented | Phase 7 신규. unified diff + section_deltas (heading 단위 added/removed/modified). |
| `pindoc.artifact.summary_since` | ✅ implemented | Phase 7 신규. `since_rev` 또는 `since_time` 기준 누적 변화 요약. |
| `pindoc.task.queue` | ✅ implemented | Reader Tasks board와 동일한 pending 의미(`task_meta.status` missing 또는 `open`)로 Task 대기열과 status/area/priority count를 반환. `default_focus="assignee_open_count"`와 item별 `ready_to_close` 신호를 포함한다. `scope.in_flight`와 다름. |
| `pindoc.task.assign` | ✅ implemented | Task assignee 단건 변경 전용 semantic shortcut. 내부적으로 `artifact.propose(shape="meta_patch", task_meta={assignee})` 경로로 수렴하며 search_receipt gate를 우회한다. `assignee=""`는 clear, 누락은 assign input에서 불가. |
| `pindoc.task.bulk_assign` | ✅ implemented | 여러 Task assignee를 한 번에 변경. `reason` 필수(2-200 runes), 부분 성공 허용, 성공 revision은 shared `bulk_op_id`로 묶는다. |
| `pindoc.task.claim_done` | ✅ implemented | Task 구현 완료 선언. 본문의 모든 `- [ ]` acceptance를 `[x]`로 토글 + `task_meta.status="claimed_done"` 한 revision에 atomic 처리. `[~]`/`[-]`는 보존. search_receipt gate 우회. |
| `pindoc.runtime.status` | ✅ implemented | Read-only 진단 스냅샷. server version / git commit / toolset_version + tool_count / 5830·5832 포트 + override / container_id / image_tag / hostname / `source` (calling Principal.Source — `loopback`\|`oauth`) / `providers[]` / `bind_addr` / transport / Go version / DB ping을 한 번에 반환. |
| `pindoc.context.for_task` | ✅ implemented | top landings + rationale + `search_receipt` + `candidate_updates[]` + stale age hint + `suggested_areas[]` + `recent_change_groups[]` + `applicable_rules[]`. Change Groups는 body 없이 `{group_id, kind, commit_summary, time, artifact_count, areas, importance}`만 반환하며 `include_change_groups` default true, cap default 5/max 20. |
| `pindoc.user.current` | ✅ implemented | 현재 MCP session에 bind된 user row를 반환한다. `PINDOC_USER_NAME` 미설정은 blocking이 아니라 `status="informational"`, `code="USER_NOT_SET"`, `hints[]`로 반환한다. agent identity 자체가 없을 때만 `not_ready`. |
| `pindoc.user.update` | ✅ implemented | bind된 user row의 display_name/email/github_handle을 수정한다. user row가 없으면 실제 mutation target이 없으므로 `USER_NOT_SET` not_ready 유지. |
| `pindoc.graph.neighbors` | 📋 planned | `artifact_edges` 테이블 + `relates_to[]` propose 필드와 함께 (Phase 11). |
| `pindoc.resource.verify` | 📋 planned | M7 Freshness. pins 모델 도입 후. V1.x. |
| `pindoc.tc.register` / `.run_result` | 📋 planned | V1.1. |

### 공통 drift 주석

- **공통 응답 envelope**: 스펙은 `ok | error | not_ready`를 말하지만 현재 `artifact.propose`는 `accepted | not_ready`. 나머지 tool은 구조체 필드 직접 반환 (status 필드 없음). Phase 12에서 envelope 통일 예정.
- **인증**: stdio MCP는 process trust로 default user owner principal을
  부여한다. streamable_http는 loopback 요청이면 동일하게 owner를 부여
  하고, 비-loopback 요청이면 `PINDOC_AUTH_PROVIDERS`가 활성화한 IdP의
  Pindoc AS-issued JWT를 요구한다(`Authorization: Bearer ...`). 비-
  loopback bind + 빈 providers + `ALLOW_PUBLIC_UNAUTHENTICATED=false`
  조합은 부팅을 거부(`ErrPublicWithoutAuth`). Decision
  `decision-auth-model-loopback-and-providers`.
- **`request_id` / 공통 `warnings[]`**: `artifact.propose` accepted path는 `warnings[]`/`warning_severities[]`를 반환하지만, tool 공통 envelope의 `request_id`와 전 도구 공통 warnings는 아직 미구현.
- **`draft_id`**: `not_ready` 응답에 포함된다고 스펙에 있으나 현재 미구현 (실패 시 agent가 동일 input으로 재호출). Phase 12에서 도입 검토.

### Tool catalog change notifications

Pindoc 서버는 부팅 시 현재 `toolset_version`을 `mcp_runtime_state`의 이전
값(없으면 최근 `mcp_tool_calls.toolset_version`)과 비교한다. 이전 값이 있고
현재 값과 다를 때만, 첫 initialized MCP session을 관찰한 뒤 SDK의 표준
경로로 `notifications/tools/list_changed`를 한 번 발송한다. Payload는 MCP
표준 JSON-RPC notification이다.

이 경로는 stdio와 streamable_http 모두 같은 SDK session list를 사용한다.
streamable_http에서는 persistent GET/SSE 채널로 push되고, stdio에서는 해당
subprocess session으로 push된다. 발송 실패나 reannounce 대상 부재는 warning
log만 남기며 서버 startup과 tool call을 막지 않는다. toolset_version이
같거나 이전 값을 판단할 근거가 없는 fresh install에서는 noise 방지를 위해
notification을 보내지 않는다.

---

## 공통 규약

### 인증

기본 셋업(loopback bind + 빈 providers)은 모든 요청을 자동 owner로
신뢰한다 — process / 127.0.0.1 trust boundary. 외부 노출이 필요하면
`PINDOC_AUTH_PROVIDERS=github` + GitHub OAuth App credentials를 추가하고,
loopback 요청은 그대로 신뢰하면서 외부 트래픽만 Pindoc AS의 Bearer JWT를
요구한다.

```jsonc
{ "mcpServers": { "pindoc": { "type": "http", "url": "http://127.0.0.1:5830/mcp" } } }
```

| env | 기본값 | 설명 |
|---|---|---|
| `PINDOC_BIND_ADDR` | `127.0.0.1:5830` | 비-loopback이면 IdP 또는 ALLOW opt-in 필수 |
| `PINDOC_AUTH_PROVIDERS` | empty | CSV. 현재 `github` 지원 |
| `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED` | `false` | 외부 노출 + IdP 없음의 명시 opt-in |

세 axes 정합 안 맞으면 부팅을 거부(`ErrPublicWithoutAuth`). Decision
`decision-auth-model-loopback-and-providers`가 이전 4-mode `PINDOC_AUTH_MODE`
enum을 supersede했다.

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

`pindoc.artifact.propose`, `pindoc.task.acceptance.transition`,
`pindoc.harness.install`, `pindoc.project.create` 등 tool-level validation을
하는 MCP write tool은 아래 error language contract를 따른다. 에이전트는
`error_code`/`error_codes`/`checklist_items[].code`로 분기하고,
사용자에게는 `checklist` 또는 `checklist_items[].message`를 표시한다.
`failed[]`는 기존 client 호환용 stable code list이며 `error_codes[]`가
canonical alias다.

```typescript
{
  status: "not_ready",
  error_code?: string,                  // first stable SCREAMING_SNAKE_CASE code
  failed?: string[],                    // legacy stable code list
  error_codes?: string[],               // canonical stable code list
  checklist?: string[],                 // localized user-facing copy
  checklist_items?: Array<{
    code: string,                       // stable SCREAMING_SNAKE_CASE
    message: string                     // localized copy for display
  }>,
  message_locale?: "en" | "ko" | "ja",  // locale used after fallback
  suggested_actions?: string[]
}
```

Stable code는 영어 identifier로 유지하며 변경 시 deprecation cycle을 거친다.
예: `DEC_NO_SECTIONS`, `NEED_VER`, `HARNESS_RESPONSE_FORMAT_INVALID`.
`MISSING_H2:Purpose`처럼 과거 suffix를 갖던 diagnostic은 canonical
`error_codes[]`/`checklist_items[].code`에서 `MISSING_H2`로 정규화된다.

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
| 17 | `pindoc.task.claim_done` | writer |
| 18 | `pindoc.runtime.status` | reader |

내부 전용 (MCP 공개 X): `artifact.commit`, `artifact.archive`, `area.delete`. Review Queue 승인 시 서버 내부 호출.

---

## 1. `pindoc.harness.install`

> `pindoc init` CLI가 서버와 통신해 PINDOC.md 생성 + CLAUDE.md 주입. 현재
> 기본 셋업(loopback bind + 빈 providers)은 agent token 없이 호출한다.

Runtime M1 shape is MCP-only: input uses `project_slug`, optional
`language`, optional `locale` (view preference), and optional
`include_section_12` (default true). `response_format` defaults to `full`
for compatibility. `file_only` returns paths/instructions/metadata/etags
without the large generated bodies. Clients may also pass the previous
`if_content_etag` / `if_style_snippet_etag`; when either matches the current
rendered payload, that payload is omitted. Output keeps legacy `body` /
`suggested_path` and also returns `pindoc_md_content` / `pindoc_md_path`
aliases in full responses. The generated content starts with YAML
frontmatter and includes Section 12 unless explicitly disabled.

### Input

```typescript
{
  project_slug: string,
  language?: "en" | "ko" | "auto",
  locale?: "en" | "ko" | "ja" | "auto",
  include_section_12?: boolean,       // default true
  response_format?: "full" | "file_only", // default full
  if_content_etag?: string,           // omit PINDOC.md body when current etag matches
  if_style_snippet_etag?: string      // omit style_snippet when current etag matches
}
```

### Output

```typescript
{
  suggested_path: "PINDOC.md",
  body?: string,                 // full only; omitted for file_only/etag match
  pindoc_md_content?: string,    // full only; alias of body
  pindoc_md_path: "PINDOC.md",
  instructions: string,
  claude_md_include_line: "@PINDOC.md",
  style_snippet?: string,        // full only; omitted for file_only/etag match
  style_snippet_targets: ["CLAUDE.md", "AGENTS.md", ".cursorrules"],
  style_snippet_marker: string,
  message: string,
  response_format: "full" | "file_only",
  content_etag: string,          // sha256:<hex>; always present
  content_url?: string,          // reserved; local MCP usually omits it
  content_omitted?: true,
  style_snippet_etag: string,    // sha256:<hex>; always present
  style_snippet_omitted?: true,
  rendered_for: {
    project_slug: string,
    project_id: string,
    primary_language: string,
    locale: string,
    include_section_12: boolean,
    pindoc_server_version: string
  }
}
```

### 에러

- `PROJECT_SLUG_REQUIRED` — project scope를 결정할 수 없음
- `HARNESS_RESPONSE_FORMAT_INVALID` — `response_format`은 `full` 또는
  `file_only`만 허용. `status="not_ready"`와 함께 `error_codes[]`,
  `checklist_items[]`, `message_locale`가 채워진다.

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
    providers: [],
    bind_addr: "127.0.0.1:5830",
    new_project_requires_reconnect: false,
    receipt_ttl_sec: number,
    receipt_exemption_limit: number
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
  id: string,
  slug: string,
  name: string,
  url: string,
  default_area: string,
  bootstrap_receipt?: string,
  search_receipt?: string,      // alias of bootstrap_receipt
  reconnect_required: false,
  activation: "in_this_session",
  next_steps: Array<{
    tool: string,
    args?: Record<string, unknown>,
    reason?: string
  }>,
  mcp_configs: Array<{
    client: "claude-code" | ...,
    config_path: string,         // 예: "~/.config/claude-code/mcp.json"
    config_fragment: object      // 주입할 JSON
  }>,
  session_bootstrap: {
    auto_call: string[],                    // V1: ["pindoc.workspace.detect"]
    cache_key_for_workspace_detect: string, // V1: "pindoc.session.default_project_slug"
    signals_from_client: string[],          // ["pindoc_md_frontmatter", "workspace_path", "git_remote_url"]
    rerun_on: string[],                     // ["user_switched_workspace", "tool_returned_PROJECT_SLUG_REQUIRED"]
    notes?: string
  }
}
```

### Session bootstrap 계약

`session_bootstrap`은 클라이언트 harness가 매 MCP 세션 시작에 한 번 자동 실행해야 할 부트스트랩을 기계 판독 가능한 형태로 노출한다. PINDOC.md 본문의 "Session bootstrap" 섹션은 이 객체의 prose mirror다 — 두 표현은 항상 같은 값을 가리켜야 한다 (자동 테스트가 검증).

V1 동작:

1. 클라이언트가 `signals_from_client`에 명시된 세 신호(PINDOC.md frontmatter / workspace_path / git_remote_url)를 로컬에서 수집.
2. `auto_call[0] = pindoc.workspace.detect`를 1회 호출, 입력으로 위 세 신호 전달.
3. 응답의 `project_slug`를 세션 cache key `cache_key_for_workspace_detect`에 보관.
4. 후속 모든 tool 호출이 그 cache에서 project_slug를 읽음. 매 호출마다 workspace.detect 재호출 금지.
5. `rerun_on`에 명시된 사건(사용자 워크스페이스 전환 / `PROJECT_SLUG_REQUIRED` 에러) 발생 시에만 재부트스트랩.

기존 클라이언트가 `session_bootstrap`을 모르면 prose 섹션을 따라 수동으로 같은 흐름을 수행할 수 있다 — 결과는 동일하다.

`next_steps[0]`는 항상 `{tool:"pindoc.harness.install",
args:{project_slug:<new-slug>}}` 형태다. `reason`은 현재 사용자 언어로
"PINDOC.md를 먼저 설치하지 않으면 이후 propose가 거부될 수 있다"는
온보딩 안내를 포함한다. `bootstrap_receipt`는 같은 MCP session에서 새
project의 첫 `pindoc.artifact.propose`에 `basis.search_receipt`로 넘기는
one-use token이다. 별도 `artifact.search` 호출 없이 첫 artifact를 만들 수
있고, 성공 검증 후에는 재사용되지 않는다.

### 에러

- `SLUG_INVALID`
- `SLUG_RESERVED`
- `SLUG_TAKEN`
- `NAME_REQUIRED`
- `LANG_REQUIRED`
- `LANG_INVALID`

위 validation error는 handler error가 아니라 `status="not_ready"` 응답으로
돌아오며, `error_codes[]`, `checklist_items[]`, `message_locale`가 함께
채워진다. DB/transaction 등 내부 오류만 handler error로 남긴다.

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
  }>,
  receipt_exempted?: {
    reason: "empty_area_first_proposes",
    n_remaining: number,
    limit: number
  }
}
```

Create path는 기본적으로 `basis.search_receipt`를 요구한다. 예외는 두 가지다:
`project.create`가 동봉한 one-use `bootstrap_receipt`를 제시하는 경우, 또는
receipt가 없더라도 대상 area의 non-template artifact가 모두 같은 `author_id`
이고 개수가 `receipt_exemption_limit` 미만인 경우다. 후자는 accepted 응답에
`receipt_exempted`를 채워 agent가 gate 면제 사실과 남은 N을 기록할 수 있게
한다. 면제는 search-before-write gate만 풀며, accepted 후 near-duplicate
warning scan은 그대로 실행된다.

### Pins vs evidence edges

`pins[]`는 commit/file/path/line/URL처럼 외부 좌표가 있는 근거를 고정한다.
`relates_to[].relation="evidence"`는 근거 자체가 다른 Pindoc artifact일 때
쓴다. `references`는 배경 맥락, `evidence`는 검증/주장 뒷받침이라는 의미를
갖는다. Reader Sidecar는 `evidence` edge를 일반 관계와 분리해 표시하고,
concrete pin은 References 영역에 계속 우선 표시한다.

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
  error_code?: string,
  failed: string[],
  error_codes: string[],
  checklist: string[],
  checklist_items: Array<{ code: string, message: string }>,
  message_locale?: "en" | "ko" | "ja",
  next_tools: Array<{
    tool: string,
    args?: Record<string, unknown>,
    reason?: string
  }>,
  expected?: {
    artifact_type?: ArtifactType,
    template_slug?: string,
    required_h2?: Array<{ label: string, aliases?: string[] }>
  },
  patchable_fields?: string[],
  suggested_actions?: string[]
}
```

H2/structure 실패(`MISSING_H2:*`, `DEC_NO_SECTIONS`, `TASK_NO_ACCEPTANCE`,
`DBG_NO_REPRO`, `DBG_NO_RESOLUTION`)에서는 `next_tools[0]`가
`{tool:"pindoc.artifact.read", args:{id_or_slug:"_template_<type>"}}` 형태로
내려간다. 같은 응답의 `expected.required_h2[]`는 해당 type의 필수 H2 slot과
en/ko alias를 함께 노출해, agent가 template read 한 번으로 재시도 본문을
보정할 수 있게 한다.

### `artifact_meta` rule fields

`artifact_meta`는 6개 epistemic axis 외에 Applicable Rules Mechanism용
선택 필드를 받는다. `rule_severity`가 존재하면 그 artifact는 정책/rule로
marking되고, `pindoc.context.for_task`가 target area/type에 맞춰
`applicable_rules[]`에 compact projection으로 surface한다.

```json
{
  "artifact_meta": {
    "source_type": "artifact",
    "confidence": "high",
    "applies_to_areas": ["ui", "experience/*"],
    "applies_to_types": ["Task"],
    "rule_severity": "binding",
    "rule_excerpt": "Use the shared empty-state component and restrained count copy."
  }
}
```

`applies_to_areas`는 area_slug, `*`, `ui/*` 같은 wildcard scope를 받는다.
생략하면 own area + sub-area에 적용된다. cross-cutting child area의 rule은
area scope가 없어도 모든 task에 적용된다. `applies_to_types`는 생략/빈 배열이면
모든 type에 적용된다.

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
  project_slug: string,
  id_or_slug: string, // UUID, slug, pindoc://slug, /p/:project/wiki/:slug
  view?: "brief" | "full" | "continuation" // default "full"
}
```

### Output

```typescript
{
  id: string,
  project_slug: string,
  area_slug: string,
  slug: string,
  type: ArtifactType,
  title: string,
  body_markdown?: string, // omitted on view=brief
  body_locale?: string,
  tags: string[],
  completeness: string,
  status: string,
  review_state: string,
  author_kind: string,
  author_id: string,
  author_version?: string,
  agent_ref: string,
  human_url: string,
  human_url_abs?: string,
  created_at: string,
  updated_at: string,
  published_at?: string,
  view: "brief" | "full" | "continuation",
  summary?: string,
  pins?: PinRef[],
  stale?: StaleSignal,
  recent_revisions?: RevisionSummaryRef[],
  relates_to?: EdgeRef[],
  related_by?: EdgeRef[],
  artifact_meta: ArtifactMeta,
  task_attention?: {
    code: "task_still_open",
    message: string,
    level: "info",
    next_tools: Array<{
      tool: "pindoc.artifact.propose",
      args?: object,
      reason?: string
    }>
  }
}
```

`task_attention`은 body footer가 아니라 별도 메타 채널이다. 서버는 다음 조건을 모두 만족할 때만 포함한다: `type="Task"`, `task_meta.status`가 비어 있거나 `open`, 호출 에이전트가 마지막 revision author 또는 `task_meta.assignee`와 일치, 호출자가 사람이 아님(`author_id` 공백 / `user:*` / `@handle` 비활성), `view`가 `full` 또는 `continuation`. `view="brief"`와 모든 non-Task artifact에는 절대 포함하지 않는다.

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
  since_revision_id?: number,
  target_type?: string,           // default "Task"
  applicable_rule_limit?: number  // default 10, max 20
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
  applicable_rules: Array<{
    artifact_id: string,
    slug: string,
    type: string,
    title: string,
    area_slug: string,
    severity: "binding" | "guidance" | "reference",
    excerpt: string,
    agent_ref: string,
    human_url: string,
    human_url_abs?: string
  }>,
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

`applicable_rules`는 semantic search 결과가 아니라 `artifact_meta.rule_severity`로 marking된 정책 wiki를 area/type metadata로 매칭한 결과다. 정렬은 `binding` → `guidance` → `reference`, 그 다음 target area와 가까운 rule 순서다. `applies_to_areas`가 비어 있으면 해당 rule artifact의 own area + sub-area에 적용되고, cross-cutting child area의 rule은 모든 task에 자동 적용된다.

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
           "claimed_done" | "blocked" | "cancelled",
  area_slug?: string,
  priority?: "p0" | "p1" | "p2" | "p3",
  assignee?: string,          // exact task_meta.assignee match; pair with compact for assigned-only view
  limit?: number,             // default 50, max 500
  compact?: boolean           // omit project-wide aggregate maps (status_counts/area_counts/priority_counts/warning_counts); items+totals preserved
}
```

### Output

```typescript
{
  source_semantics: "reader_tasks_queue_v1",
  status_filter: string,
  default_focus: "assignee_open_count",
  total_count: number,
  pending_count: number,       // status missing + open
  compact?: boolean,           // mirrors input flag — true means the four aggregate maps below are omitted
  status_counts?: {            // omitted when compact=true
    open: number,
    missing_status: number,
    claimed_done: number,
    blocked: number,
    cancelled: number,
    other: number
  },
  area_counts?: Record<string, number>,    // omitted when compact=true
  priority_counts?: Record<string, number>,// omitted when compact=true
  warning_counts?: {                       // omitted when compact=true
    TASK_STATUS_MISSING?: number,
    TASK_ACCEPTANCE_DONE_RECONCILE_PENDING?: number
  },
  items: Array<{
    artifact_id: string,
    slug: string,
    title: string,
    area_slug: string,
    status: string,
    status_bucket: string,
    missing_status?: boolean,
    priority?: string,
    assignee?: string,
    due_at?: string,
    acceptance_checkboxes_total: number,
    resolved_checkboxes: number,
    unresolved_checkboxes: number,
    partial_checkboxes: number,
    deferred_checkboxes: number,
    ready_to_close: boolean,
    ready_to_close_status:
      "ready" | "unresolved_acceptance" | "no_acceptance_checkboxes" |
      "blocked" | "terminal_status" | "not_open",
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
- Agent 기본 시야는 `default_focus="assignee_open_count"`다. historical total(`assignee_filtered_count`, `total_count`)은 reference로만 본다.
- `missing_status`는 Reader의 `no_status` 컬럼과 같은 의미이며 pending count에 포함된다.
- `ready_to_close`는 item-level acceptance checklist 신호다. Queue count는 lifecycle count이고, ready signal은 close 후보 판단 보조 필드다.
- `TASK_STATUS_MISSING`은 Task lifecycle metadata가 없는 row를 놓치지 않게 하는 guardrail이다.
- `TASK_ACCEPTANCE_DONE_RECONCILE_PENDING`은 acceptance가 100% 해결됐지만 아직 `claimed_done`으로 reconcile되지 않은 transient row를 뜻한다. `pindoc.ping`은 이런 row를 자동으로 `claimed_done`으로 전이한다.
- `pindoc.scope.in_flight`는 `[ ]` / `[~]` acceptance item 조회용이다. Task row의 lifecycle status와 혼동하지 않는다.

### 구현 상태

- ✅ registered in MCP server and toolset catalog
- ✅ default pending semantics match Reader (`missing_status` + `open`)
- ✅ counts are computed before returned item `limit`
- ✅ `compact` mode omits project-wide aggregate maps; totals + items preserved
- ✅ `assigned-only` view = existing `assignee` filter (exact match) + `compact=true`
- ✅ `default_focus` and item-level `ready_to_close` split lifecycle count from close readiness

---

## 14. `pindoc.task.assign`

> Task assignee 단건 변경용 semantic shortcut. 본문/acceptance는 건드리지 않고 `task_meta.assignee`만 meta_patch revision으로 남긴다.

### Input

```typescript
{
  slug_or_id: string,        // UUID, slug, or pindoc:// URL
  assignee: string,          // agent:<id> | user:<id> | @<handle> | "" clear; omitted is not a valid assign input
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

## 16. `pindoc.task.claim_done`

> Task 구현 완료 선언용 semantic shortcut. 본문의 모든 미체크 acceptance(`- [ ]`)를 `[x]`로 토글하고 `task_meta.status`를 `claimed_done`으로 옮기는 두 변경을 한 revision에 묶는다. `[x]`/`[~]`/`[-]`는 보존 — partial / deferred 판단을 자동 토글이 덮어쓰지 않는다.

### Input

```typescript
{
  slug_or_id: string,        // UUID, slug, or pindoc:// URL
  reason?: string,           // optional 2-200 runes; revision commit_msg
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
  agent_ref?: string,
  revision_number?: number,
  human_url?: string,
  human_url_abs?: string,
  changed_acceptance_count: number,
  prev_status?: string,
  new_status?: string,         // "claimed_done" on accepted
  error_code?: "CLAIM_DONE_MISSING_REF" | "CLAIM_DONE_TARGET_NOT_FOUND" |
               "CLAIM_DONE_NOT_A_TASK" | "CLAIM_DONE_ALREADY_DONE" |
               "CLAIM_DONE_ALREADY_VERIFIED" | "CLAIM_DONE_TASK_CANCELLED" |
               "REASON_LENGTH_INVALID",
  failed?: string[],
  checklist?: string[]
}
```

### 동작

- 본문의 4-state checkbox(`[ ]` / `[x]` / `[~]` / `[-]`) 중 `[ ]`만 `[x]`로 변경. 나머지는 그대로
- `task_meta.status`는 항상 `claimed_done`으로 shallow-merge (다른 task_meta 필드는 보존)
- 한 revision에 body + meta가 같이 기록됨 — `revision_shape="body_patch"`, `shape_payload={kind:"claim_done", changed_acceptance_count, prev_status, new_status}`
- 이벤트 `artifact.task_claimed_done` emit
- 이미 `claimed_done` / `verified` / `cancelled`인 Task는 거절 (위 error_code) — 다음 단계 도구 안내 포함

### 구현 상태

- ✅ registered in MCP server and toolset catalog
- ✅ Reason length validation (optional but enforced when supplied)
- ✅ Type guard — Task만 허용
- ✅ Status guard — claimed_done / verified / cancelled 차단
- ✅ Acceptance toggle은 미체크(`[ ]`)만 변경, 다른 4-state 마커 보존
- ✅ `revision_shape="body_patch"` + `shape_payload.kind="claim_done"`로 기존 DB CHECK constraint 유지

---

## 17. `pindoc.runtime.status`

> Read-only 진단 스냅샷. 5830/5832 포트 혼선, "재시작이 필요한가?", "현재 어느 commit이 떠있나?" 같은 환경 질문 한 번에 응답. 어떤 mutation도 발생시키지 않는다.

### Input

```typescript
{}                  // no inputs in V1
```

### Output

```typescript
{
  version: string,                    // deps.Version (build version)
  server_commit?: string,             // vcs.revision from runtime/debug.ReadBuildInfo
  build_modified?: boolean,           // vcs.modified — dirty working tree at build time
  toolset_version: string,            // catalog hash; same value pindoc.ping returns
  tool_count: number,                 // len(RegisteredTools)
  source?: "loopback" | "oauth",      // calling Principal.Source (Decision decision-auth-model-loopback-and-providers)
  providers: string[],                // active IdPs from PINDOC_AUTH_PROVIDERS (empty = loopback-only)
  bind_addr?: string,                 // PINDOC_BIND_ADDR (default 127.0.0.1:5830)
  ports: Array<{                      // configured listeners with env overrides
    name: "http" | "sidecar",
    port: number,
    healthy: boolean
  }>,
  container_id?: string,              // Docker short id when HOSTNAME is 12 hex chars
  image_tag?: string,                 // PINDOC_IMAGE_TAG env when set
  hostname?: string,                  // os.Hostname() — useful when container_id is empty
  transport?: "stdio" | "streamable_http",
  go_version?: string,                // runtime.Version()
  db_healthy: boolean,                // single deps.DB.Ping with the request ctx
  notice?: string                     // hint about toolset_version mismatch handling
}
```

### 동작

- 입력이 없는 단일 read 호출. receipt gate 면제 (read-only).
- `server_commit` / `build_modified`은 Go 1.18+ vcs stamping이 켜진 상태에서만 채워진다 (`go run ./...` 빌드는 비어 있다).
- `ports`는 `PINDOC_HTTP_PORT` / `PINDOC_SIDECAR_PORT` env가 설정되면 그 값으로, 아니면 5830/5832 default. `healthy=true`는 응답 process가 listening 중이라는 in-process 가정 — out-of-process 검증을 추가하려면 후속 분리 필요.
- `container_id`는 Docker 기본 동작(HOSTNAME = 12-hex-shortened-id)을 가정한다. Kubernetes / Podman 등에서는 empty가 정상이며 caller는 `hostname`을 fallback으로 본다.

### 구현 상태

- ✅ registered in MCP server and toolset catalog (Phase 1 handshake group)
- ✅ vcs.revision / vcs.modified extracted via runtime/debug.ReadBuildInfo
- ✅ port override env vars (`PINDOC_HTTP_PORT`, `PINDOC_SIDECAR_PORT`)
- ✅ DB ping with request context
- ✅ unit test covers port resolution + Docker container id shape predicate

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

## Telemetry metadata

`mcp_tool_calls.metadata`(migration 0024)는 호출별 의미-있는 attribute를 담는 JSONB 컬럼이다. Phase J 기본 컬럼(byte / token / error)과 별개로, tool별 사용 패턴을 SQL 한 줄로 분석할 수 있게 한다 (Decision `mcp-dx-외부-리뷰-codex-1차-피드백-6항목` 발견 4).

extractor가 없는 tool은 `'{}'`를 기록한다. V1 extractor:

| tool | metadata keys |
|------|---------------|
| `pindoc.workspace.detect` | `via` (priority chain branch — pindoc_md_path / git_remote / workspace_path / fallback 등) |
| `pindoc.area.list` | `include_templates` (boolean) |
| `pindoc.artifact.propose` | `shape` · `artifact_type` · `area_slug` (각 필드는 caller가 supply했을 때만 포함) |
| `pindoc.artifact.search` | `top_k` · `include_templates` · `hits_count` |

GIN 인덱스 `idx_tool_calls_metadata_gin`이 `jsonb_path_ops` 기반으로 같이 생성되어 `metadata @> '{"via":"pindoc_md_path"}'` 같은 containment 쿼리가 빠르다.

신규 extractor 추가 절차:

1. `internal/pindoc/mcp/tools/telemetry_wrap.go`의 `extractToolMetadata` switch에 케이스 추가
2. test (`telemetry_metadata_test.go`)에 expected payload 추가
3. 본 문서 표 업데이트

---

## 관련 문서

- Harness 스펙: [09 PINDOC.md Spec](09-pindoc-md-spec.md)
- 아키텍처 전반: [03 Architecture](03-architecture.md)
- 데이터 모델: [04 Data Model](04-data-model.md)
- 메커니즘: [05 Mechanisms](05-mechanisms.md)
- 용어집: [Glossary](glossary.md)
