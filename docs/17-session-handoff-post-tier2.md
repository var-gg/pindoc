# Session Handoff — Tier 2 이후 / Threshold 재조정 반영 직후

> 이전 handoff [docs/15](./15-session-handoff-dogfood.md)은 dogfood 진입
> 시점(Phase 16 직후)의 스냅샷이었고, 그 이후 **Phase 17 + follow-up
> a/b/c + Tier 1·2 dogfood**가 완료됐다. 이 문서는 Tier 3 진입 직전의
> state 스냅샷이자 다음 세션 프롬프트.

---

## 1. 지금까지 (Phase 17 ~ Tier 2)

### 최신 커밋 체인

```
<HEAD> Phase 17 follow-up d: Semantic conflict threshold 재조정 (이번 세션)
257c95a Tier 2 batch + pairwise calibration data (Phase 17 follow-up c)
2f093b2 Phase 17 follow-up b: HTTP /api/search parity + slug tests + Tier 2 plan
cb45858 Phase 17 follow-up: slug/include_templates/embedder_used/pin-path
e0af0f7 Phase 17: Bundled EmbeddingGemma restored as default embedder
79e8d2c Fix: Area artifact_count out of sync with artifact list
```

### DB state

13 non-template artifact (5 Tier 1 + 8 Tier 2) + 4 `_template_*`.
전부 gemma Q4 벡터 (`pindoc-reembed`로 Tier 2 8건 in-place 교체 완료,
Tier 1은 Phase 17에서 이미 gemma). Stub embedding 잔여 없음.

Tier 1 (5) — agent-only, URL, 3-tier, MCP surface, Data model.
Tier 2 (8) — 실패모드 F1-F6, Non-goals 헌법, 5 Primitive, PINDOC.md spec,
V1 Roadmap+BM, Phase 1-17 체인, Wiki Reader UX, Design System v0.

### Phase 17 follow-up — 마감된 축

- **Slug Unicode 보존** — `[^\p{L}\p{N}]+` + rune-cap 60 (ASCII-only 폐기)
- **`include_templates` 파라미터 통일** — 전 MCP tool, 기본 false
- **`embedder_used` 공통 응답 필드** — propose/search/context.for_task
- **Pin path validation (warn-level)** — `PINDOC_REPO_ROOT` 설정 시 활성
- **HTTP `/api/search` parity** — slug로 웹 reader에서도 `?q=` 검색

4건은 코드 + 문서(docs/10, 12, decisions)에 반영 완료했으나 **Decision
artifact 발행은 이번 세션에 밀려 있었음** (이전 세션 MCP가 stub embedder
상태라 발행해도 의미 없었기 때문). 다음 세션 Step 1에서 집행.

### Pairwise distance 실측 — Part C 결과

docs/16 §Part C. 13 artifact × C(13,2) = 78 쌍. gemma 기준:
- SOFT band (<0.25): 30/78 (38%)
- HARD band (<0.18): 4 쌍 — 전부 legitimate near-match
  - concept(02) → mechanism(05) → spec(09) 같은 축 depth 3겹
  - Phase chain(M1) → Roadmap(V1) 상하 계층

**결론**: 0.18 HARD는 한 제품 corpus에 과하게 빡빡. → 이번 세션에서 재조정.

---

## 2. 이번 세션(2026-04-22 밤)에 한 일

### 발견

- 이전 세션에서 `bin/pindoc-server.new.exe` 빌드만 하고 **swap 안 한 상태로
  종료** → 이번 세션의 Claude Code MCP subprocess가 OLD 바이너리(stub
  embedder + ASCII-only slug)로 spawn됨
- `pindoc.artifact.search` 응답에 `embedder_used` 필드 없음 + `notice:
  "stub embedder"` 떠서 확인
- HTTP API는 read-only — write 경로는 MCP only → **Step 1(Decision 4건
  발행) + Step 3 전부 이 세션에서 불가**

### 대안 플랜 실행

원래 순서 `Step 1 → Step 2 → Step 2b → Step 3 → commit`이었으나
Step 1이 막혔으므로:

1. **바이너리 swap 선행** — `bin/pindoc-server.exe~` = old (14:22),
   `bin/pindoc-server.exe` = Phase 17 follow-up 반영본 (18:59)
2. **pindoc-api 재기동** — `PINDOC_REPO_ROOT` set, gemma 기동 확인
3. **Step 2a만 선집행** — threshold 상수 변경
   - `semanticConflictThreshold` 0.18 → 0.13
   - `semanticAdvisoryThreshold` 0.25 → 0.30
   - 파일: `internal/pindoc/mcp/tools/artifact_propose.go:1442,1449`
4. **빌드** — `bin/pindoc-api.exe` (20:33) + `bin/pindoc-server.new.exe`
   (20:34) 둘 다 재빌드. API는 threshold 상수 import 안 하지만 일관성용
