# Glossary

Pindoc에서 쓰이는 모든 용어 정의 + 경계. **Meta-dogfooding 1호**: V1 공개 시 이 문서는 Pindoc의 `Glossary` 타입 artifact 묶음으로 마이그레이션됩니다.

용어가 애매하게 쓰인 부분을 발견하면 이 문서에 기록 → 주 설계 문서들과 교차 검증.

---

## Core Primitives

### Project
한 Pindoc 인스턴스 내의 최상위 스코프. Artifact/Area/Member/설정의 단위.
- V1 기본: `1 repo = 1 project`
- 복수 Project 공존 (Multi-project by default)
- 참조: [02 Concepts §1](02-concepts.md), [04 Data Model](04-data-model.md)

### Harness
에이전트의 base 행동 규약. 물리적으로는 Project 루트의 **`PINDOC.md`** 파일.
- `pindoc init` 이 자동 생성
- CLAUDE.md / AGENTS.md / .cursorrules 가 참조
- 참조: [09 PINDOC.md Spec](09-pindoc-md-spec.md)

### Promote
**제품의 중심 동사.** 에이전트가 세션 일부를 정제해 Artifact로 승격·발행하는 행위.
- 6단계: Trigger → Intent → Pre-flight → Conflict → Schema → Commit
- 외부 관찰자에게는 하나의 사건 ("promote = 발행")
- 기본 auto-publish, sensitive ops + confirm 모드만 Review Queue
- 참조: [02 Concepts §3](02-concepts.md), [05 Mechanisms M1](05-mechanisms.md)

### Artifact
Promote의 결과물. 타입이 정해진 구조화된 문서. 영속 자산.
- Tier A (Core) + Tier B (Domain Pack) + Tier C (V2+ Custom)
- Project 1개, Area 1개에 속함
- Agent-only write: `created_by` · `last_modified_via` 모두 AgentRef
- 참조: [04 Data Model](04-data-model.md)

### Graph
Artifact들 간의 관계망. 기억(Memory)의 실체.
- Edge는 Artifact 필드에서 **derived view** (source of truth는 필드)
- Cross-project edge 허용
- 참조: [02 Concepts §5](02-concepts.md), [04 Data Model §Graph Edge Types](04-data-model.md)

---

## Promote 관련

### Checkpoint
Promote의 트리거 순간. 별도 저장 단위 아님 (primitive 아님).
- 사용자 명시 요청 또는 에이전트 자율 판단 (PINDOC.md 휴리스틱)
- 3회 거절 시 세션 동안 자동 제안 off

### Intent
Write 요청에 수반되는 메타데이터. 에이전트가 "무엇을 왜 쓰는지" 선언.
- `kind`: new / modification / split / supersede
- `target_area`: 단수 (Artifact는 1 Area만)
- 참조: [04 Data Model §Intent](04-data-model.md)

### Pre-flight Check (Tool-driven Prompting)
Pindoc이 `propose` 에 즉답 대신 **체크리스트로 에이전트에 작업 역지시**하는 패턴.
- MCP = regulator 의 구체 구현
- 타입별 체크리스트 ([09 §9](09-pindoc-md-spec.md))
- 참조: [05 Mechanisms M0.5](05-mechanisms.md)

### Auto-publish
Pre-flight → Conflict → Schema 통과 시 **자동 저장 + 발행**. 사람 승인 없음.
- 일반 publish는 항상 auto
- Sensitive ops + `confirm` 모드만 Review Queue

### Review Queue
Sensitive ops 전용 승인 대기열. 기본 모드(`auto`)에서는 사실상 비어있음.
- 참조: [05 Mechanisms M1.5](05-mechanisms.md), [06 UI Flows Flow 3](06-ui-flows.md)

### Sensitive Ops
되돌리기 힘든 작업들: 삭제 / archive / settled 승격 / supersede / 신규 Area / --force / 대규모 supersede.

---

## Artifact 관련

### Completeness
Artifact 성숙도 축. `draft` → `partial` → `settled`.
- 기본값 `partial`
- `draft` 는 UI 링크 disabled
- `settled` 승격은 sensitive op (confirm 모드 시 Review Queue)

### Status
Artifact 생애주기 축. `published` / `stale` / `superseded` / `archived`.

### Review State
Artifact 승인 경로 축. `auto_published` / `pending_review` / `approved` / `rejected`.

### UI 뱃지 (단순화)
내부 3축(completeness/status/review_state) 을 UI에서 4뱃지로 축약:
- **draft** (`completeness=draft`)
- **live** (published + partial/settled + auto_published/approved)
- **stale**
- **archived** (archived 또는 superseded)

