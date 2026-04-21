# 02. Concepts

Varn의 핵심 개념들을 정의합니다. 이 개념들은 이후 모든 설계 문서의 공통 어휘입니다.

## 7대 Primitive

```
Project
   │
   ▼
Harness (VARN.md)
   │
   ▼
Session ─ checkpoint? ─▶ Promote ─▶ Artifact ─▶ Graph ─▶ (다음 Session 컨텍스트 재주입)
                           │
                    (auto-publish,
                     엣지 케이스만
                     Review Queue)
```

### 1. Project (최상위 컨테이너)

> 한 Varn 인스턴스에 복수의 Project가 공존한다. 권한·Area·설정의 단위.

**정의**: Varn의 최상위 스코프. 모든 Artifact/Area/Session/Member은 반드시 하나의 Project에 속한다.

**구조**:
```
Varn Instance
 ├─ Project "shop-fe"     (FE repo, Web SaaS pack)
 │    ├─ Areas: /Cart, /Payment, /Auth
 │    └─ Members: Alice(writer-agent, approver-user), Bob(reader)
 ├─ Project "shop-be"     (BE repo, Web SaaS pack)
 └─ Project "side-game"   (사이드 프로젝트, Game pack skeleton)
```

**실전 시나리오**:
- **FE/BE 분리 팀**: Project 두 개, 각 팀은 자기 Project에 writer, 매니지먼트는 양쪽 모두 접근
- **Solo + 사이드 프로젝트**: 개인이 여러 Project 보유, 모두 같은 인스턴스
- **영세 사업장 2~3명이 2개 프로젝트**: 한 인스턴스 공유, Project 단위 권한 분리

**V1 기본값**: `1 repo = 1 project`. 한 repo가 multi-project가 되는 케이스는 드물므로 이를 기본으로.

**Project 간 관계**:
- Graph edge는 **Project 경계 넘어 가능** (FE Feature ↔ BE API 링크)
- Search / Fast Landing은 **기본 현재 Project 범위**, 명시 시 cross-project
- Agent token은 **project-scoped**

---

### 2. Harness

> MCP가 연결되는 순간 Varn이 에이전트의 base 행동 규약을 주입하는 장치. Project 단위로 1개.

**정의**: Varn MCP를 install하면 각 Project 루트에 `VARN.md`가 생성되고, `CLAUDE.md` / `AGENTS.md` / `.cursorrules`에 참조가 추가된다. 에이전트는 매 세션 시작 시 이 규약을 읽는다.

**담긴 것**:
- 언제 체크포인트 제안을 할지 (mode: `auto` / `manual` / `off`)
- Propose → Pre-flight → Auto-publish 순서 강제
- Referenced Confirmation 프로토콜
- Sensitive ops(`sensitive_ops: auto|confirm`) 설정에 따라 Review Queue 활용 여부
- Area 규율, URL 처리 규약

**왜 1번인가**: Varn MCP tool들은 **"에이전트가 알아서 쓸 때"가 아니라 "Harness가 에이전트에게 쓰라고 지시할 때"만** 의미 있음. 하네싱이 없으면 제품의 나머지가 작동하지 않는다.

자세한 스펙: `docs/09-varn-md-spec.md` (배치 B에서 작성 예정).

---

### 3. Session

> 에이전트와의 raw 작업 로그. 너저분함. 휘발성.

**정의**: 코딩 에이전트(Claude Code / Cursor / Cline / Codex 등)와 사용자 간의 한 번의 작업 대화.

**특징**:
- 길다 (수천 줄~수만 줄)
- 노이즈/시그널 비율 나쁨
- 원본 가치는 낮지만 **맥락 가치** 있음
- 닫히면 에이전트 컨텍스트에서 증발 (→ F6)

**Varn 처리**: MCP로 stream 또는 bulk upload. 검색 가능한 형태로 저장하되 **1급 자산은 아님**.

---

### 4. Checkpoint

> Session 진행 중 "이 부분은 남길 가치가 있다"고 판단되는 지점.

**트리거 종류**:

1. **사용자 명시 요청** — "정리해줘", "위키에", "체크포인트"
2. **에이전트 자율 판단** (VARN.md 휴리스틱):
   - 한 주제 N턴 이상 + 결론 도달 신호
   - 디버깅에서 resolution 도달
   - 새 파일·모듈·스키마 생성
   - ADR 유발 키워드
3. **거절 반복 자동 off** — 세션 내 3회 거절 시 자율 제안 중지

**중요**: **완결된 정보만 대상이 아니다.** 유의미하면 `partial` 상태로 일단 기록. "나중에"가 아니라 **"일단 기록하고 성숙시킨다"**.

---

### 5. Artifact

> Checkpoint에서 에이전트가 publish한 것. 영속적. 자산.

**정의**: 타입이 정해진 구조화된 문서. Wiki 페이지 / 태스크 / TC.

**Tier 구조**:
- **Tier A (Core, 강제)**: Decision, Analysis, Debug, Flow, Task, TC, Glossary
- **Tier B (Domain Pack, 선택)**: Web SaaS (V1 stable), Game/ML/Mobile (V1.x+), 기타 (V2+)
- **Tier C (Custom, V2+)**: YAML 스키마로 팀 정의

**특징**:
- 타입별 **필수 스키마** (포맷 드리프트 방지)
- **Agent-only write**: `created_by`·`last_modified_via` 모두 `AgentRef` 필수
- **Project 소속 필수**: `project_id`
- **Git-pinned**: commit/PR/파일경로 고정
- **Completeness 단계**: `draft` → `partial` → `settled`

---

### 6. Graph

> Artifact들 간의 관계망. 이것이 기억(Memory)의 실체.

