# 05. Mechanisms

Pindoc이 [실패 모드 F1–F6](01-problem.md)를 해결하는 핵심 메커니즘들.

## 메커니즘 전체 지도

| 메커니즘 | 해결 대상 | V1 포함 |
|---------|--------|---------|
| **M0. Harness Reversal** | 모든 메커니즘의 전제 | ✅ Flagship |
| **M0.5. Tool-driven Pre-flight Check** | F1/F2 | ✅ |
| **M0.6. Referenced Confirmation** | UX 전반 | ✅ |
| M1. Write-Intent Router | F1 중복, F2 경계 | ✅ Flagship |
| **M1.5. Review Queue for Sensitive Ops** | 되돌리기 힘든 작업 보호 | ✅ (default auto) |
| M2. Typed Documents (Tier A/B) | F3 | ✅ Flagship |
| M3. Git Pinning | F5 기반 | ✅ (단순) |
| M4. Propagation Ledger | F5 UX | 🔄 V1.1 Dashboard |
| M5. TC Gating | F4 | 🔄 V1.1 Runner |
| **M6. Fast Landing** | F6 + 탐색 비용 | ✅ Flagship |
| **M7. Resource Freshness Re-Check** | 인덱스 정합성 | ✅ (명시 트리거 V1) |

---

## M0. Harness Reversal

> MCP 연결 순간 Pindoc이 에이전트의 base 행동 규약을 주입.

### 흐름

```
(install)
$ pindoc init
  ↓ Project 선택/생성, Domain Pack 선택, Agent token 발급
  ↓
PINDOC.md 생성 (Project 루트)
  ↓
CLAUDE.md / AGENTS.md / .cursorrules 에
"See ./PINDOC.md for this project's agent protocol." 삽입
  ↓
MCP 클라이언트 config 자동 주입 (Claude Code, Cursor, Cline, Codex)

(이후 매 세션)
에이전트 → CLAUDE.md → PINDOC.md 로드
  ↓
규약 반영: checkpoint mode / propose 순서 / Referenced Confirmation /
           sensitive_ops 정책 / Area 규율 / URL 처리
```

PINDOC.md 전체 스펙은 `docs/09-pindoc-md-spec.md` (배치 B 작성 예정).

### 왜 M0이 최상위인가

M1~M7은 "에이전트가 Pindoc 규약을 따른다"는 전제. Harness 없으면 Pindoc MCP는 "또 하나의 도구"로만 취급되고 규율이 성립 안 함.

---

## M0.5. Tool-driven Pre-flight Check

> MCP 응답이 에이전트에 작업 역지시. MCP = regulator.

### 흐름

```
Agent: pindoc.artifact.propose(type=ADR, ...)
  ↓
Pindoc: {
  status: "NOT_READY",
  checklist: [
    "✗ alternatives 최소 2개?",
    "✗ pindoc.artifact.search 로 관련 ADR 확인?",
    "✓ target_area 선언 OK",
    "✗ pins / related_resources 비어있음. 영향 경로 확인?"
  ],
  suggested_next_tools: [
    "pindoc.artifact.search(type=ADR, area=/Payment)",
    "pindoc.context.for_task(...)"
  ]
}
  ↓
Agent: 누락분 수행 → 재제출
  ↓
Pindoc: READY → M1 Router 진입
```

### 타입별 체크리스트

| 타입 | 체크 |
|------|------|
| ADR | alternatives ≥ 2, 선행 ADR search, pins/resources |
| Debug | hypotheses ≥ 1 + evidence, reproduction, symptom |
| Feature | acceptance_criteria ≥ 1, dependencies 식별 |
| Flow | Mermaid ≥ 1, actors ≥ 1 |
| APIEndpoint | method/path (schema 권장) |
| Glossary | definition 구체 |

---

## M0.6. Referenced Confirmation

> 에이전트가 사용자에게 확인 요청할 때 **항상 링크 동반**.

### 프로토콜

```
사용자 확인 요청 시 반드시:
1. 1줄 요약
2. 관련 artifact URL(들)
3. 코드 경로 있으면 repo URL + line range
4. 여러 대안이면 각각 URL
```

### 예시

**안티패턴**: `Agent: 결제 retry 이렇게 고칠까요?`

**준수**:
```
Agent: 결제 retry에 exponential backoff 도입하려 합니다.
  - 기존 ADR: https://pindoc.example.com/a/adr-042
  - 영향 코드: https://github.com/org/app/blob/a3f5e2c/src/payment/retry.ts#L10-L55
  - 대안 비교: https://pindoc.example.com/a/analysis-retry-alts
진행할까요?
```

