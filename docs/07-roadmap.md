# 07. Roadmap

Varn의 단계별 계획. V1을 최대한 좁고 선명하게, 확장은 사용자 시그널 따라.

## 원칙

1. **V1 = Flagship 완성도 100%**. 범위 넓히기보다 하나를 깊게.
2. **사용자 시그널 없이 확장하지 않는다.** 시그널 = 실사용 30일+, 이슈 3건+.
3. **OSS first**. BM 논의는 V2 이후. V1은 GitHub star + 실사용자 확보에 집중.
4. **Web SaaS/SI Domain Pack만 V1에서 완성**. 나머지는 skeleton — 각 도메인 기여자 등장 시 성숙.

## Scope Matrix

| 기능 | V1 | V1.1 | V1.x | V2+ |
|------|----|----|------|-----|
| **Harness Reversal (VARN.md 주입)** | ✅ | | | |
| MCP Layer | ✅ | | | |
| **Tool-driven Pre-flight Check** | ✅ | | | |
| **Referenced Confirmation** | ✅ | | | |
| Write-Intent Router (M1) | ✅ | | | |
| **Typed Documents Tier A (Core)** | ✅ | | | |
| **Tier B Web SaaS pack (stable)** | ✅ | | | |
| Tier B 다른 pack (skeleton) | ✅ (껍데기) | Game, ML 성숙 | Mobile 성숙 | 전체 |
| **Tier C Custom 타입** | | | | ✅ |
| Git Pinning (M3) | ✅ (단순) | ✅ (정교 AST/LLM) | | |
| **Fast Landing (M6)** | ✅ | | | |
| **Resource Freshness Re-Check (M7)** | ✅ (명시 트리거) | ✅ (자동 N회에 1회) | | |
| **Wiki Reader UI (1차)** | ✅ | | | |
| **Approve Inbox (편집 없음)** | ✅ | | | |
| Session 저장/의미검색 (F6) | ✅ | | | |
| Continuation Context (URL fetch) | ✅ | | | |
| Task 관리 (기본) | ✅ | | | |
| TC 기본 (required_for_close gate) | ✅ | TC Runner 자동 | | |
| Propagation Ledger (M4) | 이벤트 | ✅ Dashboard 3-tier | | |
| Smart Assign | | ✅ | | |
| Slack/Discord 봇 | | ✅ | | |
| Graph Explorer 시각화 | 간단 리스트 | ✅ 인터랙티브 | | |
| 멀티 에이전트 동시 작업 | | | ✅ | |
| 플러그인 시스템 | | | | ✅ |
| 클라우드 hosted (선택적 BM) | | | | ✅ |
| SSO / RBAC | | | | ✅ |
| 모바일 뷰어 | | | | ✅ |

---

## V1 — "Agent-only Wiki가 작동하는 순간"

**목표**: 솔로 개발자 1명이 세팅하고 30일 매일 쓸 수 있는 제품. 팀은 그 위에 자연스럽게 얹힘.

**기간 목표**: 설계 확정 후 **3~4개월** 내 첫 공개.

### V1 Feature Checklist

#### MCP & Harness
- [ ] MCP 서버 구현 (핵심 13개 tools, [03 참조](03-architecture.md))
- [ ] **VARN.md 템플릿** + `varn install` CLI
- [ ] CLAUDE.md / AGENTS.md / .cursorrules 자동 주입
- [ ] **Pre-flight Check responder** (타입별 체크리스트)
- [ ] **Referenced Confirmation 프로토콜** VARN.md에 박음
- [ ] Write-Intent Router (유사도 기반 conflict check)
- [ ] Session stream/upload/search

#### Data & Logic
- [ ] Tier A 7개 타입 (Decision/Analysis/Debug/Flow/Task/TC/Glossary) 스키마 + validator
- [ ] Tier B Web SaaS 4개 타입 (Feature/API/Screen/DataModel) stable
- [ ] Tier B Game/ML/Mobile 스켈레톤 등록
- [ ] Area 스키마 + Write-Intent Router 통과한 신규 Area 생성
- [ ] Related Resource edge + Pin edge 분리
- [ ] Git Pin (GitHub App, 단순 stale 판정)
- [ ] **Git Outbound URL 생성** (artifact → github.com/.../blob/...)
- [ ] Fast Landing (varn.context.for_task)
- [ ] Resource Freshness verify (명시 트리거 V1, 자동 V1.1)
- [ ] Session 의미 검색 (pgvector)

#### Web UI
- [ ] Wiki Reader (2축 트리, 본문 렌더, Related Resources 사이드 패널, Graph 사이드)
- [ ] Approve Inbox (Preview + OK/NO + 피드백 요청; 편집 버튼 없음)
- [ ] Sessions (F6 의미 검색)
- [ ] 간단 Stale 리스트
- [ ] Settings (Git repo, Domain Pack on/off, 멤버, VARN.md mode)

