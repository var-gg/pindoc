# 05. Mechanisms

Varn이 [실패 모드 F1–F5](01-problem.md)를 해결하는 핵심 메커니즘 5가지.

## 메커니즘 전체 지도

| 메커니즘 | 해결하는 실패 모드 | V1 포함 |
|---------|----------------|---------|
| M1. Write-Intent Router | F1 중복 생성, F2 경계 침범 | ✅ Flagship |
| M2. Typed Documents | F3 포맷 드리프트 | ✅ |
| M3. Git Pinning | F5 저해상도 추적 (기반) | ✅ |
| M4. Propagation Ledger | F5 저해상도 추적 (UX) | 🔄 V1.1 |
| M5. TC Gating | F4 TC 관리 공백 | 🔄 V1.1 |

---

## M1. Write-Intent Router

> **목적**: 에이전트가 artifact를 생성/수정할 때, 의도(intent)를 선언하고 기존 artifact와의 충돌을 심사받게 한다.

### 작동 흐름

```
에이전트가 artifact 쓰려 함
         │
         ▼
    propose 호출
    (intent + 본문 포함)
         │
         ▼
  ┌──────────────────┐
  │  Intent 파싱      │
  └──────────────────┘
         │
         ▼
  ┌──────────────────────────┐
  │  Conflict Check           │
  │  - 의미적 중복 검사        │
  │  - scope 충돌 검사         │
  │  - 유사도 상위 N개 산출    │
  └──────────────────────────┘
         │
         ▼
    ┌─────────────┐
    │ 충돌 있음?   │
    └──────┬──────┘
           │
    ┌──────┴──────┐
    │             │
   YES           NO
    │             │
    ▼             ▼
  심사 필요    바로 draft 승인
  - 선택지       단계로
    제시
  - justification
    필요
```

### Conflict Check 기준

에이전트가 `kind: "new"`로 propose했을 때:

1. **Vector 유사도**: 본문 임베딩 기준 상위 N개 기존 artifact와 cosine 유사도 계산
2. **Scope 겹침**: intent.scope 태그가 같은 artifact 리스트업
3. **Title 유사도**: 단순 fuzzy matching

**판정 규칙**:
- 유사도 **0.85 이상** 또는 제목 유사도 **0.9 이상** → **HARD BLOCK**. "이 문서를 업데이트하거나, 새로 만드는 이유를 제출하세요."
- 유사도 **0.7~0.85** → **SOFT WARN**. 관련 문서 리스트를 에이전트에게 반환. 에이전트가 "별개임을 확인함" 플래그와 reason을 제출하면 통과.
- 유사도 **0.7 미만** → 통과.

### Scope 침범 검사 (F2 해결)

에이전트가 `kind: "modification"`으로 기존 artifact 수정할 때:

1. 제출된 diff에서 수정된 섹션 추출
2. 원본 artifact의 scope 태그와 비교
3. diff가 declared scope 밖의 섹션을 건드리면 **BLOCK**
4. 에이전트는 두 가지 옵션:
   - scope 확장 (intent 재선언)
   - 해당 수정 제외하고 재제출

### 왜 이 메커니즘이 핵심인가

**F1 (중복 생성) 해결**: 에이전트가 "새 문서 만들려고 하면 → 기존 문서와 겹치는지 자동 검사"라는 **강제 게이트**가 생김. "알아서 찾아봐줘"를 매번 프롬프팅할 필요 없음.

**F2 (경계 침범) 해결**: Scope를 intent로 선언시키고 diff를 검증하니, "B 수정하다가 A 건드는" 일이 구조적으로 막힘.

**Notion/Linear가 못 따라오는 이유**: 이들은 write-path가 **열려있음**. MCP 플러그인 달아도 gate 설치 불가.

### Intent JSON 예시

```json
{
  "kind": "modification",
  "target_type": "Document/Flow",
  "target_id": "doc_payment_flow_v3",
  "target_scope": ["payment.retry"],
  "reason": "PG사 타임아웃 재시도 로직 추가 반영",
  "related_session": "sess_2026-04-21-abc123"
}
```

### Edge cases

- **병렬 쓰기**: 두 에이전트가 같은 artifact를 동시에 수정 시도 → 후자는 conflict로 반려, 최신 버전 기준 재제출 요구 (optimistic lock)
- **사람 직접 편집**: UI에서 직접 편집 시에도 동일한 Router 통과 (에이전트만 제약받는 게 아님)
- **긴급 스킵**: `--force` 플래그 가능하지만 로그에 명시적으로 남음. 팀 설정에서 비활성 가능

---

## M2. Typed Documents

> **목적**: 포맷이 관습이 아니라 **스키마**로 강제되게 한다.

### 작동 흐름

```
에이전트: "Debug 타입 문서 propose"
  ↓
Varn: 타입별 system prompt 주입
  ↓
에이전트: 본문 생성 (symptom, reproduction, hypotheses...)
  ↓
Varn: Schema Validator 실행
  ↓
필수 필드 누락? → BLOCK, 누락 필드 명시
  ↓
통과 → Promote UI에 도달
```