---

## M1. Write-Intent Router

> 에이전트가 artifact 생성·수정할 때 intent 선언 + 충돌 심사 → **통과 시 auto-publish**.

### 흐름

```
pindoc.artifact.propose (intent + 본문)
  ↓
M0.5 Pre-flight Check (READY 필요)
  ↓
Conflict Check (유사도 / Area 겹침 / title 유사도)
  ↓
M2 Schema Validation
  ↓
Sensitive op 판정:
  - 일반 op → 바로 auto-publish
  - sensitive op & Project.settings.sensitive_ops="auto" → auto-publish
  - sensitive op & sensitive_ops="confirm" → M1.5 Review Queue
  ↓
auto-publish 경로: artifact.commit 내부 호출 → review_state="auto_published" → 
                   Event artifact.published 발행 → Graph 업데이트
```

외부 에이전트 입장에서는 `propose` 한 번 호출 + 통과 시 "발행됨" 응답. "Publish" 라는 별도 단계는 내부 구현 용어.

### Conflict 기준

- 유사도 0.85+ 또는 Title 0.9+ → **HARD BLOCK** (justification 제출 시 통과)
- 유사도 0.7~0.85 → **SOFT WARN** (관련 artifact 반환, "별개" 확인 시 통과)
- 0.7 미만 → 통과

### Scope (Area) 침범 (F2)

`modification`에서 diff가 declared target_area 밖 섹션 건드리면 BLOCK. 에이전트는 scope 재선언 또는 diff 축소.

### Edge cases

- **병렬 쓰기**: 후자 반려, 최신 기준 재제출 (optimistic lock)
- **`--force`**: 로그 명시, `sensitive_ops: confirm` 시 Review Queue
- **사람 직접 편집**: 경로 자체 없음 (Agent-only write)

---

## M1.5. Review Queue for Sensitive Ops

> 되돌리기 힘든 작업에 한해 사람 OK 대기열. **기본은 auto, `confirm` 모드에서만 활성.**

### Sensitive 작업

- **삭제 / archive**
- **`settled` 승격**
- **`supersede`** (기존 대체)
- **신규 Area 생성** (taxonomy 오염 방지)
- **`--force`** (conflict HARD BLOCK 뚫기)
- **대규모 supersede** (N+ 한번에)

일반 publish / modification / partial 기록은 **Review Queue 안 탐**.

### 흐름

```
에이전트 sensitive op propose
  ↓
Project.settings.sensitive_ops 판정:
  - "auto"    → 그대로 auto-publish
  - "confirm" → review_state: "pending_review"
  ↓
Event: review.required 발행
  ↓
approver role 사용자 UI Inbox 노출
  ↓
OK  → review_state: "approved" → commit (publish 확정)
NO  → review_state: "rejected" → draft archive
피드백 요청 → 에이전트에게 텍스트 전달 → 에이전트 재제출
```

### 왜 기본이 "auto"인가

- 원칙 2: 사람은 방향 제시자, 승인자가 아님
- Solo / 자율 에이전트 환경에서 confirm 은 병목
- 잘못된 artifact는 "이거 지워/고쳐" 피드백 → 에이전트 후속 propose

`confirm` 은 규모·민감 도메인(금융, 규제 산업) 옵트인.

---

## M2. Typed Documents (Tier A/B)

> 포맷을 관습이 아닌 **스키마**로 강제.

### 흐름

```
에이전트: propose (type=Debug ...)
  ↓
Pindoc: 타입별 system prompt + 스키마 주입 (MCP 응답)
  ↓
에이전트: 본문 생성
  ↓
Schema Validator: 필수 필드 검증
  ↓
통과 → M1 Router 진입
```

### Tier 처리

- Tier A: install 시 항상 활성
- Tier B: Project.active_domain_packs 등록된 것만
- Tier C: V2+

---

## M3. Git Pinning

### 흐름

```
propose 시 에이전트가 pin 선언
  ↓
Pin 엣지 생성 (hard)
  ↓
Git webhook/polling: repo 변경 감지
  ↓
변경 파일이 pin과 매칭? → artifact에 potentially_stale 플래그
  ↓
Event: artifact.stale_detected (M4)
```

### 판정 & 종류

- **Commit / Branch / PR / Path-only** pin
- V1: 경로 변경 = potentially_stale (단순, 오탐 감수)
- V2: AST/LLM 의미 변화 판정

### Outbound

Pin + repo → UI 클릭 시 `https://github.com/.../blob/COMMIT/PATH#L10-L30` 직행.

---

## M4. Propagation Ledger

