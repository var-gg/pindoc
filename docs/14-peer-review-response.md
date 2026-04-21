# 14. Peer Review Response — 2026-04-22

두 건의 외부 고급추론 리포트(이하 **Review A**, **Review B**)를 받아 현 기획(M1 + Phase 7/8 완료 상태)과 대조 판단한 결과. **맹목적 수용이 아닌 구조 판단** 관점의 정리다. 이후 Phase 9-13 계획은 이 문서의 판단을 근거로 한다. ([docs/12-m1-implementation-plan.md](./12-m1-implementation-plan.md))

## 0. 이 문서가 다루는 것

- 두 리포트가 공통 지적한 지점 (P0 공통 5개)
- 각 지적에 대한 수용 / 변형 수용 / 반려 판단과 근거
- 실무 dogfood 경험에서 도출된, 리포트가 놓친 보완 항목

## 1. 두 리포트의 공통 지적 (P0 뼈대)

두 리포트 모두 **동일한 5개 축**으로 현 구현을 평가했다. drift 가 큰 순서:

1. `artifact.propose`의 계약이 "regulator" 포지션에 비해 얇다 — `operation`, `target_id`, `basis`, `pins`, `expected_version` 부재.
2. duplicate 방지가 exact-title only — semantic conflict block 필요.
3. `pindoc://<slug>` URL 하나로는 Referenced Confirmation UX가 성립 안 함 — 사용자가 브라우저로 여는 canonical HTTP path와 에이전트 재호출용 ref를 분리해야 함.
4. typed documents를 주장하지만 실제로는 markdown + 얕은 keyword guardrail — `body_json` 컬럼은 있으나 미활용.
5. stub embedder 기본값이 dogfood 신뢰를 무너뜨림 — "의미 검색 flagship"이 실제로 안 됨.

## 2. 이번 라운드에서 이미 해소된 지적

| 지적 (P0) | 해소 근거 |
|---|---|
| "multi-project 약속과 single-project env runtime 충돌" | Phase 8에서 `/p/:project/…` canonical URL + `PINDOC_MULTI_PROJECT` 토글 + V1.5 권한 모델로 연기 명시. Review B §13.6 "single-project now" 포지셔닝 권고는 자연 수용. |
| "Project Switcher UI가 inert chip" | Phase 8에서 드롭다운 실제 작동. |
| "pindoc.project.create 부재" | Phase 8에서 MCP tool 추가. 프로젝트 row + `misc` area seed + canonical URL 반환. |
| "share URL 규약 부재 / 레거시 호환 부재" | Phase 8에서 `/`, `/wiki/*`, `/tasks/*`, `/graph`, `/inbox` 모두 `/p/{default}/…` 302. PINDOC.md 템플릿에 URL convention 섹션 추가. |
| "artifact.propose가 create-only" (Review A 판단 2) | **부분 해소**. Phase 7에서 `update_of` + `commit_msg`로 update 경로 도입. revisions 테이블 + diff 엔진 + history/diff UI까지. 단, 전체 operation enum (`supersede/split/archive`)은 Phase 11+에서 축소 수용. |
| "수정 이력 부재" | Phase 7. revisions / diff / summary_since 3종 MCP + Reader UI. |
| "PINDOC.md 템플릿이 URL 공유 규약 미포함" | Phase 8에서 섹션 추가. |

**합산**: 두 리포트 P0 공통 6개 중 3개 완전 해소, 1개 부분 해소. 남은 P0 2개 (계약 강화 + semantic conflict + human_url) → Phase 9/11에서 대응.

## 3. 수용 — Phase 9~13에 반영

### 3-1. `human_url` vs `agent_ref` 분리 (Phase 9, 완료 2026-04-22)

공통 지적 해소. Phase 8로 canonical URL 구조가 박혔으므로 응답 필드 추가만으로 해결.

- `artifact.{propose,read,search}` + `context.for_task` 응답에 두 필드 분리.
  - `agent_ref`: `pindoc://<slug>` — 에이전트가 다른 propose body에 embed하거나 read에 재주입.
  - `human_url`: `/p/:project/wiki/<slug>` — 에이전트가 사용자에게 채팅 공유. 상대경로 (외부 origin은 사용자 배포 소유).
- Referenced Confirmation 규약은 실무에서도 "메신저에 URL 하나 던지면 팀이 동일 문맥 공유"가 핵심이다. 링크 품질이 제품 신뢰의 근간.

