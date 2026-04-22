# Session Handoff — Tier 3 진입 직후 / Decision 6건 발행 완료

> 이전 handoff [docs/15](./15-session-handoff-dogfood.md)은 dogfood 진입
> 시점(Phase 16 직후)의 스냅샷이었고, 그 이후 **Phase 17 + follow-up
> a/b/c/d + Tier 1·2 dogfood + Decision 6건 발행 + threshold 실전 검증**이
> 완료됐다. 이 문서는 Tier 3 중간지점 스냅샷이자 다음 세션 프롬프트.

---

## 1. 지금까지 (Phase 17 ~ Decision 6건 발행)

### 최신 커밋 체인

```
<HEAD+2> Session 결과 + pairwise Part E + decisions.md row 2 (이번 세션 추가 커밋)
<HEAD+1> docs: record b502411 커밋 해시
b502411  Phase 17 follow-up d: Semantic conflict threshold 재조정
257c95a  Tier 2 batch + pairwise calibration data (Phase 17 follow-up c)
2f093b2  Phase 17 follow-up b: HTTP /api/search parity + slug tests + Tier 2 plan
cb45858  Phase 17 follow-up: slug/include_templates/embedder_used/pin-path
e0af0f7  Phase 17: Bundled EmbeddingGemma restored as default embedder
```

### DB state — 19 non-template artifact + 4 template

| 그룹 | 수 | slug 예시 |
|---|---|---|
| Template | 4 | `_template_{analysis,decision,debug,task}` |
| Tier 1 (Phase 17) | 5 | `pindoc-agent-written-8`, `pindoc-url`, `pindoc-3-tier-a-b-types-pin-vs-resourceref`, `pindoc-mcp-tools-v1-implementation-status-spec-runtime-drift`, `pindoc-4-data-model-3-axis` (slug 일부 ASCII 축소본) |
| Tier 2 (Part B) | 8 | `pindoc-문제-f1-f6-실패-모드`, `pindoc-non-goals-헌법`, `pindoc-5-primitive-개념`, `pindoc-md-harness-spec`, `pindoc-v1-로드맵-bm-phase`, `pindoc-m1-phase-chain-1-17`, `pindoc-ui-wiki-reader-cmdk-flow`, `pindoc-design-system-v0-handoff` |
| **Decision (이번 세션)** | **6** | `decision-slug-unicode-preservation`, `decision-include-templates-unified`, `decision-embedder-used-response-field`, `decision-pin-path-warn`, `decision-conflict-threshold-recalibration-tier2`, `decision-license-apache-2-0` |

전부 gemma 벡터. 전부 `embedder_used.name == "embeddinggemma"` 응답 필드 확인.

### Threshold calibration 실전 검증 (NEW 0.13/0.30)

docs/16 §Part E에 전체 기록. 요약:

| Band | 쌍 수 | 비율 |
|---|---|---|
| HARD (<0.13) | 0 | 0% |
| 구 HARD (0.13–0.18) | 4 | — |
| Advisory (<0.30) 총 | 57 | 33% |

- 신규 Decision 6건 중 advisory band 진입 = **D4 (pin-path-warn) 1건만** (Pin/ResourceRef Analysis와 0.2828, 의미적으로 정당)
- 나머지 5건 전부 기존 corpus와 distance 0.30+ → Decision artifact가 Analysis와 **별도 의미 공간** 점유 확인

---

## 2. 이번 세션(2026-04-22 밤)의 실제 집행

### 발견 → 해결 flow

**Pre-flight 단계**:
1. `bin/pindoc-server.exe` 14:22 OLD 바이너리로 MCP spawn된 상태로 세션 시작 확인
2. 사용자 지시대로 swap: `server.exe~`(old 14:22), `server.exe`(18:59 Phase 17 follow-up c), `server.new.exe`(20:34 build after threshold change)
3. pindoc-api(20:33) 재기동 — gemma 확인
4. 첫 MCP 호출 `artifact.search` → `notice: "stub embedder"` → **세션 MCP subprocess는 구 바이너리**

