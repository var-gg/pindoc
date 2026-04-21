# 02. Concepts

Varn의 핵심 개념들을 정의합니다. 이 개념들은 이후 모든 설계 문서의 공통 어휘입니다.

## 6대 Primitive

```
Harness (VARN.md)
   │
   ▼
Session ─ checkpoint? ─▶ Promote ─▶ Artifact ─▶ Graph ─▶ (다음 Session 컨텍스트 재주입)
                           ▲
                      (사람 OK/NO만)
```

### 1. Harness (하네싱 역주입)

> MCP가 연결되는 순간 Varn이 에이전트의 base 행동 규약을 주입하는 장치.

**정의**: Varn MCP를 install하면 리포지토리의 `CLAUDE.md` / `AGENTS.md` / `.cursorrules` 등에 `VARN.md` 참조가 자동 추가되고, 에이전트는 매 세션 시작 시 이 규약을 읽는다.

**담긴 것**:
- 언제 체크포인트 제안을 할지 (휴리스틱 mode: `auto` / `manual` / `off`)
- 어떤 타입의 artifact를 써야 할지 판단 기준
- Propose → Pre-flight → Commit 순서 강제
- URL 받으면 `varn.wiki.read`로 fetch하라는 규약
- 사람에게는 edit 권한이 없다는 사실의 상기

**왜 1번인가**: Varn이 제공하는 MCP tool들은 **"에이전트가 알아서 쓸 때"가 아니라 "harness가 에이전트에게 쓰라고 지시할 때"만 의미 있음**. 하네싱 없이는 제품의 나머지가 작동하지 않는다.

---

### 2. Session

> 에이전트와의 raw 작업 로그. 너저분함. 휘발성.

**정의**: 코딩 에이전트(Claude Code, Cursor, Cline, Codex 등)와 사용자 간의 한 번의 작업 대화. 시행착오, 뒤엎은 시도, 잘못된 가설, 맞는 결론이 혼재.

**특징**:
- 길다 (수천 줄~수만 줄 가능)
- 노이즈/시그널 비율이 나쁨
- 원본 가치는 낮지만 **맥락 가치**는 있음
- 세션이 닫히면 에이전트 컨텍스트에서 증발 (→ F6 실패 모드)

**Varn에서의 처리**: MCP를 통해 제품에 stream 또는 bulk upload. 검색 가능한 형태로 저장하되, **1급 자산은 아님**. Session은 artifact의 원료.

---

### 3. Checkpoint

> Session 진행 중 "이 부분은 남길 가치가 있다"고 판단되는 지점.

**정의**: 에이전트 또는 사용자가 "지금 정리 시점"이라고 판단하는 트리거. Checkpoint는 곧 Promote의 입구.

**트리거 종류**:

1. **사용자 명시 요청** — "정리해줘", "위키에 남겨", "체크포인트"
2. **에이전트 자율 판단** — `VARN.md`의 휴리스틱 따라:
   - 한 주제에 N턴 이상 지속 + 결론 도달 신호 ("그럼 이렇게 가자", "결정됨")
   - 디버깅 세션에서 resolution 도달
   - 새 파일·모듈·스키마를 만들어냈음
   - ADR 유발 키워드 감지 ("우리 이걸로 가자")
3. **거절 반복 시 자동 off** — 같은 세션에서 3회 거절 → 세션 동안 자율 제안 중지

**중요**: **완결된 정보만 체크포인트 대상이 아니다.** 유의미하게 정리된 사안이면 `partial` 상태로 일단 기록. "아직 미완이라 나중에"가 아니라 "일단 기록하고 성숙시킨다".

---

### 4. Artifact

> Checkpoint에서 사람이 "가치 있다"고 승인한 것. 영속적. 자산.

**정의**: 타입이 정해진 구조화된 문서. Wiki 페이지이거나 태스크이거나 TC. 공유·참조의 단위.

**하위 종류** (V1 기준):

| 종류 | 타입 | 역할 |
|------|------|------|
| **Document** | Analysis | 코드/시스템/이슈 분석 리포트 |
| | ADR | 아키텍처 결정 기록 |
| | Flow | 플로우 다이어그램 중심 문서 |
| | Debug | 디버깅 세션 요약 (symptom/hypothesis/resolution) |
| | Feature | 피쳐 개요 + scope + 참조 |
| **Task** | Task | 할일 단위. Document에 연결 가능 |
| **TestCase** | TC | 검증 단위. Task/Feature의 하위 |