### Validator의 역할

[04 Data Model](04-data-model.md)에 정의된 각 타입의 필수 필드를 검사.

예: Debug 타입
- `symptom` 비어있음 → BLOCK: "Debug 타입은 symptom 필수"
- `hypotheses_tried` 배열 길이 0 → BLOCK: "최소 1개 가설 필요"
- `root_cause` 비어있고 `status: resolved` → BLOCK: "resolved 상태는 root_cause 필요"

### 타입별 System Prompt

에이전트가 promote할 때 Varn이 해당 타입의 스키마와 system prompt를 MCP 응답으로 함께 반환:

```
# Debug 타입 문서 생성 가이드

이 문서는 디버깅 세션의 기록입니다. 다음 구조를 반드시 따르세요:

## Symptom (필수)
관찰된 증상을 구체적으로 기술하세요. 에러 메시지, 로그, 
재현 환경을 포함하세요.

## Reproduction (필수)
증상을 재현하는 단계를 번호 매긴 리스트로 작성하세요.

## Hypotheses Tried (최소 1개)
각 가설에 대해:
- statement: 가설 내용
- tested: true/false
- result: confirmed | rejected | inconclusive
- evidence: 근거

## Root Cause (resolved 시 필수)
실제 원인

## Resolution (resolved 시 필수)
적용한 해결책
```

### 왜 이 메커니즘이 핵심인가

**F3 (포맷 드리프트) 해결**: "좋은 Debug 문서는 이렇게 생겼다"가 **스키마로 박제**됨. 에이전트가 대충 만들면 publish 실패.

**부가 효과**: 같은 타입의 artifact끼리 비교/집계 가능. "지난 달 resolved된 Debug 중 root cause가 race condition이었던 건들" 같은 쿼리 가능.

---

## M3. Git Pinning

> **목적**: 모든 artifact를 코드베이스의 특정 지점에 고정하여, 코드 변경을 감지할 수 있게 한다.

### 작동 흐름

```
Artifact 생성 시
  ↓
에이전트가 "이 문서는 이 코드 경로들과 관련"이라고 선언
  ↓
Pin 엣지 생성: artifact ← pinned_to → (repo, commit, paths)
  ↓
[이후] Git webhook 또는 polling으로 repo 변경 감지
  ↓
변경된 파일 경로가 어떤 pin과 매칭되는지 계산
  ↓
매칭된 artifact에 'potentially_stale' 플래그
  ↓
Propagation Ledger에 이벤트 기록 (M4로 이어짐)
```

### Pin의 종류

- **Commit pin**: 특정 커밋에 고정. 그 커밋 이후 해당 path가 변경되면 stale.
- **Branch pin**: 특정 브랜치의 HEAD 추적. 드물게 사용.
- **PR pin**: 특정 PR에 고정 (in-flight 작업용).
- **Path-only pin**: 커밋 정보 없이 경로만. 가장 약한 고정.

### Stale 판정 기준

단순히 "파일이 변경됨"으로 판정하면 노이즈 과다. 의미 있는 변경만 감지해야:

**V1 판정 로직** (단순 버전):
- pin된 path에 파일 변경이 있으면 일단 "potentially_stale"
- 사람이 dismiss 하면 pin 업데이트 (새 커밋으로 이동)

**V2 판정 로직** (정교한 버전):
- 변경된 함수/클래스의 시그니처 변화 감지
- 의미 변화 있을 때만 stale
- AST 기반 또는 LLM 기반 판정

V1은 단순 버전으로 시작. 오탐이 많더라도 "알 수 없는 stale"보다 "약간 많은 stale 알림"이 나음.

### Git 연동 옵션

- **옵션 A: GitHub/GitLab App** — webhook 받아서 실시간 반영. 프로덕션 권장.
- **옵션 B: 로컬 Git hooks** — post-commit hook으로 Varn에 push. 팀 환경에서 번거로움.
- **옵션 C: Polling** — 주기적으로 repo 스캔. 간단하지만 지연.

V1은 옵션 A와 C 지원. 옵션 B는 V2.

---

## M4. Propagation Ledger

> **목적**: 문서/코드 변경이 관련 artifact에 어떤 영향을 미치는지 추적하고, 사람이 catch-up할 수 있게 한다.

### 작동 흐름

```
변경 이벤트 발생:
  - Artifact 수정됨
  - Pin된 코드 변경됨
  - TC run 결과 바뀜
         │
         ▼
  ┌────────────────────────┐
  │ Propagation Ledger      │
  │ - 이벤트 기록            │
  │ - 영향받는 dependents    │
  │   산출 (graph traversal)│
  │ - 각 dependent에 플래그  │
  └────────────────────────┘
         │
         ▼
  Stale Dashboard에 노출
  - "재검토 필요" 리스트
  - 심각도 표시
  - 일괄 처리 액션
```

