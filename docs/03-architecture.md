# 03. Architecture

Varn의 시스템 구조. 단일 서비스 원칙, MCP 레이어, 주요 컴포넌트를 정의합니다.

## 설계 철학

### 원칙 1. Agent-only Write Surface

**사람은 위키에 직접 타이핑하지 않는다** — 오탈자, 이미지 교체, 링크 수정까지 전부 에이전트 경유.

- UI에 편집 버튼 없음. 승인·거절·"이거 고쳐줘" 피드백만 존재.
- 모든 write는 MCP 경유 (에이전트 → Varn).
- 스키마 수준에서도 `created_by: AgentRef`, `last_modified_via: AgentRef` 필수로 강제 — User 타입은 거부.

근거: [00 Vision 원칙 1](00-vision.md).

### 원칙 2. 단일 서비스 (Single Service by Default)

**마이크로서비스를 쓰지 않는다.** V1은 모놀리식 단일 서비스.

- 본인 경험: "wiki.js + OpenProject + 메신저"의 **설치 피로감**이 프로젝트 시작의 동기. 그 피로감을 사용자에게 전가하면 안 됨
- `git clone && docker compose up` 1분 내 기동
- 1인~소수가 만드는 OSS에서 컴포넌트 수 = 유지보수 비용
- 스케일 필요성이 증명되기 전까지 분리 없음

### 원칙 3. 자체 호스팅 우선 (Self-Host First)

- V1은 **자체 호스팅 전용**. Docker Compose 하나로 기동.
- 클라우드 hosted는 V2 이후 (BM 논의는 [07 Roadmap](07-roadmap.md) — OSS first 기조).
- 사용자 데이터는 사용자 인프라에 머문다 (AGPL 라이선스와 결합).

### 원칙 4. MCP가 write 1차, Wiki UI가 사용자 경험 1차

- **에이전트 write path**: MCP (write-only, 1차)
- **사용자 경험 1차**: Wiki UI (read + approve, **edit 없음**)
- **REST API**: 3차 (외부 통합, CLI 등)

이전 설계에서 "UI 3차"였으나 전환: Solo 사용자의 F6(세션 검색 지옥) 해결과 팀 공유 가치의 대부분이 **Wiki 읽기 UX**에서 발생. 단, **UI는 reader + approver**이며 **editor가 아니다**.

### 원칙 5. Tier A/B/C 타입 구조

- **Tier A (Core)**: 도메인 무관 강제 타입 (Decision, Analysis, Debug, Flow, Task, TC, Glossary)
- **Tier B (Domain Pack)**: install 시 선택 — V1에서 Web SaaS/SI pack 완성, 나머지 스켈레톤
- **Tier C (Custom)**: V2 이후 — YAML 스키마로 커뮤니티·팀 정의

근거 및 디자인은 [02 Concepts](02-concepts.md), [04 Data Model](04-data-model.md).

## 시스템 컴포넌트

```
┌──────────────────────────────────────────────────────────────┐
│                        Varn Server                            │
│ (단일 Docker 컨테이너, 혹은 단일 바이너리)                      │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                  MCP Layer (write 1차)                    │ │
│ │  ┌────────────┐  ┌──────────────┐  ┌──────────────────┐  │ │
│ │  │  Harness   │  │  Pre-flight  │  │   Referenced     │  │ │
│ │  │  Injector  │  │  Check       │  │   Confirmation   │  │ │
│ │  │ (VARN.md)  │  │  Responder   │  │   Protocol       │  │ │
│ │  └────────────┘  └──────────────┘  └──────────────────┘  │ │
│ │  ┌────────────┐  ┌──────────────┐  ┌──────────────────┐  │ │
│ │  │  Write     │  │  Schema      │  │  Context         │  │ │
│ │  │  Intent    │  │  Validator   │  │  Provider (Fast  │ │
│ │  │  Router    │  │              │  │   Landing)       │  │ │
│ │  └────────────┘  └──────────────┘  └──────────────────┘  │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                    Core Services                          │ │
│ │ ┌─────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐ │ │
│ │ │Artifact │ │ Session  │ │  Graph   │ │ Project Tree  │ │ │
│ │ │  Store  │ │  Store   │ │  Engine  │ │(Tier A+B+Area)│ │ │
│ │ └─────────┘ └──────────┘ └──────────┘ └───────────────┘ │ │
│ │ ┌─────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐ │ │
│ │ │  Git    │ │Propagation│ │ Search   │ │  Resource     │ │ │
│ │ │ Pinner  │ │  Ledger   │ │ Index    │ │  Index + M7   │ │ │
│ │ │(in+out) │ │          │ │(F6)      │ │  Freshness    │ │ │
│ │ └─────────┘ └──────────┘ └──────────┘ └───────────────┘ │ │
│ │ ┌─────────┐                                              │ │
│ │ │   TC    │   (V1.1)                                     │ │
│ │ │ Runner  │                                              │ │
│ │ └─────────┘                                              │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                    Storage Layer                          │ │
│ │  - PostgreSQL (structured)                                │ │
│ │  - Filesystem (artifact markdown, attachments)            │ │
│ │  - pgvector (embeddings for session search + conflict)    │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │             Web UI (사용자 경험 1차)                       │ │
│ │  • Wiki Reader (★ primary)                                │ │
│ │  • Approve Inbox (OK/NO, 편집 없음)                       │ │
│ │  • Stale Dashboard     • Graph Explorer                   │ │
│ │  • Sessions (F6 검색)   • Settings                        │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │           REST API (3차, 외부 통합용)                      │ │
│ │  - CLI 도구, Slack/Discord 봇(V1.1), webhook              │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
          ▲                           ▲
          │                           │
    ┌─────┴─────┐              ┌─────┴──────┐
    │  Coding   │              │    Web     │
    │  Agents   │              │  Browser   │
    │  (MCP)    │              │   (UI)     │
    └───────────┘              └────────────┘
```