`pending_review` 는 Wiki 에 노출되지 않고 Review Queue에만.

### Tier A (Core)
도메인 무관 강제 타입: Decision(ADR) / Analysis / Debug / Flow / Task / TC / Glossary.

### Tier B (Domain Pack)
도메인별 선택 타입:
- **web-saas** (V1 stable): Feature, APIEndpoint, Screen, DataModel
- **game / ml / mobile** (V1.x+ skeleton): 각 도메인 타입
- **cs-desktop / library / embedded** (V2+)

### Tier C (Custom)
V2+ 에서 YAML 스키마로 팀 정의.

### Area
Project 하위 수직 구분. 모든 Artifact는 1 Area에 속함 (단수).
- 예: `/Payment`, `/Cart`, `/Auth`, `/Misc` (최후수단)
- 신규 Area 생성은 Write-Intent Router 통과 필수

### Domain Pack
Tier B 타입들의 묶음. Project 설정으로 활성.

---

## Reference (Pin vs Related Resource)

### Pin (Hard)
Artifact를 코드의 특정 지점에 고정. **Stale 감지 대상**.
- `Artifact.pins[]` 필드에 저장
- Graph `pinned_to` edge는 이로부터 derive
- repo + commit/branch/pr + paths

### Related Resource (Soft, ResourceRef)
Artifact에 연결된 가벼운 맥락 navigation 리소스. **Stale 감지 아님**.
- `Artifact.related_resources[]` 필드
- Graph `related_resource` edge 는 이로부터 derive
- types: code / asset / api / doc / link
- M7 Freshness Re-Check로 주기 검증

### SessionRef
에이전트 대화 세션의 **외부 레퍼런스 메타**.
- Pindoc은 raw 세션 저장하지 않음 ("너절한 채팅은 해당 앱에서")
- `{ agent, session_id, timestamp, user, title_hint }`
- Artifact의 `source_session` 필드로 참조

### Continuation Context
URL → `pindoc.artifact.read` fetch 시 받는 번들: `{ artifact, project, neighbors, recent_changes, open_questions, source_session, related_resources, area_context }`.

---

## Mechanism 관련

### Harness Reversal (M0)
MCP 연결 순간 Pindoc이 에이전트 행동 규약을 주입하는 메커니즘. PINDOC.md 로 구현.

### Tool-driven Pre-flight Check (M0.5)
MCP tool 응답이 에이전트에 작업 역지시. MCP = regulator.

### Referenced Confirmation (M0.6)
에이전트 → 사용자 확인 요청 시 **링크 동반 필수** 프로토콜.

### Write-Intent Router (M1)
Intent 선언 + Conflict Check + Schema Validation → auto-publish or Review Queue.

### Git Pinning (M3)
Pin을 따라 코드 변경 감지 → `stale` 플래그.

### Propagation Ledger (M4)
Event Bus 이벤트를 dependent에 전파. V1.1 에서 Dashboard.

### TC Gating (M5)
Feature close 조건으로 TC 강제. V1.1 에서 TC Runner.

### Fast Landing (M6)
**완벽 인덱스 아님**. 핵심 1~3개 리소스로의 빠른 첫 착륙.
- `pindoc.context.for_task` 에 의해 구현
- F6 해결의 코어

### Resource Freshness Re-Check (M7)
Reverse Harnessing. 읽을 때마다(또는 주기) Pindoc이 에이전트에 related_resources 재검증 역지시.

---

## Permission / Auth

### Role
per-project: `admin` / `writer` / `approver` / `reader`.
- `admin`: 설정, 멤버, Domain Pack, agent token 발급
- `writer`: Artifact write (주로 에이전트)
- `approver`: Review Queue 처리 (사람)
- `reader`: 읽기

### Agent Token
Per-project scope. 특정 project의 writer 권한.
- 기본 90일 rotation
- `pindoc token revoke` 로 비활성

### User Session
사람용 로그인 세션. **Write 권한 없음** (스키마 수준 거부).
- read + Review Queue approve/reject 만

### OAuth
V1 self-host 기본 인증: **GitHub OAuth**. Local 시나리오는 OAuth 없음.

---

## UI 관련

### Wiki Reader
Pindoc의 **1차 사용자 경험 화면**. Artifact 읽기 + 2축 네비 + 사이드 패널.
- 편집 버튼 없음 (원칙 1)

### Cmd+K Palette
모든 액션·네비의 단일 진입점. Linear 패턴.

### Dashboard Slot
Custom Dashboard Slot 메커니즘의 각 위치:
- `hero` (최상단)
- `sidebar`
- `footer`
- `ads`
- OSS core 기본값 전부 null

