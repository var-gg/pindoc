# M1 Implementation Plan — "MCP bootable + dogfood ready"

기획 완료 → 구현 진입. 저자를 1호 사용자로 격상해서 Pindoc 자체 repo를 최초
dogfood 대상으로 쓴다.

**M1 정의**: 로컬에서 Pindoc MCP 서버 기동 → Claude Code 연결 → Pindoc 자체
repo에 artifact 쓰기 작동 → 웹 UI에서 실데이터 표시.

## 전체 단계

### Phase 1 — Infra + Server 스켈레톤

- `docker-compose.yml` — Postgres 16 + pgvector, 로컬 단일 호스트
- `cmd/pindoc-server/` Go 엔트리 + `internal/pindoc/` 레이아웃
- Official Go SDK ([modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)) 도입, stdio 전송
- `pindoc.ping` tool 하나 — handshake 검증
- Claude Code `.mcp.json` 등록 템플릿
- 마이그레이션 셋업 (goose 권장) — 빈 stub만

**Exit**: `claude mcp list`에서 pindoc 잡힘, `pindoc.ping` 왕복.

### Phase 2 — Core Artifact Lifecycle

- 마이그레이션: `projects`, `areas`, `artifacts` 테이블 (embedding 없이 body text만)
- 도구: `pindoc.harness.install`, `pindoc.project.current`, `pindoc.area.list`,
  `pindoc.artifact.propose`, `pindoc.artifact.read`
- Pre-flight Check: 타입별 필수 필드 검증
- 단순 conflict: 제목 정확일치만 (임베딩은 Phase 3에서)
- Auto-publish (Review Queue는 sensitive op 없는 V1에선 미동작)

**Exit**: Claude Code에서 "이 대화 ADR로 정리해줘" → propose → DB에 row → UI에서 읽힘.

### Phase 3 — Embedding + Fast Landing

- `internal/pindoc/embed/` — `Provider` 인터페이스 + 3개 구현 (embeddinggemma,
  bge-m3 optional build tag, http generic)
- 기본 default: **EmbeddingGemma-300M int8** (<200MB RAM, <15ms)
- Python sidecar vs onnxruntime-go: **MVP에선 Python FastAPI 사이드카**(빠른
  경로), V1.x에 onnxruntime-go로 in-process 이전
- 마이그레이션: `artifact_chunks` 테이블 (pgvector vector(768) 칼럼)
- 자동 chunking (H2/H3 boundary, 섹션별 embed)
- 도구: `pindoc.artifact.search`, `pindoc.context.for_task`

**Exit**: `context.for_task("장바구니")` 호출하면 top-3 관련 artifact + chunk snippet 반환.

### Phase 4 — Web UI 실데이터 연결

- `web/` shell에 HTTP client 추가
- `GET /api/projects/current`, `/api/areas`, `/api/artifacts/:id`, `/api/search?q=`
- Wiki Reader 페이지 React-ify (iframe 벗김) — artifact 하나 실제 렌더
- Sidebar Chrome React-ify (Area tree = API 호출)
- Task list view 실데이터 (status/priority 필터)

**Exit**: 저자 `localhost:5830/a/doc_xyz` 에서 본인이 쓴 artifact 읽음.

### Phase 5 — Harness Install + i18n

- `pindoc.harness.install` 구현: PINDOC.md 템플릿 생성 + CLAUDE.md include 한 줄 삽입
- `react-i18next` 도입, `web/src/locales/{ko,en}.json`
- 서버 NOT_READY 템플릿 ko/en 2세트 (`internal/pindoc/i18n/`)
- `pindoc init` CLI 플로우 (언어 질문 포함) → PINDOC.md에 `user.primary_language` 저장

**Exit**: 새 프로젝트에서 `pindoc init` 5분 내 완료, 언어 ko 선택, 첫 세션 생성 가능.

### Phase 6 — Meta-dogfooding 시드

- 기존 `docs/00~14.md` 를 Pindoc artifact로 임포트
  - 00-vision → Analysis
  - 01-problem → Analysis
  - 03-architecture → Analysis
  - decisions.md → 결정별 Decision artifact 분해
  - 05-mechanisms / 각 M# → Feature or Analysis
  - 07-roadmap → Analysis + V1 Tasks 추출
  - 기타: Glossary, Note 등
- `docs/` 원본 md는 archive로 남기되, 정식 편집은 Pindoc 안에서만
- CLAUDE.md에 "Pindoc 통해서만 docs 수정" 강제 문구

