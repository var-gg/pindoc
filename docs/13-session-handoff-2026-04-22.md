# Session Handoff — 2026-04-22

> **Superseded by [docs/15-session-handoff-dogfood.md](./15-session-handoff-dogfood.md).**
> 이 문서는 Phase 7~16 개발 세션의 누적 로그로 역할을 마쳤다. Dogfood
> 시작 시점의 깔끔한 state + 다음 세션 프롬프트는 15번 파일을 참조.
> 아래 내용은 Phase별 변경 이력의 archive로만 유지한다.

---

이 세션 (2026-04-21 심야 ~ 04-22 새벽) 동안 M1 전체 + Phase 7(revision)까지 완료.
다음 세션이 바로 이어갈 수 있도록 현재 상태 / 다음 작업 / 주의사항 정리.

---

## 1. 완료된 작업 요약

### M1 Phase 1~6 (기획 → 기본 dogfood)
- **[bfbba10]** plan + decisions Q1 refine
- **[590edf8]** Phase 1 end-to-end verify (`pindoc.ping`)
- **[fddc4aa]** Phase 2.2 read tools (project.current, area.list, artifact.read)
- **[80b642f]** Phase 2.3 write + harness.install
- Phase 3 — embedding layer, artifact.search, context.for_task
- Phase 4 — HTTP read API + live Wiki Reader
- Phase 5 — i18n (ko/en)
- Phase 6 — `cmd/pindoc-seed` + docs/ 15개 artifact 임포트

### M1.5 — Reader shell 풀 포팅
- **[6195fa3]** GFM+Mermaid markdown, filter visibility, avatar placeholder, rendering contract

### Phase 7 — Revision system
- `artifact_revisions` 테이블 + migration 0004
- `propose.update_of` + `commit_msg` 업데이트 경로
- Diff 엔진 (`internal/pindoc/diff/`) — unified + section_deltas
- MCP tools: `pindoc.artifact.revisions`, `.diff`, `.summary_since`
- HTTP: `GET /api/artifacts/:slug/revisions`, `GET /api/artifacts/:slug/diff` (이후 Phase 8에서 `/api/p/:project/...` 로 이동)
- UI: `/wiki/:slug/history`, `/wiki/:slug/diff?from=&to=` (이후 Phase 8에서 `/p/:project/wiki/...`)
- PINDOC.md 템플릿에 update flow 문서화

### Phase 15 — Dogfood-driven UX 완결 (2026-04-22 완료)
저자가 1호 사용자 관점에서 "지금 필요한" 네 가지 묶음. "V1.x 미루기" 패턴을 명시적으로 거부.

- **15D**: [harness_install.go](../internal/pindoc/mcp/tools/harness_install.go) PINDOC.md 템플릿에 "Task auto-proposal heuristic" 섹션 — imperative 표현 / Decision 후속 / Debug regression test / Analysis open questions 등 capture signals + code-derived vs design Task 구분 + anti-patterns.
- **15A**: Migration 0008 legacy taxonomy 기준 `architecture` 하위 `embedding-layer` / `mcp-surface` sub-area seed. [Sidebar.tsx](../web/src/reader/Sidebar.tsx) 재귀 `AreaTreeNode` + chevron toggle. DB parent_id schema는 0001부터 있던 것 — UI만 보완.
- **15C**: Migration 0009 `artifact_pins.kind` enum (`code | resource | url`). 인프라/URL 참조 artifact가 path에 억지 string 안 넣어도 됨. Preflight + PinRef 응답 필드 확장.
- **15B**: Migration 0010 `artifacts.task_meta JSONB` + 부분 인덱스. TaskMetaInput (status/priority/assignee/due_at/parent_slug). Preflight 검증 + 4 stable codes. HTTP list/detail에 task_meta + edges 노출. Reader Tasks → kanban-lite (4 column + no_status/cancelled) + Sidecar `ConnectedArtifacts` (outgoing/incoming edges 카드). Drag-drop 의도적 미구현 (agent-only write 원칙).

### Phase 14 — Operator settings + contract hardening (2026-04-22 완료)
3차 피어리뷰 반영. 수용 8 / 반려 10 / 놓침 3 — [docs/14 §9](./14-peer-review-response.md) 참조.

