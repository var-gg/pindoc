# 08. Non-Goals

하지 **않을** 것들. 범위 방어 공식 문서.

## 영원히 하지 않을 것 (Never)

### ❌ Direct Human Editing (원칙 1)

**사람이 위키를 직접 타이핑하는 모든 경로를 허용하지 않습니다.**

- Markdown 에디터 UI 없음
- WYSIWYG 없음
- 인라인 편집 없음
- 오탈자·링크·이미지 수정 전부 에이전트 경유
- API로도 User 계정은 write 권한 없음 (스키마 수준 `AgentRef` 강제)

**이유**: 대 에이전트 시대에 사람의 흐물흐물한 파편이 위키에 그대로 스며들면 노이즈. 사람의 역할은 방향 제시 — 타이핑 아님. UI에 편집 버튼 있는 순간 타협 시작 = Notion과 같아짐. 이 원칙이 Pindoc의 **구조적 해자**.

**대응**: "편집 버튼 달아달라" → 거절. "에이전트 수정 지시 UX 개선"으로 리프레임.

### ❌ 매 artifact 사람 승인 강제

원칙 1의 확장. **사람은 방향 제시자, 승인자가 아님.**

- 일반 publish는 auto. Review Queue는 **sensitive ops + `sensitive_ops: confirm` 모드**에만.
- "매 artifact 확인" 요구는 타협 없음.
- 잘못된 artifact는 "지워/고쳐" 피드백 → 에이전트 후속 propose.

**대응**: "모든 publish에 approval 게이트" 요청 → 거절. `sensitive_ops: confirm` 옵션 안내로 리프레임.

### ❌ OSS Core에 광고 Embed

**Pindoc core 코드베이스 자체에는 광고/브랜딩 내장 없음.**

- 광고는 **Custom Dashboard Slot** 메커니즘으로만 (운영자가 `settings.yaml` 로 명시 설정)
- Core 기본값: 모든 slot null
- 광고 수익은 pindoc.org 등 **공개 데모 인스턴스에서만** — 다른 self-host 사용자는 건드리지 않음

**대응**: "core에 광고 기본 embed" → 거절. "Custom Slot 설정 가이드 강화" 방향.

### ❌ Raw 에이전트 세션 파일 흡수 (V1~V1.x)

Claude Code / Cursor / Cline / Codex 의 로컬 세션 파일(JSONL, SQLite 등)을 **Pindoc이 읽거나 저장하지 않습니다.**

**이유**:
- 포맷 제각각 + OS별 파일 위치 + 사용자 PC 접근 권한 이슈
- 각 클라이언트 내부 구현이 수시 변경
- Pindoc의 scope는 **정제된 artifact** — 너절한 raw 채팅은 각 앱 책임
- "너절한 채팅로그는 Claude앱이나 Codex앱에서 보라" — scope 명확화

**V2+에서 실험적 옵션**: 에이전트 클라이언트 파트너십 후 hook 기반 통합 검토. V1~V1.x는 scope 밖.

**대응**: "내 Claude Code 세션 검색도 해달라" → 거절 + 설명. "Promote 문화로 유도" + "해당 앱의 검색 기능 활용" 안내.

### ❌ 자체 메신저

Slack/Discord/카톡 대체 안 함. 봇 통합(V1.1)으로 충분.

### ❌ 범용 Wiki / Notion 대체

Pindoc은 **에이전트가 코딩·설계 중 만든 산출물**에 최적화. 범용 노트·회의록·개인 메모는 scope 밖.

### ❌ 범용 프로젝트 관리 (Jira 대체)

스프린트 계획, 번다운, 벨로시티는 핵심 아님. 칸반은 V1.x 검토 선.

### ❌ LLM 자체 호스팅 / 모델 제공

LLM 호출은 에이전트 클라이언트 책임. Pindoc은 모델 중립.

### ❌ 코드 자동 생성 / 에이전트 실행

Cursor/Claude Code/Cline과 경쟁 안 함. TC Runner(V1.1) 예외.

### ❌ "완벽한 Resource 인덱스" 약속

Fast Landing(M6)은 **"빠른 첫 착륙지점"** — 완벽 인덱스 아님. M7로 점진 개선.

### ❌ 실시간 협업 (다중 커서 등)

Agent-only write이므로 사람 동시 편집 상황 자체가 없음. **구조적 해당 없음**.

---

## V1에서 하지 않을 것 (V2+ 고려)

| 항목 | 이유 | 고려 시점 |
|---|---|---|
| SSO / RBAC 세분화 | V1 타겟 Solo~10인. per-project role만 | V2+ |
| 멀티 테넌트 (인스턴스 간) | V1은 1 인스턴스 내 Multi-project | V2 (Hosted) |
| Hosted SaaS | OSS first | V2 |
| 모바일 전용 앱 | 데스크톱 중심 | V2+ read-only |
| Tier C Custom 타입 | API 안정화 먼저 | V2 |
| 다국어 UI | V1 영어 | V1.x+ 커뮤니티 |
| 데스크톱 CS 프로그램 (웹뷰 wrapping) | 웹 UI 충분 | V2+ 시그널 시 |