### 3-2. `capabilities` 블록 (Phase 9, 완료)

Review A/B 모두 "bootstrap 통합 tool"을 제안. 우리는 `pindoc.project.current` 응답에 `capabilities` 블록 추가로 축소 수용. 한 번의 호출로 agent는 서버가 지원하는 플래그 파악 (multi_project / retrieval_quality / auth_mode / update_via / review_queue_supported).

새로운 tool 추가 대신 기존 tool 응답 확장이 PINDOC.md 규율 ("세션 시작 시 project.current 1회")과 호환된다.

### 3-3. Real embedder (Phase 10, 다음 작업)

Review B P1. 이미 자체 roadmap에도 있었음.

### 3-4. Write contract 강화 (Phase 11)

공통 P0. 단 전체 enum을 박는 대신 **축소된 필드 셋**만 수용:
- `basis.source_session` (optional audit), `basis.search_refs[]` (optional claim), `pins[]`, `expected_version`, `supersede_of`, `relates_to[]` (cross-reference).
- `operation` enum `new|update|supersede|split|archive` 풀세트는 반려 — `split/archive`는 실 use case 증빙 없음. `new|update` (현 `update_of` 유무로 판단) + `supersede_of` 별도 필드로 충분.

### 3-5. Semantic conflict (Phase 11)

공통 P0. Phase 10 real embedder 붙인 직후 자연스럽게 가능. exact-title + `lower()` 비교만 있는 현재 preflight 강화.

### 3-6. `body_json` 최소 활용 (Phase 11)

공통 P0. 단 **per-type section schema 확정은 반려** (저자 결정). DB 컬럼은 이미 존재, 검증 구조만 활성화.
- Debug: `symptom`, `resolution` 정도
- Decision: `decision`, `rationale` 정도
- 나머지 section 구조는 Phase 13의 **template artifact**로 해결.

### 3-7. `artifact.read(view=…)` + `not_ready` machine-readable + actor hardening (Phase 12)

Review B P1 블록. envelope 통일 + view knob + stdio actor binding.

### 3-8. Template artifact as living best practice (Phase 13, 신설)

**리포트 둘 다 제안하지 않은 보완**. 실무 dogfood에서 확인된 핵심 문제 — "같은 wiki 내에서 문서마다 포맷이 중구난방이 되면 난독증이 온다. 시작부터 best practice를 강제해야 한다" — 의 구조적 답.

Phase 11의 body_json minimal field 위에, 각 타입의 "현재 권장 섹션 구조"는 **별도 artifact로 심어두고 계속 revision**. template 자체가 dogfood 결과에 따라 진화. PINDOC.md 템플릿에 "신규 propose 전 `artifact.read(_template_<type>)` 로 현 구조 참고" 규약.

이 패턴의 의미: **포맷 베스트프랙티스도 pindoc 자체의 dogfood 산물**이라는 자기 참조. server schema에 박지 않는 이유는, 이상적 포맷이 아직 미확정이기 때문.

## 4. 변형 수용 — 원안 축소 적용

### 4-1. `pindoc.bootstrap` 통합 tool → `project.current` 응답 확장 (완료)

공통 제안. 새 tool 추가는 PINDOC.md의 "session start 시 project.current 1회" 규율과 중복. 같은 정보를 기존 tool에 합쳐서 해결.

### 4-2. Server-side evidence ledger → soft enforcement

Review A 제안: "최근 10분 내 search/read 호출 없으면 propose hard block". 우리는 `basis.search_refs[]`를 optional로 받고 서버는 이벤트 로그에만 기록 (Phase 11). Hard enforce는 adversarial review의 "lazy agent가 가짜 refs 넣기" 시나리오에 취약.

### 4-3. Operation enum 풀세트 → 축소 셋

위 §3-4 참조.

### 4-4. Short/verbose/debug response mode → 반려

현 규모 (17 artifacts)에서 과설계. 실 dogfood 중 토큰 병목 드러나면 재검토.

## 5. 반려 — 수용 안 할 지적

### 5-1. "agent-only wiki" 포지셔닝 완화 (Review A/B 공통)

리포트는 "interaction style에 불과"하다고 평가절하, "memory protocol"로 재작성 제안.