#### 배포 & 운영
- [ ] Docker Compose 설치 (1분 내 기동)
- [ ] 기본 인증 (Agent token + User session)
- [ ] Self-host 운영 가이드

#### 문서 & 마케팅
- [ ] Quickstart (5분 내 harness install + 첫 promote 체험)
- [ ] MCP 연동 가이드 (Claude Code, Cursor, Cline, Codex)
- [ ] 30초 Promote 데모 gif
- [ ] 라이선스 확정 (AGPL-3.0 유력)

### V1 Non-goals (명시적 제외)

- ❌ 사람 직접 편집 (원칙 1, 영원히)
- ❌ 메신저 기능 (Slack 봇도 V1.1)
- ❌ 멀티 팀/멀티 테넌트
- ❌ SSO, RBAC
- ❌ TC Runner (수동 기록 + `required_for_close` gate만)
- ❌ Propagation Dashboard 풍부 UX (간단 리스트)
- ❌ Graph Explorer 시각화 (인접 리스트)
- ❌ 클라우드 hosted
- ❌ 모바일
- ❌ Tier C Custom 타입

상세 이유: [08 Non-Goals](08-non-goals.md).

### V1 Launch Criteria

1. **Solo dogfooding 30일**: 저자 본인이 매일 사용 (팀 확보 대기하지 않음 — Solo 1급이므로 이게 성립)
2. **Meta-dogfooding**: **이 설계 문서들을 Varn으로 관리**. 지금 이 대화의 산출물이 Varn에 들어있는 상태로 V1 공개.
3. **External 2+ 인스턴스**: Solo 2명 또는 팀 1개 + Solo 1명 — 30일 이상 설치+사용
4. **문서 완비**: README, Quickstart, MCP 연동 가이드 4종(Claude Code/Cursor/Cline/Codex), Self-host 가이드
5. **데모 gif**: 30초 "사용자 요청 → 에이전트 제안 → 링크 동반 확인 → 승인 → 위키 반영" 전 과정
6. **깨끗한 VM에 10분 내 세팅 가능** 검증
7. **라이선스 확정**

---

## V1.1 — "사용 중 발견된 것들"

**목표**: V1을 써본 사용자 피드백 중 자주 나온 것부터. 현재 예측이지 약속 아님.

### V1.1 예상 우선순위

- ✅ **Slack/Discord 봇**: 슬래시 커맨드로 artifact 공유, 새 artifact 알림
- ✅ **M7 자동 트리거**: 읽기 시점 N회에 1회 Resource Freshness 재검증
- ✅ **Propagation Dashboard 3-tier**: High/Medium/Low + bulk 액션
- ✅ **TC Runner**: AI-가능 TC 자동 실행
- ✅ **Smart Assign**: 에이전트가 Task에 assignee 추천 (도메인 기반)
- ✅ **Git Pin 정교화**: AST/LLM 기반 의미 변경 판정 (오탐 감소)
- ✅ **Graph Explorer 인터랙티브**: d3/Cytoscape
- ✅ **Tier B Game/ML pack 성숙** (커뮤니티 기여)

**기간 목표**: V1 공개 후 2~3개월.

---

## V1.x — 성숙화 (Feedback-Driven)

V1.x는 기능 예측보다 **이슈 트래커 기반**:

- **멀티 에이전트 동시 작업** — Task 하나에 에이전트 3개 병렬 할당
- **Tier B 다른 pack 완성** — Mobile, CS Desktop 각 도메인 유저 등장 시
- **타입별 뷰 최적화** — ADR 전용 뷰, TC 매트릭스
- **검색 품질 향상** — 하이브리드 검색, re-ranking
- **세션 자동 분석** — 자동 Checkpoint 감지 고도화

---

## V2 — OSS 성장과 선택적 hosted

**V2에서 처음으로 BM 본격 논의.**

### V2 Feature 후보

- **Tier C Custom 타입** — YAML 스키마로 팀·도메인 정의
- **플러그인 시스템** — 커스텀 전파 규칙, 커스텀 UI 위젯
- **공식 에이전트 integrations** — Claude Code/Cursor/Cline/Codex 공식 파트너십
- **클라우드 hosted** — 자체 호스팅 원치 않는 사용자 대상

### BM Stance (단순)

현재 단순 입장: **OSS first, 잘 되면 hosted 추가**.

이미 검증된 선례들 (Sentry, PostHog, Supabase, Plausible, n8n):
- Core는 OSS (AGPL 유력)
- Managed cloud는 선택적 subscription
- 엔터프라이즈 기능(SSO, 감사로그)은 필요 시 상용 tier 고려

