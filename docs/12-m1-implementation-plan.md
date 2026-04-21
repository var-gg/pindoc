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

## Phase 10 — Real embedder dogfood (다음 작업)

`PINDOC_EMBED_PROVIDER=stub` 기본값이 `pindoc.artifact.search` / `context.for_task`의 품질을 hash 기반으로 고정함. 외부 리포트 공통 P1.

- `services/embed-sidecar/` Python FastAPI 기동 (EmbeddingGemma-300M 기본)
- `PINDOC_EMBED_PROVIDER=http`, `PINDOC_EMBED_ENDPOINT=http://127.0.0.1:5860/v1/embeddings` 로 전환
- 기존 `artifact_chunks` 전체 재-embed 배치 스크립트 (15개 artifact 한 번에)
- 스모크: `/api/p/pindoc/search?q=…` 한국어 쿼리 5개로 의미 검색 품질 확인

## Phase 11 — Write contract 강화 + semantic conflict (핵심 블록)

피어리뷰 공통 P0 + 저자 확정 범위. "typed section schema 확정은 out-of-scope" — 스키마 최소 필드 + 진화형 template artifact 구조만.

- `artifact.propose` 입력 확장:
  - `basis.source_session` (optional) — 감사 추적용 세션 ID
  - `basis.search_refs[]` (optional) — "봤다" 주장. 서버는 이벤트 로그만 남김, hard-enforce 안 함
  - `pins[]: [{repo, commit, path, lines?}]` — code-linked 증거
  - `expected_version` — `update_of` 경로에서 optimistic lock (`max(revision_number)` 비교)
  - `supersede_of` — 기존 artifact를 `status=superseded`로 전환하면서 새 artifact 생성
  - `relates_to[]: [{target_id, relation}]` — `implements | references | blocks` 등
- 신규 테이블 `artifact_edges(source_id, target_id, relation, created_at)` — `relates_to[]`의 persistence.
- `body_json` 활용 시작 — Debug/Decision/Analysis/Task 4 타입에 최소 필드만 (Debug=`symptom/resolution`, Decision=`decision/rationale` 수준). 나머지 섹션 구조는 **template artifact**로 풀어서 별도 artifact로 관리 (Phase 13).
- **Semantic conflict block** — `artifact.propose(new)` 시 `artifact.search`로 top-K 유사도 distance가 임계치 이하면 `not_ready` + `next_tools: [artifact.read]` + `related: [...]`. exact-title만 막는 현재 guard 상향.

## Phase 12 — Agent ergonomics (envelope + view + actor)

피어리뷰 P1 블록 통합.

- **Machine-readable `not_ready`**: `{status, draft_id?, failed:[stable_code], next_tools:[...], related:[{id,url}]}`. stable code 테이블은 docs/에 별도.
- **`artifact.read(view=brief|full|continuation)`**: brief=title/summary/pins/stale, continuation=brief + 최근 revision delta + relates_to neighbors. 기존 default는 `full` 유지.
- **Actor hardening (stdio)**: `author_id`는 표시용 metadata로 재정의. 서버가 session 단위 `agent_id` (UUID) 를 ping 첫 호출 시 발급, propose 감사에 기록. `author_id` spoof 공격 surface 축소.

## Phase 13 — Template artifact seed

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
