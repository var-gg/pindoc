# 03. Architecture

Pindoc의 시스템 구조. Multi-project · Harness · MCP · UI · 배포 시나리오.

## URL convention (canonical)

모든 사용자-facing 경로는 **프로젝트 스코프 접두사**를 갖는다. 공유된 URL이
받는 쪽의 "현재 프로젝트"에 따라 다른 문서를 열지 않도록 하는 장치.

### UI 경로

| 형태 | 의미 |
|------|------|
| `/p/:project/wiki` | 프로젝트의 Wiki Reader (artifact 목록) |
| `/p/:project/wiki/:slug` | 단일 artifact |
| `/p/:project/wiki/:slug/history` | 수정 이력 |
| `/p/:project/wiki/:slug/diff?from=&to=` | 리비전 비교 |
| `/p/:project/tasks` · `/tasks/:slug` | Task 뷰 (Reader를 type=Task로 필터) |
| `/p/:project/graph` | Graph (M1에선 stub) |
| `/p/:project/inbox` | Review Queue |
| `/design`, `/design/preview/:slug`, `/ui/:slug` | 개발 scaffold, 프로젝트 무관 |

### 레거시 redirect

`/wiki/...`, `/tasks/...`, `/graph`, `/inbox`, 그리고 루트 `/` 는 모두
**`/p/:default/...` 로 302 redirect** 된다 (`:default` = `PINDOC_MULTI_PROJECT`
환경의 `PINDOC_PROJECT` 값, 기본 `pindoc`).
`/api/config.default_project_slug` 가 참조 source of truth.

### HTTP API

UI mirror. 프로젝트 스코프 = URL 접두사.

| Method · Path | 용도 |
|---------------|------|
| `GET /api/config` | `{ default_project_slug, multi_project, version }` |
| `GET /api/projects` | 인스턴스 내 프로젝트 전부 (switcher 용) |
| `GET /api/p/:project` | 단일 프로젝트 detail (이전 `/api/projects/current`) |
| `GET /api/p/:project/areas` | |
| `GET /api/p/:project/artifacts` · `/:idOrSlug` · `/:idOrSlug/revisions` · `/:idOrSlug/diff` | |
| `GET /api/p/:project/search?q=` | 프로젝트 스코프 의미 검색 |
| `GET /api/health` | 인스턴스 헬스 |

### 프로젝트 생성

- **최초 프로젝트**: 서버 기동 시 seed 마이그레이션이 생성 (V1.5에서 `pindoc init`
  CLI 로 대체 예정).
- **이후 프로젝트**: `pindoc.project.create(slug, name, primary_language[, color, description])`
  MCP tool. UI에는 "+ 새 프로젝트" 버튼 없음 (원칙 1: agent-only write surface) —
  Project Switcher 드롭다운에 안내 문구만.

### 멀티프로젝트 토글

`PINDOC_MULTI_PROJECT=true|false` (기본 false). Switcher 드롭다운은 토글과
무관하게 현재 프로젝트 + 목록을 보여주지만, false 인스턴스에선 UI 카피에
"프로젝트는 하나" 뉘앙스가 담긴다. V1.5에서 본격적인 멀티프로젝트 권한 모델이
들어올 때 이 플래그가 확장 지점이다.

## 설계 철학

### 원칙 1. Agent-only Write Surface

사람 직접 편집 경로 전무. `created_by` · `last_modified_via` 모두 `AgentRef` 필수.

### 원칙 2. Human as Direction-setter, Not Gatekeeper

매 artifact 승인 강제 없음. Auto-publish 기본. Review Queue는 **sensitive ops + confirm 모드**에만.

### 원칙 3. Single Service by Default

V1 모놀리식. `git clone && docker compose up` 1분 기동.

### 원칙 4. Self-Host First

V1 self-host 전용. 클라우드 hosted는 V2+.

### 원칙 5. MCP가 Write 1차, Wiki UI가 사용자 경험 1차

