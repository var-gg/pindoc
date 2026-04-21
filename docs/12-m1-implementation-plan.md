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
