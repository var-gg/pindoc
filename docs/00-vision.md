# 00. Vision

> **북극성 한 문장:** 에이전트의 작업이 개인과 팀의 기억이 되는 곳.
> *Where agent work becomes lasting memory — pinned to code, written by agents.*

## 제품 정의 (한 문장)

> **사람은 타이핑하지 않는 위키.**
> 모든 쓰기는 에이전트를 통해서만 일어나고, 사람은 승인·거절·방향 제시만 한다.

## 왜 지금인가

2026년, 코딩 에이전트는 개인 생산성을 10배 끌어올렸습니다. 하지만 **지식의 생산성**은 아직 그 레버리지를 흡수하지 못하고 있습니다. 에이전트의 출력이 영속 자산으로 변환되는 파이프라인이 없기 때문입니다.

현재 개발자의 현실:

- 디버깅 2시간 → 세션 닫으면 증발 → 일주일 뒤 같은 문제 또 2시간
- "그때 그 대화 어디 있었지" → 월 단위로 쌓인 세션·채팅 뒤지기 → 대부분 포기
- 레거시 분석을 에이전트로 정리 → Notion/Confluence에 수동 복붙 → 포맷 제각각
- 팀원이 본 문서의 반은 이미 stale, 본인은 모름
- 애자일하게 일할수록 문서와 현실의 거리가 벌어짐

**과도기의 수동 해결책들**이 있습니다. "wiki.js + OpenProject + 수동 스킬"로 개인·팀의 번역가 역할을 하는 사람이 각 팀마다 한 명씩 나타나고 있습니다. 스케일하지 않고, 그 개인이 없으면 무너집니다.

Pindoc은 이 역할을 **제품화**합니다.

## 디자인 원칙 (우선순위 순)

### 원칙 1. Human-writable surface is zero.

**사람은 위키에 직접 타이핑하지 않는다.** 오탈자 교정, 링크 수정, 이미지 교체까지 **전부 에이전트와의 대화를 통해서만** 일어난다. UI에는 편집 버튼이 없다.

이유:
- 대 에이전트 시대에 사람의 흐물흐물하고 구체적이지 않은 파편들이 문서에 그대로 스며들면 **곧 노이즈**
- 사람의 역할은 판단·승인·거절·방향 제시. 기계적 정리는 에이전트의 일
- UI에 편집 버튼이 있는 순간 타협이 시작되고, 타협이 시작되면 Notion과 같아짐

이 원칙이 Pindoc을 **기존의 모든 wiki/task 제품과 근본적으로 다르게** 만든다. Notion/Confluence/Wiki.js는 자기 사용자 기반 때문에 절대 받아들일 수 없는 선언이고, 이것이 **구조적 해자**.

### 원칙 2. 사람은 승인자가 아니라 방향 제시자다.

매 artifact에 사람 승인 게이트를 걸지 않는다. Promote는 **에이전트 주도 + auto-publish** 가 기본. 사람이 개입하는 건:
- 대화 중 방향 제시 ("이건 이렇게 정리해줘")
- 발행 후 문제 발견 시 피드백 ("이거 지워/고쳐")
- 되돌리기 힘든 민감 작업(삭제·supersede·settled 승격)에 한해 Review Queue — 이것도 옵션 모드

원칙 1의 확장입니다. "타이핑하지 않는 = 승인도 거의 안 하는".

### 원칙 3. Promote가 중심 동사다.

기존 툴들의 동사는 `Create`입니다 (Notion, Linear). Pindoc의 중심 동사는 **`Promote`** — 에이전트가 세션 중 체크포인트 시점에 사용자에게 **역으로** 제안하고, 사용자가 승인하면(또는 auto 모드면 바로) 에이전트가 정제된 artifact로 승격시키는 행위.

"쌓인 출력 중에서 무엇을 건질까"의 세계관.

**꼭 완결된 정보만 남길 필요는 없습니다.** 유의미하면 `partial` 상태로 일단 기록. Artifact는 `draft → partial → settled`로 성숙.

### 원칙 4. 문서와 Tool은 active agent다.

기존 wiki는 문서를 **수동적 객체**로 취급. Pindoc의 문서와 tool은 **능동적 주체**. 에이전트가 뭔가 쓰려 할 때 Tool이 **"그 전에 이거부터 탐색했냐, 기존에 비슷한 거 없는지 봤냐, 필수 필드 다 채웠냐"** 를 역으로 되묻는다.

### 원칙 5. 제약이 곧 가치다.

Notion은 "뭐든 써라" 제품. Pindoc은 **"이 규칙 지켜야 써라"** 제품. 에이전트 write volume이 사람의 100배인 시대에 필요한 건 자유가 아니라 **규율**.

이 규율의 핵심 분류축은 [Area Taxonomy](19-area-taxonomy.md)다. Type은 문서 형식이고 Area는
8 concern skeleton(`strategy`, `context`, `experience`, `system`, `operations`, `governance`,
`cross-cutting`, `misc`)이다. `Decision` 같은 Type이나 `roadmap` 같은 time view를 top-level Area로
다시 인코딩하지 않는다. Sub-area 운영은 [Sub-area Promotion Policy](20-sub-area-promotion-policy.md)와
[Cross-cutting Admission Rule](21-cross-cutting-admission-rule.md)을 따른다.