**특징**:
- 타입별로 **필수 스키마**가 있음 (포맷 드리프트 방지)
- 생성 시 **intent 선언** 필요 (중복 방지)
- **agent-only write**: 모든 수정은 에이전트 경유, `last_modified_via: agent_id` 필수
- **git-pinned**: 커밋/PR/파일경로에 고정
- **completeness 단계** 존재: `draft` → `partial` → `settled`
- 다른 artifact와 **양방향 링크**
- 변경 시 **dependents에 전파**

---

### 5. Graph

> Artifact들 간의 관계망. 이것이 기억(Memory)의 실체.

**정의**: Artifact들이 서로 참조·의존하는 그래프 구조.

**노드와 엣지**:

- 노드: Artifact (Document / Task / TC)
- 엣지 타입:
  - `references` — 이 문서가 저 문서를 인용
  - `derives_from` — 이 태스크가 저 문서/세션에서 파생
  - `validates` — 이 TC가 저 Feature를 검증
  - `pinned_to` — 이 artifact가 저 commit/file에 고정
  - `supersedes` — 이 문서가 저 문서를 대체
  - `continuation_of` — 이 세션이 저 artifact에서 이어짐

**왜 중요한가**:
- 신입 에이전트가 "결제 플로우 버그" 작업 시작 → graph 조회 → 관련 artifact 10개 자동 컨텍스트 주입
- 코드 변경 시 pinned 엣지 따라 stale 대상 자동 산출
- "이 결정이 어떻게 내려졌지?" 추적은 supersedes 엣지 역탐색
- URL 던지면 해당 artifact의 graph 이웃까지 함께 컨텍스트로 (Continuation Context)

---

### 6. Promote

> 제품의 중심 동사. Session의 일부를 Artifact로 승격시키는 행위.
> **에이전트가 제안하고 사람이 OK 한다** (역방향).

**정의**: 세션 진행 중 Checkpoint가 트리거되면, 에이전트가 intent를 선언하고 Varn이 심사를 걸고 에이전트가 작업을 보완한 뒤, 사람이 최종 OK/NO만 하는 과정.

**기존 툴과의 대비**:
- Notion의 `Create`: 빈 페이지를 사람이 어떻게 채울까의 세계관
- Varn의 `Promote`: 쌓인 출력 중 무엇을 건질까의 세계관
- **사람은 타이핑하지 않는다**. 승인만 한다.

**Promote 8단계**:

1. **Trigger** — 사용자 요청 or 에이전트 체크포인트 자율 판단
2. **Intent Declaration** — 에이전트가 kind/target_type/scope/reason 선언
3. **Pre-flight Check** (★) — Varn이 에이전트에게 **되묻는다**: "관련 artifact 검색했냐, 영향 경로 탐색했냐, scope 일치하냐". 체크 미통과 시 에이전트가 추가 작업 후 재제출
4. **Conflict Check** — 기존 artifact와 중복/충돌 검사 (유사도, scope 겹침)
5. **Schema Validation** — 타입별 필수 필드 검증
6. **Draft Generation** — 에이전트가 스키마 맞춘 초안 생성
7. **Human Approve** — 사람이 diff 검토 후 OK/NO. **편집은 불가**, 수정이 필요하면 "이거 고쳐줘" 피드백 → 에이전트가 다시 제출
8. **Publish** — Artifact로 영속화, Graph 업데이트, 전파 이벤트 발행

---

## 보조 개념들

### Pin (고정)

Artifact를 코드베이스의 특정 지점(commit/PR/file path)에 고정.

```
Artifact {
  id: "doc_payment_flow_v3"
  pinned_to: {
    repo: "company/main-app",
    commit: "a3f5e2c",
    paths: ["src/payment/*.ts", "src/checkout/flow.ts"]
  }
}
```

**효과**: pin된 경로가 변경되면 → artifact에 `potentially_stale` 플래그.

### Stale (낡음)

Artifact가 현실(코드)과 어긋났을 가능성이 있는 상태.

판정: pinned 경로의 코드가 변경됐거나, 참조 artifact가 supersede됐거나, TC의 마지막 run 이후 관련 코드 변경.