### Project Switcher
Topbar의 현재 Project 선택 UI. Multi-project 1급 시민화 구현.

---

## Architecture 관련

### Event Bus
Pindoc 내부 이벤트 발행 인프라. Postgres LISTEN/NOTIFY 또는 outbox 패턴.

### Event Types (V1)
`artifact.published` / `artifact.stale_detected` / `artifact.superseded` / `artifact.archived` / `pin.changed` / `git.push_received` / `tc.failed` / `tc.run_completed` / `resource.verified` / `resource.broken` / `review.required` / `review.approved` / `review.rejected` / `project.area_created` / `project.member_added`.

### pgvector
PostgreSQL 확장. Artifact embedding 저장 + 의미 검색. F6 해결의 기술적 코어.

### Git Pinner
in = repo 변경 감지 → stale 이벤트 발행
out = Artifact의 pin/resource → GitHub/GitLab URL 자동 생성 (outbound)

---

## 용어 경계 (혼동 방지)

### "Artifact" vs "Wiki 페이지" vs "Document"
- **Artifact**: 내부 기술·데이터 모델 용어. 이 문서·04/05 등에서 사용.
- **Wiki 페이지** 또는 **문서**: UI·README·홍보 문구 표현.
- **Document**: Artifact 타입 중 하나의 **상위 카테고리** (Document/Task/TC). 약간 모호하니 "Document 타입 Artifact" 처럼 명시 권장.

### "Publish" vs "Promote" vs "Commit"
- **Promote**: 외부 동사. "세션 일부를 artifact로 승격" 전체 과정.
- **Publish**: 단어 자체는 쓰지 않음 (이전 설계의 혼란 원인). 대신 "promote 완료"로 표현.
- **Commit**: 내부 구현 용어. `artifact.commit` 은 Review Queue 승인 시 서버 내부 호출. MCP 공개 tool 아님.

### "Session" vs "SessionRef"
- **Session**: 개념적으로는 "에이전트와 사용자의 한 번 대화". Pindoc은 **저장하지 않음**.
- **SessionRef**: 에이전트 클라이언트의 세션을 가리키는 **외부 레퍼런스 메타**. Pindoc이 저장하는 것은 이것.

### "Pin" vs "Related Resource"
- **Pin**: Hard. 정합 필수. Stale 감지 대상. `pins[]` 필드.
- **Related Resource**: Soft. 맥락 navigation. Stale 감지 아님 (M7 주기 검증). `related_resources[]` 필드.

### "Review Queue" vs "Approve Inbox"
- **Review Queue**: 현재 용어. **Sensitive ops + confirm 모드에만**.
- **Approve Inbox**: 폐기된 이전 용어 (매 artifact 승인 강제하던 모델). 사용 금지.

### "Harness" vs "PINDOC.md"
- **Harness**: 추상 개념 (에이전트 행동 규약).
- **PINDOC.md**: 그 Harness를 담은 물리 파일.

### "pindoc.org" vs "Pindoc" vs "var.gg"
- **Pindoc**: 제품명.
- **pindoc.org**: 공개 데모 인스턴스 도메인 (V1 오픈 예정).
- **var.gg**: 생태계 브랜드. Pindoc은 var.gg 의 첫 플래그십.

### "Checkpoint" vs "체크포인트 제안"
- **Checkpoint**: Promote의 트리거 순간 (개념).
- **체크포인트 제안**: 에이전트가 사용자에게 역으로 하는 "정리할까요?" 액션.

### "Pre-flight Check" vs "Conflict Check" vs "Schema Validation"
Promote 6단계의 각 단계:
- **Pre-flight Check (M0.5)**: 에이전트가 "충분히 준비했는가" 검사 (search 했나, pin 있나 등). NOT_READY 반환 시 에이전트 재제출.
- **Conflict Check (M1 내)**: 유사도·중복 검사. HARD BLOCK / SOFT WARN / 통과.
- **Schema Validation (M2)**: 필수 필드 검증.

---

## 메타

이 Glossary 자체가 Pindoc의 `Glossary` 타입 Artifact의 예시입니다. 각 용어는 실제 Pindoc 인스턴스에서:

```
Glossary {
  term: "Promote",
  definition: "제품의 중심 동사. 에이전트가 세션 일부를 정제해 Artifact로 승격·발행하는 행위.",
  context: "Promote 6단계의 전체 흐름. 외부 관찰자에게는 하나의 사건.",
  aliases: [],
  see_also: [ref("02-concepts#promote"), ref("05-mechanisms#m1")]
}
```

형태로 분해될 것입니다.
