# 05. Mechanisms

Varn이 [실패 모드 F1–F6](01-problem.md)를 해결하는 핵심 메커니즘들.

## 메커니즘 전체 지도

| 메커니즘 | 해결 대상 | V1 포함 |
|---------|--------|---------|
| **M0. Harness Reversal** | 모든 메커니즘의 전제 | ✅ Flagship |
| **M0.5. Tool-driven Pre-flight Check** | F1/F2 (에이전트가 더 탐색) | ✅ |
| **M0.6. Referenced Confirmation** | UX 전반 (링크 동반 확인) | ✅ |
| M1. Write-Intent Router | F1 중복 생성, F2 경계 침범 | ✅ Flagship |
| M2. Typed Documents (Tier A/B) | F3 포맷 드리프트 | ✅ Flagship |
| M3. Git Pinning | F5 기반 | ✅ (단순 판정) |
| M4. Propagation Ledger | F5 UX | 🔄 V1.1 |
| M5. TC Gating | F4 TC 관리 | 🔄 V1.1 |
| **M6. Fast Landing (Resource Indexing)** | F6 세션 검색 지옥 + 탐색 비용 | ✅ Flagship |
| **M7. Resource Freshness Re-Check** | 인덱스 정합성 유지 | ✅ (경량) |

---

## M0. Harness Reversal

> **목적**: MCP 연결 순간 Varn이 에이전트의 base 행동 규약을 주입. 이후 모든 에이전트 동작이 Varn 규율을 따름.

### 작동 흐름

```
(install)
$ varn install
  ↓
VARN.md 생성 (프로젝트 루트)
  ↓
CLAUDE.md / AGENTS.md / .cursorrules 에
"See ./VARN.md for this project's agent protocol." 삽입

(이후 매 세션)
에이전트 세션 시작
  ↓
CLAUDE.md(등) 로드 → VARN.md 참조 발견 → VARN.md 로드
  ↓
규약 반영:
  - Checkpoint 휴리스틱 (mode: auto/manual/off)
  - Propose → Pre-flight → Commit 순서 준수
  - Referenced Confirmation 프로토콜 적용
  - Area 규율
  - URL 받으면 varn.wiki.read로 fetch
```

### VARN.md 초안 구조

```markdown
# VARN.md — Agent Protocol for this Project

## 1. Checkpoint 판단
Mode: auto (project-level setting; can be overridden by user per session)

에이전트가 다음 상황에서 사용자에게 "정리할까요?"를 제안:
- 사용자 명시 요청 ("정리해줘", "위키에", "체크포인트")
- 한 주제에 약 30턴 이상 지속 + 결론 도달 신호 감지
- 디버깅에서 resolution 도달
- 새 파일·모듈·스키마를 만들어냄
- ADR 유발 키워드 ("우리 이걸로 가자", "결정")

같은 세션에서 3회 거절 시 자율 제안 세션 동안 off.

## 2. Write 프로토콜
모든 artifact 생성·수정은 다음 순서:
  1. varn.artifact.search 로 관련 선행 artifact 확인
  2. varn.artifact.propose (intent + 초안) 호출
  3. 응답이 NOT_READY면 checklist 대응 후 재제출
  4. READY 받으면 varn.artifact.commit (사람 승인 후)

## 3. Referenced Confirmation
사용자에게 확인 요청할 때 반드시 다음을 동반:
  - 1줄 요약
  - 관련 artifact URL(들)
  - 코드 경로가 있으면 repo URL + line range
단편 설명만으로 확인 요청하지 않는다.

## 4. Area 규율
새 artifact는 반드시 기존 Area 중 하나에 배치.
해당하는 Area가 없으면 varn.area.propose로 신청 (Write-Intent Router 통과).
/Misc는 최후수단.

## 5. URL 처리
사용자가 varn:// 또는 이 인스턴스 URL을 주면:
  varn.wiki.read(url) → ContinuationContext 수령 → 본문+이웃+resources 기반으로 대화 재개.
```

### 왜 M0이 최상위인가