MCP = write (write-only), Wiki UI = read + (엣지) approve. REST API = 3차.

### 원칙 6. Tiered Types (Tier A/B/C)

Tier A core 강제 + Tier B Domain Pack + Tier C Custom(V2+).

### 원칙 7. Multi-project by Design (V1 runtime: one MCP session = one project)

한 Pindoc 인스턴스는 복수 Project를 호스팅하도록 **설계**됐다 (schema, URL, Web UI 모두 `/p/:project/…` 스코프). Solo 사이드 프로젝트 / FE·BE 분리 / 영세 2~3명 복수 프로젝트가 1급 시민.

**V1 runtime 제약**: MCP subprocess 하나는 **한 프로젝트에 고정**된다 (`PINDOC_PROJECT` env로 결정, 세션 중 switch 불가). 이유:
- stdio MCP transport에서 session-level scope switching은 SDK 수준 지원이 제한적
- "wrong-project write"는 치명적 UX 실패 — hidden state switching보다 **"새 프로젝트를 쓰려면 새 MCP subprocess"** 가 안전
- V1.5 agent token + per-project 권한 모델 도입 시 `pindoc.project.switch` 재검토

Web UI는 멀티프로젝트 switcher를 이미 지원 (`/p/:project/…` canonical URL). MCP 쪽 single-project scope는 V1 구현의 의도적 단순화이며, 외부 에이전트가 프로젝트 간 전환하려면 새 MCP 연결을 여는 게 현재 운영 모델.

### 원칙 8. Customization via Slots, Not Forks

대시보드·브랜딩·광고 등 운영 자율성은 Custom Dashboard Slot으로. OSS core 중립.

### 원칙 9. Pin is in the name

제품명 `Pindoc`의 `pin`은 **코드-문서 결합 보증**. 모든 artifact는 git-pinned.

## 시스템 컴포넌트

```
┌──────────────────────────────────────────────────────────────┐
│                      Pindoc Server                            │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                MCP Layer (write 1차)                      │ │
│ │  Harness Injector · Pre-flight Check · Referenced Confirm │ │
│ │  Write-Intent Router · Schema Validator · Context Provider │ │
│ │  Project-scoped (agent token per project)                  │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                  Core Services                             │ │
│ │ Project Manager · Artifact Store · Graph Engine            │ │
│ │ Area/Tree · Git Pinner (in+out) · Propagation Ledger       │ │
│ │ Search Index (artifact) · Resource Index+M7                │ │
│ │ Permission Service · Event Bus · TC Runner(V1.1)           │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                    Storage                                  │ │
│ │  PostgreSQL · Filesystem · pgvector (artifact embeddings)   │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │             Web UI (사용자 경험 1차)                        │ │
│ │  Wiki Reader(★) · Project Switcher · Review Queue          │ │
│ │  Stale · Graph · Dashboard(+Custom Slot) · Settings        │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │        REST API (3차) · Auth (OAuth / Local)                │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
     ▲                   ▲                   ▲
     │ MCP               │ HTTPS             │ CLI/Webhook
 Coding Agents     Web Browser         `pindoc` CLI / Slack bot
```

> 기존 설계의 "Session Store" 서비스는 **제거**됐습니다. Pindoc은 raw 세션을 저장하지 않고 `SessionRef` 메타만 유지.

## 핵심 컴포넌트 설명

### MCP Layer

Project-scoped. Agent token이 특정 Project의 write 권한을 가짐.

**V1 MCP Tools**:

| Tool | 역할 |
|------|------|
| `pindoc.harness.install` | PINDOC.md 생성 + CLAUDE.md/AGENTS.md/.cursorrules 주입 |
| `pindoc.project.list` / `.switch` | 접근 가능 project 목록·전환 |
| `pindoc.artifact.search` | 기존 artifact 검색 (intent pre-check, F6 해결) |
| `pindoc.artifact.propose` | Promotion 제출 → Pre-flight Check → auto-publish or Review Queue |
| `pindoc.artifact.read` | URL/ID → artifact + Continuation Context |
| `pindoc.graph.neighbors` | graph 이웃 |
| `pindoc.context.for_task` | Fast Landing 번들 |
| `pindoc.resource.verify` | M7 Freshness 트리거 |
| `pindoc.area.propose` | 신규 Area 신청 (Write-Intent Router 통과) |
| `pindoc.tc.register` / `.run_result` | TC 관리 |