**14A — settings infra**:
- Migration 0007 `server_settings` 단일 row 테이블. env는 first-boot seed만, DB가 source of truth (Ghost/Plausible 패턴).
- `internal/pindoc/settings/` package, `cmd/pindoc-admin` CLI (list/get/set).
- pindoc-server + pindoc-api 둘 다 Settings 로드 + env seed.
- `capabilities` 블록 확장: `scope_mode: "fixed_session"`, `new_project_requires_reconnect: true`, `receipt_ttl_sec: 1800`, `requires_expected_version: true`, `public_base_url`.
- `auth_mode: "none"` → `"trusted_local"` rename. Receipt TTL 10분 → 30분.
- HTTP `/api/config`에 `public_base_url` 노출.

**14B — contract hardening**:
- `human_url_abs` 필드: 4개 MCP tool + RelatedRef/EdgeRef/CandidateUpdate/SearchHit/ContextLanding 응답에 (DB public_base_url 있을 때만).
- `project.create` 응답에 `reconnect_required: true`, `activation: "not_in_this_session"`, `next_steps[]` — onboarding dead-end machine-readable로 해결.
- `artifact.propose(update_of=…)` **expected_version hard enforce**: 미제공 시 `NEED_VER` + current head 정보 포함. 1차 때 soft였던 결정 뒤집음.
- 모든 not_ready 응답에 `patchable_fields[]` 추가 — stable code → 수정 필드 매핑 (`patchFieldsFor`).
- Accepted create 응답에 `warnings[]` 추가 — `RECOMMEND_READ_BEFORE_CREATE` (semantic distance 0.18-0.25 band 이웃 존재 시).
- `harness.install` 템플릿 강화: "create 전 context.for_task/artifact.search 필수", "update path는 expected_version 필수", `failed[] + patchable_fields[]` 중심 대응.

### Phase 13 — Template artifact seed (2026-04-22 완료)
- Migration 0006: `_template_debug`, `_template_decision`, `_template_analysis`, `_template_task` 4개 artifact를 pindoc 프로젝트 `misc` area에 seed. 각 body에 권장 섹션 구조 (Debug: 증상/재현/가설/원인/해결/검증/Open questions, Decision: Context/Decision/Rationale/Alternatives/Consequences, Analysis: TL;DR/Scope/Findings/조사시점/재조회방법/Open, Task: 목적/범위/TODO(acceptance)/리소스/TC/DoD/Open). 모두 `tags: ['_template']`.
- `pindoc.project.create` tool도 신규 프로젝트 생성 시 template 4개 자동 seed — migration과 template body 동기화는 `internal/pindoc/mcp/tools/templates.go`의 `templateSeeds` slice 기준.
- **HTTP API filter**: `/api/p/:project/artifacts` 기본 응답에서 `_template_` prefix 제외 (NOT starts_with). `?include_templates=true` 쿼리 시 포함.
- **Reader UI "Show templates" 토글**: Sidebar에 새 section. LayoutTemplate 아이콘. `showTemplates` state → `useReaderData(project, slug, includeTemplates)` → `api.artifacts(project, {includeTemplates})` 파이프라인. i18n ko/en 추가 (`sidebar.view`, `sidebar.templates`, `sidebar.templates_hint`).
- **PINDOC.md 템플릿 ("Template-first propose")**: 신규 Debug/Decision/Analysis/Task propose 전 `pindoc.artifact.read(_template_<type>)` 먼저 호출, 섹션 구조를 skeleton으로 사용하라는 규약. Template 자체는 `update_of`로 evolving.
- Template 4개 real embedder로 재-embed → `context.for_task` / `search` 결과에 포함되므로 agent가 자연스럽게 발견 가능.