**반려.** 실무 dogfood 결과, **사람이 편집에 개입하면 섹션 순서/깊이/이름이 문서마다 drift한다.** 여러 팀이 중장기 협업할 때 이 drift가 가장 큰 가독성 비용. "agent-only + server-enforced template"이 format drift를 차단하는 구조적 답이고, 이건 style이 아니라 wedge의 본체. 외부 README hero copy는 원안 유지 ("AI가 쓰는 위키"), 내부 technical doc에서는 "server-enforced promote layer" 언어 병행.

### 5-2. Harness 템플릿의 "approval 과잉 유도" 완화 (Review A)

리포트 제안: "ask only when sensitive"로 문구 수정.

**반려.** Referenced Confirmation은 sensitive op 한정이 아니라 **모든 확인 요청에 적용**되는 기본 규약 ("팀이 동일 문맥 공유"의 전제). approval spam 위험 지적은 맞지만, 답은 문구 완화가 아니라 `human_url` 품질 개선 (§3-1) + template artifact로 "언제 confirm할지"를 베스트프랙티스 artifact로 푸는 것.

### 5-3. Tier B / Task / TC 제거, M1을 3 타입(Debug/Decision/Analysis)으로 축소 (Review B)

**반려.** Task는 실무에서 문서/결정의 실행 단계. "Tasks and wiki in separate solutions"가 pain이고 "한 graph에 Task+Decision+Debug"가 pindoc의 wedge 일부. M1 Tier A 7종 유지, Tier B (Web-SaaS pack) 4종도 seed 완료 상태.

### 5-4. Every-call `project_ref` on MCP tools (Review A)

**반려.** `PINDOC_PROJECT` env → 한 subprocess = 한 project. Claude Code가 repo rooted로 spawn하므로 wrong-project 위험 낮음. Phase 8로 HTTP는 URL 기반 scope 박힘. MCP에 explicit project_ref는 에이전트 시맨틱 부담만 늘림.

### 5-5. "License 확정", "comparison page 공개", "landscape positioning 재작성" (Review B §13)

**반려 현재 우선순위 아님.** V1 직전 작업. 지금은 contract + dogfood loop 완성이 먼저.

### 5-6. `bootstrap tool`, `bundle_id` lazy hydrate, stable short codes (DEC/ANL/DBG) (Review B §9)

**반려 과설계.** 현 규모에서 가치 대비 복잡도 큼.

## 6. 리포트가 놓친 지적 — 실무 dogfood에서 도출

리포트 두 개 모두 다루지 않았으나 실 사용 관점에서 중요한 항목들. Phase 11/13에 반영됨.

### 6-1. `relates_to[]` cross-artifact edge = Pindoc의 wedge

실무에서 `wiki + task tracker` 분리 환경은 교차 참조가 링크로만 유지되고 쉽게 깨짐 ("#736 이게 뭐였지?"). pindoc의 구조적 답은 Decision → Debug → Task가 **한 graph의 typed edge**로 묶이는 것. Phase 11 `artifact_edges` 테이블 + `relates_to[]` 입력 필드로 코드화.

### 6-2. `open_questions[]` / 재조회 메타는 가치 있으나 **optional 필드로** 도입

실무 문서 패턴에서 "Open Issues", "재조회 방법", "조사 시점" 같은 섹션은 다음 세션 이어받기에 핵심 실마리. 단 **타입별 필수 section schema 확정은 저자 판단상 out-of-scope** — 지금 샘플 구조가 best라고 말할 수 없음. 대신 Phase 13 template artifact에서 권장 패턴으로 쌓고, template revision을 통해 진화.

### 6-3. "포맷 evolving artifact" 자체가 pindoc의 dogfood

Phase 13은 이 원칙을 코드화한다. `body_json` 검증을 minimal로 유지한 이유도 여기 — 스키마가 early lock-in되면 format best practice의 진화를 막는다.

## 7. 정리 — 리포트 활용 원칙

- **수용 6개** (Phase 9/10/11/12/13)
- **변형 수용 4개** (원안 축소)
- **반려 6개** (철학/설계/우선순위)
- **보완 추가 3개** (리포트 놓침, 실무 dogfood 기반)

이 문서 자체도 `update_of`로 revision 대상. 다음 라운드 고급추론 리포트를 받으면 이 판단을 갱신한다.

## 참고

- [Phase 9-13 계획](./12-m1-implementation-plan.md)
- [MCP Tools 구현 상태](./10-mcp-tools-spec.md)
- [Phase 8 세션 핸드오프](./13-session-handoff-2026-04-22.md)