5. **pindoc-api 재기동** — 새 api 바이너리로 health check ok
6. **docs 업데이트** — 16 §Part C 끝에 "변경 반영 2026-04-22" 줄 +
   decisions.md Resolved row 1건 추가
7. **docs/17 작성** (이 문서) — 다음 세션 프롬프트 포함

### 남아서 다음 세션으로 간 일

- **Step 1** — Phase 17 follow-up Decision artifact 4건 발행 (D1-D4)
- **Step 2b** — threshold 재조정 Decision artifact 발행
- **Step 3** — Tier 3 착수 또는 추가 handoff

---

## 3. 다음 세션 시작 시 할 일

### 0. 재개 직전 체크

```bash
cd A:/vargg-workspace/pindoc

# 0-a. 바이너리 swap — 이번 세션 빌드분(threshold 적용)을 실제 경로로
mv bin/pindoc-server.exe bin/pindoc-server.exe~
mv bin/pindoc-server.new.exe bin/pindoc-server.exe

# 0-b. pindoc-api 재기동
ps -ef | grep pindoc-api | grep -v grep  # 있으면 kill
PINDOC_REPO_ROOT="$PWD" ./bin/pindoc-api.exe &

# 0-c. health check — embeddinggemma 떠야 성공
curl -s http://127.0.0.1:5831/api/health

# 0-d. MCP 검증 — artifact.search 응답에 embedder_used.name == "embeddinggemma"
```

MCP 첫 호출에서 `embedder_used` 필드 있으면 OK. 없으면 또 swap 누락 의심.

### Step 1 — Decision artifact 4건 발행

docs/16 §Part A 표대로. 전부 MCP `pindoc.artifact.propose` 경유:

| # | Title | Slug | relates_to |
|---|---|---|---|
| D1 | Slug 정책: Unicode 보존 | `decision-slug-unicode-preservation` | `pindoc-agent-written-8` (implements) |
| D2 | `include_templates` 파라미터 통일 | `decision-include-templates-unified` | `pindoc-mcp-tools-v1-implementation-status-spec-runtime-drift` (implements) |
| D3 | `embedder_used` 공통 응답 필드 | `decision-embedder-used-response-field` | D2 (relates_to), `pindoc-url` (references) |
| D4 | Pin path validation (warn-level) | `decision-pin-path-warn` | `pindoc-3-tier-a-b-types-pin-vs-resourceref` (implements) |

- type: Decision / area_slug: decisions / completeness: settled
- 본문: `_template_decision` 양식 (Context / Options / Decision / Consequences / Rollback)
- basis.search_receipt: 매 발행마다 새로 — `context.for_task(<title>)` 또는 `artifact.search`
- pins: `internal/pindoc/mcp/tools/artifact_propose.go` (공통)

관찰 포인트:
- `embedder_used.name == "embeddinggemma"` (필수)
- `warnings: ["RECOMMEND_READ_BEFORE_CREATE"]` 발동해야 정상 —
  **새 advisory 0.30** 밴드에선 더 넓게 잡힐 것. 그래도 미발동이면
  `findSemanticAdvisories` 로직 의심
- `pins_stored == 1`

### Step 2b — Threshold 재조정 Decision 발행

이번 세션에서 코드·문서는 반영했으므로 Decision artifact만 남음.

- title: "Semantic conflict threshold 재조정 (Tier 2 calibration)"
- slug: `decision-conflict-threshold-recalibration-tier2`
- area_slug: decisions / type: Decision / completeness: settled
- relates_to:
  - `pindoc-m1-phase-chain-1-17` (implements)
  - `pindoc-mcp-tools-v1-implementation-status-spec-runtime-drift` (references)
- pins:
  - `internal/pindoc/mcp/tools/artifact_propose.go` (line 1442, 1449 근방 두 상수)
  - `docs/16-tier2-preflight.md`
- body: Context (Tier 2 pairwise 실측 근거) / Options (0.15 vs 0.13 vs title-only) / Decision (0.13 HARD + 0.30 advisory) / Consequences (near-match 경고 범위 확대, false-block 감소) / Rollback (상수만 되돌림, 재-embed 불요)

### Step 3 — Tier 3 착수 또는 handoff

남은 시간으로 판단:

**충분(30분+) → Tier 3 착수 후보**:

1. **decisions.md Resolved rows → 개별 Decision artifact**
   현재 30+ row가 한 파일에 있음. 주요 후보 row 분해:
   - "Primitive 7 → 5" → Decision / vision
   - "Raw 세션 파일 흡수 V1~V1.x Never" → Decision / vision
   - "BM Phase 1: EthicalAds + GitHub Sponsors" → Decision / roadmap
   - "프로젝트명 Varn → Pindoc" → Decision / vision
   - "Tier A/B/C 타입 체계" → Decision / architecture
   - "Pin(hard) vs Related Resource(soft) 분리" → Decision / architecture
   - "Graph edge = Derived View" → Decision / architecture
   - "MCP tool 네임스페이스 정리" → Decision / mechanisms
   - "AGENTS.md (복수) 통일" → Decision / architecture
   - "Conflict threshold V1 하드코딩" (→ 이미 이번 세션 재조정 Decision으로 superseded 관계 설정 가능)
   - "Meta-dogfooding V1 M1 즉시 착수" → Decision / roadmap

   각 row 1 artifact. `decisions` area에 모음. 본문은 decisions.md의
   Rationale 열 확장. `supersede_of` 대신 `relates_to` references로
   decisions.md 자체 pin.