나머지 메커니즘(M1~M7)은 **"에이전트가 Varn 규약을 따른다"는 전제**에서만 작동. 하네싱이 없으면 에이전트는 Varn MCP tool을 "그냥 또 하나의 도구"로만 취급하고 규율이 성립 안 함.

---

## M0.5. Tool-driven Pre-flight Check

> **목적**: Tool 응답이 에이전트에게 작업을 **역지시**. MCP가 단순 응답 서버가 아니라 **에이전트 regulator**가 됨.

### 흐름

```
Agent: varn.artifact.propose(type=ADR, ...)
  ↓
Varn: {
  status: "NOT_READY",
  checklist: [
    "✗ alternatives 최소 2개 탐색했는가? (현재 0개)",
    "✗ 관련 ADR을 varn.artifact.search로 확인?",
    "✓ scope 선언 OK (/Payment)",
    "✗ 영향 파일 경로 확인? (pins/related_resources 비어있음)"
  ],
  suggested_next_tools: [
    "varn.artifact.search(scope=Payment, type=ADR)",
    "varn.context.for_task('결제 재시도')"
  ]
}
  ↓
Agent: (누락분 수행: 대안 탐색, 관련 ADR 확인, 파일 경로 확보)
  ↓
Agent: 재제출
  ↓
Varn: { status: "READY", draft_id: "draft_xxx" }
```

### 타입별 체크리스트 예시

| 타입 | 체크 항목 |
|------|---------|
| ADR | alternatives ≥ 2, 선행 ADR search, file paths (pins/resources) |
| Debug | hypotheses ≥ 1 with evidence, reproduction 확인, symptom 구체 |
| Feature | scope 명확, acceptance_criteria ≥ 1, dependencies 식별 |
| Flow | Mermaid 1개 이상, actors ≥ 1 |
| API Endpoint | method/path, request/response schema (선택이지만 권장) |

체크리스트는 Tier A/B 각 타입별로 VARN.md와 서버 양쪽에 정의.

### 왜 신선한가

기존 MCP는 **요청 → 응답 단방향**. Varn은 **요청 → "더 일하고 와" → 재요청**의 대화형. [00 Vision 원칙 3 "문서와 Tool은 active agent"](00-vision.md)의 가장 구체적 구현.

---

## M0.6. Referenced Confirmation

> **목적**: 에이전트가 사용자에게 확인·승인 요청할 때 **항상 링크 동반**. 단편 설명 없이 맥락 전체로 판단 가능하게.

### 프로토콜 (VARN.md 항목 3)

에이전트가 사용자에게 확인 요청할 때 반드시:

1. **1줄 요약**
2. **관련 artifact URL(들)** (varn:// 또는 https://varn.<host>/a/<slug>)
3. **코드 경로가 있으면** repo URL + line range (예: `https://github.com/org/repo/blob/COMMIT/path/file.ts#L10-L30`)
4. **여러 대안이면** 각각 URL, 3개 이상이면 artifact로 묶어 단일 URL

### 안티패턴 vs 준수 예시

**안티패턴** (기존 에이전트 UX):
```
Agent: 결제 retry 로직을 이렇게 고칠까요?
```

**Varn 준수**:
```
Agent: 결제 retry에 exponential backoff 도입하려고 합니다.
  - 기존 ADR: https://varn.example.com/a/adr-042-retry-policy
  - 영향 코드: https://github.com/org/app/blob/a3f5e2c/src/payment/retry.ts#L10-L55
  - 대안 비교: https://varn.example.com/a/analysis-retry-alternatives
승인하시겠습니까?
```

### 왜 필요한가

기존 에이전트-사용자 확인 UX의 문제:
- "이거 이렇게 할까요?" (1줄, 맥락 없음)
- 사용자는 "이거"가 뭔지 모름 → 되묻기 또는 눈감고 OK
- 서사는 별도 문서/코드에 있는데 접근 경로 없음

Varn 프로토콜 적용 후: 사용자는 항상 **"링크 위에서의 판단"** 가능. Varn의 존재 자체가 이 프로토콜을 성립시킴 (링크할 대상이 이미 구조화되어 존재).

---

## M1. Write-Intent Router

> **목적**: 에이전트가 artifact 생성·수정할 때 intent 선언 + 기존 artifact와 충돌 심사.

### 작동 흐름

```
propose 호출 (intent + 본문)
  ↓
Intent 파싱
  ↓
[M0.5 Pre-flight Check가 우선 실행]
  ↓
Conflict Check:
  - 의미적 중복 (vector 유사도)
  - Scope/Area 겹침
  - Title 유사도
  ↓
충돌 있음? ──YES──▶ 심사 필요 (선택지 제시 + 에이전트 justification)
         └──NO───▶ Schema Validator로 진행
```

### Conflict Check 기준

- 유사도 **0.85+** 또는 Title **0.9+** → **HARD BLOCK**: "이 문서를 업데이트하거나 새로 만드는 이유를 제출하세요"
- 유사도 **0.7~0.85** → **SOFT WARN**: 관련 문서 리스트 반환. 에이전트가 "별개임을 확인함" + reason 제출 시 통과
- 유사도 **0.7 미만** → 통과

### Scope 침범 검사 (F2)

에이전트가 `kind: "modification"`으로 수정할 때:
- diff에서 수정된 섹션 추출
- 원본 artifact의 `area`와 비교
- declared area 밖 섹션 건드리면 **BLOCK**
- 옵션: scope 확장 재선언 또는 해당 수정 제외 재제출

### Intent 예시

```json
{
  "kind": "modification",
  "target_type": "Document/Flow",
  "target_id": "doc_payment_flow_v3",
  "target_scope": ["Payment"],
  "reason": "PG사 타임아웃 재시도 로직 반영",
  "related_session": "sess_2026-04-21-abc123"
}
```

### Edge cases

- **병렬 쓰기**: 두 에이전트가 같은 artifact 동시 수정 시도 → 후자 반려, 최신 버전 기준 재제출 (optimistic lock)
- **긴급 스킵**: `--force` 플래그 가능, 로그 명시 기록. 팀 설정에서 비활성 가능.
- ~~사람 직접 편집 시에도 Router 통과~~ — **해당 없음. Agent-only write이므로 사람 직접 편집 경로 자체가 존재하지 않음.**

---

## M2. Typed Documents (Tier A/B)

> **목적**: 포맷을 관습이 아닌 **스키마**로 강제.

### 작동 흐름

```
에이전트: propose (type=Debug ...)
  ↓
Varn: 타입별 system prompt + 스키마 주입 (MCP 응답)
  ↓
에이전트: 본문 생성 (symptom, reproduction, hypotheses...)
  ↓
Schema Validator:
  - 필수 필드 누락? → BLOCK (누락 필드 명시)
  - resolved 상태인데 root_cause 비어있음? → BLOCK
  ↓
통과 → Promote UI (사람 승인)
```

### Tier 처리

- Tier A core types: install 시 스키마 항상 활성
- Tier B domain pack: 프로젝트가 활성화한 pack만 스키마 적용
- Tier C (V2+): 커스텀 YAML 스키마

[04 Data Model](04-data-model.md) 참조.

---

## M3. Git Pinning

> **목적**: Artifact를 코드베이스의 특정 지점에 고정, 코드 변경 감지.

### 흐름

```
propose 시 에이전트가 "이 문서는 이 경로들과 관련" 선언
  ↓
Pin 엣지 생성 (hard)
  ↓
[이후] Git webhook/polling으로 repo 변경 감지
  ↓
변경 파일 경로가 어떤 pin과 매칭되는지 계산
  ↓
매칭 artifact에 potentially_stale 플래그
  ↓
Propagation Ledger 이벤트 기록 (M4)
```

### Pin 종류 & 판정

- **Commit pin / Branch pin / PR pin / Path-only pin**
- V1 단순 판정: 경로 변경 = potentially_stale
- V2 정교한 판정: AST/LLM 기반 의미 변화 감지
- V1은 오탐 감수 — "알 수 없는 stale"보다 "약간 많은 알림"이 나음

### Git 연동 옵션

- GitHub/GitLab App (webhook) — 프로덕션 권장
- Polling — 간단, 지연

### Outbound (NEW)

Pin + repo 정보 → UI 클릭 시 GitHub URL `https://github.com/.../blob/COMMIT/PATH#L10-L30`로 직행.

---

## M4. Propagation Ledger

> **목적**: 변경 영향 추적 + 사람이 catch-up 가능하게.

```
변경 이벤트 (artifact 수정 / pin된 코드 변경 / TC 실패)
  ↓
Propagation Ledger:
  - 이벤트 기록
  - 영향받는 dependents 산출
  - stale 플래그 전파
  ↓
Stale Dashboard 노출
```

**Ledger Entry**:
```
LedgerEntry {
  id, timestamp, event_type,
  source_ref, affected_refs[], severity,
  status: "open" | "acknowledged" | "resolved" | "dismissed",
  resolution: "updated" | "superseded" | "dismissed_no_change" | "archived"
}
```

V1: 이벤트 발행 + 간단 리스트.
V1.1: Dashboard 3-tier (High/Medium/Low) + bulk 액션.

---

## M5. TC Gating

> **목적**: TC를 1급 객체로, Feature close 조건으로 강제.

### 핵심 제약

```
Feature.status = "shipped"
REQUIRES
  ∀ tc ∈ Feature.tcs WHERE tc.required_for_close == true:
    tc.last_status == "passing"
```

API/UI에서 강제. `--force` 없음.

### AI-가능 TC vs 사람 TC

- `executable_by: "agent"` — AI 자동 실행 (V1.1 TC Runner)
- `executable_by: "human_e2e"` — 사람이 UI에서 결과 입력
- `executable_by: "hybrid"` — 부분 자동 + 최종 사람 확인

### TC Runner (V1.1)

```
사용자: "이 Feature의 모든 AI TC 돌려"
  ↓
Runner:
  for tc in feature.tcs where executable_by == "agent":
    - 스크립트 실행 (jest/pytest/playwright)
    - 결과 캡처, tc.runs에 추가
    - tc.last_status 업데이트
  ↓
Feature close 가능 여부 재계산
  ↓
사람 보고 + human_e2e TC 할당
```

---

## M6. Fast Landing (Resource Indexing)

> **목적**: "완벽한 인덱스"가 아니라 **"핵심 리소스 1~3개로의 빠른 첫 착륙"**. 에이전트 탐색 비용 감소 + F6 해결.

### 흐름

```
Agent: varn.context.for_task("장바구니 재시도 로직")
  ↓
Varn 처리:
  - 세션 의미 검색 (F6)
  - Area/scope 일치하는 artifact
  - 각 artifact의 related_resources 집계
  ↓
응답:
  {
    artifacts: [top-3 by area/recency/relevance],
    resources: [
      {type: "code", ref: "src/cart/retry.ts", purpose: "핵심 로직"},
      {type: "code", ref: "src/api/cart.ts", purpose: "API wrapper"},
      {type: "doc", ref: "https://pg.example/docs", purpose: "외부 스펙"}
    ],
    sessions: [related sessions (F6)]
  }
  ↓
Agent: 1~2개 Read → 컴파일러/LSP로 주변 자동 추적
```

### 포지셔닝 (정직하게)

- **Fidelity 100%는 보장되지 않음** — related_resources는 에이전트가 발행 시 등록하는 게 유일한 source
- **가치는 "첫 진입점 제공"** — 나머지는 에이전트 + 언어 도구(LSP, 컴파일러, grep)의 몫
- **쓰면 쓸수록 좋아짐** (compound value via M7)

### 왜 V1 flagship인가

- F6 (세션 검색 지옥) = Solo 사용자 entry pain 해결
- 에이전트 탐색 비용 감소 = 측정 가능한 지표 ("agent lands on right code in one shot")
- Varn 고유 해자 — Mem0/Cursor Rules 등과 구조적으로 다른 가치

---

## M7. Resource Freshness Re-Check (Reverse Harnessing)

> **목적**: Related Resource 인덱스의 정합성을 주기적 자가 검증. 읽을 때마다 개선.

### 흐름

```
사용자가 Artifact 읽음 (또는 N회에 1회 자동)
  ↓
Varn → 에이전트 (백그라운드 verify 요청):
  "doc_xxx.related_resources 검증:
    - ref 1: src/cart/retry.ts (2주 전 등록)
    - ref 2: src/api/cart.ts (2주 전 등록)
   아직 valid? path 변경? 누락된 핵심?"
  ↓
에이전트: (파일 존재 확인, diff 체크, 주변 grep)
  ↓
에이전트 → Varn:
  - ref 1: valid
  - ref 2: renamed to src/api/cart/index.ts
  - suggestion: new candidate src/hooks/useCart.ts
  ↓
Varn → 사용자: "자동 업데이트 확인?" (Referenced Confirmation 준수)
  ↓
사용자 OK → related_resources 갱신, verified_at 업데이트
```

### 특징

- **Reverse Harnessing 활용**: Varn이 에이전트에게 일을 역지시 (M0.5와 동류)
- **사람 개입 최소**: OK/NO만 (Referenced Confirmation 준수, 링크 동반)
- **쓰면 쓸수록 인덱스 정확도 상승**: M6의 compound value 핵심
- **고빈도 artifact 우선**: 자주 읽히는 것부터 검증

### V1 vs V1.1

- **V1**: 명시적 트리거만 (사용자 `varn.resource.verify` 또는 "verify resources" 명령 시)
- **V1.1**: 자동 트리거 (읽기 시점 N회에 1회, 백그라운드)

---

## 메커니즘 간 상호작용

```
[에이전트가 Debug 세션 promote 요청]
  │
  ├─ M0: VARN.md 규율 적용 (Checkpoint 판정, 프로토콜 준수)
  │
  ├─ M0.5 Pre-flight Check → "영향 파일 경로 확인 필요" → 에이전트 Grep 후 재제출
  │
  ├─ M1: Intent 선언, Conflict Check → 기존 Debug 2개 발견
  │                                  → 에이전트 "별개 이슈" 증명
  │
  ├─ M2: Debug 타입 스키마 검증 → root_cause 누락 → 재생성
  │
  ├─ M3: Pin 설정 → src/payment/retry.ts @ commit-abc
  │
  ├─ M6: Related Resources 3개 등록 → Fast Landing 풀 업데이트
  │
  ├─ M0.6: 사람 승인 요청 → artifact URL + repo line range 동반
  │
  ├─ [publish 후] M4: Propagation Ledger 이벤트 → 관련 Task 3개에 알림
  │
  ├─ [1주 후] M7: 읽기 시점 verify 트리거 → ref 1개 rename 탐지 → 사람 OK
  │
  └─ [연결 Feature close 시도] M5: TC 3개 중 1개 pending → close 거부
```

이 루프가 Varn의 통합 가치입니다.

---

## 메커니즘이 하지 않는 것

- Varn은 **자체적으로 artifact 내용을 생성하지 않는다**. LLM 호출은 에이전트 클라이언트 책임.
- Varn은 **자동으로 stale을 수정하지 않는다**. 사람이 새 세션을 돌려 에이전트가 업데이트해야 함.
- Varn은 **의미 판단을 하지 않는다**. 유사도 스코어, diff 범위, 스키마 필드 유무 같은 기계적 판단만.
- Varn은 **사람의 직접 편집을 허용하지 않는다**. 수정은 반드시 에이전트 경유 — 사람은 승인/거절/피드백만.
- Varn의 Fast Landing은 **완벽한 인덱스를 약속하지 않는다**. "첫 착륙지점"만 제공, 이후는 에이전트·컴파일러·LSP의 일.

이 경계를 지키는 것이 Varn이 "자율 에이전트 플랫폼"이 아니라 **"사람-에이전트 협업 워크벤치"** 인 이유입니다.
