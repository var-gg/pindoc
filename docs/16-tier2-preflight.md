# Tier 2 Preflight — 다음 세션용 배치 발행 계획

Phase 17 + follow-up 이후 첫 MCP spawn이 real embedder + Unicode slug +
`include_templates` + `embedder_used` + pin-path validation을 전부 갖춘 상태로
뜬 순간부터 바로 집행할 수 있도록, 미리 확정해 둔 Tier 2 + Decision 배치
계획.

---

## 재개 직전 체크리스트

1. **바이너리 swap** (pindoc-server):
   ```bash
   mv bin/pindoc-server.exe bin/pindoc-server.exe~
   mv bin/pindoc-server.new.exe bin/pindoc-server.exe
   ```
2. **pindoc-api 기동** (`PINDOC_REPO_ROOT` 필수, pin path validation 활성):
   ```bash
   PINDOC_REPO_ROOT="$PWD" ./bin/pindoc-api.exe &
   ```
3. **MCP ping 후 검증**:
   - `pindoc.project.current` → capabilities 확인
   - `pindoc.artifact.search q="agent"` → 응답에 `embedder_used: {name: "embeddinggemma", ...}` 떠야 함. 안 뜨면 구 바이너리 fallback.

---

## Part A — Decision artifacts 4건 (Phase 17 follow-up)

이전 세션에서 커밋만 하고 artifact 발행은 미뤘던 네 건. 구 MCP의 stub
embedder + ASCII-only slugify 문제 때문이었음. 새 MCP에서 **explicit slug**
없이 발행해도 Unicode slug로 저장됨. 그래도 일관성을 위해 아래 slug 고정
권장.

| # | Title | Type | Area | Slug (권장 명시) | relates_to |
|---|---|---|---|---|---|
| D1 | Slug 정책: Unicode 보존 | Decision | governance/taxonomy-policy | `decision-slug-unicode-preservation` | `pindoc-agent-written-8` (implements) |
| D2 | `include_templates` 파라미터 통일 | Decision | system/mcp | `decision-include-templates-unified` | `pindoc-mcp-tools-v1-implementation-status-spec-runtime-drift` (implements) |
| D3 | `embedder_used` 공통 응답 필드 | Decision | system/mcp | `decision-embedder-used-response-field` | D2 (relates_to), `pindoc-url` (references) |
| D4 | Pin path validation (warn-level) | Decision | system/data | `decision-pin-path-warn` | `pindoc-3-tier-a-b-types-pin-vs-resourceref` (implements) |

**공통 pins**: `internal/pindoc/mcp/tools/artifact_propose.go` (구현 중심 파일).
**공통 basis**: `context.for_task(<decision 제목>)` 응답의 search_receipt.

**본문 구성 템플릿** (Decision/ADR 양식 따름):
- Context: 왜 결정이 필요했나 (Phase 17 follow-up 맥락)
- Options considered: (보통 2-3개 대안)
- Decision: 채택한 것
- Consequences: 후속 영향 / 유지보수 비용
- Rollback: 어렵기 vs 쉽기 분류

각 Decision 발행 시 관찰:
- `warnings[]`에 `RECOMMEND_READ_BEFORE_CREATE` 떠야 정상 (Decision 주제가
  Tier 1 artifact와 의미적으로 가까우므로 band 0.22 이내 예상)
- `embedder_used.name == "embeddinggemma"` 확인
- `pins_stored == 1` (artifact_propose.go 참조)

---

## Part B — Tier 2 본편 8개 (완료 · 2026-04-22)

> 실행 완료. MCP 경로는 구 바이너리(stub embedder + ASCII slugify) 상태였
> 으나 **explicit `slug` 지정**으로 한글 보존, 발행 후 `pindoc-reembed`로
> gemma 벡터 in-place 교체. 17/17 (4 template + 5 Tier 1 + 8 Tier 2)
> reembed ok, stub embedding 잔여 없음.

**실제 발행 결과**:

| # | Title | Slug | Edges | Pins |
|---|---|---|---|---|
| T2-1 | 실패 모드 F1-F6 | `pindoc-문제-f1-f6-실패-모드` | 2 | 2 |
| T2-2 | Non-goals 헌법 | `pindoc-non-goals-헌법` | 2 | 2 |
| T2-3 | 5 Primitive | `pindoc-5-primitive-개념` | 3 | 2 |
| T2-4 | PINDOC.md 스펙 | `pindoc-md-harness-spec` | 2 | 2 |
| T2-5 | V1 Roadmap + BM | `pindoc-v1-로드맵-bm-phase` | 3 | 2 |
| T2-6 | M1 Phase 1-17 | `pindoc-m1-phase-chain-1-17` | 3 | 2 |
| T2-7 | UI Wiki Reader + Cmd+K | `pindoc-ui-wiki-reader-cmdk-flow` | 3 | 4 |
| T2-8 | Design System v0 | `pindoc-design-system-v0-handoff` | 2 | 3 |