---

## 유사해 보이지만 우리가 아닌 것들

### 기존 위키·태스크

| 제품 | 겹침 | Pindoc 차이 |
|---|---|---|
| **Notion / Confluence** | 범용 위키 | 사람 타이핑 중심 vs **Pindoc은 사람이 타이핑 안 함** |
| **Linear / Jira** | 태스크 관리 | Task는 artifact 한 종류, 중심은 지식-태스크-코드 결합 |
| **GitHub Issues** | 코드 협업 | 플러그인 수준, Pindoc은 MCP 1급 |
| **Wiki.js / BookStack / Outline** | self-host | 범용·자율 vs **Tier A/B opinionated + agent-only** |

### 에이전트 메모리 / 컨텍스트

| 제품 | 겹침 | Pindoc 차이 |
|---|---|---|
| **Claude Projects / Memory** | 프로젝트 컨텍스트 | 개인 단위, 타입·스키마·graph·팀 공유 없음 |
| **Cursor Rules / `.cursorrules`** | 에이전트 instructions | 정적 텍스트 write-back 없음. PINDOC.md가 대체+확장 |
| **Codex / Copilot `AGENTS.md`** | 프로젝트 지시문 | 정적. Pindoc은 active 시스템 |
| **Cline Memory Bank** | 세션 간 `.md` 기억 | 로컬 read. 충돌·중복 관리 없음. Pindoc 원시 형태 |
| **Mem0 / Zep / Letta** | 에이전트 메모리 백엔드 | Vector + 요약. 타입·스키마·승인·graph 없음. 인프라 레이어 |
| **Continue.dev / Sourcegraph Cody** | RAG | **읽기** 중심. 문서 **쓰기** 아님 |

### 에이전트 실행 / 자동화

| 제품 | 겹침 | Pindoc 차이 |
|---|---|---|
| **GitHub Copilot Workspace / Cursor Background** | 에이전트 코드 작성 | Pindoc은 그들의 **산출물 레이어** |
| **cairn-dev/cairn** | 백그라운드 에이전트 | cairn은 코드 자동화, Pindoc은 지식화 |

### MCP 서버들

- **기존 MCP** (Notion MCP, Linear MCP 등): 기존 제품의 **연결자**
- **Pindoc MCP**: 첫날부터 MCP-native. Harness Reversal + Pre-flight Check = 제품 핵심. **regulator**.

### 한 줄 포지셔닝

> 이들 대부분이 **agent-readable memory**인데, Pindoc은 **agent-writable, human-direction-guided, typed, graph-linked, pin-backed, multi-project knowledge substrate** — 그리고 raw 세션은 건드리지 않음.

---

## 거절의 기술 (이슈 응대 가이드)

```
1. 사람 직접 편집 요구? → 즉시 거절 (원칙 1)
2. 매 artifact 승인 강제? → 즉시 거절 (원칙 2)
3. OSS core에 광고 embed? → 거절, Custom Slot 안내
4. Raw 세션 파일 흡수? → 거절, scope 밖임을 설명 (Promote 문화로 유도)
5. "에이전트 → 자산" 루프 강화? NO → non-goals → 거절
6. 이미 다른 도구로 해결? (메신저=Slack) → 연결만 제공
7. Tier B 특정 Domain Pack이면 stable? NO → 커뮤니티 기여
8. 최소 3명 요청? NO → 🔖 백로그
9. V1 flagship 성숙 늦춤? YES → V1.x+
```

---

## 결정 로그

| 날짜 | 변경 | 이유 |
|------|------|------|
| 2026-04-21 | 초기 non-goals 확정 | 설계 단계 |
| 2026-04-21 | **Direct human editing Never 1번** | 제품 헌법 1조 |
| 2026-04-21 | "완벽한 Resource 인덱스 약속" Never | Fast Landing 정직한 포지셔닝 |
| 2026-04-21 | 실시간 협업을 "구조적 해당 없음" | Agent-only write와 양립 불가 |
| 2026-04-21 | **"매 artifact 사람 승인 강제" Never** | Auto-publish + Review Queue(sensitive ops only) |
| 2026-04-21 | **"OSS core에 광고 embed" Never** | Custom Slot 메커니즘으로만 |
| 2026-04-21 | Multi-project 기본 지원 | 현실 시나리오 반영 |
| 2026-04-21 | **"Raw 세션 파일 흡수" V1~V1.x Never** | scope 좁힘, 각 클라이언트 책임 |
| 2026-04-21 | **프로젝트명 Varn → Pindoc** | 이름이 제품 핵심(pin + doc) 즉시 전달 |

---

## 마무리

이 문서가 지켜져야 Pindoc이 **선명한 제품**으로 살아남습니다.

**Feature creep은 1인·소수 OSS의 최대 적**이고, **원칙 1·2 타협은 제품 정체성의 죽음**입니다.

"할 수 있는 것" vs "해야 하는 것" 구분 + **"절대 하지 않을 것" 명시** — 메인테이너의 일.
