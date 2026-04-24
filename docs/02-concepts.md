# 02. Concepts

Pindoc의 핵심 개념들. 이후 모든 설계 문서의 공통 어휘.

## 5대 Primitive

```
Project (최상위 스코프)
   │
   ▼
Harness (PINDOC.md — 에이전트 규약)
   │
   ▼
Promote ─▶ Artifact ─▶ Graph ─▶ (다음 Session의 Continuation Context)
```

### 1. Project

> 한 Pindoc 인스턴스에 복수의 Project가 공존. 권한·Area·설정의 단위.

모든 Artifact/Area/Member는 반드시 하나의 Project에 속한다.

```
Pindoc Instance
 ├─ Project "shop-fe"   (FE repo, Web SaaS pack)
 │    ├─ Areas: /strategy, /experience/ui, /system/api, /misc
 │    └─ Members: Alice(writer-agent, approver-user), Bob(reader)
 ├─ Project "shop-be"   (BE repo)
 └─ Project "side-game" (사이드 프로젝트, Game pack skeleton)
```

**시나리오**:
- FE/BE 분리 팀: Project 2개, 매니지먼트가 양쪽 접근
- Solo + 사이드 프로젝트: 한 인스턴스에 여러 Project
- 영세 2~3명이 복수 프로젝트: 한 인스턴스 공유, Project 단위 권한

**V1 기본값**: `1 repo = 1 project`.

**Cross-project**: Graph edge는 Project 경계 넘어 가능 (FE Feature ↔ BE API). Search / Fast Landing은 기본 현재 Project, 명시 시 cross-project.

---

### 2. Harness

> MCP 연결 순간 Pindoc이 에이전트의 base 행동 규약을 주입. Project 단위 1개.

Pindoc MCP install 시 각 Project 루트에 **`PINDOC.md`** 생성, `CLAUDE.md` / `AGENTS.md` / `.cursorrules`에 참조 추가. 에이전트는 매 세션 시작 시 이 규약 로드.

**담긴 것**:
- Checkpoint 휴리스틱 (mode: `auto` / `manual` / `off`)
- Propose → Pre-flight → 자동 publish 순서
- Referenced Confirmation 프로토콜
- Sensitive ops 정책 (`sensitive_ops: auto` | `confirm`)
- Area 규율
- URL 처리 규약

**왜 1번 인프라인가**: M1~M7은 "에이전트가 규약을 따른다"는 전제에서만 작동. Harness 없으면 Pindoc MCP가 "또 하나의 도구"로만 취급됨.

자세한 스펙은 `docs/09-pindoc-md-spec.md` (배치 B에서 작성 예정).

---

### 3. Promote

> 제품의 **중심 동사**. 에이전트가 Session 일부를 정제해 Artifact로 발행하는 행위.
> **에이전트 주도 + auto-publish 기본.** 사람은 방향 제시자 — 승인자가 아님.

**Promote 6단계** (외부에서 보면 한 동작):

```
1. Trigger        — 사용자 요청 or 에이전트 체크포인트 자율 판단
2. Intent         — 에이전트가 kind/target_type/target_area/reason 선언
3. Pre-flight     — Pindoc이 "더 일하고 와" 체크리스트 역지시 (M0.5)
4. Conflict Check — 기존 artifact와 중복/충돌 심사
5. Schema Valid.  — 타입별 필수 필드 검증
6. Commit         — 내부 저장 + Graph 업데이트 + 이벤트 발행
```

6단계는 에이전트 입장에서 `propose` 한 번 호출 + 통과 시 자동 진행. 외부 관찰자 관점으로는 **"promote = 발행"** 하나의 사건.

**Sensitive ops는 예외**: `Project.settings.sensitive_ops == "confirm"` 모드이고 해당 작업(삭제/supersede/settled 승격/신규 Area)이면 5단계까지 통과해도 6단계가 **Review Queue 대기**로 바뀜. 사람 OK 후 commit.

**꼭 완결일 필요 없음**: 유의미하면 `partial` 로 일단 기록. `draft → partial → settled` 단계 성숙.

---

### 4. Artifact

> Promote의 결과물. 영속적. 자산.

타입이 정해진 구조화된 문서. Wiki 페이지 / 태스크 / TC.

**Tier 구조**:
- **Tier A (Core, 강제)**: Decision(ADR), Analysis, Debug, Flow, Task, TC, Glossary
- **Tier B (Domain Pack, 선택)**: Web SaaS (V1 stable), Game/ML/Mobile (V1.x+ skeleton), 기타 V2+
- **Tier C (Custom, V2+)**: YAML 스키마로 팀 정의

**특징**:
- 타입별 **필수 스키마** (포맷 드리프트 방지)
- **Agent-only write**: `created_by` · `last_modified_via` 모두 `AgentRef` 필수
- **Project 소속**: `project_id` 필수
- **Git-pinned**: commit/PR/파일경로 고정
- **Area 소속**: 1개의 primary concern Area (고정 8 skeleton + depth 1 sub-area)
- **Completeness**: `draft` / `partial` / `settled`

---

### 5. Graph

> Artifact들 간의 관계망. 이것이 기억(Memory)의 실체.

**노드**: Artifact (Document / Task / TC).

**엣지 타입**: `references` / `derives_from` / `validates` / `implements` / `supersedes` / `pinned_to` / `related_resource` / `blocked_by` / `relates_to` / `continuation_of`.

**Project 경계**: edge는 Project 넘어 가능. Cross-project edge 선언에는 양쪽 읽기 권한 필요.

