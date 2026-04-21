# 07. Roadmap

Varn의 단계별 계획. V1을 최대한 좁고 선명하게, 확장은 사용자 시그널 따라.

## 원칙

1. **V1 = Flagship feature 완성도 100%**. 범위 넓히기보다 하나를 깊게.
2. **사용자 시그널 없이 확장하지 않는다.** 5명이 실제로 쓰고 원하는 것만 추가.
3. **BM 논의는 V2 이후**. V1은 OSS star와 실사용자 확보에 집중.

## Scope Matrix

| 기능 | V1 | V1.1 | V1.x | V2+ |
|------|----|----|------|-----|
| MCP Layer | ✅ | | | |
| Write-Intent Router (M1) | ✅ | | | |
| Typed Documents (M2) | ✅ | | | |
| Git Pinning (M3) | ✅ (단순) | ✅ (정교) | | |
| Promote UI | ✅ | | | |
| Library (artifact 열람/편집) | ✅ | | | |
| Session 저장/검색 | ✅ | | | |
| Context Injection (읽기) | ✅ | | | |
| Task 관리 (기본) | ✅ | | | |
| TC 기본 | ✅ | | | |
| Propagation Ledger (M4) | 기본 이벤트만 | ✅ Dashboard | | |
| TC Runner (M5) | | ✅ | | |
| Smart Assign (에이전트 기반 태스크 분배) | | ✅ | | |
| Slack/Discord 봇 | | ✅ | | |
| Graph Explorer (시각화) | 간단 리스트 | ✅ 인터랙티브 | | |
| 멀티 에이전트 동시 작업 지원 | | | ✅ | |
| 커스텀 artifact 타입 | | | ✅ | |
| 플러그인 시스템 | | | | ✅ |
| 클라우드 호스팅 / SaaS | | | | ✅ |
| SSO / RBAC | | | | ✅ |
| 모바일 뷰어 | | | | ✅ |

---

## V1 — "Promote가 작동하는 순간"

**목표**: 한 명의 개발자가 혼자 세팅하고, 팀 2–5명이 실제로 매일 쓸 수 있는 제품.

**기간 목표**: 설계 확정 후 **3개월 내** 첫 공개 릴리스.

### V1 Feature Checklist

#### MCP & Core
- [ ] MCP 서버 구현 (핵심 10개 tools)
- [ ] Session stream/upload
- [ ] Artifact CRUD
- [ ] Intent 선언 + Router (유사도 기반)
- [ ] 타입 스키마 검증 (6개 Document 타입 + Task + TC)
- [ ] Git Pin (GitHub App 기반, 단순 판정)
- [ ] Context Injection API (태스크/경로 기준 관련 artifact 번들)

#### Data
- [ ] PostgreSQL 스키마
- [ ] pgvector 임베딩 & 검색
- [ ] 전문 검색 인덱스

#### Web UI
- [ ] Promote 플로우 (4 steps)
- [ ] Library (artifact 리스트, 상세, 편집)
- [ ] Sessions 리스트
- [ ] 간단한 Stale 리스트 (Dashboard 형태는 V1.1)
- [ ] Settings (git repo 연결, 멤버)

#### 배포
- [ ] Docker Compose 설치
- [ ] 1분 내 기동 가능
- [ ] 기본 인증 (유저/비번 + 세션)

#### 문서
- [ ] Quickstart (5분 내 첫 promote)
- [ ] MCP 연동 가이드 (Claude Code, Cursor, Cline)
- [ ] Self-host 운영 가이드

### V1 Non-goals (명시적 제외)

- ❌ 메신저 기능 (Slack 봇 조차 V1.1)
- ❌ 멀티 팀/멀티 테넌트
- ❌ SSO, RBAC
- ❌ TC Runner (수동 기록만)
- ❌ Propagation Dashboard (간단 리스트만)
- ❌ Graph Explorer 시각화 (인접 리스트만)
- ❌ 클라우드 호스팅 제공
- ❌ 모바일
- ❌ 커스텀 artifact 타입 UI

자세한 이유는 [08 Non-Goals](08-non-goals.md).

### V1 Launch Criteria

다음이 다 되어야 V1 공개:

1. **Dogfooding 1개월**: 본인 팀에서 매일 사용하며 30일 이상 버그 수정
2. **External Alpha 2팀**: 본인 팀 외 2개 팀이 설치 후 2주간 사용, 피드백 수렴
3. **문서 완비**: README, Quickstart, MCP 연동 가이드 3종
4. **데모 gif**: 30초 promote 데모 (wow moment)
5. **베포 검증**: Docker Compose로 깨끗한 VM에 10분 내 세팅 가능
6. **라이선스 확정**: AGPL-3.0 또는 대안

---

## V1.1 — "혼자서 쓸 만한 → 팀이 쓸 만한"

**목표**: V1을 써본 팀에서 나온 피드백 중 가장 자주 나온 것부터 해결.

예상 우선순위 (실제 피드백으로 조정):

### V1.1 예상 기능

- ✅ **Slack/Discord 봇**: 1줄 슬래시 커맨드로 artifact 공유, 새 artifact 알림
- ✅ **Propagation Dashboard**: Stale 3 tier (high/medium/low) + bulk 액션
- ✅ **TC Runner**: AI-가능 TC 자동 실행
- ✅ **Smart Assign**: 새 Task에 대해 에이전트가 팀원 도메인 보고 assignee 추천
- ✅ **Git Pinning 정교화**: AST/LLM 기반 의미 변경 판정 (오탐 감소)
- ✅ **Graph Explorer 인터랙티브**: d3/Cytoscape 기반 시각화