**Option A 실험 (연구원 모드 전환 후)**:
1. `tasklist` + `wmic`로 pindoc-server.exe 프로세스 식별 (PID 40544, parent claude.exe PID 110396 = 내 세션)
2. PID 40544 kill
3. `pindoc.ping` 호출 → 응답 → **Claude Code는 MCP 죽으면 다음 tool call에서 자동 재spawn**
4. `artifact.search` 재호출 → `embedder_used.name == "embeddinggemma"`, distance 0.6x (실제 semantic), 한글 slug → **새 바이너리로 복구 성공**

이 학습이 연구원 모드의 첫 발견 — MCP lifecycle은 Claude Code parent에서
관리되고 stdio pipe 기반이라 child 재spawn이 투명하다.

### Step 1 (D1-D4, OLD threshold 18:59 바이너리)

Phase 17 follow-up 4건 Decision artifact 전부 `accepted`:

| # | slug | pins | edges | warnings |
|---|---|---|---|---|
| D1 | `decision-slug-unicode-preservation` | 2 | 2 | 없음 |
| D2 | `decision-include-templates-unified` | 3 | 2 | 없음 |
| D3 | `decision-embedder-used-response-field` | 4 | 3 | 없음 |
| D4 | `decision-pin-path-warn` | 2 | 2 | 없음 |

`RECOMMEND_READ_BEFORE_CREATE` **미발동** — 사용자 예상("0.18-0.25 band에 걸림")과 달랐음. 관찰: Decision이 기존 Analysis와 distance 0.4+라 OLD advisory 0.25 초과 = 정상 판정. Phase 14b 버그 아님.

### Step 2b 전 단계: 바이너리 swap + MCP respawn

1. `mv bin/pindoc-server.exe bin/pindoc-server.exe.followupc`
2. `mv bin/pindoc-server.new.exe bin/pindoc-server.exe` (NEW threshold 20:34)
3. `taskkill /PID 166964 /F` (Option A 재사용)
4. 다음 MCP ping으로 재spawn 확인 — PID 244756, 22:07:56

### Step 2b + Step 3a (NEW threshold 20:34 바이너리)

| # | slug | pins | edges | warnings |
|---|---|---|---|---|
| threshold | `decision-conflict-threshold-recalibration-tier2` | 3 | 3 | 없음 |
| Apache | `decision-license-apache-2-0` | 3 | 3 | 없음 |

두 artifact 전부 기존 corpus와 distance 0.52+ → NEW advisory 0.30도 미발동.

### Step 3b — pairwise 재측정

docker psql로 `artifact_chunks.embedding <=> embedding` 쿼리 실행. 결과 docs/16 §Part E.

---

## 3. 다음 세션 착수 (Tier 3 잔여 + 관측)

### 0. 재개 직전 체크 (바이너리 이미 swap 완료 상태)

```bash
cd A:/vargg-workspace/pindoc

# 0-a. MCP 재spawn 방식 확정 — swap 대신 kill만 하면 새 바이너리로 뜸
# 현재 bin/pindoc-server.exe = 20:34 = NEW threshold 바이너리
# (이번 세션 끝에 swap 수행 완료)

# 0-b. pindoc-api 상태 확인
curl -s http://127.0.0.1:5831/api/health
# → embedder=embeddinggemma 확인되면 skip 0-c

# 0-c. 필요 시 재기동
PINDOC_REPO_ROOT="$PWD" ./bin/pindoc-api.exe &

# 0-d. MCP 검증
# 첫 pindoc.artifact.search 호출에서 embedder_used 있어야 정상
# 없으면 Option A 재실행:
#   wmic process where "name='pindoc-server.exe'" get ProcessId,ParentProcessId
#   taskkill /PID <내 세션의 MCP pid> /F
#   다음 tool call에서 자동 재spawn
```

### Step 4 — Tier 3 잔여 (decisions.md row 분해)

