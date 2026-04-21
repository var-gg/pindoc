# 08. Non-Goals

하지 **않을** 것들. 범위 방어를 위한 공식 문서입니다.
이슈·피드백으로 "이것도 해달라"가 들어왔을 때 여기서 판단 기준을 찾습니다.

## 영원히 하지 않을 것 (Never)

### ❌ Direct Human Editing (원칙 1)

**사람이 위키를 직접 타이핑하는 모든 경로를 허용하지 않습니다.**

- Markdown 에디터 UI 없음
- WYSIWYG 없음
- 인라인 편집 없음
- 오탈자 수정, 링크 교정, 이미지 교체 전부 에이전트 경유
- API로도 User 계정은 write 권한 없음 (스키마 수준에서 `AgentRef` 강제)

**이유**:
- 대 에이전트 시대에 사람의 흐물흐물한 파편이 위키에 그대로 스며들면 노이즈
- UI에 편집 버튼 있는 순간 타협 시작, 타협 시작되면 Notion과 같아짐
- 이 원칙이 Varn의 **구조적 해자**. 기존 경쟁자가 사용자 기반 때문에 받아들일 수 없는 선언.

**대응 원칙**: "편집 버튼 달아주세요" 요청은 거절. "에이전트에게 수정 지시하기 UX 개선" 방향으로 리프레임.

### ❌ 자체 메신저

Slack/Discord/카톡 대체하지 않습니다.

**이유**:
- 메신저는 네트워크 효과 시장. 기능이 좋아도 팀이 갈아타지 않음.
- Varn 가치("에이전트 작업 → 자산")는 메신저 없어도 100% 달성.
- 대신 Slack/Discord **봇**으로 연결 (V1.1).

### ❌ 범용 Wiki / Notion 대체

모든 문서 유형을 담는 범용 툴이 되지 않습니다.

**이유**:
- Varn은 **에이전트가 코딩·설계 작업하며 만든 산출물**에 최적화
- 범용으로 가는 순간 차별화 소실
- 회의록, 일기, 개인 메모, 디자인 스펙은 다른 도구에서

**대응 원칙**: Tier A + 활성 Tier B 타입만 지원. 그 외는 "Notion에서 쓰세요"라고 명시.

### ❌ 범용 프로젝트 관리 도구

Jira/Linear의 전체 스펙을 추구하지 않습니다.

**이유**:
- 스프린트 계획, 번다운, 벨로시티, 워크로드 관리는 에이전트 워크플로우의 핵심이 아님
- 이 길로 가면 feature parity 경쟁 → 필패
- Task는 artifact의 한 타입으로서 artifact-centric으로만 다룸

**대응 원칙**: "Gantt 차트 추가해달라" 거절. 칸반은 V1.x에 검토.

### ❌ LLM 자체 호스팅 / 모델 제공

Varn이 자체 LLM을 돌리거나 특정 모델에 종속되지 않습니다.

**이유**:
- LLM은 에이전트 클라이언트의 책임 (Claude Code가 Claude를, Cursor가 자체 모델을)
- Varn이 LLM 호출 시작하면 비용/인프라 부담
- 모델 중립이어야 모든 에이전트를 품을 수 있음

**대응 원칙**: Varn 서버는 LLM 직접 호출 없음. 선택 기능으로 "외부 LLM API 연결" 정도만.

### ❌ 코드 자동 생성 / 에이전트 실행

Varn 자체가 코딩 에이전트가 되지 않습니다.

**이유**:
- Cursor, Claude Code, Cline과 경쟁하는 순간 가치 제안 붕괴
- Varn은 그들이 만든 출력을 다루는 **인프라 레이어**
- "Varn에서 직접 코드 수정"은 범위 밖

**대응 원칙**: TC Runner(V1.1)는 예외적으로 자동 실행 에이전트를 둠. 그 외 범용 코드 생성 없음.

### ❌ "완벽한 Resource 인덱스" 약속

Fast Landing(M6)이 **완전한 리소스 인덱스가 아님을 명시**.

**이유**:
- 인덱스는 에이전트가 artifact 발행 시 등록하는 게 유일한 source
- 시점에 따라 fidelity 변동
- Varn의 약속은 "**완벽한 인덱스**"가 아니라 "**빠른 첫 착륙지점**" — 나머지는 에이전트 + 컴파일러 + LSP의 일
- M7 Freshness Re-Check로 점진 개선

**대응 원칙**: "왜 X 파일이 related_resources에 없냐" 이슈는 "에이전트가 등록한 시점의 상태 + M7 주기 검증 중" 으로 설명.

---

## V1에서 하지 않을 것 (V2+ 고려)

### 🔄 SSO / RBAC / 엔터프라이즈

**이유**: V1 타겟은 Solo ~ 10인 팀. 엔터프라이즈는 V1에 불필요.
**고려**: V2 이후. 상용 tier 가능성.

### 🔄 멀티 테넌트

**이유**: V1은 self-host 1 인스턴스 = 1 Solo 또는 1 팀.
**고려**: 클라우드 hosted 시작 시 (V2).

### 🔄 모바일 전용 앱

**이유**: 코딩 에이전트 워크플로우는 데스크톱.
**고려**: 모바일 read-only 뷰어를 V2 이후.

### 🔄 Tier C Custom 타입 시스템

**이유**: 너무 일찍 열면 API 안정화 전 외부 의존성 락인.
**고려**: V1.x 내부 구조 안정화 후, V2 공식화.

### 🔄 다국어 UI

**이유**: V1은 영어만. 한국어는 저자 내부 사용으로만.
**고려**: V1.x 이후 커뮤니티 기여로.

### 🔄 실시간 협업 (다중 커서 등)