### 원칙 6. 코드와 문서는 결합되어야 한다.

모든 artifact는 커밋/PR/파일 경로에 **고정(pin)** 됩니다. 코드 변경 시 artifact stale 자동. 제품 이름 `Pindoc`이 이 원칙에서 파생.

### 원칙 7. Multi-project by Default.

한 인스턴스 = 복수 Project. Solo의 사이드 프로젝트 / FE·BE 분리 팀 / 영세 사업장 2~3명의 복수 프로젝트 전부 1급.

### 원칙 8. Customization via Slots, Not Forks.

대시보드·브랜딩·광고 같은 운영 자율성은 **Custom Dashboard Slot**으로 흡수. 브랜치 분리나 포크 없음. OSS core는 중립.

## 타겟 사용자

**V1 타겟: 코딩 에이전트를 이미 쓰는 Solo 개발자 ~ 10인 소규모 팀.**

구체적으로:
- 최소 한 명이 Claude Code / Cursor / Cline / Codex 일상 사용
- 세션·채팅이 월 단위로 누적되어 "그때 그거" 찾기 고통
- (팀) Notion+Linear, Confluence+Jira 등 조합
- 변경 잦은 애자일 워크플로우
- 도구 선택 자율성

### Solo 개발자 — 1급 시민

세션 원본 흡수 없이도 가치 성립. **"Promote 문화 + 구조화된 artifact + 의미 검색"** 이 F6(과거 맥락 재발견) 해결책.

### 자율 에이전트 환경 (OpenClaw 등)

PINDOC.md `mode: auto`로 human-out-of-the-loop 완전 지원. 관리자는 정리된 위키·태스크만 보고 프로젝트 파악.

### V1 제외

- 대기업 엔터프라이즈 (SSO, 감사로그)
- 비개발 지식노동자 팀
- 코딩 에이전트 미사용자 (Pindoc은 agent-only write이므로 read-only만 가능)

## 유즈케이스 (V1 상정)

1. **Solo 아카이브** — 평소 Claude Code로 작업. 유의미한 사안마다 에이전트가 역으로 "정리할까요?" → 승인 → 반-wiki 누적. 나중에 URL 던져서 대화 재개.
2. **신규 프로젝트 부트스트랩** — "설계 정리해줘" → 에이전트가 Tier A 스켈레톤 draft. (이 리포 자체가 1호 사례.)
3. **레거시 역공학** — 에이전트가 기존 repo 스캔 → Feature/Flow 자동 생성 제안 → 팀 지식 부트스트랩.
4. **팀 협업 (Multi-project)** — FE/BE 분리 Project, 매니지먼트 양쪽 접근. 에이전트들 간 중복·충돌 자동 감지.

## 성공의 정의

**단기 (OSS)**
- GitHub star 5,000+
- Hacker News 1면
- Solo·팀 합쳐 최소 5 인스턴스 30일+ 가동
- pindoc.org 공개 인스턴스 지속 운영

**중기 (사용성)**
- Solo 10+ 명 매일 사용
- 팀 3+ 개 3개월 지속
- 사용자당 artifact 100+ 누적

**장기 (생태계)**
- Claude Code / Cursor / Cline / Codex 공식 integration
- Pindoc memory가 개인·팀 자산화

## 비성공의 정의

- **Human-writable surface 허용됨** (편집 버튼 타협)
- **매 artifact 사람 승인 강제로 회귀**
- "또 하나의 Notion-like" 포지셔닝
- 범용 협업 플랫폼 확장 (메신저, CRM 등)
- 엔터프라이즈 기능이 V1 우선순위 침범

## 한 장의 그림

```
┌───────────────────────────────────────────────────────────┐
│                                                             │
│   Coding Agent     ── MCP ──▶     Pindoc Server            │
│   (Claude Code,                  ┌─────────────┐            │
│    Cursor, Cline,                │ Harness     │            │
│    Codex, ...)                   │ (PINDOC.md) │            │
│                                  │             │            │
│   [user prompt]                  │ Promote     │            │
│        ↕                         │   (pre-     │            │
│   [agent work] ─checkpoint?─▶    │    flight   │            │
│        ↕                         │    check)   │            │
│   [propose]  ◀─do more work───   │   ↓         │            │
│        ↕                         │ Artifact    │            │
│   (auto-publish or Review Queue) │ (typed,     │            │
│                                  │  pinned,    │            │
│                                  │  multi-     │            │
│                                  │  project)   │            │
│                                  │   ↓         │            │
│                                  │ Graph       │            │
│                                  └──────┬──────┘            │
│                                         │                   │
│                                         ▼                   │
│   ┌──────────────────────────────────────────────────┐     │
│   │  Wiki (read-only UI)                              │     │
│   │  • Human: read + (엣지 케이스) approve/reject     │     │
│   │  • Agent: propose → pre-flight → publish          │     │
│   │  • URL → agent fetch (Continuation Context)       │     │
│   └──────────────────────────────────────────────────┘     │
│                                                             │
└───────────────────────────────────────────────────────────┘
```
