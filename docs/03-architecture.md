# 03. Architecture

Varn의 시스템 구조. Multi-project · Harness · MCP · UI · 배포 시나리오를 정의합니다.

## 설계 철학

### 원칙 1. Agent-only Write Surface

사람은 위키에 직접 타이핑하지 않는다. 오탈자·이미지·링크까지 전부 에이전트 경유. 스키마 수준에서도 User write 거부.

### 원칙 2. Single Service by Default

V1은 모놀리식 단일 서비스. `git clone && docker compose up` 1분 내 기동.

### 원칙 3. Self-Host First

V1은 자체 호스팅 전용. 클라우드 hosted는 V2+.

### 원칙 4. MCP가 Write 1차, Wiki UI가 사용자 경험 1차

- MCP: write 1차 (write-only)
- Wiki UI: read + (엣지 케이스) approve 1차 (**editor 없음**)
- REST API: 3차 (외부 통합, CLI)

### 원칙 5. Tiered Types (Tier A/B/C)

Tier A core 강제 + Tier B Domain Pack 선택 + Tier C Custom(V2+).

### 원칙 6. Multi-project by Default

**한 인스턴스 = 복수 Project.** Solo의 사이드 프로젝트 / FE·BE 분리 팀 / 2~3명이 복수 프로젝트 시나리오 전부 1급 지원.

### 원칙 7. Customization via Slots, Not Forks

대시보드·브랜딩·광고 같은 운영 자율성은 **Custom Dashboard Slot**으로 흡수. 브랜치 분리나 포크 유도 없음.

## 시스템 컴포넌트

```
┌──────────────────────────────────────────────────────────────┐
│                      Varn Server                              │
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
│ │ Project Manager · Artifact Store · Session Store           │ │
│ │ Graph Engine · Area/Tree · Git Pinner (in+out)             │ │
│ │ Propagation Ledger · Search Index · Resource Index+M7      │ │
│ │ Permission Service · Event Bus · TC Runner(V1.1)           │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                  Storage                                    │ │
│ │  PostgreSQL · Filesystem · pgvector                         │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │            Web UI (사용자 경험 1차)                         │ │
│ │  Wiki Reader(★) · Project Switcher · Review Queue          │ │
│ │  Sessions · Stale · Graph · Dashboard(+Custom Slot)        │ │
│ │  Settings (OAuth, members, VARN.md mode)                   │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │        REST API (3차) · Auth (OAuth / Local)               │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
     ▲                   ▲                   ▲
     │ MCP               │ HTTPS             │ CLI/Webhook
 Coding Agents     Web Browser         `varn` CLI / Slack bot
```

## 핵심 컴포넌트 설명

### MCP Layer

Project-scoped. Agent token이 특정 Project의 write 권한을 가짐.

**주요 MCP Tools (V1, 상세는 10-mcp-tools-spec.md 참조)**:

| Tool | 역할 |
|------|------|
| `varn.harness.install` | VARN.md 생성 + CLAUDE.md/AGENTS.md/.cursorrules 주입 |
| `varn.project.list` / `.switch` | 현재 agent token이 접근 가능한 project 목록·전환 |
| `varn.session.stream` / `.upload` / `.search` | 세션 저장·의미 검색 (F6) |
| `varn.artifact.search` | 기존 artifact 검색 (intent pre-check) |
| `varn.artifact.propose` | Promotion draft 제출 → Pre-flight Check |
| `varn.artifact.commit` | Auto-publish or Review Queue 적재 |
| `varn.artifact.read` | Artifact + Continuation Context |
| `varn.wiki.read` | URL 기반 fetch |
| `varn.graph.neighbors` | graph 이웃 |
| `varn.context.for_task` | Fast Landing 번들 |
| `varn.resource.verify` | M7 Freshness 트리거 |
| `varn.area.propose` | 신규 Area 신청 (Write-Intent Router 통과) |
| `varn.tc.register` / `.run_result` | TC 관리 |

### Core Services

- **Project Manager**: Project CRUD, Domain Pack 활성, Area 트리 관리
- **Artifact Store**: `project_id` 필수, Tier A + 활성 Tier B 스키마
- **Session Store**: 시간순 + F6 의미 검색 (pgvector)
- **Graph Engine**: 엣지 관리, cross-project edge 지원
- **Permission Service**: per-project role (admin/writer/approver/reader)
- **Git Pinner (in+out)**: in = stale 감지, out = GitHub/GitLab URL 자동 생성
- **Event Bus**: `artifact.published`, `artifact.stale_detected`, `pin.changed`, `tc.failed`, `resource.verified`, `review.required` 등 발행
- **Propagation Ledger**: Event를 dependent로 전파, stale 플래그
- **Resource Index + M7**: Related Resource 인덱스 + 주기 verify 스케줄
- **Search Index**: Artifact 전문 + Session 의미 검색