> 이전 설계의 `varn.session.stream` / `.upload` / `.search` 는 **V1에서 제거**. Raw 세션 흡수는 Pindoc 범위 밖. 향후 V2+에서 옵션으로 재검토.
>
> 이전 `varn.wiki.read(url)`은 `pindoc.artifact.read(url_or_id)`로 흡수.

**Write 경로 특이 패턴**:
- **Pre-flight Check** — `propose`는 즉답 대신 체크리스트로 역지시 ([05 M0.5](05-mechanisms.md))
- **Referenced Confirmation** — 에이전트 → 사용자 확인 요청 시 URL 동반 필수 ([05 M0.6](05-mechanisms.md))

### Core Services

- **Project Manager**: Project CRUD, Domain Pack 활성, Area 트리
- **Artifact Store**: `project_id` 필수, Tier A + 활성 Tier B 스키마 검증
- **Graph Engine**: 엣지 (Artifact 필드에서 derive), cross-project edge 지원
- **Permission Service**: per-project role (admin/writer/approver/reader)
- **Git Pinner (in + out)**: in = stale 감지, out = GitHub/GitLab URL 자동 생성
- **Event Bus**: `artifact.published` / `artifact.stale_detected` / `pin.changed` / `tc.failed` / `resource.verified` / `review.required` 등 발행
- **Propagation Ledger**: Event를 dependent에 전파
- **Resource Index + M7**: Related Resource 인덱스 + 주기 verify 스케줄
- **Search Index**: Artifact 전문 + 의미 검색 (pgvector) — F6 해결의 코어
- **Embedding Layer**: Pluggable provider + 자동 chunking (아래 상세)

### Embedding Layer

Semantic search / Fast Landing / Conflict check 의 공통 의존성. 3가지 설계 원칙:

**1. Pluggable Provider Interface**

```go
type Provider interface {
    Embed(ctx context.Context, req Request) (Response, error)
    Info() Info
}

type Info struct {
    Name        string   // "embeddinggemma" | "bge-m3" | "http" | ...
    ModelID     string
    Dimension   int      // 768 (gemma) / 1024 (bge-m3) / 3072 (openai-3-large)
    MaxTokens   int      // 2048 (gemma) / 8192 (bge-m3) / 8191 (openai)
    TaskPrefix  bool     // Gemma uses task-aware prefix
    Distance    string   // "cosine" | "dot"
    Multilingual bool
}
```

**2. V1 내장 구현 3종 + Config swap**

| Provider | 용도 | RAM | 기본 여부 |
|---|---|---|---|
| `embeddinggemma` | On-device dev/small-team default | ~200MB | ✅ default |
| `bge-m3` | 고품질 self-host (GPU or 8GB+ RAM) | 3-5GB | 옵션 |
| `http` | Ollama / TEI / OpenAI / Cohere / Vertex 등 외부 | N/A | 옵션 |

Swap은 `pindoc.config.yaml` or PINDOC.md frontmatter:

```yaml
embedding:
  provider: http
  endpoint: https://api.openai.com/v1/embeddings
  model: text-embedding-3-large
  api_key_env: OPENAI_API_KEY
  info:
    dimension: 3072
    max_tokens: 8191
```

설치 시 모델 선택 화면 없음 (default + `pindoc init --embedding=<name>` flag). [06 Flow 0 §onboarding](06-ui-flows.md).

**3. Automatic Chunking (V1 필수)**

Artifact body가 `Provider.MaxTokens` 초과 가능 (특히 한국어 장문 — 한국어 2000자 ≈ 1000-1500 토큰). 따라서:

```
Artifact
  ├─ title_vec           (embed(title), 항상 1)
  ├─ body_chunk_vecs[]   (섹션 boundary로 분할, 각 chunk 임베딩)
  └─ summary_vec         (에이전트가 제출한 1-문장 요약, optional)
```

**Chunking 규칙**:
- 우선: H2/H3 heading boundary 기준
- 초과 시: 문단 boundary 기준
- 각 chunk는 parent artifact의 title을 prefix로 carry (retrieval 맥락 유지)
- Chunk별 `chunk_idx`, `span_start`/`span_end` 저장 (UI에서 hit highlight)

**Retrieval 흐름**:
1. Query embed → top-K 유사 chunk
2. Parent artifact 그룹화 + 랭킹 재계산 (chunk 합치기 — multi-hit artifact 부스트)
3. Return: 각 artifact의 best chunk 를 snippet으로, 전체 artifact 링크

**Pre-flight와의 연동**:
- `pindoc.artifact.propose` 시 서버가 `len(body_tokens) vs Provider.MaxTokens` 체크
- 80% 초과: WARN · 100% 초과: NOT_READY 체크리스트에 "split into sections" 힌트
- 에이전트는 `pindoc.project.current` 응답에서 `embedding_provider.max_tokens` 를 알아냄 → 애초에 장문 쓸 때 예산 고려

### Web UI

사람의 read + (엣지 케이스) approve. 편집 없음. [06 UI Flows](06-ui-flows.md).

### REST API (3차)

- `pindoc` CLI 바이너리
- Slack/Discord 봇 (V1.1)
- Webhook 수신

---

## Custom Dashboard Slot

Pindoc core의 기본 기능. 운영 자율성을 **fork/branching 없이** 흡수.

```yaml
# settings.yaml (운영자가 편집; 에이전트 경유 원칙의 예외 — 서버 config)
dashboard_slots:
  hero:     null | { type: "markdown", source: "./custom/hero.md" }
  sidebar:  null | { type: "html",     source: "..." }
  footer:   null | { type: "iframe",   source: "..." }
  ads:      null | { type: "ethicalads" | "carbonads", publisher_id: "..." }
```

**OSS 중립성**: core 기본값 `null`. 모든 slot 설정은 open-source config file. 비밀 embed 없음.

**유즈케이스**:
- **pindoc.org 공개 인스턴스**: EthicalAds 슬롯 + GitHub Sponsors + "hosting $XX/month" 투명 공개
- **기업 self-host**: 사내 공지 / 팀 로고
- **Solo**: 기본 null (깔끔)

---

## 배포 시나리오

### A. Local Single-user

솔로 / 개인 프로젝트.

```
localhost:5733 — Pindoc + PostgreSQL (Docker)
```

- 인증: 없음 (단일 사용자). 로컬 파일 agent token (`~/.pindoc/token`)
- OAuth: 불필요
- MCP 설정: `pindoc init` 시 자동 주입

### B. Self-host Domain (V1 기본 팀 배포)

2~10인 팀.

```
pindoc.mycompany.dev — Pindoc + PostgreSQL + TLS Proxy (Caddy/Traefik)
```

- 인증: **GitHub OAuth** (V1 기본). Pindoc 인스턴스당 OAuth App 1개 등록
- 로그인 시 User 생성/매핑
- Agent token은 User가 Settings에서 발급 (per-agent, per-project)
- per-project role (admin/writer/approver/reader)

### C. Hosted SaaS (V2+, 선택적 BM)

V1 없음. Sentry/Supabase/n8n 모델.

- 인증: GitHub + Google OAuth
- Agent token: 가입 시 auto-provision
- Multi-tenant: 조직 격리
- 월 구독

---

## `pindoc init` — Zero-friction Onboarding CLI

첫 설치 번거로움 최소.

```bash
$ cd my-project
$ pindoc init
```

플로우 (7단계):