현재 decisions.md Resolved rows = 30+ 개. 이번 세션에서 분해한 건 Apache 2.0 1건.

**분해 우선순위** (관측 가치 순):

| 우선 | row | 분해 artifact 유형 | 기대 관계 |
|---|---|---|---|
| 1 | "Primitive 7 → 5" | Decision/vision | `pindoc-5-primitive-개념` implements |
| 2 | "Publish ≡ Promote 통합" | Decision/mechanisms | `pindoc-m0-m7-harness-reversal-6` implements |
| 3 | "Raw 세션 파일 흡수 V1~V1.x Never" | Decision/vision | `pindoc-non-goals-헌법` implements |
| 4 | "Tier A/B/C 타입 체계" | Decision/data-model | `pindoc-3-tier-a-b-types-pin-vs-resourceref` implements |
| 5 | "Pin(hard) vs Related Resource(soft) 분리" | Decision/data-model | `pindoc-3-tier-a-b-types-pin-vs-resourceref` implements |
| 6 | "Graph edge = Derived View" | Decision/architecture | — |
| 7 | "MCP tool 네임스페이스 정리" | Decision/mcp-surface | `pindoc-mcp-tools-*` implements |
| 8 | "BM Phase 1: EthicalAds + GitHub Sponsors" | Decision/roadmap | `pindoc-v1-로드맵-bm-phase` implements |
| 9 | "프로젝트명 Varn → Pindoc" | Decision/vision | `pindoc-agent-written-8` references |
| 10 | "Human Approve 단계 삭제, Auto-publish 기본" | Decision/mechanisms | `pindoc-m0-m7-harness-reversal-6` implements |

상위 5건만이라도 분해하면 주요 축이 artifact graph에 드러남. 관측
포인트:
- 같은 Analysis를 여러 Decision이 `implements`하는 edge 집중도
- 서로 다른 Decision 간 distance (NEW HARD 0.13 진입 발생하는지)

### Step 5 — Glossary Tier 3 구조 결정 + 발행

`docs/glossary.md` 용어 12+개. 가설:
- 단일 `Glossary` artifact: 검색 응답 noise 낮음, 용어 간 비교 쉬움
- 용어별 artifact: Graph에서 용어 간 관계 가시화, but artifact.search top-hit 독점 위험

**권장**: 먼저 단일 Glossary artifact 1건 발행하고, 발행 후 artifact.search에서 Glossary가 어떻게 ranking 되는지 관측. 문제 있으면 쪼갬.

### Step 6 — docs/14-peer-review-response 흡수

3차 peer review 수용/거부 기록. 1 Analysis artifact로 통합 (피드백별
쪼개면 micro-artifact noise).

- title: "3차 Peer Review 수용/거부 근거"
- area: decisions (또는 misc — 판단 필요)
- pins: docs/14, 수용된 변경이 반영된 code 몇 곳

### Step 7 (선택) — 추가 pairwise 관측

Tier 3 분해 완료 시 artifact 총수 30+ 예상. C(30,2) = 435 쌍.
현 HARD 0 쌍이 유지되는지, advisory 비율 변화(현 33%)가 어떤지 재측정.

HARD 진입 쌍이 발생하면:
- 의미적으로 정당한지 확인
- 정당하면 threshold 추가 조정
- 중복이면 `supersede_of`로 통합

### Step 8 (선택) — 관측 포인트 기록

각 발행에서 다음 지표 기록:
- `embedder_used.name` 일관성 (전부 `embeddinggemma`)
- `warnings` 발동 빈도
- `pins_stored` 기대값 일치
- `edges_stored` 관계 보존
- distance top-3 (relates_to 후보 제안 품질 관찰)

---

## 4. Scope 밖 (건드리지 말 것)

- **Docker embed 컨테이너 재-wiring** — V1 release optional
- **기존 19 artifact slug 변경** — immutable
- **기존 Tier 1·2 artifact 내용 update** — 필요 시 `supersede_of` 사용
  (in-place edit 금지)