### Phase 12 — Agent ergonomics (2026-04-22 완료)
- **12a `not_ready` envelope primacy**: `artifact.propose` 응답에 `NextTools[]` (fail code → 다음 호출 매핑) + `Related[]` (RelatedRef: id/slug/type/title + agent_ref/human_url + reason) 필드 추가. `CONFLICT_EXACT_TITLE` / `POSSIBLE_DUP`에 실제 related refs 채움. 기존 `Checklist` / `SuggestedActions` (자연어)는 backward-compat 유지.
- **12b `artifact.read(view=brief|full|continuation)`**:
  - `brief`: body_markdown 제외, `summary` (첫 paragraph/240자) + `pins[]` + `stale` 플래그. 스캔용.
  - `full`: 기존 동작 (default, backward-compat).
  - `continuation`: brief + `recent_revisions[]` (최근 3개) + `relates_to[]` / `related_by[]` edges.
  - 응답에 `view` 필드로 현재 모드 echo.
- **12c actor hardening (stdio)**: server startup 시 `PINDOC_AGENT_ID` env 또는 random `ag_<hex>` 생성 → `Deps.AgentID`. `artifact_revisions.source_session_ref` JSONB에 `{agent_id, reported_author_id, source_session}` 저장. `author_id`는 client-reported 표시용, `agent_id`는 server-trusted provenance.

### Phase 11 — Write contract 강화 + semantic conflict (2026-04-22 완료)
- Migration 0005: `artifact_pins`, `artifact_edges` (relation ∈ implements/references/blocks/relates_to), `_unsorted` area seed. `pindoc.project.create` tool도 `misc` + `_unsorted` 둘 다 seed하게 업데이트.
- `artifact.propose` 입력 확장: `pins[]`, `relates_to[]`, `expected_version`, `supersede_of`, `basis{search_receipt, source_session}`. update/supersede/create 세 경로 + 상호 배제 검증.
- **search_receipt hard enforce (11b)**: `internal/pindoc/receipts/` — in-memory TTL 10분 ledger. `artifact.search`/`context.for_task` 응답에 receipt 포함. `artifact.propose(create)`에서 필수 — 없으면 `NO_SRCH`, 만료면 `RECEIPT_EXPIRED`, 다른 프로젝트면 `RECEIPT_WRONG_PROJECT`.
- **Semantic conflict block (11b)**: exact-title 통과 후, title+body 첫 800자 embed → top 근접 artifact distance < 0.18이면 `POSSIBLE_DUP` 차단. stub provider일 땐 비활성 (false-positive 방지).
- **context.for_task 확장 (11c)**: `candidate_updates[]` (distance < 0.22인 landings), `stale[]` (60일+ updated_at 없는 landings, 추후 pin-diff로 교체), `search_receipt` 발급.
- **Preflight stable codes**: 기존 자연어 checklist와 병행해서 `Failed[]` 배열 (stable code). Phase 12에서 primary로 승격 예정. 추가 코드: `TYPE_INVALID`, `TITLE_EMPTY`, `BODY_EMPTY`, `AREA_EMPTY`, `AUTHOR_EMPTY`, `COMPLETENESS_INVALID`, `TASK_NO_ACCEPTANCE`, `DEC_NO_SECTIONS`, `DBG_NO_REPRO`, `DBG_NO_RESOLUTION`, `PIN_PATH_EMPTY`, `PIN_LINES_INVALID`, `REL_TARGET_EMPTY`, `REL_INVALID`, `REL_TARGET_NOT_FOUND`, `VER_INVALID`, `VER_CONFLICT`, `UPDATE_SUPERSEDE_EXCLUSIVE`, `SUPERSEDE_TARGET_NOT_FOUND`, `NO_SRCH`, `RECEIPT_UNKNOWN`, `RECEIPT_EXPIRED`, `RECEIPT_WRONG_PROJECT`, `POSSIBLE_DUP`, `CONFLICT_EXACT_TITLE`, `UPDATE_TARGET_NOT_FOUND`.
- **Debug 타입 keyword 강화**: `DBG_NO_REPRO` / `DBG_NO_RESOLUTION` — "Debug body에 증상/해결 정보 필요" (ko/en 키워드 모두 커버).
- Propose 응답에 `pins_stored`, `edges_stored`, `superseded` 카운트 추가 → agent가 재-read 없이 저장 확인.