### Intent

Write 요청에 수반되는 메타데이터. 에이전트가 "내가 무엇을 왜 쓰는지"를 선언.

```json
{
  "kind": "modification",
  "target_type": "Document/Analysis",
  "target_id": "doc_payment_flow_v3",
  "target_scope": ["payment", "retry"],
  "reason": "결제 retry 로직 변경 반영",
  "related_session": "sess_2026-04-21-xxx"
}
```

Intent 없이는 write 불가.

### Pre-flight Check (tool-driven prompting)

Varn이 에이전트의 `propose` 호출에 대해 **즉답하지 않고** 체크리스트를 응답으로 돌려주는 패턴.

```
Agent:  varn.artifact.propose(type=ADR, ...)
Varn:   { status: "NOT_READY", checklist: [
          "✗ 대안(alternatives) 최소 2개 탐색?",
          "✗ 관련 ADR을 varn.artifact.search로 확인?",
          "✓ scope 선언 OK",
          "✗ 영향 파일 경로 확인?"
        ] }
Agent:  (누락분 수행) → 재제출
Varn:   { status: "READY", draft_id: "..." }
```

**이게 왜 신선한가**: 기존 MCP는 "요청 → 응답" 단방향. Varn은 "요청 → 서버가 에이전트에게 더 시킴 → 재요청". MCP tool이 **능동적**이라는 원칙의 구현.

### Promotion Draft

Promote 과정에서 에이전트가 생성하는 artifact의 예비 버전.
- 스키마 검증을 통과한 상태
- 사람 승인 전
- 승인 시 영속화, 거부 시 폐기
- **사람이 직접 편집하지 않는다** — 수정 필요 시 에이전트에게 재지시

### Continuation Context

사용자가 위키 URL을 에이전트 채팅에 던질 때, 에이전트가 `varn.wiki.read(url)`로 fetch하면 받는 번들.

```
ContinuationContext {
  artifact: Artifact         // 본문 전체
  neighbors: Artifact[]      // graph 상 직접 이웃 N개
  recent_changes: Event[]    // 최근 이 artifact와 이웃의 변경 이력
  open_questions: string[]   // 본문에 남겨진 질문들
  source_session: SessionRef?  // 이 artifact가 파생된 세션
}
```

**효과**: 사용자는 그냥 URL만 던지면 되고, 에이전트는 이 번들로 대화 재개. 딥링크·IDE 통합 없이 동작.

### Completeness

Artifact의 성숙도.

- `draft` — Promote 직후, 구조만 잡힌 상태
- `partial` — 의미 있는 내용 있으나 미완결 (기본값)
- `settled` — 사람이 "이건 완결"로 승인

"완결된 것만 남기지 않는다"는 원칙의 데이터 표현.

---

## 개념 간 관계도

```
┌────────────────────────────────┐
│  Harness (VARN.md)             │
│  - 에이전트 규약 주입           │
│  - 체크포인트 휴리스틱 mode     │
└───────────────┬────────────────┘
                │ 규약
                ▼
┌─────────────┐
│   Session   │  (에이전트 대화 로그, 휘발성 → F6)
└──────┬──────┘
       │
       │ Checkpoint trigger
       │  (사용자 요청 or 에이전트 자율)
       ↓
┌─────────────────────────────────┐
│   Promote (에이전트 주도)        │
│   ├── Intent Declaration         │
│   ├── Pre-flight Check (tool-  │
│   │    driven prompting) ★     │
│   ├── Conflict Check             │
│   ├── Schema Validation          │
│   ├── Draft Generation           │
│   └── Human Approve (OK/NO)    │
└────────┬────────────────────────┘
         │
         ↓
┌─────────────┐
│  Artifact   │  (typed, pinned, completeness단계)
│  ├─Document │       ↕ pin
│  ├─Task     │       ↕ reference
│  └─TestCase │       ↕ derive
└──────┬──────┘
       │
       │ Graph 구성
       ↓
┌─────────────┐
│   Memory    │  (검색·참조·컨텍스트 주입 대상)
└──────┬──────┘
       │
       │ Continuation Context
       │   (URL → agent fetch)
       ↓
  [ 다음 Session ]
```

이 루프가 **Varn의 핵심**입니다.
