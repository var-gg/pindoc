# 08. Non-Goals

하지 **않을** 것들. 범위 방어를 위한 공식 문서입니다.

## 영원히 하지 않을 것 (Never)

### ❌ Direct Human Editing (원칙 1)

**사람이 위키를 직접 타이핑하는 모든 경로를 허용하지 않습니다.**

- Markdown 에디터 UI 없음
- WYSIWYG 없음
- 인라인 편집 없음
- 오탈자·링크·이미지 수정 전부 에이전트 경유
- API로도 User 계정은 write 권한 없음 (스키마 수준 `AgentRef` 강제)

이 원칙이 Varn의 **구조적 해자**. 기존 경쟁자(Notion/Confluence/Wiki.js)는 사용자 기반 때문에 받아들일 수 없는 선언.

**대응 원칙**: "편집 버튼 달아주세요" → 거절. "에이전트에게 수정 지시하기 UX 개선" 방향으로 리프레임.

### ❌ 매 artifact 사람 승인 강제

원칙 1의 확장. **사람은 방향 제시자이지 승인자가 아님**.

- 일반 publish는 auto. Review Queue는 **sensitive ops + `sensitive_ops: confirm` 모드**에만.
- "매 artifact 확인" 요구는 원칙 1과 마찬가지로 타협 없음.
- 잘못된 artifact는 사용자가 "이거 지워/고쳐" 피드백 → 에이전트가 후속 propose.

**대응 원칙**: "모든 publish에 approval 게이트를" 요청 → 거절. `sensitive_ops` 세분화 제안으로 리프레임.

### ❌ OSS Core에 광고 Embed

**Varn core 코드베이스 자체에는 광고/브랜딩이 내장되지 않습니다.**

- 광고는 **Custom Dashboard Slot** 메커니즘을 통해서만 (운영자가 `settings.yaml` 로 명시 설정)
- Core 기본값: 모든 slot null
- 광고 수익은 `varn.var.gg` 등 **공개 데모 인스턴스에서만** — 다른 self-host 사용자는 건드리지 않음

이게 "OSS 중립성" 을 유지하면서도 공개 인스턴스 운영비를 충당하는 경로.

**대응 원칙**: "core에 광고 기본 embed" → 거절. "Custom Slot 설정 가이드 강화" 방향으로.

### ❌ 자체 메신저

Slack/Discord/카톡 대체하지 않음.

**이유**: 메신저는 네트워크 효과 시장. 봇 통합(V1.1)으로 충분.

### ❌ 범용 Wiki / Notion 대체

Varn은 **에이전트가 코딩·설계 중 만든 산출물**에 최적화. 범용 노트, 회의록, 개인 메모, 디자인 스펙은 범위 밖.

**대응 원칙**: Tier A + 활성 Tier B 타입만 지원.

### ❌ 범용 프로젝트 관리 (Jira 대체)

스프린트 계획, 번다운, 벨로시티 등은 에이전트 워크플로우의 핵심이 아님. 칸반은 V1.x 검토 선까지.

### ❌ LLM 자체 호스팅 / 모델 제공

LLM 호출은 에이전트 클라이언트 책임. Varn은 모델 중립.

### ❌ 코드 자동 생성 / 에이전트 실행

Cursor/Claude Code/Cline과 경쟁 안 함. TC Runner(V1.1)만 예외.

### ❌ "완벽한 Resource 인덱스" 약속

Fast Landing(M6)은 **"빠른 첫 착륙지점"** — 완벽 인덱스 아님. 나머지는 에이전트 + 컴파일러 + LSP. M7로 점진 개선.

**대응 원칙**: "왜 X 파일이 related_resources에 없냐" → "에이전트가 등록한 시점 + M7 검증 주기"로 설명.

---

## V1에서 하지 않을 것 (V2+ 고려)

| 항목 | 이유 | 고려 시점 |
|---|---|---|
| SSO / RBAC 세분화 | V1 타겟은 Solo~10인 팀. per-project role만 | V2+ |
| 멀티 테넌트 (인스턴스 간) | V1은 1 인스턴스 내 Multi-project. 인스턴스 격리는 Hosted에서 | V2 (Hosted) |
| Hosted SaaS | OSS first 기조 | V2 |
| 모바일 전용 앱 | 데스크톱 중심 워크플로우 | V2+ read-only |
| Tier C Custom 타입 | API 안정화 먼저 | V2 |
| 다국어 UI | V1은 영어 | V1.x+ 커뮤니티 |
| 실시간 협업 (다중 커서) | Agent-only write이므로 동시 편집 자체 없음 | **구조적 해당 없음** |
| 데스크톱 CS 프로그램 | 웹 UI 충분, 유지보수 부담 | V2+ 사용자 시그널 |

---

## 유사해 보이지만 우리가 아닌 것들

### 기존 위키·태스크 도구