**이유**: Artifact는 agent-only write이므로 사람이 동시 편집할 일 자체가 없음.
**고려**: 해당 없음. 원칙 1과 구조적 모순.

### 🔄 데스크톱 CS 프로그램 (웹뷰 wrapping)

**이유**: 웹 UI만으로 충분. 네이티브 앱은 유지보수 비용.
**고려**: 사용자 시그널 명확할 때 V2+.

---

## 유사해 보이지만 우리가 아닌 것들

V1 출시 후 "그거랑 뭐가 다르냐" 질문이 나올 것. 미리 정리:

### 기존 위키·태스크 도구들

| 제품 | 겹치는 부분 | Varn과의 차이 |
|---|---|---|
| **Notion / Confluence** | 범용 위키 | Notion은 사람이 타이핑. Varn은 **사람이 타이핑하지 않음**. 타입 강제 O. |
| **Linear / Jira** | 태스크 관리 | Linear는 태스크 중심 워크플로우. Varn은 Task가 artifact 한 종류. 중심은 지식-태스크-코드 결합. |
| **GitHub Issues** | 코드 협업 | GitHub은 repo에 붙은 도구. 에이전트 통합은 플러그인 수준. Varn은 MCP가 1급. |
| **Wiki.js** | self-host wiki | Wiki.js는 범용 구조 자율. Varn은 **Tier A/B opinionated + agent-only write**. |

### 에이전트 코딩 도구 / 메모리

| 제품 | 겹치는 부분 | Varn과의 차이 |
|---|---|---|
| **Claude Projects / Memory** | 프로젝트 컨텍스트 | 개인 단위, 타입·스키마 없음, graph 없음, 팀 공유 없음. Varn의 부분집합. |
| **Cursor Rules / `.cursorrules`** | 에이전트 instructions | 정적 텍스트, write-back 없음. Varn의 `VARN.md`가 이걸 대체 + 확장. |
| **Codex / GitHub Copilot `AGENTS.md`** | 프로젝트 지시문 | 정적. Varn은 **생성·전파·검증까지 하는 active 시스템**. |
| **Cline Memory Bank** | `.md` 파일 세션 간 기억 | 로컬 폴더 read. 충돌·중복 관리 없음. Varn의 원시 형태. |
| **Mem0 / Zep / Letta** | 에이전트 메모리 백엔드 | Vector store + 요약. 타입·스키마·사람 승인·graph 없음. 인프라 레이어. |
| **Continue.dev / Sourcegraph Cody** | 에이전트 + 코드 이해 | RAG 기반 **읽기** 중심. 문서 **쓰기**는 아님. |
| **GitHub Copilot Workspace / Cursor Background** | 에이전트 직접 코드 작성 | Varn은 그들이 만든 **산출물 저장·관리·재사용** 레이어. |
| **cairn-dev/cairn** | 백그라운드 에이전트 실행 | cairn은 코드 작성 자동화. Varn은 에이전트 출력의 지식화. |

### MCP 서버들 (Notion MCP, Linear MCP 등)

- 기존 MCP: 기존 제품의 **연결자**. 그 제품의 제약/강점 그대로.
- **Varn MCP**: **첫날부터 MCP-native 설계**. Harness Reversal과 Pre-flight Check가 제품 핵심. 연결자가 아닌 에이전트 regulator.

### 핵심 포지셔닝 한 줄

> 이들 대부분이 **agent-readable memory**인데, Varn은 **agent-writable, human-approvable, typed, graph-linked, pin-backed knowledge substrate**.

이 조합(agent-only write + typed + graph + pinned)은 경쟁자 중 누구도 완전히 커버하지 못함.

---

## 거절의 기술 (이슈 응대 가이드)

GitHub Issue 기능 추가 요청 판단 플로우:

```
1. 사람 직접 편집 요구하는가?
   YES → 즉시 거절 (원칙 1, Never)
   NO → 다음
   
2. "에이전트 → 자산" 루프를 강화하는가?
   NO → 거절 (non-goals)
   YES → 다음
   
3. 이미 다른 도구로 해결되는가?
   YES (예: 메신저는 Slack) → 연결만 제공, 자체 구현 X
   NO → 다음
   
4. Tier A 공통이 아니라 특정 Domain Pack이라면,
   해당 pack이 stable인가?
   NO → 커뮤니티 기여 기다림
   YES → 다음
   
5. 최소 3명 이상 요청했는가?
   NO → 🔖 백로그
   YES → 다음
   
6. V1 flagship의 성숙을 늦추는가?
   YES → V1.x 이후
   NO → V1 검토
```

---

## 결정 로그

향후 non-goals 추가·제외를 기록:

| 날짜 | 변경 | 이유 |
|------|------|------|
| 2026-04-21 | 초기 non-goals 확정 | 설계 단계 |
| 2026-04-21 | **Direct human editing**을 Never 1번으로 확정 | 제품 헌법 1조 |
| 2026-04-21 | "완벽한 Resource 인덱스 약속"을 Never 등재 | Fast Landing의 정직한 포지셔닝 |
| 2026-04-21 | 실시간 협업을 "구조적 모순"으로 분류 (V2+ 고려에서 제외) | Agent-only write와 양립 불가 |

---

## 마무리

이 문서가 지켜져야 Varn이 **선명한 제품**으로 살아남습니다.

**Feature creep은 1인·소수 OSS의 최대 적**이고, **원칙 1 타협은 제품 정체성의 죽음**입니다.

"할 수 있는 것"과 "해야 하는 것"을 구분하는 것이 메인테이너의 일이고, **"절대 하지 않을 것"을 명시하는 것이 이 문서의 일**입니다.