```
[1/7] Server 감지 (localhost 자동 또는 URL 입력 또는 docker compose up 제안)
[2/7] 인증 (Local: auto token / Self-host: GitHub OAuth 브라우저)
[3/7] Project 선택/생성 (repo 자동 감지)
[4/7] Domain Pack 선택 (신규 Project만)
[5/7] Agent token 자동 발급 (~/.pindoc/tokens/<project-slug>.token)
[6/7] MCP 클라이언트 자동 설정
       - Claude Code → ~/.config/claude-code/mcp.json
       - Cursor      → ~/.cursor/mcp.json
       - Cline       → VS Code settings
       - Codex       → ~/.codex/agents.toml
[7/7] Harness 설치
       - PINDOC.md 생성 (Domain Pack 반영)
       - CLAUDE.md / AGENTS.md / .cursorrules 에 참조 추가

✓ Setup complete
```

실패 지점: 정확한 copy-paste 명령 제시. 사용자는 기본 `pindoc init` + Y/N.

---

## 보안과 프라이버시

### 3-tier 인증

| 시나리오 | 사용자 인증 | Agent Token |
|---|---|---|
| Local | 없음 | 자동 생성 로컬 파일 |
| Self-host 도메인 | **GitHub OAuth** | User가 Settings에서 발급, per-agent, per-project |
| Hosted (V2+) | GitHub + Google | auto-provision |

### Agent Token

- per-project scope
- 90일 rotation (기본)
- `pindoc token revoke <id>` 즉시 비활성
- Server: hash + last_used_at

### User Session

- **write 권한 없음** (스키마 수준 거부)
- read + Review Queue 처리
- 쿠키 + CSRF

### 기타

- MCP 인증: Agent token
- 데이터 암호화: 호스팅 인프라 위임
- Git credentials: 사용자 제공, read-only
- LLM 호출: Pindoc 서버 직접 호출 없음
- **Raw 세션 흡수 없음** — 민감정보 유출 리스크도 자연 제거 (Pindoc은 정제된 artifact만 저장)

---

## 기술 스택 (제안)

| 레이어 | 기술 |
|---|---|
| 백엔드 | **Go** |
| DB | **PostgreSQL** |
| 벡터 | **pgvector** (artifact embedding) |
| 프론트 | **React + TypeScript** |
| UI | **shadcn/ui** or **Radix** |
| 마크다운 | **remark/rehype** |
| 다이어그램 | **Mermaid** |
| MCP | **공식 MCP SDK** |
| 배포 | **Docker Compose** |
| OAuth | GitHub (V1), Google (V2+) |

## 확장성 (V1 scope 밖)

- Hosted SaaS (V2)
- 외부 LLM 연동 (옵션)
- Tier C Custom
- 플러그인 시스템
- 모바일 read-only
- 에이전트 클라이언트 raw 세션 통합 (V2+ 실험)

---

## 아키텍처 의사결정 기록

| 결정 | 되돌리기 | 이유 |
|---|---|---|
| **Agent-only write** | 매우 어려움 | 원칙 1 |
| **No raw session ingest** | 쉬움 | scope 좁힘, 프라이버시 |
| **Multi-project by default** | 중간 | 현실 시나리오 반영 |
| **GitHub OAuth V1 기본** | 쉬움 | 개발자 타겟 + Git 통합 |
| **Custom Dashboard Slot** | 쉬움 | 운영 자율성 흡수 |
| **Auto-publish + Review Queue(sensitive only)** | 중간 | 원칙 2 구현 |
| **Typed Tier A/B/C** | 매우 어려움 | 데이터 모델 기반 |
| **MCP-First + Wiki UI primary** | 어려움 | 제품 정체성 |
| **Harness Reversal (PINDOC.md)** | 어려움 | 에이전트 규율 근간 |

**Agent-only write + No raw session + MCP-First + Typed (A/B/C) + Harness Reversal + Multi-project**이 타협 불가 영역.