결정은 V1 공개 + 6개월 실사용 데이터를 본 후. **지금은 아무 것도 약속하지 않음**.

---

## 유즈케이스별 V1 가치

| 사용자 | V1에서 얻는 것 |
|--------|-------------|
| **Solo 개발자** | 세션 검색 지옥(F6) 해결 + 반-자동 아카이브. Tier B Web SaaS 사용 시 프로젝트 구조화. |
| **신규 프로젝트 부트스트랩** | "설계 정리해줘" → Tier A 스켈레톤 자동 생성. (이 리포가 1호 사례) |
| **레거시 프로젝트 역공학** | 기존 repo 스캔 → Feature/Flow 스켈레톤 자동 생성 → 사람 OK |
| **소규모 팀 협업** | 여러 에이전트 사용자가 같은 인스턴스에 붙어 중복·충돌 자동 감지 |
| **자율 에이전트 환경 (OpenClaw 등)** | VARN.md mode=auto로 설정, 에이전트 자체 체크포인트 기록. 관리자는 Wiki만 확인 |

---

## 마일스톤 (잠정)

| 시점 | 마일스톤 |
|------|---------|
| M0 | 설계 문서 확정 (지금 여기) |
| M0–1 | 데이터 모델 + MCP 스켈레톤 + Harness install |
| M1–2 | Pre-flight Check + Write-Intent Router + Tier A 타입 |
| M2–3 | Wiki Reader + Approve Inbox + Fast Landing |
| M3 | V1 내부 alpha, Solo dogfooding 시작 |
| M3–4 | External alpha → V1.1 준비 |
| M4 | V1 공개 릴리스 (lazy target) |
| M4–6 | External 30일 이상 사용 데이터 수집 |
| M6 | V1.1 공개 |
| M6–12 | V1.x 성숙 |
| Year 2 | V2 BM 결정 |

일정은 엄격한 약속이 아닙니다. 품질 + Agent-only write의 선명함이 우선.

---

## 위험 요소

### 기술적 위험

- **MCP 스펙 변경**: Anthropic 스펙 변동. 대응: 빠른 적응이 오히려 경쟁력.
- **Pre-flight Check 오버엔지니어링**: 체크리스트 너무 타이트하면 에이전트 생산성 저하. 대응: 체크리스트 per-type 세밀 튜닝 + 사용자 feedback으로 조정.
- **벡터 검색 품질**: 중복 감지 오탐/미탐. 대응: threshold 튜닝, 사용자 피드백 루프.
- **Git 연동 복잡도**: Webhook, rate limit. 대응: V1 작은 repo 타겟.

### 제품 위험

- **Checkpoint 제안 피로**: 에이전트가 너무 자주 제안 → 거절 남발. 대응: VARN.md mode 설정 (manual/auto/off) + 3회 거절 시 자동 off.
- **Typed documents 경직성**: 팀이 스키마를 답답해함. 대응: V1 Tier A/B 유지, V1.x부터 Tier B 확장, V2에서 Tier C.
- **"Notion 있는데 왜?" 저항**: 기존 툴 관성. 대응: **Agent-only write** + **Fast Landing** + **Pre-flight Check** 3개를 데모 gif 30초에 압축.
- **편집 금지가 답답하다** 피드백: 초기에 반드시 나올 것. 대응: **이게 제품 정체성**임을 문서·FAQ에 명시, 타협 없음.

### 비즈니스 위험

- **경쟁자**: Cursor Rules, Cline Memory Bank, Mem0, Continue.dev 등 유사 각도. 대응: **Agent-only write + Harness Reversal**이라는 선명한 철학적 차별화.
- **메인테이너 번아웃**: 1인 OSS 고질. 대응: 커뮤니티 조기 형성, Tier B 도메인 pack은 커뮤니티 기여로.

---

## 성공/피벗 판단 기준

**6개월 시점 점검**:

- GitHub star < 500 → 마케팅/포지셔닝 재검토
- 실사용 인스턴스 < 5 (Solo+팀 합쳐) → 온보딩/Quickstart 대수술
- 핵심 이탈 사유가 "편집 금지가 답답하다" → **원칙 방어**. 이건 타협 아님.
- 핵심 이탈 사유가 "Checkpoint 제안이 피곤하다" → VARN.md mode 기본값 + 휴리스틱 재조정
- "Notion/Linear로 충분한데?" 피드백 > 50% → 차별화 전달 방식 재정의 (원칙은 유지)

위 중 2+ 해당 시 **피벗 검토**. 단, **Agent-only write는 피벗 대상이 아니다** — 이걸 풀면 Varn 존재 이유 소실.