## 핵심 컴포넌트 설명

### MCP Layer (write 1차)

에이전트가 Varn과 대화하는 공식 창구.

**MCP Tools (V1 초안)**:

| Tool | 역할 |
|------|------|
| `varn.harness.install` | VARN.md 생성 + CLAUDE.md/AGENTS.md/.cursorrules에 include 지시 |
| `varn.session.stream` | 세션 로그를 실시간 저장 |
| `varn.session.upload` | 완료된 세션을 일괄 업로드 |
| `varn.session.search` | 세션 의미 검색 (F6 해결) |
| `varn.artifact.search` | 기존 artifact 검색 (intent check 전처리) |
| `varn.artifact.propose` | Promotion draft 제출 (→ Pre-flight Check 거침) |
| `varn.artifact.commit` | Draft 영속화 (사람 승인 후) |
| `varn.artifact.read` | Artifact 본문 + continuation context |
| `varn.wiki.read` | URL 기반 fetch (Continuation) |
| `varn.graph.neighbors` | 특정 artifact의 graph 이웃 |
| `varn.context.for_task` | 키워드/파일 기준 Fast Landing 번들 |
| `varn.resource.verify` | M7 Freshness Re-Check 트리거 |
| `varn.tc.register` | TC 정의 추가 |
| `varn.tc.run_result` | TC 실행 결과 보고 |

**Write 경로의 특이 패턴**:

- **Pre-flight Check (tool-driven prompting)**: `propose`는 즉답하지 않고 체크리스트로 에이전트에게 추가 작업 요구. [05 M0.5](05-mechanisms.md)
- **Referenced Confirmation**: 에이전트가 사용자에게 확인 요청할 때 artifact/repo URL 동반 필수. VARN.md에 규약으로 박힘. [05 M0.6](05-mechanisms.md)

### Core Services

**Artifact Store**
- Tier A + 활성 Tier B 타입의 CRUD (write는 MCP 경유만)
- 버전 관리 (모든 수정은 버전으로 보존)
- 타입별 스키마 검증
- `last_modified_via: AgentRef` 강제

**Session Store**
- Session 로그 저장 (시간순)
- 기본 보존 90일 (설정 가능)
- 의미 검색 인덱스 (F6 핵심)

**Graph Engine**
- Artifact 간 엣지 관리 (`references`, `derives_from`, `pinned_to`, `related_resource` 등)
- 그래프 쿼리
- 의존성 전파 계산

**Project Tree**
- Tier A core types (강제)
- 활성 Tier B domain pack 관리
- Area 트리 관리 (수직 구분)
- UI navigation 트리 제공

**Git Pinner (in + out)**
- **Inbound**: Git repo 변경 감지 → stale 이벤트 발행 (기존)
- **Outbound (NEW)**: Artifact의 pin/related_resource로 GitHub/GitLab URL 자동 생성 → UI 클릭 시 바로 코드로 이동

**Propagation Ledger**
- Artifact 변경 이벤트 → 영향받는 dependents 산출
- stale 플래그 관리
- 대시보드 노출 큐

**Search Index**
- Artifact 전문 검색 (키워드)
- Session 의미 검색 (F6)
- Vector 검색 (중복 감지, Fast Landing 에 사용)
- V1: pgvector 내장. V2: 외부 벡터 DB 옵션

