# Session Handoff — Dogfood 시작 지점

이 세션에서 M1 구현 + Phase 7-16 + 3차 peer review 반영 모두 완료. 다음
세션부터 저자가 **1호 사용자로 Pindoc에 Pindoc 자체의 구조를 wiki로
발행**하는 dogfood를 시작한다. 이 문서는 그 시작점의 완전한 state
스냅샷이다.

이전 handoff [docs/13](./13-session-handoff-2026-04-22.md)은 여기로 대체된다.

---

## 1. 현재 state 요약

### 최신 커밋
`4840840 Phase 16: OSS readiness — license + CLA + projects.owner_id tenancy hook`

### 완료된 Phase 체인

| Phase | 내용 |
|---|---|
| 1-6 | Infra / MCP 기본 / artifact lifecycle / embed layer / Web UI / i18n / seed |
| 7 | Revision 시스템 (revisions / diff / summary_since) |
| 8 | URL 멀티프로젝트 재구조화 (`/p/:project/…` canonical) |
| 9 | `human_url`/`agent_ref` 분리 + capabilities 블록 + spec↔runtime drift 가시화 |
| 10 | Real embedder (Docker TEI + multilingual-e5-base) |
| 11 | Write contract 강화 (search_receipt hard / pins / expected_version / supersede / relates_to / semantic conflict) |
| 12 | Agent ergonomics (not_ready NextTools+Related / read view modes / agent_id) |
| 13 | Template artifact seed (_template_debug/decision/analysis/task) |
| 14 | Operator settings (server_settings + pindoc-admin CLI + human_url_abs + expected_version hard) |
| 15 | Dogfood-driven UX (Task heuristic / Area hierarchy / pin kind / task_meta + kanban) |
| 16 | OSS readiness (Apache 2.0 + CLA + projects.owner_id) |

### DB 최종 state (클린징 완료)

```
$ docker compose exec -T db psql -U pindoc -d pindoc -c \
  "SELECT slug, type FROM artifacts WHERE project_id=(SELECT id FROM projects WHERE slug='pindoc') ORDER BY slug;"

        slug        |   type
--------------------+----------
 _template_analysis | Analysis
 _template_debug    | Debug
 _template_decision | Decision
 _template_task     | Task
(4 rows)
```