### Phase 10 — Real embedder dogfood (2026-04-22 완료)
- Docker TEI (`intfloat/multilingual-e5-base`, 768 dim, `--auto-truncate`) compose 서비스 추가.
- `embed/http.go`에 E5 prefix (`query: ` / `passage: `) 로직, config/registry에 wiring.
- `cmd/pindoc-reembed` CLI 신규: per-artifact 트랜잭션, 32개 배치 전송.
- Makefile에 `embed-up`, `server-run-http`, `api-run-http`, `reembed-build` 타겟 + `EMBED_ENV` 블록.
- 17개 artifact 전체 재-embed. 의미 검색 품질 실측 확인 (한국어 쿼리 distance 0.14-0.17, stub 때 랜덤 수준에서 실제 의미 매칭으로 전환).
- `capabilities.retrieval_quality` 자동으로 `"http"` 반영.

### Phase 9 — Referenced Confirmation hardening (2026-04-22 완료)
- `artifact.{propose,read,search}` + `context.for_task` 응답에 `agent_ref` (`pindoc://<slug>`) + `human_url` (`/p/:project/wiki/<slug>`) 분리.
- `project.current` 응답에 `capabilities` 블록 (`multi_project`, `retrieval_quality`, `auth_mode`, `update_via`, `review_queue_supported`). bootstrap 1 call.
- `docs/10-mcp-tools-spec.md`에 "Implementation Status" 테이블 + tool별 ✅/🟡/📋 뱃지 추가 — 외부 리뷰 P0 "spec↔runtime drift" 해소.
- `docs/14-peer-review-response.md` 신설 — 두 고급추론 리포트 수용/변형수용/반려 판단 구조화.
- `docs/12-m1-implementation-plan.md`에 Phase 9~13 체인 기록 (Phase 10 real embedder → Phase 11 write contract + semantic conflict → Phase 12 envelope/view/actor → Phase 13 template artifact seed).

### Phase 8 — URL 멀티프로젝트 재구조화 (2026-04-22 완료)
- **UI canonical**: `/p/:project/{wiki,tasks,graph,inbox}/...`. 모든 라우트 전환 완료.
- **HTTP canonical**: `/api/p/:project/...` — 단일 프로젝트 detail은 `/api/p/:project` 로 단순화.
- **인스턴스 레벨 엔드포인트**: `/api/config`, `/api/projects`, `/api/health`.
- **레거시 redirect**: `/`, `/wiki/*`, `/tasks/*`, `/graph`, `/inbox` 모두 `/p/{default}/...` 로 302 (LegacyRedirect 컴포넌트가 `/api/config` 로 default 결정).
- **MCP tool 신규**: `pindoc.project.create(slug, name, primary_language[, color, description])` — 프로젝트 row + `misc` area seed + canonical URL 반환.
- **TopNav**: Project Switcher 드롭다운 실제로 열리게 구현. 현재 프로젝트 + 기타 프로젝트 목록 + "새 프로젝트는 에이전트에게 요청" 안내. inert placeholder 제거.
- **env 토글**: `PINDOC_MULTI_PROJECT=true|false` — V1.5 권한 모델 확장 지점.
  *(2026-04-26 후속: env 제거. `multi_project` 는 `projects.CountVisible(...) > 1`
  로 자동 도출. 두 번째 프로젝트가 생기는 즉시 switcher 가 켜진다.)*
- **Home 이동**: 기존 `/` Home 페이지는 `/design` 로 이동 (design-system preview scaffold 접근성 유지).
- **PINDOC.md 템플릿**: URL 규약 섹션 추가, `pindoc.project.create` 호출 방법 명시.
- **docs/03-architecture.md**: "URL convention" 섹션 신규.
- **docs/12-m1-implementation-plan.md**: Phase 8 + V1.5 블록 기록.
- **dead code 제거**: `web/src/routes/Wiki.tsx` (App.tsx가 ReaderShell로 대체한 뒤 고아 상태였음) 삭제.

**git log oneline 확인**: `git log --oneline -20` 으로 전체 체인 보기.

---

## 2. 다음 작업 — 실 dogfood + V1.5 auth

Phase 8~15 전부 완료. M1 구현 + 3차 peer review + 1호 사용자 dogfood readiness 모두 마감. 다음 큰 선택지:

### 옵션 A: 실 dogfood 시작 (저자가 pindoc을 pindoc으로 관리)
- 기존 wikijs + OpenProject 흐름을 pindoc MCP로 전환
- 실사용 중 발견되는 UX/bug/gap 수집 → 4차 리뷰 or 직접 반영
- 관찰할 지표: NO_SRCH / POSSIBLE_DUP / CONFLICT_EXACT_TITLE 빈도, task_meta.status 전환 패턴, Area tree 실제 깊이, pin kind 분포

### 옵션 B: V1.5 인증 착수
- GitHub OAuth + agent token (per-project scope) + per-project ACL
- MCP tool 응답에 server-resolved principal 바인딩 (현 `agent_id`를 actor_id 정식화)
- Settings UI 구현 (현 `pindoc-admin` CLI 대체)
- Hot reload settings (Phase 14에서 숙제로 남긴 것)

### 옵션 C: 4차 외부 peer review
- Phase 14 + 15 반영 후 상태로 돌려보기
- Dogfood 전에 받는 의미는 있을지 판단 필요

저자 결정 대기.

리뷰 판단 근거: [docs/14-peer-review-response.md](./14-peer-review-response.md).

### 실행 환경 재개 체크리스트 (Phase 10 이후)

현재 세션 종료 후 다음 세션 시작 시:

```bash
docker compose up -d db embed   # Postgres + TEI 둘 다 기동
make api-run-http &             # pindoc-api with http embedder
cd web && pnpm dev              # Vite (필요 시)
```

또는 `make server-run-http` / `make api-run-http` 로 개별 기동.

TEI 첫 기동 시 모델 다운로드 2-3분. 이후는 volume 캐시 재사용.

### 아래는 Phase 8 계획 (완료됨 — 참고용 스냅샷)

**배경**: 현재 URL `/wiki/:slug` 는 프로젝트 스코프가 없어서 동료 공유 시
"받는 쪽의 현재 프로젝트"에 따라 다른 문서 열릴 수 있음. 다중 프로젝트는
V1.5+로 미뤄졌지만 URL 구조는 **지금** 박아야 미래에 안 깨짐.

### 결정사항 (저자와 합의 완료)

1. **canonical URL 형태**: `/p/:project/wiki/:slug` 등 모든 라이브 경로 앞에
   `/p/:project/` 접두사 추가. Notion/Slack 패턴.
2. **`/wiki/...` 레거시 경로는 302 redirect** → 현재 기본 프로젝트로.
3. **프로젝트 생성**:
   - 최초: `pindoc init` CLI (아직 미구현, 지금은 seed 마이그레이션이 대신함)
   - 이후: `pindoc.project.create` MCP tool (신규)
   - UI에서 "+ New Project" 버튼은 **없음** (원칙 1 — 에이전트 경유만)
4. **Project Switcher UI**: 지금은 항목 1개 ("pindoc 현재") + "+ 새 프로젝트 —
   에이전트에게 요청" 안내. 실제 멀티 프로젝트는 V1.5.
5. **인증은 별도 관심사**: 이번 작업에 포함 안 함. V1.5에서 같은 URL 구조
   위에 GitHub OAuth + agent token + 초대 플로우 얹음.

### 구현 범위 (약 2시간 분량)

**Backend**:
- `internal/pindoc/httpapi/router.go` — 모든 핸들러를 `/api/p/:project/...` 로
  이동. 기존 `/api/projects/current` 등은 deprecated redirect 또는 유지.
- `internal/pindoc/httpapi/handlers.go` — `r.PathValue("project")` 로 프로젝트
  해석 (지금은 `deps.ProjectSlug` 하드코딩).
- `internal/pindoc/httpapi/history.go` — 동일 수정.
- `internal/pindoc/mcp/tools/project.go` — `RegisterProjectCreate` 추가:
  ```
  pindoc.project.create(slug, name, primary_language, color?) → {id, slug}
  ```
  INSERT into projects (ON CONFLICT DO NOTHING, 에러 메시지 친화적).
- `internal/pindoc/mcp/server.go` — `RegisterProjectCreate` 등록.
- 멀티프로젝트 모드 토글: `PINDOC_MULTI_PROJECT=true|false` env (default false).
  false면 Project Switcher 숨김, 새 프로젝트 생성은 허용되되 경고 로그.
  *(2026-04-26 후속: env 제거. switcher 가시성은 `projects.CountVisible > 1`
  derived value 로 일원화.)*

