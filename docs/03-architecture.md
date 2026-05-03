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
**`/p/:default/...` 로 302 redirect** 된다 (`:default` = `PINDOC_PROJECT` env
값, 기본 `pindoc`). `/api/config.default_project_slug` 가 참조 source of truth.

### HTTP API

UI mirror. 프로젝트 스코프 = URL 접두사.

| Method · Path | 용도 |
|---------------|------|
| `GET /api/config` | `{ default_project_slug, multi_project, version }` |
| `GET /api/projects` | 인스턴스 내 프로젝트 전부 (switcher 용) |
| `GET /api/p/:project` | 단일 프로젝트 detail (이전 `/api/projects/current`) |
| `GET /api/p/:project/areas` | |
| `GET /api/p/:project/artifacts` · `/:idOrSlug` · `/:idOrSlug/revisions` · `/:idOrSlug/diff` | |
| `GET /api/p/:project/task-flow` | Reader Task Flow Lens. `project_scope=current|visible`, `actor_scope`, `flow_scope` query로 MCP `pindoc.task.flow`와 같은 derived sequence를 읽는다. |
| `GET /api/p/:project/search?q=` | 프로젝트 스코프 의미 검색 |
| `GET /api/health` | 인스턴스 헬스 |

### 프로젝트 생성

- **최초 프로젝트**: 서버 기동 시 seed 마이그레이션이 생성 (V1.5에서 `pindoc init`
  CLI 로 대체 예정).
- **이후 프로젝트**: `pindoc.project.create(slug, name, primary_language[, color, description])`
  MCP tool. UI에는 "+ 새 프로젝트" 버튼 없음 (원칙 1: agent-only write surface) —
  Project Switcher 드롭다운에 안내 문구만.

### 멀티프로젝트 가시성

`/api/config.multi_project` 와 `pindoc.project.current` 응답
`capabilities.multi_project` 는 모두 **DB의 보이는 프로젝트 수에서 자동
도출**된다 (`projects.CountVisible(...) > 1`). 즉, `pindoc.project.create`
또는 Reader UI 의 "+ New project" 로 두 번째 프로젝트가 생기는 즉시 Switcher
가 켜지고, env flag 를 따로 토글할 필요는 없다. V1.5 권한 모델이 들어오면
같은 함수가 ACL 필터를 추가해 "이 caller에게 보이는 프로젝트 수" 로 의미가
자연스럽게 확장된다.

## 설계 철학

### 원칙 1. Agent-only Write Surface

사람 직접 편집 경로 전무. `created_by` · `last_modified_via` 모두 `AgentRef` 필수.

### 원칙 2. Human as Direction-setter, Not Gatekeeper

매 artifact 승인 강제 없음. Auto-publish 기본. Review Queue는 **sensitive ops + confirm 모드**에만.

### 원칙 3. Single Service by Default

V1 모놀리식. 현재 M1 개발 스택은 `git clone && docker compose up -d --build`
로 Postgres + Pindoc HTTP daemon + Reader SPA를 띄운다. 같은 daemon이 MCP,
Reader API, `/health`를 함께 서빙한다. Host-native `pindoc-server -http
127.0.0.1:5830` 경로는 코드 수정 중 빠른 로컬 디버깅을 위한 보조 경로다.

### 원칙 4. Self-Host First

V1 self-host 전용. 클라우드 hosted는 V2+.

### 원칙 5. MCP가 Write 1차, Wiki UI가 사용자 경험 1차

MCP = write (write-only), Wiki UI = read + (엣지) approve. REST API = 3차.

### 원칙 6. Tiered Types (Tier A/B/C)

Tier A core 강제 + Tier B Domain Pack + Tier C Custom(V2+).

### 원칙 7. Multi-project by Design (account-level MCP scope)

한 Pindoc 인스턴스는 복수 Project를 호스팅하도록 **설계**됐다 (schema, URL, Web UI 모두 `/p/:project/…` 스코프). Solo 사이드 프로젝트 / FE·BE 분리 / 영세 2~3명 복수 프로젝트가 1급 시민.

`pindoc-server`는 두 transport를 지원하지만 scope 모델은 동일하다:

**stdio (기본, subprocess-per-session)** — `pindoc-server` 단독 실행. Claude Code가 `.mcp.json`의 `command` entry로 launch. `PINDOC_PROJECT` env는 `pindoc.project.current`의 fallback일 뿐이고, project-scoped tool은 `project_slug` input으로 scope를 고른다. `capabilities`: `scope_mode="per_call"`, `new_project_requires_reconnect=false`, `transport="stdio"`.

**streamable_http (데몬, V1 멀티프로젝트 운영 모델)** — `pindoc-server -http 127.0.0.1:5830` 단일 데몬을 띄우면 다수 Claude Code/Codex 세션이 같은 `.mcp.json` URL `http://127.0.0.1:5830/mcp`에 attach한다. 연결 URL은 account-level이고, 매 tool call의 `project_slug`가 project scope를 결정한다. 한 데몬에서 RAM·embedder·telemetry가 공유되고, 새 프로젝트는 다음 tool call부터 바로 addressable하다. 같은 데몬이 같은 포트에서 Reader read-only HTTP API(`/api/...`)와 liveness probe(`/health`)도 함께 서빙하므로 V1 운영에서는 별도 `pindoc-api` 프로세스가 필요 없다. `capabilities`: `scope_mode="per_call"`, `new_project_requires_reconnect=false`, `transport="streamable_http"`, `providers=$PINDOC_AUTH_PROVIDERS`, `bind_addr=$PINDOC_BIND_ADDR`. Decision `mcp-scope-account-level-industry-standard`.

기본값은 loopback bind + 빈 providers. Decision `decision-auth-model-loopback-and-providers`가 framing 자체를 폐기했다 — 더 이상 `auth_mode` enum이 아니라 세 직교축(외부 노출 / IdP / public 허용 opt-in)으로 인증 정책을 표현한다. 자세한 흐름은 본 문서 § principal resolution 참조.

Web UI(`pindoc-api`)는 두 transport와 무관하게 멀티프로젝트 switcher를 지원 (`/p/:project/…` canonical URL).

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
│ │  Account-level transport · per-call project_slug scope      │ │
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

Account-level MCP connection. Project-scoped tool은 `project_slug` input으로
scope를 고른다. V1 local 셋업은 loopback bind + 빈 providers — 모든 요청이
Source=loopback owner principal로 매핑된다. Agent token은 공개 self-host /
SaaS 인증 도입 시점의 별도 관심사다.

**V1 MCP Tools**:

| Tool | 역할 |
|------|------|
| `pindoc.harness.install` | PINDOC.md 생성 + CLAUDE.md/AGENTS.md/.cursorrules 주입 |
| `pindoc.project.current` / `.create` | 프로젝트 메타 조회·생성 |
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

**2. 4가지 Provider 분류 (배포 경제학 기준)**

| 분류 | Provider | 모델 예 | RAM | 기본 여부 | V1 상태 |
|---|---|---|---|---|---|
| **Bundled on-device** | `gemma` / `embeddinggemma` | google/embeddinggemma-300m (Q4, 768 dim) | ~200MB | ✅ **default** | Phase 17 구현 완료 |
| Bundled on-device (premium) | `bge-m3` | bge-m3 내장 | 3-5GB | 옵션 | V1.5 |
| **Sidecar local** (http 어댑터) | `http` | HuggingFace TEI / Ollama / llama.cpp server | 별도 프로세스 | 옵션 | http 구현 완료 |
| **Cloud** (http 어댑터) | `http` | OpenAI / Cohere / Vertex / Anthropic | 0 (원격) | 옵션 | http 구현 완료 |
| (테스트용) | `stub` | 해시 기반 pseudo-vec | 0 | ❌ (명시적 opt-in만) | 테스트 전용 |

**배포 경제학 근거**: AWS t3.medium / Hetzner CX22 같은 ~$5/월 tier에서 Postgres + Pindoc + 임베딩까지 **한 인스턴스**에 돌아가는 footprint가 목표. `gemma` Q4 variant는 그 예산 안에 들어가도록 고른 유일한 기본값. TEI·bge-m3는 더 큰 박스나 별도 GPU 머신을 가진 operator의 선택지.