Event Bus 상의 이벤트를 dependent로 전파. V1은 이벤트 발행 + 간단 리스트, V1.1 Dashboard 3-tier + bulk 액션.

```
Event (artifact.stale_detected / pin.changed / tc.failed / ...)
  ↓
Graph traversal
  ↓
각 dependent에 플래그 + UI Inbox
```

**LedgerEntry**: `id, timestamp, event_type, source_ref, affected_refs[], severity, status, resolution`.

---

## M5. TC Gating

```
Feature.status = "shipped"
REQUIRES
  ∀ tc ∈ Feature.tcs WHERE tc.required_for_close == true:
    tc.last_status == "passing"
```

V1.1 TC Runner가 `executable_by: "agent"` 자동 실행.

---

## M6. Fast Landing

> "완벽 인덱스"가 아니라 **"핵심 1~3개 리소스로의 빠른 첫 착륙"**. F6(과거 맥락 재발견) 해결.

### 흐름

```
Agent: pindoc.context.for_task("장바구니 재시도")
  ↓
Pindoc:
  - 현재 project 내 artifact 의미 검색 (pgvector)
  - Area/type/recency 기준 top-3
  - 각 artifact의 related_resources 집계
  ↓
응답: { artifacts, resources, related_areas }
  ↓
Agent: 1~2개 read → LSP/컴파일러로 주변 자동 추적
```

### F6 해결 Scope (정직하게)

- **Pindoc이 해결**: promote된 artifact 내에서의 의미 검색, 한국어↔영어 gap, Area/Type 필터, Cmd+K.
- **Pindoc이 안 함**: raw 에이전트 세션 파일 흡수·검색. 그건 해당 클라이언트(Claude 앱/Codex 앱) 책임.
- **Promote 문화로 유도**: PINDOC.md `mode: auto`가 유의미한 맥락을 promote로 끌어냄 → 검색 가능한 것이 계속 늘어남.

### Multi-project

- 기본: 현재 Project 범위
- 명시 시: `pindoc.context.for_task(..., scope: "cross_project")` 연결된 Project까지

---

## M7. Resource Freshness Re-Check (Reverse Harnessing)

> Related Resource 인덱스 자가 검증. 읽을 때마다 개선.

### 흐름

```
사용자 Artifact 읽음 (또는 N회에 1회 자동 — V1.1)
  ↓
Pindoc → 에이전트 (백그라운드 verify 요청):
  "doc_xxx.related_resources 아직 valid? rename? 누락?"
  ↓
에이전트: 파일 존재 확인, diff 체크, 주변 grep
  ↓
에이전트: report (rename/broken/new candidate)
  ↓
Pindoc → 사용자: Referenced Confirmation 형태
  "자동 업데이트 확인?" + 각 변경 링크
  ↓
사용자 OK → related_resources 갱신, verified_at 업데이트
```

### V1 vs V1.1

- V1: **명시 트리거만** (`pindoc.resource.verify` 호출 또는 "verify resources" 명령)
- V1.1: 자동 트리거 (읽기 시점 N회에 1회 백그라운드)

---

## 메커니즘 간 상호작용

```
[에이전트가 Debug propose]
  │
  ├─ M0: PINDOC.md 규율 적용
  ├─ M0.5: Pre-flight → "영향 파일 확인 필요" → 에이전트 추가 탐색 후 재제출
  ├─ M1: Intent + Conflict → 기존 Debug 발견 → "별개" 증명
  ├─ M2: Schema 검증 → root_cause 누락 → 재생성
  ├─ M3: Pin 설정
  ├─ M6: Related Resources 등록 → Fast Landing 풀 업데이트
  ├─ [sensitive 아니므로] → auto-publish
  ├─ M0.6: 발행 통지 시 Referenced Confirmation ("published at [URL]")
  ├─ [publish 후] M4: Event → 관련 Task 알림
  ├─ [1주 후] M7: verify 트리거 → ref 1개 rename 탐지 → M0.6 형태 사람 OK
  └─ [Feature close 시도] M5: TC 1개 pending → 거부
```

---

## 메커니즘이 하지 않는 것

- artifact 내용을 Pindoc이 생성하지 않음 (LLM 호출 없음)
- 자동 stale 수정 없음 (에이전트 새 세션 필요)
- 의미 판단 없음 (기계적 판단만)
- **사람 직접 편집 허용 없음**
- **매 publish 사람 승인 강제 없음** (Review Queue는 sensitive ops + confirm 모드만)
- **Raw 세션 흡수 없음** — 각 에이전트 클라이언트 책임
- Fast Landing은 완벽 인덱스 약속 없음