**Exit**: 저자가 새 결정 추가할 때 `docs/` 안 건드리고 Claude Code에 "이거 결정으로
등록" → Pindoc UI에서 확인.

## 시간 감각

AI-driven 구현이라 **hours 단위**. 각 Phase는 1세션~반나절. 총 10-15시간 분량을
여러 세션으로 잘라 진행.

막히는 구간:
- Phase 1: Go 설치 + go-sdk 처음 붙이기 (1-2h)
- Phase 3: EmbeddingGemma 모델 다운로드 + onnxruntime 또는 Python 추론 배선 (2-3h, 모델 다운 시간 별도)
- Phase 4: Wiki Reader React-ify (설계한 iframe 버리고 HTML prototype 기준으로 recreate) (2-3h)
- Phase 6: docs import matching (타입 매핑 결정 필요) (1-2h)

## Decision 체크포인트 (플랜 내 고정 확인 지점)

- [ ] Phase 1 끝: Claude Code가 pindoc ping 성공? → 진행
- [ ] Phase 2 끝: 첫 artifact 저장·읽기 정상? → 진행
- [ ] Phase 3 끝: 검색이 실제로 "장바구니" 같은 한국어 쿼리에 의미있는 답? → NO면 BGE-M3 swap
- [ ] Phase 4 끝: 저자가 Wiki Reader UI로 artifact 훑기 편한가? → 디자인 이슈 발견 시 Claude Design iterate
- [ ] Phase 5 끝: `pindoc init` 이 진짜 5분 내 끝나나? → 초과 시 재단순화
- [ ] Phase 6 중: 첫 meta-dogfooding 문서 변경이 실제로 Pindoc을 통해 일어나면 성공

## 외부 의존성 (사용자가 설치해야 할 것)

- **Go 1.24+** — MCP 서버 빌드. `winget install GoLang.Go` (Windows) 또는 nodejs.org/msi.
- **Docker Desktop 27+** — Postgres 컨테이너. ✅ 이미 있음 (`Docker version 27.3.1`).
- **pnpm 10+** — 웹 UI. ✅ 이미 있음 (`10.30.2`).
- **Node 20.15+** — 웹 UI 런타임. ✅ 이미 있음 (20.15.0, Node 22 권장).
- **Python 3.12+** — Phase 3 embedding 사이드카 (onnxruntime-go로 옮기면 제거).

## Go 설치 가이드 (저자 기계에 아직 없음)

Windows:
```powershell
winget install GoLang.Go
# 또는 https://go.dev/dl/ 에서 MSI 다운로드
```

설치 후:
```bash
go version   # go version go1.24.x windows/amd64 가 나오면 OK
```

## 실행 룩업 (완성 시)

```bash
# 1회 세팅
docker compose up -d      # Postgres + pgvector 기동
make migrate              # 스키마 적용

# 서버 개발 (별도 터미널에서 계속 run)
make dev-server           # Go MCP 서버 (stdio 대기) + HTTP read API
make dev-web              # pnpm dev, :5830
make dev-embed            # Python embedding 사이드카 (Phase 3+)

# Claude Code 등록 (1회)
claude mcp add pindoc <path-to-pindoc-server>

# 검증
claude mcp list           # pindoc 있음
# 새 Claude Code 세션에서: "pindoc.ping 실행" → pong
```

## 이 플랜이 변경될 수 있는 지점

- Phase 3 embedding 경로 (Python sidecar vs onnxruntime-go) — Phase 1 끝나고 재평가
- Phase 6의 docs import 매핑 — Phase 2 끝나면 실제 artifact schema로 머릿속 재조정 필요
- BGE-M3 swap — Phase 3에서 EmbeddingGemma 품질 미흡이면 즉시 전환

이 문서는 **진행하면서 계속 업데이트**. 각 Phase 끝날 때마다 "exit 조건 달성
여부 + 다음 Phase 조정 사항" 기록.

## Phase 8 — URL multi-project restructure (완료 · 2026-04-22)

멀티 프로젝트를 공식 V1.5로 미루되 **URL 구조만** 지금 박아서 나중에 URL 깨지지
않게 하는 중간 단계.

- 모든 UI 경로에 `/p/:project/` 접두사 (`/p/pindoc/wiki/...` 같은 형태)
- 모든 HTTP API를 `/api/p/:project/...` 로 이동 — 추가로 `/api/config`,
  `/api/projects` 인스턴스 레벨 엔드포인트
- `pindoc.project.create(slug, name, primary_language[, color, description])`
  MCP tool — 새 프로젝트 DB 생성 + `misc` area seed