관찰:
- 모든 한글 slug explicit 지정 — 8/8 accepted, collision 0건
- 구 MCP 응답에 `embedder_used` 필드 없음 확인 (예상대로)
- `warnings[]` 전부 빈 배열 — stub embedder라 `RECOMMEND_READ_BEFORE_CREATE` 판정 무의미
- 본 HARD/SOFT 판정은 **reembed 후 재측정** (Part C)

---

### 원래 계획 (historical)

### 발행 순서 (dependency 기반)

1. **01-problem.md** → Analysis / strategy/vision (Vision의 "왜 지금인가" 확장)
2. **08-non-goals.md** → Decision / strategy/vision (명시적 반-목표 선언)
3. **02-concepts.md** → Analysis / strategy/vision (primitive 5개 — Project/Harness/Promote/Artifact/Graph)
4. **09-pindoc-md-spec.md** → Analysis / system/mechanisms (Harness Reversal M0의 포맷 spec)
5. **07-roadmap.md** → Analysis / strategy/roadmap (V1/V1.x/V2, BM)
6. **12-m1-implementation-plan.md** → Analysis / strategy/roadmap (Phase별 진행)
7. **06-ui-flows.md** → Analysis / experience/ui (Flow 0-N, Cmd+K, Sidecar)
8. **11-design-system-handoff.md** → Analysis / experience/ui (디자인 토큰, 영감원)

2번(non-goals)만 Type=Decision. 나머지는 Analysis. Glossary는 Tier 3으로 밀
림 — 12개 이상 용어 정의라 별건 batch가 낫다.

### 각 artifact 메타 (propose 입력 스케치)

#### T2-1 · 01-problem.md → "실패 모드 F1–F6과 공백의 맥락"
- `area_slug: strategy/vision`
- `type: Analysis`
- `pins: [docs/01-problem.md, docs/00-vision.md]`
- `relates_to: [pindoc-agent-written-8 (references)]`
- body: F1 "세션 증발" / F2 "area 경계" / F3 "format drift" / F4 "TC 공백" /
  F5 "stale" / F6 "과거 맥락 재발견" 6개 실패 모드 + 각각 어느 M-메커니즘과
  매핑되는지 + Vision 원칙 대응

#### T2-2 · 08-non-goals.md → "Pindoc이 되지 않는 것들 (Never list)"
- `area_slug: strategy/vision`
- `type: Decision` (anti-charter)
- `pins: [docs/08-non-goals.md]`
- `relates_to: [pindoc-agent-written-8 (references)]`
- body: "Human-writable surface 허용", "매 artifact 승인 강제", "Jira 대체",
  "raw 세션 흡수", "메신저/CRM 확장" 등 명시적 Non-goals

#### T2-3 · 02-concepts.md → "5 Primitive 정의와 경계"
- `area_slug: strategy/vision`
- `type: Analysis`
- `pins: [docs/02-concepts.md, docs/00-vision.md]`
- `relates_to: [pindoc-agent-written-8 (references), pindoc-3-tier-a-b-types-pin-vs-resourceref (implements)]`
- body: Project / Harness / Promote / Artifact / Graph 다섯 primitive 각각의
  정의·예시·다른 primitive와의 관계. Session/Checkpoint가 보조로 밀려난 이유

#### T2-4 · 09-pindoc-md-spec.md → "PINDOC.md Harness 포맷 스펙"
- `area_slug: system/mechanisms`
- `type: Analysis`
- `pins: [docs/09-pindoc-md-spec.md, internal/pindoc/harness (존재 시)]`
- `relates_to: [pindoc-m0-m7-harness-reversal-6 (implements)]`
- body: Frontmatter 필수 필드, `mode: auto|manual|off`, checkpoint 휴리스틱,
  PINDOC.md version vs server version compatibility

#### T2-5 · 07-roadmap.md → "V1 / V1.x / V2 로드맵과 BM Phase"
- `area_slug: strategy/roadmap`
- `type: Analysis`
- `pins: [docs/07-roadmap.md]`
- `relates_to: [pindoc-agent-written-8 (references), pindoc-url (references)]`
- body: M1–M7 milestones, Launch criteria, EthicalAds + Sponsors BM Phase 1,
  Hosted SaaS V2+

