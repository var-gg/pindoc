# Session Handoff — 2026-04-22

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

### Phase 8 — URL 멀티프로젝트 재구조화 (이번 세션, 2026-04-22)
- **UI canonical**: `/p/:project/{wiki,tasks,graph,inbox}/...`. 모든 라우트 전환 완료.
- **HTTP canonical**: `/api/p/:project/...` — 단일 프로젝트 detail은 `/api/p/:project` 로 단순화.
- **인스턴스 레벨 엔드포인트**: `/api/config`, `/api/projects`, `/api/health`.
- **레거시 redirect**: `/`, `/wiki/*`, `/tasks/*`, `/graph`, `/inbox` 모두 `/p/{default}/...` 로 302 (LegacyRedirect 컴포넌트가 `/api/config` 로 default 결정).
- **MCP tool 신규**: `pindoc.project.create(slug, name, primary_language[, color, description])` — 프로젝트 row + `misc` area seed + canonical URL 반환.
- **TopNav**: Project Switcher 드롭다운 실제로 열리게 구현. 현재 프로젝트 + 기타 프로젝트 목록 + "새 프로젝트는 에이전트에게 요청" 안내. inert placeholder 제거.
- **env 토글**: `PINDOC_MULTI_PROJECT=true|false` — V1.5 권한 모델 확장 지점.
- **Home 이동**: 기존 `/` Home 페이지는 `/design` 로 이동 (design-system preview scaffold 접근성 유지).
- **PINDOC.md 템플릿**: URL 규약 섹션 추가, `pindoc.project.create` 호출 방법 명시.
- **docs/03-architecture.md**: "URL convention" 섹션 신규.
- **docs/12-m1-implementation-plan.md**: Phase 8 + V1.5 블록 기록.
- **dead code 제거**: `web/src/routes/Wiki.tsx` (App.tsx가 ReaderShell로 대체한 뒤 고아 상태였음) 삭제.

**git log oneline 확인**: `git log --oneline -20` 으로 전체 체인 보기.

---

## 2. 다음 작업 — 실제 embedding 붙이기 (Phase 8 완료 후 다음)

Phase 8 (URL 멀티프로젝트 재구조화)은 완료됨 — 하단 §5 참고. 다음 큰 블록:

### 실제 embedding 붙이기
- `services/embed-sidecar/` Python FastAPI 기동 + `PINDOC_EMBED_PROVIDER=http` 로 스위치.
- 기존 artifact_chunks 재-embed 배치 (작은 스크립트 하나).
- 스모크: `/api/p/pindoc/search?q=…` 한국어 쿼리에 의미 있는 답.

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
- [ ] `PINDOC_MULTI_PROJECT=true` 환경에서 두 번째 프로젝트 URL `/p/<new>/wiki` 정상

---

## 3. 재개 체크리스트

### 현재 실행 중이어야 할 서비스
- **Postgres + pgvector**: `docker compose ps` → `pindoc-db` healthy
- **pindoc-api**: port 5831. 멈춰있으면 `./bin/pindoc-api.exe &`
- **Vite dev**: port 5830. 멈춰있으면 `cd web && pnpm dev`
- **MCP 서버**: stdio, Claude Code가 필요 시 subprocess spawn. `.mcp.json` 등록됨.

### 환경
- 저장소 루트: `A:\vargg-workspace\pindoc` (폴더명 아직 `pindoc` 이 아니라
  레거시 `varn` 인 경우 있을 수 있음 — Windows 파일 핸들 때문에 rename 안 됨.
  `git remote` 는 `pindoc.git`)
- Go 1.26.2: `%LOCALAPPDATA%\Programs\Go\bin` (user PATH 등록 완료)
- Node 20.15 + pnpm 10
- Python 3.11 (embed sidecar 필요 시, 현재 stub 사용 중)
- Docker Desktop 27.3

### 현재 DB 상태 (스냅샷)
- 1 project (pindoc, primary_language=ko)
- 9 areas
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