- `/wiki/...`, `/tasks/...`, `/graph`, `/inbox`, `/` 모두 기본 프로젝트로 302
- TopNav Project Switcher 드롭다운 활성화 (현재 프로젝트 목록 + "새 프로젝트는
  에이전트에게" 안내)
- `PINDOC_MULTI_PROJECT=true|false` env — V1.5 권한 모델 확장 지점
- Home / design-system 스캐폴드는 `/design`, `/design/preview/:slug` 로 이동

## Phase 9 — Referenced Confirmation hardening (완료 · 2026-04-22)

외부 피어리뷰 (docs/14) 에서 P0로 지적된 "share URL이 `pindoc://slug` 한 가지뿐이라 사용자가 브라우저에서 못 열어본다"를 해소. Phase 8로 canonical URL 구조가 박혔으므로 필드 추가만으로 해결.

- `pindoc.artifact.{propose,read,search}` + `context.for_task` 응답에 **`agent_ref`** (에이전트 재호출용 `pindoc://<slug>`) + **`human_url`** (사용자가 채팅에 붙여넣는 `/p/:project/wiki/<slug>`) 두 URL을 분리 반환.
- `pindoc.project.current` 응답에 **`capabilities` 블록** 추가: `multi_project`, `retrieval_quality` (`stub`|`http`), `auth_mode` (`none`), `update_via` (`update_of`), `review_queue_supported` (`false`). 에이전트가 bootstrap 1회로 서버가 지원하는 플래그 파악.
- PINDOC.md 템플릿에 "agent가 사용자에게 링크 공유 시 `human_url` 값을 그대로 붙여넣어라" 규약 명시.

## Phase 10 — Real embedder dogfood (완료 · 2026-04-22)

`PINDOC_EMBED_PROVIDER=stub` 기본값이 `pindoc.artifact.search` / `context.for_task`의 품질을 hash 기반으로 고정함. 외부 리포트 공통 P1.

**Python sidecar가 아니라 Docker 기반 TEI 채택**. 이유: 저자 환경에 이미 Docker 있음, Python stack 설치 불필요, 한 줄 compose 서비스로 끝남, 모델 weight 캐시도 volume으로 자동 관리.

- [docker-compose.yml](../docker-compose.yml)에 `embed` 서비스 추가 (`ghcr.io/huggingface/text-embeddings-inference:cpu-1.6`, model: `intfloat/multilingual-e5-base`, 768 dim, 다국어 XLM-RoBERTa backbone)
- `--auto-truncate` 플래그로 512-token 초과 chunk 자동 truncate
- [embed/http.go](../internal/pindoc/embed/http.go)에 E5-style `query: ` / `passage: ` prefix 로직 추가 (`PINDOC_EMBED_PREFIX_QUERY` / `_DOCUMENT` env)
- [embed/registry.go](../internal/pindoc/embed/registry.go) + [config/config.go](../internal/pindoc/config/config.go)에 prefix 필드 wiring
- [cmd/pindoc-reembed](../cmd/pindoc-reembed/main.go) CLI 신규 — 전체 artifact 재-embed, per-artifact 트랜잭션, 32개씩 배치 전송 (TEI의 `max_batch_requests=8` 대응)
- [Makefile](../Makefile)에 `embed-up`, `server-run-http`, `api-run-http`, `reembed-build` 타겟 추가. `EMBED_ENV` 블록으로 env 세트 한 곳에서 관리.
- 17개 artifact 전체 재-embed 성공, stub → http 전환 후 의미 검색 품질 실측:
  - "Harness Reversal" → mechanisms M0 섹션 1순위 (distance 0.17)
  - "중복 문서 방지" → problem-space F1. 중복 생성 1순위 (0.16)
  - "agent 쓰기 규율" → architecture 원칙 1. Agent-only Write Surface 1순위 (0.14)
- `capabilities.retrieval_quality` 자동으로 `"http"`로 전환 (Phase 9 capabilities 블록 반영).

## Phase 11 — Write contract 강화 + semantic conflict (완료 · 2026-04-22)

피어리뷰 1차+2차 공통 P0 + 저자 확정 범위. "typed section schema 확정은 out-of-scope" — 스키마 최소 필드 + 진화형 template artifact 구조만.

- **`search_receipt` — 2차 피어리뷰 반영, soft→hard 업그레이드**:
  - `artifact.search` / `context.for_task` 응답에 서버 발급 opaque token (TTL 10분) 포함.
  - `artifact.propose`는 `basis.search_receipt` 필수 (create 경로만). Update 경로(`update_of`)는 receipt 불필요 — 대상 artifact read가 이미 증거.
  - 서버는 receipt의 project/session/TTL을 검증. 1차 피어리뷰 때 우려한 "lazy agent가 가짜 refs 넣기"는 receipt 기반이라 우회 불가.
  - Hard block — receipt 없으면 `not_ready` + `NO_SRCH` code.
- **`artifact.propose` 입력 확장**:
  - `basis.search_receipt` (필수, create 경로)
  - `basis.source_session` (optional) — 감사 추적용 세션 ID
  - `pins[]: [{repo, commit, path, lines?}]` — code-linked 증거
  - `expected_version` — `update_of` 경로에서 optimistic lock (`max(revision_number)` 비교)
  - `supersede_of` — 기존 artifact를 `status=superseded`로 전환하면서 새 artifact 생성
  - `relates_to[]: [{target_id, relation}]` — `implements | references | blocks` 등
- 신규 테이블 `artifact_edges(source_id, target_id, relation, created_at)` — `relates_to[]`의 persistence.
- **`context.for_task` 응답 확장 — 2차 피어리뷰 반영**:
  - `search_receipt` 발급
  - `candidate_updates[]` — 현 landings 중 "새 propose 대신 update_of 대상으로 의심되는" artifact
  - `stale[]` — pin이 commit diff와 어긋나 보이는 artifact
- `body_json` 활용 시작 — Debug/Decision/Analysis/Task 4 타입에 최소 필드만 (Debug=`symptom/resolution`, Decision=`decision/rationale` 수준). 나머지 섹션 구조는 **template artifact**로 풀어서 별도 artifact로 관리 (Phase 13).
- **Semantic conflict block** — `artifact.propose(new)` 시 `artifact.search`로 top-K 유사도 distance가 임계치 이하면 `not_ready` + `next_tools: [artifact.read]` + `related: [...]`. exact-title만 막는 현재 guard 상향.
- **`_unsorted` area auto-seed (2차 피어리뷰 반영)** — area 판단 불가 시 agent가 쓸 fallback. `misc`와 구분 (misc는 의도된 area, _unsorted는 "분류 필요" 큐). Reader UI에 "분류 필요" 위젯, 주기적으로 agent가 재분류.

## Phase 12 — Agent ergonomics (완료 · 2026-04-22)

피어리뷰 1차+2차 P1 블록 통합.

- **Machine-readable `not_ready`**: `{status, draft_id?, failed:[stable_code], next_tools:[...], related:[{id,url}]}`. 2차 피어리뷰가 구체 stable code까지 제안 — **채택**:
  - `NO_SRCH` — `basis.search_receipt` 없거나 만료
  - `NEED_PIN` — code-linked type(Debug/Decision/Feature)에 pin 없음
  - `NEED_VER` — `update_of` 경로인데 `expected_version` 없음
  - `VER_CONFLICT` — `expected_version`이 현 revision과 불일치 (race)
  - `AREA_BAD` — 없는 area_slug
  - `POSSIBLE_DUP` — semantic conflict hit, `candidate_updates`와 함께 반환
  - `DBG_NO_REPRO` — Debug인데 reproduction section 부재
  - `DEC_NO_ALT` — Decision인데 alternatives/rationale 부재
- **`artifact.read(view=brief|full|continuation)`**: brief=title/summary/pins/stale, continuation=brief + 최근 revision delta + relates_to neighbors. 기존 default는 `full` 유지.
- **Actor hardening (stdio)**: `author_id`는 표시용 metadata로 재정의. 서버가 session 단위 `agent_id` (UUID) 를 ping 첫 호출 시 발급, propose 감사에 기록. `author_id` spoof 공격 surface 축소.
- **Mode split은 not_ready 응답에만 한정 (2차 피어리뷰 권고 축소 반영)**: default는 compact (fail codes만), `verbose` 모드에서 자연어 hint 추가. tool 전체에 mode 파라미터 붙이는 건 현 규모에서 과설계 — 반려.

## Phase 14 — Operator settings + contract hardening (완료 · 2026-04-22)

3차 외부 피어리뷰 반영. 수용 목록은 [docs/14 §9](./14-peer-review-response.md) 참조.

**14A — settings infra**:
- Migration 0007 `server_settings` 단일 row 테이블 (typed columns). env는 first-boot seed만, DB가 source of truth (Ghost/Plausible 패턴).
- `internal/pindoc/settings/` package: Store + Reload/Get/Set/SeedFromEnv. atomic pointer로 lock-free read.
- `cmd/pindoc-admin` CLI: list/get/set. 재시작 필요 명시 (hot-reload는 V1.x).
- Server startup 시 `PINDOC_PUBLIC_BASE_URL` 있고 DB row 비었으면 1회 seed.
- pindoc-api도 `db.Migrate` + settings 로드 추가.

**14A — capabilities 확장**:
- `pindoc.project.current.capabilities`에 `scope_mode: "fixed_session"`, `new_project_requires_reconnect: true`, `receipt_ttl_sec: 1800`, `requires_expected_version: true`, `public_base_url` (DB 값) 추가.
- `auth_mode: "none"` → `"trusted_local"` rename (보안 모델을 정확히 반영).
- `receipts.DefaultTTL` 10분 → 30분.

**14B — human_url_abs**:
- `artifact.{read,search,propose}` + `context.for_task` 응답에 `human_url_abs` 필드 (DB의 public_base_url 있을 때만, 없으면 생략).
- `RelatedRef`, `EdgeRef`, `CandidateUpdate`, `SearchHit`, `ContextLanding`에도 전파.
- HTTP `/api/config`에 `public_base_url` 노출.

**14B — project.create onboarding**:
- 응답에 `reconnect_required: true`, `activation: "not_in_this_session"`, `next_steps[]` 추가. "create했지만 session은 old project에 묶여 있음"을 machine-readable로.

**14B — expected_version hard enforce**:
- `artifact.propose(update_of=…)`에서 `expected_version` 필수. 미제공 시 `NEED_VER` + `patchable_fields: ["expected_version"]` + `Related[]`에 현재 head 정보. Mismatch면 `VER_CONFLICT`.
- stale overwrite 방어 + "update 전 read" 간접 강제.

**14B — `patchable_fields[]` + candidate warning**:
- 모든 not_ready 응답에 `patchable_fields[]` (stable code → 수정할 필드 리스트). Agent가 전체 body 재전송 대신 필드만 바꿔 retry.
- Accepted create 응답에 `warnings: ["RECOMMEND_READ_BEFORE_CREATE"]` — semantic distance가 conflict block threshold(0.18)와 advisory threshold(0.25) 사이인 이웃이 있을 때.

**14B — harness.install 강화**:
- Pre-flight Check 섹션에 "create 전 context.for_task/artifact.search로 search_receipt 받기 필수" 명시.
- Update path 섹션 신규: `expected_version` 필수, `update_of`와 `supersede_of` 배제 관계 명시.
- `not_ready` 대응에 `failed[] + patchable_fields[]` 중심 언급.

## Phase 13 — Template artifact seed (완료 · 2026-04-22)

Phase 11에서 body_json 최소 필드로 검증을 좁히는 대신, **각 타입의 "현재 best practice" 구조는 artifact 자체로 관리**. 이것이 "포맷도 evolving artifact"라는 저자 원칙의 코드화.

- Seed migration 신규 — `_template_debug`, `_template_decision`, `_template_analysis`, `_template_task` 4개 artifact 생성 (slug prefix `_template_`은 검색/목록에서 기본 숨김).
- 각 template body는 현 시점 권장 섹션 구조 + 작성 예시. PINDOC.md 템플릿에 "신규 artifact propose 전에 `artifact.read(_template_<type>)` 를 호출해서 구조 참고" 규약 추가.
- Template 자체도 일반 artifact니까 `update_of`로 계속 revision. 외부 리서치 + 실 dogfood로 포맷 best practice가 쌓이면 template이 진화.

## V1.5 — 인증 + 멀티프로젝트 권한 (다음 큰 블록)

URL 구조는 이미 준비됨. V1.5에서 그 위에:

- **GitHub OAuth 로그인** — self-host 인스턴스 기준 (03-architecture.md §배포 B)
- **Agent token** — per-project, 사용자가 Settings에서 발급 / rotate / revoke
- **초대 플로우** — project admin이 email/username 초대 → role 부여
- **권한 모델**: `admin | writer | approver | reader` per (user, project)
- **진짜 `pindoc init` CLI** — 지금 seed migration이 하는 역할을 proper CLI 로
  분리 (언어 선택 + 에이전트 클라이언트 자동 감지 + .mcp.json 패치)
- **Project Switcher UI 확장**: V1.5에선 삭제/아카이브 + 멤버 목록

이 블록들은 URL 구조를 재설계하지 않는다. 기존 `/p/:project/...` 위에 auth
middleware만 얹는 형태.