#### T2-6 · 12-m1-implementation-plan.md → "M1 Phase 체인과 현재 상태"
- `area_slug: strategy/roadmap`
- `type: Analysis`
- `pins: [docs/12-m1-implementation-plan.md]`
- `relates_to: [T2-5 (references), pindoc-url (references)]`
- body: Phase 1-17 + follow-up까지 체인 요약. Phase별 "왜 그 순서였나"

#### T2-7 · 06-ui-flows.md → "Wiki Reader UX Flow 0-N"
- `area_slug: experience/ui`
- `type: Analysis`
- `pins: [docs/06-ui-flows.md, web/src/reader/ReaderShell.tsx, web/src/reader/CmdK.tsx]`
- `relates_to: [pindoc-agent-written-8 (references)]`
- body: Onboarding, Search Cmd+K, Sidecar, Review Queue, Graph (stub)

#### T2-8 · 11-design-system-handoff.md → "디자인 토큰과 영감원"
- `area_slug: experience/ui`
- `type: Analysis`
- `pins: [docs/11-design-system-handoff.md, web/src/styles/reader.css, web/public/design-system]`
- `relates_to: [T2-7 (references)]`
- body: Linear + Obsidian + GitHub PR + Cmd+K 영감, 컬러/타이포/spacing 토큰

### 배치 실행 형태

Decision 4건 → Tier 2 8건 순차. 각 발행 사이에 `warnings` 관찰 + pin path
validation 결과(`PIN_PATH_NOT_FOUND` 없는지) 체크.

발행 간격은 **5-10초**(agent 본문 작성 시간). 전체 12 발행이면 2-5분 예상.

---

## Part C — Pairwise distance 2차 실측 (Tier 2 발행 후 · 완료 2026-04-22)

Tier 1 + Tier 2 = 13 artifact, C(13,2) = 78 쌍. gemma real embedder 기준.

**SOFT band (<0.25) 안 비율**:
- Tier 1만: 6/10 (60%)
- Tier 1 + Tier 2 (13): 30/78 (38%) — 희석되긴 했지만 여전히 상당 비율

**HARD band (<0.18) 진입 4 쌍 (Tier 1 때는 0건)**:

| a | b | min body dist | 의미 |
|---|---|---|---|
| `pindoc-m0-m7-harness-reversal-6` | `pindoc-md-harness-spec` | **0.1705** | M0 메커니즘 ↔ 그 구현 포맷 spec |
| `pindoc-m1-phase-chain-1-17` | `pindoc-v1-로드맵-bm-phase` | 0.1721 | Phase 체인 ↔ 상위 Roadmap |
| `pindoc-5-primitive-개념` | `pindoc-md-harness-spec` | 0.1742 | Concepts(Harness primitive) ↔ 실 스펙 |
| `pindoc-5-primitive-개념` | `pindoc-m0-m7-harness-reversal-6` | 0.1913 | Concepts(Harness) ↔ M0 메커니즘 |

**판정**: 4 쌍 전부 의미적으로 **정당한 near-match** — 중복 아님. 한 제품의 서로 다른 깊이에서 같은 주제를 논함(개념 → 메커니즘 → 스펙). 즉 **현 HARD 0.18 threshold가 한 제품 corpus에 너무 빡빡**.

**Threshold 재조정 제안 (V1.x calibration)**:
- `semanticConflictThreshold` (HARD BLOCK): **0.18 → 0.12-0.15**. 진짜 duplicate(거의 동일 내용)만 차단.
- `semanticAdvisoryThreshold` (RECOMMEND_READ_BEFORE_CREATE): **0.25 → 0.30**. Near-match advisory 범위 확대.
- 또는: threshold를 title + 첫 문단 한정으로 signature 비교 (body 전체가 아닌)
- 또는: 같은 Area / 같은 Type 내에서만 HARD 발동하고 cross-Area는 SOFT only

Threshold 확정은 Tier 3+ 추가 발행 후 재검토. 현 상수는 `internal/pindoc/mcp/tools/artifact_propose.go`의 `semanticConflictThreshold` / `semanticAdvisoryThreshold`.

**변경 반영 2026-04-22 커밋 `b502411`** — `semanticConflictThreshold` 0.18 → 0.13, `semanticAdvisoryThreshold` 0.25 → 0.30 적용. 근거 Decision artifact [`decision-conflict-threshold-recalibration-tier2`](/p/pindoc/wiki/decision-conflict-threshold-recalibration-tier2) 발행 완료.

---

## Part E — 발행 후 재측정 (19 artifact, NEW threshold)

Step 1 D1-D4 + Step 2b threshold Decision + Step 3a Apache 2.0 Decision = 6 신규 → 총 19 non-template artifact. C(19,2) = 171 쌍.

