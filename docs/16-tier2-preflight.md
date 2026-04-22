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
| D1 | Slug 정책: Unicode 보존 | Decision | decisions | `decision-slug-unicode-preservation` | `pindoc-agent-written-8` (implements) |
| D2 | `include_templates` 파라미터 통일 | Decision | decisions | `decision-include-templates-unified` | `pindoc-mcp-tools-v1-implementation-status-spec-runtime-drift` (implements) |
| D3 | `embedder_used` 공통 응답 필드 | Decision | decisions | `decision-embedder-used-response-field` | D2 (relates_to), `pindoc-url` (references) |
| D4 | Pin path validation (warn-level) | Decision | decisions | `decision-pin-path-warn` | `pindoc-3-tier-a-b-types-pin-vs-resourceref` (implements) |

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

## Part B — Tier 2 본편 8개

### 발행 순서 (dependency 기반)

1. **01-problem.md** → Analysis / vision (Vision의 "왜 지금인가" 확장)
2. **08-non-goals.md** → Decision / vision (명시적 반-목표 선언)
3. **02-concepts.md** → Analysis / vision (primitive 5개 — Project/Harness/Promote/Artifact/Graph)
4. **09-pindoc-md-spec.md** → Analysis / mechanisms (Harness Reversal M0의 포맷 spec)
5. **07-roadmap.md** → Analysis / roadmap (V1/V1.x/V2, BM)
6. **12-m1-implementation-plan.md** → Analysis / roadmap (Phase별 진행)
7. **06-ui-flows.md** → Analysis / ui (Flow 0-N, Cmd+K, Sidecar)
8. **11-design-system-handoff.md** → Analysis / ui (디자인 토큰, 영감원)

2번(non-goals)만 Type=Decision. 나머지는 Analysis. Glossary는 Tier 3으로 밀
림 — 12개 이상 용어 정의라 별건 batch가 낫다.

### 각 artifact 메타 (propose 입력 스케치)

#### T2-1 · 01-problem.md → "실패 모드 F1–F6과 공백의 맥락"
- `area_slug: vision`
- `type: Analysis`
- `pins: [docs/01-problem.md, docs/00-vision.md]`
- `relates_to: [pindoc-agent-written-8 (references)]`
- body: F1 "세션 증발" / F2 "area 경계" / F3 "format drift" / F4 "TC 공백" /
  F5 "stale" / F6 "과거 맥락 재발견" 6개 실패 모드 + 각각 어느 M-메커니즘과
  매핑되는지 + Vision 원칙 대응

#### T2-2 · 08-non-goals.md → "Pindoc이 되지 않는 것들 (Never list)"
- `area_slug: vision`
- `type: Decision` (anti-charter)
- `pins: [docs/08-non-goals.md]`
- `relates_to: [pindoc-agent-written-8 (references)]`
- body: "Human-writable surface 허용", "매 artifact 승인 강제", "Jira 대체",
  "raw 세션 흡수", "메신저/CRM 확장" 등 명시적 Non-goals

#### T2-3 · 02-concepts.md → "5 Primitive 정의와 경계"
- `area_slug: vision`
- `type: Analysis`
- `pins: [docs/02-concepts.md, docs/00-vision.md]`
- `relates_to: [pindoc-agent-written-8 (references), pindoc-3-tier-a-b-types-pin-vs-resourceref (implements)]`
- body: Project / Harness / Promote / Artifact / Graph 다섯 primitive 각각의
  정의·예시·다른 primitive와의 관계. Session/Checkpoint가 보조로 밀려난 이유

#### T2-4 · 09-pindoc-md-spec.md → "PINDOC.md Harness 포맷 스펙"
- `area_slug: mechanisms`
- `type: Analysis`
- `pins: [docs/09-pindoc-md-spec.md, internal/pindoc/harness (존재 시)]`
- `relates_to: [pindoc-m0-m7-harness-reversal-6 (implements)]`
- body: Frontmatter 필수 필드, `mode: auto|manual|off`, checkpoint 휴리스틱,
  PINDOC.md version vs server version compatibility

#### T2-5 · 07-roadmap.md → "V1 / V1.x / V2 로드맵과 BM Phase"
- `area_slug: roadmap`
- `type: Analysis`
- `pins: [docs/07-roadmap.md]`
- `relates_to: [pindoc-agent-written-8 (references), pindoc-url (references)]`
- body: M1–M7 milestones, Launch criteria, EthicalAds + Sponsors BM Phase 1,
  Hosted SaaS V2+

#### T2-6 · 12-m1-implementation-plan.md → "M1 Phase 체인과 현재 상태"
- `area_slug: roadmap`
- `type: Analysis`
- `pins: [docs/12-m1-implementation-plan.md]`
- `relates_to: [T2-5 (references), pindoc-url (references)]`
- body: Phase 1-17 + follow-up까지 체인 요약. Phase별 "왜 그 순서였나"

#### T2-7 · 06-ui-flows.md → "Wiki Reader UX Flow 0-N"
- `area_slug: ui`
- `type: Analysis`
- `pins: [docs/06-ui-flows.md, web/src/reader/ReaderShell.tsx, web/src/reader/CmdK.tsx]`
- `relates_to: [pindoc-agent-written-8 (references)]`
- body: Onboarding, Search Cmd+K, Sidecar, Review Queue, Graph (stub)

#### T2-8 · 11-design-system-handoff.md → "디자인 토큰과 영감원"
- `area_slug: ui`
- `type: Analysis`
- `pins: [docs/11-design-system-handoff.md, web/src/styles/reader.css, web/public/design-system]`
- `relates_to: [T2-7 (references)]`
- body: Linear + Obsidian + GitHub PR + Cmd+K 영감, 컬러/타이포/spacing 토큰

### 배치 실행 형태

Decision 4건 → Tier 2 8건 순차. 각 발행 사이에 `warnings` 관찰 + pin path
validation 결과(`PIN_PATH_NOT_FOUND` 없는지) 체크.

발행 간격은 **5-10초**(agent 본문 작성 시간). 전체 12 발행이면 2-5분 예상.

---

## Part C — Pairwise distance 2차 실측 (Tier 2 발행 후)

Tier 1 + Tier 2 = 13 artifact 기준 C(13,2)=78 쌍. Tier 1 때와 같은 SQL 쿼리로:

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

관찰 포인트:
- SOFT band (0.18–0.25) 안 비율 — Tier 1만 때는 6/10. Tier 2 추가로 희석되는
  지 vs 비슷하게 유지되는지 → `candidate_updates[]` threshold 0.22 재검토의
  직접 근거.
- HARD BLOCK (<0.15) 등장 여부 — 있다면 진짜 duplicate 의심 → 기존 artifact
  하나 supersede 검토.

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