**기간 목표**: V1 공개 후 **2–3개월**.

---

## V1.x — 성숙화

V1.x는 **피드백 주도**입니다. 기능을 선언하기보다, 이슈 트래커에 쌓인 것을 우선순위로.

예상되는 트렌드:

- **멀티 에이전트 동시 작업** 지원 (에이전트 3개가 같은 Task에 붙어 각자 접근 시도)
- **커스텀 Artifact 타입** (YAML 스키마로 팀별 커스터마이징)
- **타입별 뷰 최적화** (ADR 전용 뷰, TC 매트릭스 등)
- **검색 품질 향상** (하이브리드 검색, re-ranking)
- **세션 원본에서 자동 ADR/Debug 감지** (promote 제안 수준 올리기)

---

## V2 — 생태계와 지속 가능성

**V2에서 처음으로 BM 논의.**

### V2 Feature 후보

- **클라우드 호스팅**: 자체 호스팅 원치 않는 팀 대상. 주 BM 후보.
- **멀티 테넌트**: 대규모 조직 대상
- **SSO / RBAC**: 엔터프라이즈 진입
- **플러그인 시스템**: 커스텀 전파 규칙, 커스텀 스키마, 커스텀 UI 위젯
- **공식 에이전트 integrations**: Claude Code/Cursor/Cline과의 공식 파트너십

### V2 BM 옵션 (아직 미정)

**Option A: Open Core**
- Core는 AGPL OSS
- 엔터프라이즈 기능(SSO, RBAC, 감사 로그)은 상용 라이선스
- 예: GitLab, Sentry 모델

**Option B: Managed Cloud**
- OSS는 그대로
- 유료는 "우리가 호스팅해드림" 서비스
- 예: Supabase, PostHog 모델

**Option C: 유지 (No BM)**
- 계속 OSS, 스타만 받음
- 서포트는 컨설팅으로 개인 수입
- 예: 다수의 1인 메인테이너 프로젝트

**현재 stance**: Option B를 선호. 이유:
- 제품 자체가 "자체 호스팅의 번거로움"을 풀려고 시작 → 클라우드로 완결
- Open/Closed 분할 없이 전체가 OSS로 가시성 유지
- 엔터프라이즈 영업보다 셀프 서빙 구독이 1인 운영에 적합

결정은 V2 직전에. 사용자 규모와 피드백에 따라.

---

## 마일스톤 (잠정)

| 시점 | 마일스톤 |
|------|---------|
| Month 0 | 설계 문서 확정 (지금 여기) |
| Month 0–1 | 데이터 모델 + MCP 스켈레톤 |
| Month 1–2 | Promote 플로우 end-to-end 동작 |
| Month 2–3 | V1 내부 alpha |
| Month 3 | V1 공개 릴리스 |
| Month 3–5 | External alpha → V1.1 |
| Month 6 | V1.1 공개 |
| Month 6–12 | V1.x 성숙 |
| Year 2 | V2 BM 결정 |

일정은 엄격한 약속이 아닙니다. 품질이 우선.

---

## 위험 요소

### 기술적 위험

- **MCP 스펙 변경**: Anthropic이 MCP 스펙을 크게 바꾸면 적응 비용. 대응: 스펙 변경에 빠르게 대응하는 것이 오히려 경쟁력.
- **벡터 검색 품질**: 중복 감지 오탐/미탐. 대응: threshold 튜닝, 피드백 루프로 학습.
- **Git 연동 복잡도**: Webhook, rate limit, large repo. 대응: V1은 작은 repo 타겟, 규모 대응은 점진적.

### 제품 위험

- **Promote 허들이 너무 높음**: 사용자가 귀찮아서 promote 안 함. 대응: Auto-select, AI 초안 품질 향상, 단축키.
- **Typed documents의 경직성**: 팀이 스키마를 답답해함. 대응: V1은 6개 고정, V1.x부터 커스텀 허용.
- **"Notion 있는데 왜?" 저항**: 기존 툴 관성. 대응: MCP 강제/git pin/TC gating — Notion이 절대 못 하는 3개를 wow gif로 압축.

### 비즈니스 위험

- **경쟁자**: 같은 각도로 만드는 사람이 전 세계 수백 명. 대응: 속도 + 특정 디자인 철학(Promote, Typed, Pin, Enforce)의 일관성.
- **메인테이너 번아웃**: 1인 OSS의 고질적 리스크. 대응: 사용자 커뮤니티 조기 형성, contribution 진입 장벽 낮춤.

---

## 성공/피벗 판단 기준

**6개월 시점에서 다음 지표 점검**:

- GitHub star < 500 → 마케팅/포지셔닝 재검토
- 실사용자 팀 < 3 → 온보딩/Quickstart 대수술
- 핵심 사용자 이탈 사유가 "Promote가 귀찮다" → UX 전면 재설계
- "Notion/Linear로 충분한데?" 피드백 > 50% → 차별화 포인트 재정의

위 중 2개 이상 해당하면 **피벗 검토**. OSS는 빠른 피벗이 가능한 게 장점.