**Band 분포** (NEW threshold 기준):

| Band | 쌍 수 | 비율 | 의미 |
|---|---|---|---|
| HARD (<0.13) | **0** | 0% | Conflict block 완전 제거 — 과잉 보호 해소 확인 |
| 구 HARD (0.13–0.18) | 4 | 2% | 이전 block 대상 → advisory로 강등. 의미 보존 |
| 구 advisory (0.18–0.25) | 22 | 13% | 기존 band 유지 |
| 신규 advisory (0.25–0.30) | 31 | 18% | 확대된 하위 band |
| **Advisory 총** | **57** | **33%** | — |

**신규 Decision 6건의 advisory band 진입**:

- `decision-pin-path-warn` ↔ `pindoc-3-tier-a-b-types-pin-vs-resourceref`: 0.2828 (의미: implements — pin 구현 vs pin 정의, 정당한 관계)
- `decision-pin-path-warn` ↔ `pindoc-m0-m7-harness-reversal-6`: 0.2905 (pin 관련 M3 언급으로 인한 간접 유사)
- 그 외 4건(D1 slug, D2 templates, D3 embedder_used, threshold Decision, Apache Decision)은 전부 **advisory band 바깥(>0.30)** — Decision artifact가 기존 Analysis corpus와 의미적으로 충분히 differentiated.

**관측 결론**:
1. **NEW HARD 0.13은 conservative**: 현 corpus에서 block 쌍 0. V1 기간 중 duplicate가 실제 발생하면 관측되는지 지켜볼 포인트.
2. **Advisory 33%는 적정 밀도**: agent가 새 propose 시 대부분 relates_to 선택을 고민하게 됨. 과하면 noise, 부족하면 놓침 — 현 비율은 신호 밀도로 합리적.
3. **Decision이 Analysis와 구분됨**: D1-D3 / threshold / Apache Decision은 advisory band 밖. Decision artifact가 analysis와는 다른 의미 공간을 점유 — 타입별 semantic niche 존재.

**HARD 재진입 후보**: Tier 3에서 decisions.md row → Decision artifact 분해 시, 기존 Decision과의 distance가 HARD(<0.13)에 진입할 가능성. 본 Decision row 표기 같은 Rationale 서술이 중복되면 발생. Tier 3 진입 시 재측정 필요.

### SQL 재현

```sql
SELECT a1.slug AS a, a2.slug AS b,
       round(MIN(c1.embedding <=> c2.embedding)::numeric, 4) AS min_body_dist
FROM artifacts a1 JOIN artifacts a2 ON a1.slug < a2.slug
JOIN artifact_chunks c1 ON c1.artifact_id = a1.id AND c1.kind = 'body'
JOIN artifact_chunks c2 ON c2.artifact_id = a2.id AND c2.kind = 'body'
WHERE a1.slug NOT LIKE '\_template\_%' ESCAPE '\'
  AND a2.slug NOT LIKE '\_template\_%' ESCAPE '\'
GROUP BY a1.slug, a2.slug
HAVING MIN(c1.embedding <=> c2.embedding) < 0.30
ORDER BY min_body_dist ASC;
```

---

## Part D — follow-up-of-follow-up 후보 (관찰 누적 후)

1. **`candidate_updates[]` threshold tuning** — Tier 2 결과로 판단. 0.22 유지
   vs 0.25로 완화 vs title-only vs body-mean 기준 전환.
2. **`tokenizer.json` 다운로드 제거** — 현재 쓰이지 않음 (~20MB 절약). Phase
   17의 gemma_model.go `gemmaSharedFiles` 슬라이스에서 drop.
3. **Release artifact에 onnxruntime lib 동봉 전략 결정** — 현재는 첫 run 자동
   download. Air-gapped 배포 시 `pindoc models install` CLI 필요.
4. **Glossary Tier 3 배치** — 용어 12+ 개를 12+ 개 artifact로 나눌지, 하나의
   큰 Glossary artifact로 묶을지 구조 결정.
5. **Area rename/merge 전략** — slug 변경 시 redirect / supersede_of 조합. 아
   직 미결 Decision (docs/decisions.md §Open Questions 후보).

---

## 기록

- 이전 세션: `cb45858` Phase 17 follow-up 코드 + 문서 완료
- 이 문서 자체는 Tier 2 발행의 프리플라이트로, 발행 완료 후 해당 섹션에
  "완료 · 2026-04-22" 표기 + 실제 slug로 갱신
- 발행된 Decision/Analysis artifact는 Part A/B 표의 slug와 대응해 기록