| 제품 | 겹치는 부분 | Varn 차이 |
|---|---|---|
| **Notion / Confluence** | 범용 위키 | 사람 타이핑 중심 vs **Varn은 사람이 타이핑 안 함** |
| **Linear / Jira** | 태스크 관리 | Task는 artifact 한 종류, 중심은 지식-태스크-코드 결합 |
| **GitHub Issues** | 코드 협업 | GitHub은 플러그인 수준, Varn은 MCP 1급 |
| **Wiki.js / BookStack / Outline** | self-host wiki | 범용·구조 자율 vs **Tier A/B opinionated + agent-only write** |

### 에이전트 메모리 / 컨텍스트 도구

| 제품 | 겹치는 부분 | Varn 차이 |
|---|---|---|
| **Claude Projects / Memory** | 프로젝트 컨텍스트 | 개인 단위, 타입·스키마·graph·팀 공유 없음. Varn의 부분집합. |
| **Cursor Rules / `.cursorrules`** | 에이전트 instructions | 정적 텍스트 write-back 없음. VARN.md가 대체+확장. |
| **Codex / Copilot `AGENTS.md`** | 프로젝트 지시문 | 정적. Varn은 active 시스템. |
| **Cline Memory Bank** | 세션 간 기억 `.md` | 로컬 read. 충돌·중복 관리 없음. Varn 원시 형태. |
| **Mem0 / Zep / Letta** | 에이전트 메모리 백엔드 | Vector + 요약. 타입·스키마·승인·graph 없음. 인프라 레이어. |
| **Continue.dev / Sourcegraph Cody** | RAG | **읽기** 중심. 문서 **쓰기** 아님. |

### 에이전트 실행 / 자동화

| 제품 | 겹치는 부분 | Varn 차이 |
|---|---|---|
| **GitHub Copilot Workspace / Cursor Background** | 에이전트 코드 작성 | Varn은 그들의 **산출물 레이어** |
| **cairn-dev/cairn** | 백그라운드 에이전트 | cairn은 코드 자동화, Varn은 지식화 |

### MCP 서버들

- **기존 MCP** (Notion MCP, Linear MCP 등): 기존 제품의 **연결자**
- **Varn MCP**: 첫날부터 MCP-native. Harness Reversal + Pre-flight Check가 제품 핵심. **regulator**이지 connector 아님.

### 한 줄 포지셔닝

> 이들 대부분이 **agent-readable memory**인데, Varn은 **agent-writable, human-approvable(엣지만), typed, graph-linked, pin-backed, multi-project knowledge substrate**.

---

## 거절의 기술 (이슈 응대 가이드)

GitHub Issue 기능 요청 판단 플로우:

```
1. 사람 직접 편집 허용하라는 요구?
   YES → 즉시 거절 (원칙 1)
   NO → 다음

2. 매 artifact 승인 강제?
   YES → 즉시 거절 (원칙 1 확장)
   NO → 다음

3. OSS core에 광고/브랜딩 embed?
   YES → 거절, "Custom Slot 설정 가이드" 방향 제시
   NO → 다음

4. "에이전트 → 자산" 루프를 강화?
   NO → non-goals 매칭 확인 → 거절
   YES → 다음

5. 다른 도구로 이미 해결?
   YES (예: 메신저 = Slack) → 연결만 제공
   NO → 다음

6. Tier B 특정 Domain Pack이면 해당 pack이 stable?
   NO → 커뮤니티 기여 기다림
   YES → 다음

7. 최소 3명 요청?
   NO → 🔖 백로그
   YES → 다음

8. V1 flagship 성숙 늦추는가?
   YES → V1.x 이후
   NO → V1 검토
```

---

## 결정 로그

| 날짜 | 변경 | 이유 |
|------|------|------|
| 2026-04-21 | 초기 non-goals 확정 | 설계 단계 |
| 2026-04-21 | **Direct human editing Never 1번** 확정 | 제품 헌법 1조 |
| 2026-04-21 | "완벽한 Resource 인덱스 약속" Never 등재 | Fast Landing 정직한 포지셔닝 |
| 2026-04-21 | 실시간 협업을 "구조적 해당 없음" 분류 | Agent-only write와 양립 불가 |
| 2026-04-21 | **"매 artifact 사람 승인 강제" Never 등재** | Auto-publish + Review Queue(sensitive ops only) 확정 |
| 2026-04-21 | **"OSS core에 광고 embed" Never 등재** | Custom Slot 메커니즘으로만 허용, 중립 유지 |
| 2026-04-21 | Multi-project 기본 지원 반영 | 영세 팀/FE·BE 분리/Solo 사이드 프로젝트 현실 |

---

## 마무리

이 문서가 지켜져야 Varn이 **선명한 제품**으로 살아남습니다.

**Feature creep은 1인·소수 OSS의 최대 적**이고, **원칙 1·2(편집 금지/승인 강제 금지) 타협은 제품 정체성의 죽음**입니다.

"할 수 있는 것"과 "해야 하는 것"의 구분 + **"절대 하지 않을 것"의 명시** — 메인테이너의 일.