**Sidecar vs Cloud는 동일 계약**: 둘 다 `http` provider로 흡수 — operator가 OpenAI-shape 호환 endpoint를 가리키는 한 code 변경 없이 swap. TEI는 `http://localhost:5860/v1/embeddings`, OpenAI는 `https://api.openai.com/v1/embeddings`.

**Bundled 구현 (Phase 17, 2026-04-22)**:
- **런타임**: [yalue/onnxruntime_go v1.27](https://github.com/yalue/onnxruntime_go) — onnxruntime C API 1.24.1 shared lib을 런타임에 동적 로드
- **ONNX 모델**: [onnx-community/embeddinggemma-300m-ONNX](https://huggingface.co/onnx-community/embeddinggemma-300m-ONNX) Q4 variant (가중치 ~197MB)
- **Tokenizer**: [eliben/go-sentencepiece](https://github.com/eliben/go-sentencepiece) — Gemma용 pure Go BPE 구현
- **자동 download**: 첫 run 시 onnxruntime shared lib + 모델 assets를 `<user cache>/pindoc/{runtime,models/embeddinggemma-300m}/`에 download (Windows/Linux/macOS amd64+arm64). 이후 cache hit.
- **Matryoshka truncation**: 현재 전체 768 dim 유지 (pgvector column 호환). V1.5에 512/256/128 Matryoshka 옵션 추가 검토.
- **Variant override**: `PINDOC_EMBED_GEMMA_VARIANT=q4f16|quantized|fp16`

**Swap 예**:
```yaml
# Bundled gemma 그대로 (아무 설정 없음 = 기본)

# TEI sidecar (로컬 Docker)
embedding:
  provider: http
  endpoint: http://localhost:5860/v1/embeddings
  model: multilingual-e5-base
  dimension: 768

# OpenAI cloud
embedding:
  provider: http
  endpoint: https://api.openai.com/v1/embeddings
  model: text-embedding-3-large
  api_key_env: OPENAI_API_KEY
  dimension: 3072
```

설치 시 모델 선택 화면 없음 — default (gemma) 자동. [06 Flow 0 §onboarding](06-ui-flows.md).

**Silent fallback 금지** (Phase 17): `PINDOC_EMBED_PROVIDER`가 알 수 없는 값이면 기동 실패. 과거 stub으로 조용히 떨어지던 동작은 제거 — 임베딩은 product의 **필수 기능**이고 누락을 warn으로 처리하지 않는다. stub은 `PINDOC_EMBED_PROVIDER=stub`으로 명시한 unit test 환경에서만 허용. Docker Compose daemon은 host의 `PINDOC_EMBED_PROVIDER`를 전달하지 않고 `PINDOC_COMPOSE_EMBED_PROVIDER`만 읽는다. 한 번의 shell session에 남은 `PINDOC_EMBED_PROVIDER=stub`이 컨테이너에 새어 들어가 hash embedding을 만드는 사고를 막기 위한 hardening이다. 실제 stub가 켜지면 server startup log에 multi-line warning box를 남긴다. 사고 후 회복은 정상 provider로 재기동한 뒤 `go run ./cmd/pindoc-reembed`로 affected artifact를 재임베딩한다.

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
localhost:5830 — Pindoc HTTP daemon + PostgreSQL
```

- 현재 M1 dev: `docker compose up -d --build`
- legacy Windows NSSM 서비스가 5830을 점유하면 `PINDOC_DAEMON_PORT=5832`
  같은 host port override로 side-by-side 기동 후 NSSM을 관리자 권한으로 제거
- 인증: 기본 `PINDOC_BIND_ADDR=127.0.0.1:5830` + 빈 `PINDOC_AUTH_PROVIDERS` — 모든 loopback 요청은 자동 owner principal (Loopback Trust)
- OAuth: 불필요
- MCP 설정: account-level URL `http://127.0.0.1:5830/mcp`, project scope는
  tool input의 `project_slug`

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
[1/7] Server 감지 (localhost:5830 자동 또는 URL 입력 또는 local daemon 기동 제안)
[2/7] 인증 (Local: loopback bind 자동 신뢰 / Self-host: GitHub IdP 활성화 + 브라우저 OAuth)
[3/7] Project 선택/생성 (repo 자동 감지)
[4/7] Domain Pack 선택 (신규 Project만)
[5/7] Project scope 기록 (PINDOC.md frontmatter project_slug)
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

### Principal resolution

Decision `decision-auth-model-loopback-and-providers`가 정의한 흐름.
사용자 식별의 진실 공급원은 `Principal.Source` 한 필드 — `loopback` 또는
`oauth` — 이고, 어느 trust 경로로 도착했는지를 표현한다.

```
HTTP 요청 도착
  ├─ remote_addr ∈ {127.0.0.1, ::1, localhost} ?
  │    └─ Yes → Loopback Trust → default user를 Source=loopback owner로
  │            (RFC 9728 endpoint 우회, OAuth 미들웨어 bypass)
  │
  └─ No → RFC 9728 PRM 흐름 강제
         ├─ Authorization: Bearer <jwt> 검증 (Pindoc AS)
         ├─ JWT user_id 클레임 → users 행 매칭 → Source=oauth
         ├─ project_members 조회로 role 결정 (per-call project_slug)
         └─ 401 시 WWW-Authenticate 헤더로 PRM URL 안내

stdio MCP transport
  └─ subprocess (process trust boundary) → loopback과 동일하게
     default user owner principal 부여
```

세 env가 정책을 결정한다:

| env | 기본값 | 의미 |
|---|---|---|
| `PINDOC_BIND_ADDR` | `127.0.0.1:5830` | 외부 노출 결정 |
| `PINDOC_AUTH_PROVIDERS` | empty | 활성 IdP CSV (예: `github`) |
| `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED` | `false` | "외부 노출 + IdP 없음"의 명시적 opt-in |

비-loopback bind + empty providers + opt-in false 조합은 부팅을 거부한다
(Public-Without-Auth Refusal — `ErrPublicWithoutAuth` sentinel).

### AS / IdP / RS 역할 분리

| 역할 | 실체 |
|---|---|
| AS (Authorization Server) | Pindoc daemon (`ory/fosite`) — RFC 9728 PRM + RFC 8414 metadata + `/oauth/{authorize,token,revoke}` |
| RS (Resource Server) | Pindoc daemon — Bearer JWT 검증 후 MCP/HTTP 리소스 제공 |
| IdP (Identity Provider) | 외부 위임 — `PINDOC_AUTH_PROVIDERS`의 항목 (현재 `github` 한 개, 미래 `google` / `local-password` / `passkey` 등) |
| MCP client | Claude Desktop / Code / Codex / Cursor — Pindoc AS 토큰만 사용 |

`users` 행 1개에 IdP identity N개가 link되는 SSO 모델 — 어떤 IdP로
로그인하든 같은 `users.id`로 수렴한다.

`bootstrapUserID` fallback은 loopback origin에서만 발동한다. Providers 미설정 시 OAuth endpoint들은 SPA로 빠지지 않고 503 + JSON `auth_not_configured`로 응답한다.

### 시나리오 매핑

| 시나리오 | bind | providers | 운영자 경험 |
|---|---|---|---|
| Solo 1인 / 같은 박스 | loopback | empty | env 작업 0. 모든 요청 owner |
| Solo 1인 / cross-device | external | github | OAuth 1회. loopback 운영자는 그대로 |
| 2~10인 팀 | external | github | invite 발급 → 친구 GitHub 로그인 → project_members |
| LAN-only / proxy 신뢰망 | external | empty + ALLOW=true | 명시 opt-in. 인증 없음을 운영자가 보증 |
| Hosted (V2+) | external | github,google | Multi-tenant 격리는 별도 설계 |

### Agent Token

- Local V1에서는 미사용. MCP daemon은 loopback bind + 자동 신뢰로 동작.
- 공개 self-host / SaaS에서 도입.
- account-level token + per-call `project_slug` 권한 확인.
- 90일 rotation (기본)
- `pindoc token revoke <id>` 즉시 비활성
- Server: hash + last_used_at

### User Session

- **write 권한 없음** (스키마 수준 거부)
- read + Review Queue 처리
- 쿠키 + CSRF

### 기타

- MCP 인증: Local V1은 loopback bind 자동 신뢰; 공개 self-host / SaaS는 `PINDOC_AUTH_PROVIDERS=github,...` 활성화로 OAuth + Agent token 도입
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