- **Threshold 추가 조정** — Tier 3 완료 후 재측정 기반으로만
- **`tokenizer.json` 다운로드 제거** — docs/16 §Part D 기록만, 실행은
  Tier 4+

---

## 5. 관측 포인트 (다음 세션 말에 기록)

**측정 대상**: Tier 3 발행 10+ 건.

| 지표 | 기대 | 추적 방법 |
|---|---|---|
| `embedder_used` 일관성 | 10/10 gemma | 각 propose 응답 필드 |
| `RECOMMEND_READ_BEFORE_CREATE` 발동 | 같은 Area 내 1-2건/10건 | `warnings[]` |
| HARD block 발동 | 0 | 발동 시 `not_ready` 응답 |
| Decision↔Decision distance | 0.25 이상 분산 | SQL 재실행 |
| `pins_stored` 합 | pin 입력 합과 일치 | 응답 필드 |

**관측 가설**:
- 동일 Analysis를 구현하는 Decision 다수가 생기면 그들끼리 distance
  어떻게 나올지 (주제는 같지만 Rationale이 다름 → 0.25~0.35 예상)
- decisions.md row의 Rationale 열을 그대로 옮기면 문체가 유사해서
  Decision 간 distance 낮아질 수 있음 — 이게 HARD 진입의 주요 위험

---

## 6. 사용자 복붙용 프롬프트 블록

```
Pindoc 작업 재개. 이전 세션(2026-04-22 밤)에 Decision 6건 발행 +
threshold recalibration 실전 검증 완료. 상세 docs/17 §2,3.

## 착수
cd A:/vargg-workspace/pindoc
curl -s http://127.0.0.1:5831/api/health
# embeddinggemma 확인되면 skip, 아니면 재기동:
# PINDOC_REPO_ROOT="$PWD" ./bin/pindoc-api.exe &

## MCP 검증 (첫 호출에서)
첫 pindoc.artifact.search 응답에 embedder_used.name=="embeddinggemma"
+ distance 실제 semantic(0.4~0.7 범위) 확인. stub이면 Option A:
  wmic process where "name='pindoc-server.exe'" get ProcessId,ParentProcessId
  - 내 세션의 pindoc-server PID 식별 (Parent가 내 claude.exe)
  - taskkill /PID <pid> /F
  - 다음 tool call에서 자동 재spawn

## Tier 3 순차 (docs/17 §Step 4 표 참조)
Step 4  — decisions.md row 상위 5건(Primitive, Publish≡Promote, Raw세션,
          Tier A/B/C, Pin vs ResourceRef) 개별 Decision artifact 분해
Step 5  — Glossary 단일 artifact 발행, ranking 관측
Step 6  — docs/14 peer review → Analysis artifact 흡수
Step 7  — pairwise 재측정 (docs/16 SQL)

발행마다 embedder_used/warnings/pins_stored 관측해서 세션 말 docs
업데이트. 코드 변경 없으면 artifact 발행은 DB만 바뀜 → git commit 불요.

시작.
```

---

## 기록

- 이 세션 커밋:
  - `b502411` Phase 17 follow-up d — threshold 상수 변경
  - `02b6e27` docs: record b502411 해시
  - `<이번 commit>` Decision 6건 발행 결과 + Part E + decisions.md rows
- 이전 handoff 체인: [13](./13-session-handoff-2026-04-22.md) → [15](./15-session-handoff-dogfood.md) → **17** (현재)
- docs/16은 Tier 2 발행 프리플라이트 + Part C (13 artifact) + Part E (19
  artifact) pairwise 실측 누적
- 이번 세션의 주요 학습 (연구원 모드):
  - Claude Code MCP는 child kill 시 다음 tool call에서 **투명하게 재spawn**
  - 이 메커니즘을 알고 나면 session 시작 후 binary swap → MCP kill →
    새 바이너리로 복구까지 in-session에서 가능
  - "다음 세션" 회피는 MCP lifecycle 오해에서 온 것
