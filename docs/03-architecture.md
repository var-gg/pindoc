# 03. Architecture

Varn의 시스템 구조. 단일 서비스 원칙, MCP 레이어, 주요 컴포넌트를 정의합니다.

## 설계 철학

### 단일 서비스 원칙 (Single Service by Default)

**마이크로서비스를 쓰지 않는다.** V1은 모놀리식 단일 서비스.

이유:
- 본인 경험: "wiki.js + OpenProject + 메신저"의 **설치 피로감**이 프로젝트 시작의 핵심 동기였음. 그 피로감을 사용자에게 다시 전가하면 안 됨
- `git clone && docker compose up` 1분 내 기동이 가능해야 함
- 1인~소수가 만들고 유지하는 OSS는 컴포넌트 수가 곧 유지보수 비용
- 스케일 필요성이 증명되기 전까지 분리하지 않음

### 자체 호스팅 우선 (Self-Host First)

- V1은 **자체 호스팅 전용**. Docker Compose 하나로 기동.
- 클라우드 매니지드는 V2 이후. BM 논의는 [07 Roadmap](07-roadmap.md).
- 사용자 데이터는 사용자 인프라에 머문다 (AGPL 라이선스와 결합).

### MCP가 1차 인터페이스 (MCP-First)

- 에이전트가 MCP를 통해 Varn과 상호작용하는 것이 **1급 경로**
- REST API는 2급 (MCP가 커버 못 하는 경우)
- UI는 3급 (사람의 검수/읽기 용도)

Notion/Linear와 정반대 순위. 이것이 "에이전트-native" 제품의 의미.

## 시스템 컴포넌트

```
┌────────────────────────────────────────────────────────────┐
│                         Varn Server                         │
│  (단일 Docker 컨테이너, 혹은 단일 바이너리)                   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                  MCP Layer (1차)                      │  │
│  │  - write-intent router                                │  │
│  │  - schema validator                                   │  │
│  │  - conflict detector                                  │  │
│  │  - context injector                                   │  │
│  └─────────────────────┬────────────────────────────────┘  │
│                        │                                    │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                 Core Services                         │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐  │  │
│  │  │ Artifact │ │ Session  │ │  Graph   │ │  Git   │  │  │
│  │  │  Store   │ │  Store   │ │  Engine  │ │ Pinner │  │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └────────┘  │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐             │  │
│  │  │Propagation│ │  Search  │ │  TC      │             │  │
│  │  │  Ledger  │ │  Index   │ │ Runner   │             │  │
│  │  └──────────┘ └──────────┘ └──────────┘             │  │
│  └──────────────────────────────────────────────────────┘  │
│                        │                                    │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                   Storage Layer                       │  │
│  │  - PostgreSQL (structured data)                       │  │
│  │  - Filesystem (artifact markdown, attachments)        │  │
│  │  - Vector index (pgvector or embedded)                │  │
│  └──────────────────────────────────────────────────────┘  │
│                        │                                    │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                 Web UI (3차)                          │  │
│  │  - Promote UI    - Artifact viewer                    │  │
│  │  - Stale dash    - Graph explorer                     │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │               REST API (2차, 선택)                     │  │
│  │  - 외부 통합(Slack 봇 등)                              │  │
│  │  - CLI 도구                                           │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
           ▲                           ▲
           │                           │
     ┌─────┴─────┐              ┌─────┴──────┐
     │ Coding    │              │   Web      │
     │ Agents    │              │  Browser   │
     │ (MCP)     │              │   (UI)     │
     └───────────┘              └────────────┘
           │                           
           │                           
     ┌─────┴─────────────────────────┐
     │ Slack/Discord Bot (선택)       │
     │ (artifact 링크 공유만)         │
     └────────────────────────────────┘
```

## 핵심 컴포넌트 설명

### MCP Layer

에이전트가 Varn과 대화하는 공식 창구.

**제공 MCP tools** (V1 초안):

| Tool | 역할 |
|------|-----|
| `varn.session.stream` | 세션 로그를 실시간으로 Varn에 저장 |
| `varn.session.upload` | 완료된 세션을 일괄 업로드 |
| `varn.artifact.search` | 기존 artifact 검색 (intent check 전처리용) |
| `varn.artifact.propose` | Promotion draft 제출 (intent + 내용) |
| `varn.artifact.commit` | Draft를 영속화 (사람 승인 후) |
| `varn.artifact.read` | Artifact 본문 조회 |
| `varn.graph.neighbors` | 특정 artifact의 관련 artifact 조회 |
| `varn.context.for_task` | 태스크/파일 경로 기준 관련 컨텍스트 번들 반환 |
| `varn.tc.register` | TC 정의 추가 |
| `varn.tc.run_result` | TC 실행 결과 보고 |

이 tools의 핵심은 **write 경로에 gate가 있다**는 것. 에이전트는 `propose` → (Varn 심사) → `commit` 두 단계를 반드시 거친다. 자세한 흐름은 [05 Mechanisms](05-mechanisms.md).

### Core Services

**Artifact Store**
- 문서/태스크/TC의 CRUD
- 버전 관리 (모든 수정은 버전으로 보존)
- 타입별 스키마 검증