2. **docs/14-peer-review-response.md → Analysis / decisions**
   3차 peer review 수용/거부 근거. 1 artifact로 묶을지, 피드백 항목별
   쪼갤지는 내용 보고 결정.

3. **Glossary Tier 3**
   `docs/glossary.md` 용어 12+개. 1 artifact(type=Glossary, area=misc)로
   묶는 게 검색 응답 noise 적음 — 용어별 쪼개면 artifact.search에서
   Glossary가 top hit 독점할 위험.

4. **AGPL-3.0 → Apache 2.0 전환 Decision (Phase 16)**
   Phase 16 자체는 decisions.md row에 없음 — 공백. 별도 Decision artifact
   필요. title 제안: "OSS 라이선스: Apache 2.0 채택" / area: misc 또는
   `decisions`.

**부족 → 세션 종료**. docs/18-session-handoff-tier3-entry.md 신규 작성.

### Step 4 — Commit

- Tier 3 artifact들은 DB만 바뀜 — git commit 불요
- 코드 변경 발생하면 별도 커밋
- Handoff 문서 신규 시 한 커밋

---

## 4. Scope 밖 (다음 세션도 건드리지 말 것)

- Docker embed 컨테이너 재-wiring — V1 release에서 optional
- 기존 13 artifact slug 변경 — immutable
- `.mcp.json` embed env 추가 — 불필요
- **Threshold 변경 후 기존 13 artifact 재-embed** — 필요 없음. 다음 propose부터 적용
- `tokenizer.json` 다운로드 제거 (Phase 17 gemma_model.go `gemmaSharedFiles`) — docs/16 §Part D에 기록만, 실행은 추후

---

## 5. 관찰 포인트 (다음 세션 말에 기록)

- Decision 4건(D1-D4) + threshold Decision 발행 시:
  - `embedder_used.name == "embeddinggemma"` 비율 (기대 5/5)
  - `RECOMMEND_READ_BEFORE_CREATE` 발동 쌍 수 (advisory 0.30 확대 효과)
  - `pins_stored == N` pin 수 일치 여부
- Decision 5건 발행 후 pairwise 재실측 (docs/16 §Part C SQL 재실행):
  - HARD(<0.13) 신규 쌍 0건이어야 정상
  - advisory(0.13 ~ 0.30) 쌍 수 변화
- Tier 3 진입 시 decisions.md row → artifact 분해 비용 체감
- MCP `embedder_used` 필드가 HTTP 응답과 일관되는지 (Phase 17 follow-up d 검증)

---

## 6. 사용자 복붙용 프롬프트 블록

```
Pindoc 작업 재개. 이전 세션(2026-04-22 밤)에 threshold 재조정
(0.18→0.13, 0.25→0.30) 빌드·문서만 완료하고 Decision 발행은 미룸.
전체 맥락은 docs/17-session-handoff-post-tier2.md.

## 착수 (1회)
cd A:/vargg-workspace/pindoc
mv bin/pindoc-server.exe bin/pindoc-server.exe~
mv bin/pindoc-server.new.exe bin/pindoc-server.exe
ps -ef | grep pindoc-api | grep -v grep | awk '{print $2}' | xargs -r kill
PINDOC_REPO_ROOT="$PWD" ./bin/pindoc-api.exe &
sleep 2
curl -s http://127.0.0.1:5831/api/health
# 첫 pindoc.artifact.search 응답에 embedder_used.name=="embeddinggemma" 필수

## 순차
Step 1  — Decision 4건(D1-D4) 발행 — docs/16 §Part A 표, slug 고정
Step 2b — Threshold 재조정 Decision 발행 — docs/17 §Step 2b
Step 3  — Tier 3 착수 or 추가 handoff — docs/17 §Step 3

발행마다 embedder_used / warnings / pins_stored 관찰해서 세션 말 docs
기록. Tier 3 코드 변경 생기면 별도 커밋.

시작.
```

---

## 기록

- 이 세션 커밋: `<이번 커밋 해시>` Phase 17 follow-up d — threshold 재조정
- 이전 handoff 체인: [13](./13-session-handoff-2026-04-22.md) → [15](./15-session-handoff-dogfood.md) → **17** (현재)
- docs/16은 Tier 2 발행 프리플라이트 + Part C 실측 데이터로 유지 (aging ok)