**노드와 엣지**:
- 노드: Artifact (Document / Task / TC)
- 엣지: `references`, `derives_from`, `validates`, `pinned_to`, `related_resource`, `supersedes`, `continuation_of`, `implements`, `blocked_by`, `relates_to`

**Project 경계**: edge는 Project를 넘어 연결 가능 (FE Feature ↔ BE API).

---

### 7. Promote

> 제품의 중심 동사. Session의 일부를 Artifact로 승격시키는 행위.
> **에이전트가 제안·실행하고, publish는 자동.** 사람은 방향 제시자 — **승인자가 아님**.

### Promote 6단계 (경량화)

```
1. Trigger                 ─ 사용자 요청 or 에이전트 체크포인트 자율 판단
2. Intent Declaration      ─ 에이전트가 kind/target_type/scope/reason 선언
3. Pre-flight Check        ─ Varn이 에이전트에 "더 일하고 와" 체크리스트 역지시
4. Conflict Check          ─ 기존 artifact와 중복/충돌 검사
5. Schema Validation       ─ 타입별 필수 필드 검증
6. Publish (auto)          ─ 바로 commit. 사람 승인 없음.
```

**이전 설계의 "Human Approve" 단계는 삭제**되었습니다. 이유:
- 매 artifact에 사람 승인을 거는 건 원칙 1("사람은 방향 제시자")과 어긋남
- Solo 사용자·자율 에이전트 환경에 마찰 과잉
- 잘못 발행된 artifact는 사용자가 **"이거 지워/고쳐"** 하면 에이전트가 후속 propose로 처리
- Completeness(`draft`/`partial`/`settled`)가 이미 "아직 미완"을 표현

### Review Queue (엣지 케이스만 — 선택적)

기본은 auto-publish. 단 아래 **되돌리기 힘든/민감한 작업**은 `sensitive_ops: confirm` 설정 시 Review Queue에 올라 사람 OK를 기다림:

- **삭제 / archive**
- **`settled` 승격** (완결 선언)
- **`supersede`** (기존 문서를 대체)
- **신규 Area 생성** (중복 방지 목적)
- **`--force` 요청** (conflict HARD BLOCK 뚫기)

기본값은 `sensitive_ops: auto` (모든 걸 auto-publish). 팀이 "중요 변경은 한 번 더 보자" 하면 `confirm`으로 전환. 어느 쪽이든 **일반 publish는 항상 auto**.

---

## 보조 개념들

### Pin (Hard)

```
Pin {
  repo, ref_type: "commit"|"branch"|"pr"|"path_only",
  commit_sha?, branch?, pr_number?, paths[],
  pinned_at, pinned_by
}
```

Pin된 경로 변경 → `stale` 플래그 + Propagation Ledger 이벤트.

### Related Resource (Soft)

```
ResourceRef {
  type: "code"|"asset"|"api"|"doc"|"link",
  ref, purpose,
  added_at, added_by,
  last_verified_at?, verified_status
}
```

Stale 감지 대상 아님. Navigation + Context bundle 용도. **M7 Freshness Re-Check**로 주기 검증.

### Completeness

- `draft`: 구조만. UI 링크 disabled.
- `partial`: 기본값. 의미 있는 내용.
- `settled`: 완결. 사람 승인 필요 (Review Queue).

### Intent

```json
{
  "kind": "new" | "modification" | "split" | "supersede",
  "target_type": "...",
  "target_scope": ["Payment"],
  "target_id": "...",
  "reason": "...",
  "related_session": "..."
}
```

### Pre-flight Check (Tool-driven Prompting)

MCP 응답이 즉답 대신 체크리스트로 에이전트에 작업 역지시하는 패턴. [05 M0.5](05-mechanisms.md).

### Continuation Context

URL → `varn.wiki.read()` fetch 시 받는 번들: `{ artifact, neighbors, recent_changes, open_questions, source_session, related_resources, area_context }`.

### Project Permission (Role)

- `admin` — 프로젝트 설정, 멤버, Domain Pack 관리
- `writer` (주로 에이전트 토큰) — Artifact write 권한
- `approver` (사람) — Review Queue 처리
- `reader` — 읽기만

한 사람/에이전트가 여러 Project에 서로 다른 role을 가질 수 있음.

### Review Queue

**엣지 케이스만** 올라오는 대기열. 일반 publish는 auto. [06 Flow 3](06-ui-flows.md) 참조.

---

## 개념 간 관계도

```
┌────────────────────────────┐
│  Project (최상위)          │
│  - 권한 스코프              │
│  - Areas 보유              │
│  - Harness (VARN.md)       │
└──────────────┬─────────────┘
               │
               ▼
┌─────────────┐
│   Session   │  (에이전트 대화, 휘발성 → F6)
└──────┬──────┘
       │ Checkpoint trigger
       ▼
┌─────────────────────────────────┐
│   Promote (에이전트 주도, 6단계) │
│   1. Trigger                    │
│   2. Intent                     │
│   3. Pre-flight Check ★         │
│   4. Conflict Check             │
│   5. Schema Validation          │
│   6. Publish (auto)             │
└────────┬────────────────────────┘
         │
         │ (Sensitive ops 만
         │  → Review Queue → 사람 OK)
         ▼
┌─────────────┐
│  Artifact   │
│  Tier A+B   │
│  project_id │
└──────┬──────┘
       │ Graph 구성
       ▼
┌─────────────┐
│   Memory    │
└──────┬──────┘
       │ Continuation (URL fetch)
       ▼
  [ 다음 Session ]
```

이 루프가 Varn의 핵심입니다. 사람은 **대화 파트너**이자 **방향 제시자** — 타이핑하지 않고 승인 버튼도 거의 누르지 않습니다.