**Resource Index + M7**
- `related_resource` soft link 관리
- Fast Landing 쿼리 (`varn.context.for_task`)
- M7 Freshness Re-Check 스케줄링 — 읽기 시점 N회에 1회 에이전트에 verify 요청

**TC Runner (V1.1)**
- AI 가능한 TC는 자동 실행 에이전트에 위임
- E2E TC는 사람 할당
- 결과 aggregation

### Web UI (사용자 경험 1차)

**사람의 read + approve 인터페이스. 편집 없음.**

주요 화면:
- **Wiki Reader (★)** — 트리 네비게이션 (Tier A/B/Area 두 축), 본문, Related Resources 사이드 패널, "이어가기" 버튼
- **Approve Inbox** — 에이전트 propose draft 검토, OK/NO (편집 불가, 수정 필요 시 피드백 → 에이전트 재제출)
- **Sessions** — 세션 리스트, 의미 검색 (F6), "이 세션에서 promote된 artifact" 링크
- **Stale Dashboard** — 낡은·전파 대기 artifact (V1 간단 리스트, V1.1 3-tier)
- **Graph Explorer** — 관계 시각화 (V1 간단 인접 리스트, V1.1 인터랙티브)
- **Settings** — Git repo 연결, Domain Pack 추가·변경, 멤버, VARN.md mode

자세한 UX: [06 UI Flows](06-ui-flows.md).

## 기술 스택 (제안)

V1 기준, 변경 가능.

| 레이어 | 기술 | 이유 |
|--------|-----|-----|
| 백엔드 언어 | **Go** | 단일 바이너리, 개발 속도, MCP SDK |
| DB | **PostgreSQL** | 범용성, pgvector 내장 |
| 벡터 검색 | **pgvector** | 별도 서비스 불필요 |
| 프론트엔드 | **React + TypeScript** | 에코시스템 |
| UI 라이브러리 | **shadcn/ui** 또는 **Radix** | 커스터마이징 자유 |
| 마크다운 렌더 | **remark/rehype** | Mermaid 플러그인 |
| 다이어그램 | **Mermaid** | 에이전트 생성 쉬움 |
| MCP | **공식 MCP SDK** | Anthropic 스펙 준수 |
| 배포 | **Docker Compose** | 설치 피로감 최소화 |

## 보안과 프라이버시

### 인증 재설계 (Agent-only write에 맞춰)

- **Agent Token (1차)**: 모든 write 경로 필수. per-agent 발급·로테이션. 토큰 탈취 시 해당 에이전트만 격리.
- **User Session (2차)**: read + approve/reject 전용. **write 권한 없음** (스키마 수준에서 User 타입 거부). 간단한 user/password + 쿠키.
- SSO/OAuth는 V2

### 기타

- **MCP 인증**: Agent token
- **데이터 암호화**: 저장 시 DB 레벨 암호화는 호스팅 인프라에 위임. Varn 자체는 평문 저장.
- **코드 연동**: git credentials는 사용자 제공. Varn은 read-only.
- **LLM 호출**: Varn 서버는 LLM을 직접 부르지 않음. 에이전트 클라이언트 책임.
- **Session 민감정보**: 세션 로그에 secrets가 섞일 수 있음 — 90일 보존이지만 MVP 이후 redaction 규칙 고려 (V1.x).

## 확장성 (V1 scope 밖)

- 멀티 테넌트 (한 인스턴스에 여러 팀)
- 외부 LLM 연동 (서버측 요약 등)
- Tier C Custom 타입 시스템
- 브라우저 확장 (세션 자동 캡처)
- 모바일 read-only 뷰어

## 아키텍처 의사결정 기록

| 결정 | 되돌리기 | 이유 |
|------|---------|------|
| **Agent-only write** | **매우 어려움** | 제품 정체성 1조 |
| 단일 서비스 | 쉬움 | 필요 시 쪼개면 됨 |
| PostgreSQL | 중간 | 스키마 마이그레이션 필요 |
| **MCP-First** | **어려움** | 제품 정체성 |
| Self-Host First | **어려움** | 라이선스 + 문화 결정 |
| **Typed Documents (Tier A/B/C)** | **매우 어려움** | 전체 데이터 모델 기반 |
| Wiki UI as primary | 중간 | UX 전환 가능 |
| Harness Reversal (VARN.md) | **어려움** | 에이전트 규율 체계 근간 |

**Agent-only write + MCP-First + Self-Host + Typed Documents + Harness Reversal**이 타협 불가 영역입니다.