### Web UI

**사람의 read + (엣지) approve 인터페이스. 편집 없음.**

주요 화면은 [06 UI Flows](06-ui-flows.md).

### REST API (3차)

- CLI 도구 (`varn` 바이너리)
- Slack/Discord 봇 (V1.1)
- Webhook 수신 (GitHub)

---

## Custom Dashboard Slot

Varn core의 기본 기능. **운영 자율성을 fork/brancing 없이** 흡수.

### 제공 슬롯

```yaml
# settings.yaml (self-host 운영자가 편집, 에이전트 경유 원칙 예외 — 서버 설정)
dashboard_slots:
  hero:     null | { type: "markdown", source: "./custom/hero.md" }
  sidebar:  null | { type: "html",     source: "..." }
  footer:   null | { type: "iframe",   source: "..." }
  ads:      null | { type: "ethicalads" | "carbonads", publisher_id: "..." }
```

### OSS 중립성 보증

- **OSS core 자체에는 광고/브랜딩 embed 없음** — `null` 기본
- 운영자가 명시적으로 설정해야만 활성
- 모든 슬롯 설정은 **open-source 설정 파일**에 존재 (비밀 embed 없음)

### 유즈케이스

- **`varn.var.gg` 공개 인스턴스**: EthicalAds 슬롯 + GitHub Sponsors 링크 + "운영비 이 서버 월 $XX" 투명 공개
- **기업 self-host**: 사내 공지 markdown 슬롯, 팀 로고
- **Solo**: 기본 null (깔끔)

### 왜 Branch/Fork 대신 Slot?

- 유지보수 1개 코드베이스
- 보안 패치 즉시 반영
- 모든 self-host 사용자가 같은 메커니즘 사용 — 기업 공지, 스폰서 로고 등 일반적 수요 커버
- 이게 곧 기존 wiki(BookStack/Outline/Wiki.js)가 약했던 **"자율 커스터마이징"** 영역의 Varn 해답

---

## 배포 시나리오

사용자·조직 규모에 따라 3가지. 각 시나리오별로 **인증·권한·기술**이 다름.

### 시나리오 A: Local Single-user

**대상**: 솔로 개발자, 개인 프로젝트

```
localhost:5733
 ├─ Varn Server (Docker)
 └─ PostgreSQL (Docker)
```

- **인증**: 없음 (단일 사용자 전제) — 로컬 파일 기반 agent token (`~/.varn/token`)
- **OAuth**: 불필요
- **MCP 설정**: `varn init` 시 localhost URL + 자동 token → 에이전트 설정 파일에 자동 주입
- **Project**: 필요에 따라 여러 Project 보유

**장점**: 1분 기동, 계정 관리 0.

### 시나리오 B: Self-host Domain (V1 기본 팀 배포)

**대상**: 2~10인 소규모 팀, 영세 사업장

```
varn.mycompany.dev
 ├─ Varn Server (Docker)
 ├─ PostgreSQL (Docker)
 └─ Reverse Proxy + TLS (Caddy/Traefik)
```

- **인증**: **GitHub OAuth** (V1 기본)
  - Varn 인스턴스 당 GitHub OAuth App 1개 등록
  - 팀원이 GitHub 계정으로 로그인 → User 생성/매핑
  - Agent token은 User가 Settings에서 발급 (per-agent, per-project)
- **Permission**: per-project role (admin/writer/approver/reader)
- **Project**: 복수 Project 운영, FE/BE 분리 가능
- **Custom Dashboard Slot**: 사내 공지·내부 링크 등 자유롭게

**장점**: 검증된 OAuth, Git pin과 같은 토큰 재사용 가능.

### 시나리오 C: Hosted SaaS (V2+, 선택적 BM)

V1에 없음. V2에서 검토. Sentry/Supabase/n8n 모델.

- **인증**: GitHub OAuth + Google OAuth
- **Agent token**: 가입 시 auto-provision
- **Multi-tenant**: 조직 단위 격리
- **월 구독**: 운영비 모델

---

## `varn init` — Zero-friction Onboarding CLI

Varn 첫 설치 시 번거로움을 최소로.

```bash
$ cd my-project
$ varn init
```

플로우:

```
[1/7] Server 감지
  - 로컬 localhost:5733 확인?  ──YES──▶ 자동 연결
  - 없으면: "Server URL을 입력하세요" 또는
           "로컬로 docker compose up 하시겠습니까?"

[2/7] 인증
  - Local 시나리오: 자동 (로컬 토큰 생성, ~/.varn/token)
  - Self-host 도메인: GitHub OAuth 브라우저 오픈

[3/7] Project 선택/생성
  - 기존 Project 목록 표시 → 선택
  - 없으면 "새 Project 만들기" → name/slug 입력
  - repo(s) 연결 (현재 git repo 자동 감지)

[4/7] Domain Pack 선택 (신규 Project만)
  ☑ Web SaaS/SI (stable, 권장)
  ☐ Game (skeleton)
  ☐ ML/AI (skeleton)
  ☐ Mobile (skeleton)
  ...

[5/7] Agent token 자동 발급
  - 이 Project 대상 writer role agent token 생성
  - 저장: ~/.varn/tokens/<project-slug>.token

[6/7] MCP 클라이언트 자동 설정
  - 설치된 에이전트 CLI 감지:
    - Claude Code → ~/.config/claude-code/mcp.json 수정
    - Cursor → ~/.cursor/mcp.json 수정
    - Cline → VS Code settings.json 수정
    - Codex → ~/.codex/agents.toml 수정
  - 각 MCP config에 varn server URL + token 자동 주입

[7/7] Harness 설치
  - VARN.md 생성 (프로젝트 루트, Domain Pack 반영)
  - CLAUDE.md / AGENTS.md / .cursorrules 에 참조 추가

✓ Setup complete
  Claude Code를 열고 아무거나 물어보세요.
  첫 체크포인트 제안이 뜨면 Varn이 작동 중입니다.
```

**실패 지점 대응**: 각 단계에서 자동화 실패 시 **정확한 copy-paste 명령**을 표시. 사용자는 `varn init` 한 번 + Y/N 정도만.

---

## 보안과 프라이버시

### 3-tier 인증 모델

| 시나리오 | 사용자 인증 | Agent Token |
|---|---|---|
| **Local** | 없음 (단일) | 자동 생성, 로컬 파일 |
| **Self-host 도메인** | **GitHub OAuth** | User가 Settings에서 발급, per-agent, per-project |
| **Hosted (V2+)** | GitHub + Google OAuth | 가입 시 auto-provision |

### Agent Token

- **per-project scope**: 한 token은 정확히 하나의 Project에만 write 가능
- **로테이션**: 기본 90일, 운영자 설정 가능
- **유출 대응**: `varn token revoke <id>` CLI/UI로 즉시 비활성
- **저장**: client 쪽은 `~/.varn/tokens/`, server 쪽은 hash + last_used_at

### User Session

- **write 권한 없음** — 스키마 수준 거부
- read + Review Queue 처리
- 쿠키 + CSRF token

### 기타

- **MCP 인증**: Agent token
- **데이터 암호화**: 저장 시 DB 레벨은 호스팅 인프라에 위임
- **Git credentials**: 사용자가 제공, read-only (V1)
- **LLM 호출**: Varn 서버는 LLM 직접 호출 없음
- **Session 민감정보**: 90일 보존 평문. secrets 혼입 리스크 — V1.x에서 redaction 규칙 검토

---

## 기술 스택 (제안)

| 레이어 | 기술 |
|---|---|
| 백엔드 언어 | **Go** |
| DB | **PostgreSQL** |
| 벡터 검색 | **pgvector** |
| 프론트엔드 | **React + TypeScript** |
| UI 라이브러리 | **shadcn/ui** 또는 **Radix** |
| 마크다운 | **remark/rehype** |
| 다이어그램 | **Mermaid** |
| MCP | **공식 MCP SDK** |
| 배포 | **Docker Compose** |
| OAuth | GitHub (V1), Google (V2+) |

## 확장성 (V1 scope 밖)

- 멀티 테넌트 (Hosted SaaS V2)
- 외부 LLM 연동 (서버측 요약)
- Tier C Custom 타입
- 플러그인 시스템 (Dashboard Slot의 진화판)
- 모바일 read-only 뷰어

---

## 아키텍처 의사결정 기록

| 결정 | 되돌리기 | 이유 |
|------|---------|------|
| **Agent-only write** | **매우 어려움** | 원칙 1 |
| **Multi-project by default** | 중간 | 1 인스턴스 = 1 팀 가정 탈피 |
| **GitHub OAuth V1 기본** | 쉬움 | 개발자 타겟 + Git 통합 자연스러움 |
| **Custom Dashboard Slot** | 쉬움 | 운영 자율성 흡수 |
| 단일 서비스 | 쉬움 | 필요 시 쪼개기 |
| PostgreSQL | 중간 | 스키마 마이그레이션 |
| **MCP-First** | **어려움** | 제품 정체성 |
| **Self-Host First** | **어려움** | 라이선스 + 문화 |
| **Typed Documents (Tier A/B/C)** | **매우 어려움** | 데이터 모델 기반 |
| Wiki UI as primary | 중간 | UX 전환 |
| Harness Reversal (VARN.md) | **어려움** | 에이전트 규율 근간 |

**Agent-only write + MCP-First + Self-Host + Typed Documents + Harness Reversal + Multi-project**이 타협 불가 영역.