**Frontend (`web/`)**:
- `src/App.tsx` 라우트 전면 개편:
  ```tsx
  <Route path="/p/:project/wiki"               element={<ReaderShell view="reader" />} />
  <Route path="/p/:project/wiki/:slug"         element={<ReaderShell view="reader" />} />
  <Route path="/p/:project/wiki/:slug/history" element={<History />} />
  <Route path="/p/:project/wiki/:slug/diff"    element={<Diff />} />
  <Route path="/p/:project/tasks"              element={<ReaderShell view="tasks" />} />
  <Route path="/p/:project/tasks/:slug"        element={<ReaderShell view="tasks" />} />
  <Route path="/p/:project/graph"              element={<ReaderShell view="graph" />} />
  <Route path="/p/:project/inbox"              element={<ReaderShell view="inbox" />} />
  <Route path="/wiki/*"                        element={<LegacyRedirect />} />
  <Route path="/tasks/*"                       element={<LegacyRedirect />} />
  <Route path="/"                              element={<DefaultProjectRedirect />} />
  ```
  `LegacyRedirect` / `DefaultProjectRedirect` 는 `/api/projects/current` 로
  현재 프로젝트 slug 얻어서 302.
- `src/reader/ReaderShell.tsx` — `useParams<{ project: string; slug?: string }>()`
  로 바꾸고 `project`를 상태/링크에 반영.
- `src/reader/History.tsx`, `src/reader/Diff.tsx` — 동일.
- `src/api/client.ts` — 엔드포인트 경로에 project 파라미터 추가:
  `api.currentProject(slug)`, `api.areas(project)`, `api.artifacts(project, filters)`,
  `api.artifact(project, slug)`, `api.revisions(project, slug)`, `api.diff(project, slug, ...)`.
- `src/reader/TopNav.tsx` — project chip 드롭다운을 **실제로** 열기 (지금은 inert).
  드롭다운에 현재 프로젝트 1개 + 안내 문구 "+ 새 프로젝트 — 에이전트에게 요청하세요".
- `src/reader/CmdK.tsx` — 결과 링크를 `/p/:project/wiki/:slug` 로 생성.
- i18n 추가: `nav.new_project_hint`, `nav.no_other_projects`.

**Docs**:
- `docs/03-architecture.md` — "URL convention" 섹션 신규 (위 결정사항).
- `docs/12-m1-implementation-plan.md` — M1.5 / V1.5 타임라인에 auth / invite /
  multi-project UI 항목 명시.
- `internal/pindoc/mcp/tools/harness_install.go` — PINDOC.md 템플릿에
  `pindoc.project.create` 언급 + URL 공유 규약 추가.

### 체크포인트
- [ ] 빌드: `go build ./...` + `pnpm typecheck`
- [ ] 수동 검증: `/` → `/p/pindoc/wiki` 리다이렉트 작동
- [ ] 기존 `/wiki/decisions-log` 가 `/p/pindoc/wiki/decisions-log` 로 감
- [ ] 탑 네비 project chip 드롭다운 열림 (inert 아님)
- [ ] `pindoc.project.create` tool 스모크 — 새 slug 넣으면 DB에 row 생성
- [ ] 두 번째 프로젝트 row 가 생기면 `/api/config.multi_project` 가 자동 true,
  Reader switcher 가 노출되고 `/p/<new>/wiki` URL 이 정상 동작

---

## 3. 재개 체크리스트

### 현재 실행 중이어야 할 서비스
- **Postgres + pgvector**: `docker compose ps` → `pindoc-db` healthy
- **pindoc-api**: port 5831. 멈춰있으면 `./bin/pindoc-api.exe &`
- **Vite dev**: port 5830. 멈춰있으면 `cd web && pnpm dev`
- **MCP 서버**: stdio, Claude Code가 필요 시 subprocess spawn. `.mcp.json` 등록됨.

### 환경
- 저장소 루트: 로컬 작업 디렉터리의 Pindoc checkout (`git remote` 는
  `pindoc.git`)