**Graph = Derived View**: Edge는 Artifact 필드(`pins[]`, `related_resources[]`, `references[]` 등)에서 **도출되는 view**. Source of truth는 Artifact 필드. 이중 저장 아님.

---

## 보조 개념들

### Checkpoint

Promote의 트리거 순간. 별도 저장 단위 아님.

- 사용자 명시 요청 ("정리해줘")
- 에이전트 자율 (N턴 + 결론 / 디버깅 resolution / 새 모듈 생성 / ADR 유발 키워드)
- 3회 거절 시 세션 동안 자동 제안 off

### SessionRef

에이전트 대화 세션의 **외부 레퍼런스**. Pindoc은 raw 세션을 저장하지 않음.

```
SessionRef {
  agent: "claude-code" | "cursor" | "cline" | "codex" | ...
  session_id: string     // 해당 에이전트 클라이언트 내부 ID
  timestamp: timestamp
  user: UserRef
  title_hint: string?    // Promote 시 에이전트가 제공한 1줄
}
```

Artifact의 `source_session: SessionRef` 필드로 참조. 사용자가 원하면 해당 클라이언트에서 원본 open — Pindoc은 원본 흡수하지 않음. 철학: **"너절한 채팅 로그는 해당 앱에서 보고, 관리 대상은 정돈된 artifact"**.

### Pin (Hard) vs Related Resource (Soft)

| 타입 | 의미 | Stale 감지 | 저장 위치 |
|---|---|---|---|
| `Pin` | hard pin, 정합 필수 | ✅ 자동 | `Artifact.pins[]` |
| `ResourceRef` | soft link, 맥락 navigation | ❌ (M7로 주기 검증) | `Artifact.related_resources[]` |

Graph의 `pinned_to` / `related_resource` 엣지는 위 필드에서 derive.

### Intent

```json
{
  "kind": "new" | "modification" | "split" | "supersede",
  "target_type": "Feature/Debug",
  "target_area": "system/api",      // 단수 — Artifact는 1개 Area에만 속함
  "target_id": "doc_xxx?",
  "reason": "PG 타임아웃 재시도 반영",
  "related_session": "SessionRef"
}
```

**cross-area 는?** 1개 Area 원칙. 여러 area를 가로지르는 reusable named concern은
`cross-cutting/<concern>`에 둔다. 특정 primary area의 단일 instance는 subject area + Tag로 표현한다.
한 문서가 실제로 여러 subject를 독립적으로 다루면 별도 Artifact 여러 개 + Graph `relates_to` 엣지로 분리한다.

**Decision은?** `Decision`은 Artifact type이다. Decision artifact도 `system/mcp`,
`experience/information-architecture`, `governance/taxonomy-policy` 같은 subject area에 배치한다.
`decisions` Area는 사용하지 않는다.

### Pre-flight Check (Tool-driven Prompting)

MCP 응답이 즉답 대신 체크리스트로 에이전트에 작업 역지시. [05 M0.5](05-mechanisms.md).

### Referenced Confirmation

에이전트가 사용자에게 확인 요청 시 **항상 링크 동반** 규약. [05 M0.6](05-mechanisms.md).

### Review Queue

**Sensitive ops만** 올라오는 대기열 (`sensitive_ops: confirm` 모드에서). 일반 publish는 auto. [06 Flow 3](06-ui-flows.md).

### Continuation Context

URL → `pindoc.artifact.read(url)` fetch 시 받는 번들: `{ artifact, neighbors, recent_changes, open_questions, source_session, related_resources, area_context, project }`.

### Completeness

- `draft`: 구조만. UI 링크 disabled.
- `partial`: 기본값. 의미 있는 내용.
- `settled`: 완결. 사람 승인 필요 (Review Queue).

### Project Permission (Role)

- `admin` — 설정, 멤버, Domain Pack, agent token
- `writer` (주로 에이전트) — Artifact write
- `approver` (사람) — Review Queue 처리
- `reader` — 읽기

한 사람/에이전트가 여러 Project에 서로 다른 role.

---

## 용어 경계 (Artifact vs Wiki vs Page 등)

내부 기술 문서·데이터 모델에서는 **Artifact**.
UI 라벨은 타입명 직접 노출 ("Debug", "Feature", "Task" 등) 또는 "페이지".
README/홍보에서는 **Wiki / Pindoc wiki** 사용 가능.

이 경계가 혼동되면 `docs/glossary.md` (배치 B에서 작성) 참조.

## 개념 간 관계도

```
┌──────────────────────────────┐
│  Project (최상위)            │
│  - 권한 스코프               │
│  - Areas 보유                │
│  - Harness (PINDOC.md)       │
└──────────────┬───────────────┘
               │
               ▼
        [Agent Session]       (외부, Pindoc이 저장 안 함)
               │
               │ Checkpoint trigger
               ▼
┌─────────────────────────────────┐
│   Promote (에이전트 주도)        │
│   1. Trigger                     │
│   2. Intent                      │
│   3. Pre-flight Check ★          │
│   4. Conflict Check              │
│   5. Schema Validation           │
│   6. Commit (auto-publish        │
│      or Review Queue if sensitive)│
└────────┬────────────────────────┘
         │
         ▼
┌─────────────┐
│  Artifact   │  source_session: SessionRef (외부 ref만)
│  Tier A+B   │  project_id, area, pins[], related_resources[]
└──────┬──────┘
       │
       │ Graph (derived view)
       ▼
┌─────────────┐
│   Memory    │
└──────┬──────┘
       │
       │ Continuation (URL → agent fetch)
       ▼
   [ 다음 Agent Session ]
```

**사람의 위치**: 대화 파트너 + 방향 제시자 + (엣지 케이스) approver. 타이핑·편집 없음.