### Ledger Entry 구조

```
LedgerEntry {
  id: string
  timestamp: timestamp
  event_type: "artifact_modified" | "code_changed" | "tc_failed" | ...
  source_ref: ArtifactRef | PinRef
  affected_refs: ArtifactRef[]
  severity: "low" | "medium" | "high"
  status: "open" | "acknowledged" | "resolved" | "dismissed"
  resolved_by: Actor?
  resolved_at: timestamp?
  resolution: "updated" | "superseded" | "dismissed_no_change" | "archived"
}
```

### Dashboard UX (간단히)

- 3개 섹션:
  - 🔴 **High**: 직접 pin된 코드가 바뀜, TC failing 등
  - 🟡 **Medium**: 참조 artifact가 수정됨
  - 🔵 **Low**: 약한 관계의 업데이트 알림

- 각 항목에서 바로 action:
  - "최신화" → 에이전트에게 업데이트 task 던짐 (새 세션 시작)
  - "무시" → dismiss
  - "Archive" → artifact 은퇴

### 왜 V1.1인가

V1에서 Pin과 이벤트 발행까지는 구현하되, Dashboard의 풍부한 UX는 V1.1로. 이유:
- V1 사용자 수가 적을 때는 이벤트 양이 적어서 간단 리스트로 충분
- Dashboard는 데이터가 쌓여야 설계가 정교해짐

---

## M5. TC Gating

> **목적**: TC를 1급 객체로 다루고, Feature close 조건으로 강제한다.

### 핵심 제약

Feature의 status를 `shipped`로 변경하려면:

```
Feature.status = "shipped" 
REQUIRES
  ∀ tc ∈ Feature.tcs WHERE tc.required_for_close == true:
    tc.last_status == "passing"
```

조건 미충족 시 API/UI에서 거부. `--force` 없음.

### AI-가능 TC vs 사람 필수 TC

TC 생성 시 `executable_by` 필드로 분류:

- **agent**: AI가 실행 가능. Varn이 자동 실행 에이전트에 위임 (V1.1 TC Runner)
- **human_e2e**: 사람이 E2E로 수동 실행. 사용자가 웹 UI에서 run 결과 입력
- **hybrid**: 일부는 에이전트, 최종 확인은 사람

### TC Runner (V1.1)

AI-가능 TC를 일괄 실행하는 에이전트:

```
사용자: "이 Feature의 모든 AI TC 돌려줘"
  ↓
TC Runner:
  for tc in feature.tcs where executable_by == "agent":
    - 스크립트 실행 (jest/pytest/playwright)
    - 결과 캡처
    - tc.runs에 추가
    - tc.last_status 업데이트
  ↓
Feature close 가능 여부 재계산
  ↓
사람에게 결과 보고 + human_e2e TC 할당
```

### 왜 이 메커니즘이 중요한가

**F4 (TC 관리 공백) 해결**: 
- TC가 구조화된 객체가 됨 → 실행 여부, 마지막 결과, 실행 이력 전부 기계적으로 추적
- "이 feature 닫아도 돼?"에 대한 기계적 답이 나옴
- AI-가능/사람-필수 구분이 데이터에 박혀서 자동 라우팅 가능

**부가 효과**: Feature 완료 버튼 누르기 전에 TC 상태가 보이니 **사람이 "올클리어"를 수동으로 체크하지 않아도 됨**.

---

## 메커니즘 간 상호작용

```
[에이전트가 Debug 세션 promote]
  │
  ├─ M1: Intent 선언, 중복 검사 → 기존 Debug 2개 발견
  │                           → 에이전트가 "별개 이슈" 증명
  │
  ├─ M2: Debug 타입 스키마 검증 → root_cause 누락 → 에이전트 재생성
  │
  ├─ M3: pin 설정 → src/payment/retry.ts @ commit-abc
  │
  ├─ [publish 후] M4: Propagation Ledger에 이벤트 기록
  │                   → 관련 Task 3개에 "참조 문서 업데이트됨" 알림
  │
  └─ [연결된 Feature close 시도] M5: TC 3개 중 1개 pending
                                   → close 거부
```

다섯 메커니즘이 **루프를 이루며** 작동합니다. 이게 Varn의 통합 가치입니다.

---

## 메커니즘이 하지 않는 것

**오해 방지 차원에서 명시**:

- Varn은 **자체적으로 artifact 내용을 생성하지 않는다**. LLM 호출은 에이전트 클라이언트의 책임. Varn은 gate와 validator.
- Varn은 **자동으로 stale을 수정하지 않는다**. 사람이 새 세션을 돌려서 업데이트해야 함.
- Varn은 **의미 판단을 하지 않는다**. 유사도 스코어, diff 범위, 스키마 필드 유무 같은 기계적 판단만. 궁극적 판단은 사람.

이 경계를 지키는 것이 Varn이 "자율 에이전트 플랫폼"이 아니라 **"사람-에이전트 협업 워크벤치"** 인 이유입니다.