- Go 1.26.2: `%LOCALAPPDATA%\Programs\Go\bin` (user PATH 등록 완료)
- Node 20.15 + pnpm 10
- Python 3.11 (embed sidecar 필요 시, 현재 stub 사용 중)
- Docker Desktop 27.3

### 현재 DB 상태 (스냅샷)
- 1 project (pindoc, primary_language=ko)
- 9 areas (legacy pre-reform snapshot; post-reform taxonomy는 `docs/19-area-taxonomy.md`)
- 17 artifacts (15 seeded + 2 테스트)
  - `decisions-log`는 revision 2까지 있음 (Phase 7 스모크 테스트)
  - `phase-3-embedding-test`, `phase-2-3-smoke-test` 는 제거해도 됨 (테스트 잔재)
- 400+ artifact_chunks (stub embedding, 의미 검색 품질 낮음)

### MCP 사용 준비
이 세션 시작 시점에 MCP tool들이 이미 로드됨:
- `mcp__pindoc__pindoc_ping`
- `mcp__pindoc__pindoc_project_current`
- `mcp__pindoc__pindoc_area_list`
- `mcp__pindoc__pindoc_artifact_read`
- `mcp__pindoc__pindoc_artifact_propose`
- `mcp__pindoc__pindoc_artifact_search`
- `mcp__pindoc__pindoc_context_for_task`
- `mcp__pindoc__pindoc_artifact_revisions`
- `mcp__pindoc__pindoc_artifact_diff`
- `mcp__pindoc__pindoc_artifact_summary_since`
- `mcp__pindoc__pindoc_harness_install`

스키마는 deferred 상태라 `ToolSearch` 로 로드 후 사용.

---

## 4. 알려진 지점 / 주의

- **Windows CRLF 경고** — git에서 LF→CRLF 변환 경고 계속 찍힘. 무해.
- **stub embedder** — `PINDOC_EMBED_PROVIDER=stub` 이 기본. 검색 품질 low.
  실제 모델 쓰려면 `services/embed-sidecar/` Python FastAPI 기동 + env
  `PINDOC_EMBED_PROVIDER=http`, `PINDOC_EMBED_ENDPOINT=http://127.0.0.1:5860/v1/embeddings`.
- **Graph view**: iframe 스텁. `@xyflow/react` 도입이 M1.5+ 과제.
- **Inbox**: 비어있음 (auto-publish만 있어 review_state=pending_review 항목 없음).
- **Task view**: Reader를 type=Task 필터로 재사용. 실제 Task-전용 UI (칸반 X,
  리스트 + status/priority chip)는 V1.1.
- **Avatar 우상단** = placeholder `—`. V1.5 auth 연결.
- **마이그레이션은 서버 시작 시 자동 적용**. schema_migrations 테이블에
  추적됨. 롤백은 수동 (SQL의 `-- +goose Down` 블록 참조).
- **로컬 폴더명 rename** — Claude Code 세션 열려있으면 Windows 파일 핸들 락
  때문에 실패. 세션 종료 후 수동 rename.

---

## 5. 그 다음 큰 블록 (URL 재구조화 후)

순서대로:

1. **실제 embedding 붙이기** — `services/embed-sidecar/` Python FastAPI 기동 +
   `PINDOC_EMBED_PROVIDER=http` 로 스위치. 기존 artifact_chunks 재-embed 배치
   (작은 스크립트 하나).
2. **Graph view React-ify** — `@xyflow/react` 도입, 현재 iframe 스텁 교체.
   Graph edge 도출은 Pin / supersedes / implements 3가지 엣지만 실데이터 있음.
3. **V1.5 인증** — GitHub OAuth + agent token + invite flow. 위 섹션 2의
   결정사항 참조.
4. **V1.5 `pindoc init` CLI** — 현재 seed 마이그레이션이 하고 있는 역할을
   proper CLI 로 분리. 언어 선택 + 에이전트 클라이언트 자동 감지 + .mcp.json
   패치.

---

## 6. 참고 파일

- `docs/12-m1-implementation-plan.md` — 원래 M1 계획 (체크박스)
- `docs/decisions.md` — resolved / open question 로그
- `.mcp.json` — Claude Code MCP 등록
- `docker-compose.yml` — DB 기동
- `Makefile` — dev 편의 명령

끝.