**Session Store**
- Session 로그 저장 (시간순)
- 검색 가능하지만 1급 자산은 아님
- 기본 보존 90일 (설정 가능)

**Graph Engine**
- Artifact 간 엣지 관리
- 그래프 쿼리 (이 artifact의 이웃, 이 경로의 가장 관련 높은 artifact 등)
- 의존성 전파 계산

**Git Pinner**
- Git repo 연동 (로컬 경로 또는 GitHub/GitLab API)
- 커밋/PR/파일 경로 메타데이터 추적
- 코드 변경 감지 → stale 이벤트 발행

**Propagation Ledger**
- Artifact 변경 이벤트 → 영향받는 dependents 산출
- stale 플래그 관리
- 대시보드에 노출될 변경 큐

**Search Index**
- Artifact 전문 검색 (키워드)
- 벡터 검색 (의미 유사도) — 중복 감지, 컨텍스트 주입에 사용
- V1은 pgvector 내장, V2에서 외부 벡터 DB 옵션

**TC Runner (V1.1)**
- AI 가능한 TC는 자동 실행 에이전트에 위임
- E2E TC는 사람 할당
- 결과 aggregation

### Web UI

**사람 전용 인터페이스.** 주요 화면:

- **Sessions** — 세션 리스트, promote 버튼
- **Promote** — 세션을 artifact로 승격하는 편집기 (diff 검토, schema 편집)
- **Artifact Viewer** — 문서/태스크/TC 조회 + 편집
- **Graph Explorer** — 관련 artifact 시각화
- **Stale Dashboard** — 낡은/충돌난/전파 대기 artifact 모음
- **Settings** — 팀원, git repo 연결, 타입 스키마 커스터마이징

자세한 UX는 [06 UI Flows](06-ui-flows.md).

## 기술 스택 (제안)

V1 기준. 변경 가능.

| 레이어 | 기술 | 이유 |
|--------|-----|-----|
| 백엔드 언어 | **Go** 또는 **Rust** | 단일 바이너리 배포 용이, 성능 |
| DB | **PostgreSQL** | 범용성, pgvector 내장 가능 |
| 벡터 검색 | **pgvector** | 별도 서비스 불필요 |
| 프론트엔드 | **React + TypeScript** | 에코시스템 두꺼움, 컴포넌트 재활용 |
| UI 라이브러리 | **shadcn/ui** 또는 **Radix** | 커스터마이징 자유, 디자인 일관성 |
| 마크다운 렌더 | **remark/rehype** | Mermaid 플러그인 생태계 |
| 다이어그램 | **Mermaid** | 에이전트가 생성 쉬움 |
| MCP | **공식 MCP SDK** | Anthropic 스펙 준수 |
| 배포 | **Docker Compose** | 설치 피로감 최소화 |

**Go와 Rust 중 선택 기준**:
- Go: 개발 속도 빠름, 팀 확장 쉬움, Varn이 우선 필요로 하는 건 빠른 반복
- Rust: 성능 극한, 안전성, 장기적 관점

V1은 **Go 추천**. Rust는 hot path만 필요시 도입.

## 보안과 프라이버시

- **인증**: 팀 단위 self-host이므로 V1은 간단한 사용자/비밀번호 + 세션 쿠키. SSO/OAuth는 V2.
- **MCP 인증**: API 토큰 per agent. 팀원별/에이전트별 토큰 발급.
- **데이터 암호화**: 저장 시 DB 레벨 암호화는 호스팅 인프라에 위임. Varn 자체는 평문 저장.
- **코드 연동**: git credentials는 사용자가 제공. Varn은 read-only 접근 원칙.
- **LLM 호출**: Varn이 자체적으로 LLM을 호출하는 기능은 **에이전트 클라이언트에 위임**. 서버가 사용자 코드/문서를 외부 LLM에 직접 보내지 않음 (선택 기능으로는 가능).

## 확장성 (V1 scope 밖)

V1 이후 고려:
- 멀티 테넌트 (여러 팀 한 인스턴스)
- 외부 LLM 연동 (서버 측 요약 생성 등)
- 플러그인 시스템 (커스텀 artifact 타입, 커스텀 전파 규칙)
- 브라우저 확장 (Claude Code 세션 직접 캡처)
- 모바일 뷰어 (read-only)

## 아키텍처 의사결정 기록

이 문서의 결정 중 되돌릴 수 있는 것과 되돌리기 힘든 것을 구분:

| 결정 | 되돌리기 | 이유 |
|------|---------|------|
| 단일 서비스 | 쉬움 | 필요 시 쪼개면 됨 |
| PostgreSQL | 중간 | 스키마 마이그레이션 필요 |
| MCP-First | **어려움** | 이게 제품 정체성 |
| Self-Host First | **어려움** | 라이선스 + 문화 결정 |
| Typed Documents | **매우 어려움** | 전체 데이터 모델의 기반 |

**MCP-First, Self-Host, Typed Documents는 타협 불가 영역**입니다. 다른 것들은 실무에서 조정 가능.