이전 dummy data (Phase 6에서 import한 docs/*.md 15개 + Phase smoke test 3개)는
전부 삭제됨. `_template_*` 4개만 남김 (dogfood 시 agent가 읽을 양식
참조용). Events도 cascade 클린. Areas는 유지 (vision / architecture /
data-model / decisions / mechanisms / roadmap / ui / misc / _unsorted /
cross-cutting + architecture 하위 embedding-layer / mcp-surface).

Dogfood는 **빈 캔버스 + 양식 4개** 상태에서 시작.

### 환경 상태

| 컴포넌트 | 상태 | 재기동 |
|---|---|---|
| `pindoc-db` (Postgres 16 + pgvector) | docker, 계속 up | `docker compose up -d db` |
| `pindoc-embed` (TEI + multilingual-e5-base) | docker, 계속 up | `docker compose up -d embed` |
| `pindoc-api` | host binary, 세션 간 재기동 필요 가능 | `make api-run-http` |
| `pindoc-server` (MCP) | Claude Code가 subprocess로 spawn | `.mcp.json` 등록됨 |
| Vite dev | 5830 port | `cd web && pnpm dev` |
| `public_base_url` | `https://wiki.acme.dev` (테스트 값) | `./bin/pindoc-admin set public_base_url <your-url>` |

### 알려진 cosmetic 이슈

- `_template_*` 4개는 `artifact_revisions` row가 없음 (Phase 13 migration이 직접 INSERT라 Phase 7 backfill을 통과하지 않음). History 뷰가 빈 상태로 뜨지만, 첫 `update_of`가 들어오면 revision 1부터 자연 복구. 의도 아니고 그냥 미세한 빈틈.

---

## 2. Dogfood 시작 시 주의점

### 저자의 사고 모델
- **레포 내 `docs/*.md`는 repo의 공식 설계 문서**. git 버전 관리 대상.
- **Pindoc wiki artifact는 1호 사용자 dogfood의 결과물**. Agent가 작성한 것, 사용자가 참조한 것.
- **두 계층을 섞지 말 것**. repo md를 다시 artifact로 자동 import 유혹 금지 — 그건 Phase 6 pindoc-seed binary가 하던 일이고 이번엔 반복 안 함.

### dogfood 발행 흐름 (예상)

저자가 "현재 repo 구조를 wiki로 발행해줘"류의 지시를 할 때 agent가 따를 절차:

1. **scope 확인**: `pindoc.project.current` — `owner_id=default, slug=pindoc`.
2. **retrieval receipt**: `pindoc.context.for_task("repo 구조 요약")` 또는 `pindoc.artifact.search` — 빈 캔버스라 0 hit, receipt만 받음.
3. **template 참조**: `pindoc.artifact.read(id_or_slug="_template_analysis")` — Analysis 템플릿 구조 확인.
4. **repo 파일 읽기**: Read tool로 `docs/03-architecture.md` 같은 원본 확인.
5. **structured body 작성**: template 섹션 구조 기반 + repo 실 내용.
6. **propose**: `pindoc.artifact.propose`
   - `type="Analysis"`
   - `area_slug="architecture"` (또는 맞는 것)
   - `title`, `body_markdown`
   - `basis.search_receipt=<receipt>`
   - `pins=[{kind:"code", path:"docs/03-architecture.md"}]` 또는 주요 코드 path
   - `relates_to=[...]` (이미 발행한 다른 artifact가 있으면)
7. **사용자 확인 요청 시 `human_url_abs` 포함** (Referenced Confirmation 규약).

### 자주 부딪힐 것

- **첫 create에서 NO_SRCH**: receipt 없이 바로 propose 하면 막힘. harness 읽으면 해결.
- **POSSIBLE_DUP**: 여러 artifact 연속 발행 시 앞의 것과 유사도 0.18 이하면 차단. `update_of` 또는 title 좁히기.
- **PIN_KIND_INVALID**: code가 아닌 pin (예: AWS 리소스)은 `kind="resource"` 또는 `"url"`.
- **TASK_META_WRONG_TYPE**: Decision/Analysis에 task_meta 붙이면 거절.

### 관찰 포인트 (4차 review / 향후 개선용)

- search_receipt 재발행 빈도 (TTL 30분이 실제 루프에 맞는지)
- `candidate_updates[]` / `RECOMMEND_READ_BEFORE_CREATE` 경고 정확성 (real corpus에서 false-positive / miss)
- Reader Task kanban-lite 체감
- Sidecar `ConnectedArtifacts` 유용성
- Area hierarchy 실제 필요 깊이
- Template format이 실제 문서 작성에 얼마나 잘 맞는지 (template artifact revision 필요성)

---

## 3. 재개 체크리스트

### 다음 세션 시작 시

```bash
cd A:/vargg-workspace/pindoc

# 1. DB + embed 기동
docker compose up -d db embed

# 2. pindoc-api 기동 (http embedder env 포함) — 백그라운드
make api-run-http &

# 3. Vite dev (웹 UI 확인 원할 때)
cd web && pnpm dev
# → http://localhost:5830/p/pindoc/wiki 접속

# 4. health check
curl -s http://127.0.0.1:5831/api/health
curl -s http://127.0.0.1:5831/api/config
curl -s http://127.0.0.1:5831/api/p/pindoc | head -c 300
```

### MCP 연결 확인

Claude Code가 세션 시작 시 자동으로 pindoc MCP subprocess를 spawn. `.mcp.json`은 repo 내 등록됨. 세션에 아래 deferred tool들이 노출되어야:

- `mcp__pindoc__pindoc_ping`
- `mcp__pindoc__pindoc_project_current`
- `mcp__pindoc__pindoc_project_create`
- `mcp__pindoc__pindoc_area_list`
- `mcp__pindoc__pindoc_artifact_{read,propose,search,revisions,diff,summary_since}`
- `mcp__pindoc__pindoc_context_for_task`
- `mcp__pindoc__pindoc_harness_install`

---

## 4. 다음 세션 오프닝 프롬프트

아래 그대로 복사해 다음 세션 첫 입력으로 사용:

```
Pindoc dogfood 시작한다. 이전 세션에서 M1 + Phase 7-16까지 전부 완료됐고,
DB는 _template_* 4개 artifact만 남기고 클린징됐다. 현 repo 구조를 Pindoc
wiki로 직접 발행하면서 실 사용감을 관찰하려 한다.

상세 state: docs/15-session-handoff-dogfood.md 참조.

착수 순서:
1. 환경 check: docker compose ps / curl /api/health / curl /api/config.
   필요하면 docker compose up -d db embed + make api-run-http.
2. pindoc.project.current로 scope 확인.
3. 첫 wiki 발행 대상을 내가 지정하거나, 네가 repo 스캔 후 우선순위
   제안해도 된다. 발행 절차는 harness 규약 (context/search receipt →
   template.read → propose with basis.search_receipt + pins +
   relates_to). PINDOC.md 템플릿은 pindoc.harness.install로 재생성
   가능.
4. 발행 중 부딪히는 not_ready / NO_SRCH / POSSIBLE_DUP / 기타 마찰점은
   기록. 4차 peer review 자료 + 이후 phase 개선 근거가 된다.

기본 원칙:
- repo 내 docs/*.md 원본을 자동 import하지 말 것. 그 계층과 wiki
  artifact 계층은 다르다. 필요한 부분만 사람 수준 판단으로 압축/
  재구성해서 propose.
- 레퍼런스 확인 필요 시 docs/03-architecture.md 등 repo 파일 Read
  도구로 직접 읽고 body 구성.
- 사용자(나) 확인 요청은 항상 human_url 포함 (Referenced Confirmation).
- 네 agent_id는 서버가 발급 (env PINDOC_AGENT_ID 없으면 ag_<hex>).

시작해.
```

---

## 5. 참고 파일

- 최신 MCP tools 상태: [docs/10-mcp-tools-spec.md](./10-mcp-tools-spec.md) Implementation Status
- Phase 전체 계획: [docs/12-m1-implementation-plan.md](./12-m1-implementation-plan.md)
- 3차 peer review 판단: [docs/14-peer-review-response.md](./14-peer-review-response.md)
- 이전 handoff (superseded): [docs/13-session-handoff-2026-04-22.md](./13-session-handoff-2026-04-22.md)
