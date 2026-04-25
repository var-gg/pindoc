# Revision Shapes Spec

한 번의 dog-food 세션에서 관찰한 마찰(`CLAIMED_DONE_INCOMPLETE`를 피하려고
body에서 acceptance 박스를 삭제하는 관행, metadata-only 갱신에서도
body를 통째로 재전송해야 하는 비용, `expected_version=0`이 valid인지
invalid인지 애매한 Go pointer hack)이 공통의 뿌리를 드러냈다. Artifact
mutation이 "body blob을 다시 넣기"라는 단일 경로만 있어서 모든 편집이
그 경로의 제약을 뒤집어쓴다는 것. Phase A–H가 그 경로를 네 갈래로 쪼갰다.

---

## 1. 4 shape

`pindoc.artifact.propose.shape`는 다음 중 하나다 (생략 시 `body_patch`).

| Shape | 변경 대상 | body_markdown | 대표 케이스 |
|---|---|---|---|
| `body_patch` | body | 새 본문 or BodyPatch | "문단 3을 다시 쓰고 싶다" |
| `meta_patch` | status/tags/meta | 불필요 | "Task status만 claimed_done" |
| `acceptance_transition` | 체크박스 1개 | AcceptanceTransition 페이로드 | "acceptance[2]를 [~]로" |
| `scope_defer` | 체크박스 1개 + graph edge | ScopeDefer 페이로드 | "이 항목은 task-X로 이관" |

- `body_patch`는 기존 동작을 그대로 보존 — 명시 없이 호출하면 이 경로.
  BodyPatch(`section_replace` / `checkbox_toggle` / `append`)와
  full `body_markdown` 양쪽 다 받는다.
- `meta_patch`는 본문을 쓰지 않는다. revision row는
  `body_markdown=NULL`, `body_hash=hash(currentBody)`로 저장 — 읽기
  경로는 `artifacts.body_markdown` head만 보므로 영향 없음.
- `acceptance_transition`은 `[ ]` / `[x]` / `[~]` / `[-]` 중 한 마커로
  단일 체크박스만 바꾼다. `[~]` (partial) / `[-]` (deferred)는 reason
  필수. `[x]` / `[ ]` (reopen)는 reason optional. 한 바이트만 수정한
  새 body가 `body_markdown`에 들어간다.
- `scope_defer`는 `acceptance_transition`에 "어디로 옮겼는지"를 더한
  것. 같은 tx에서 `artifact_scope_edges`에 행 하나 기록하고, 소스
  마커는 `[-]`, reason은 `moved to <to_slug>: <agent-supplied reason>`
  으로 자동 조립.

---

## 2. 구체적 필드

### `body_patch` (default)
```json
{
  "shape": "body_patch",  // 또는 생략
  "body_markdown": "…",   // 또는 body_patch 블록
  "body_patch": {"mode": "section_replace|checkbox_toggle|append", …}
}
```

### `meta_patch`
```json
{
  "shape": "meta_patch",
  "update_of": "task-slug",
  "expected_version": 3,
  "commit_msg": "bump status to claimed_done",
  "task_meta": {"status": "claimed_done"},
  "completeness": "settled",
  "tags": ["launch-ready"],
  "artifact_meta": {"verification_state": "partially_verified"}
}
```
- `tags` / `completeness` / `task_meta` / `artifact_meta` 중 하나 이상
  필수 (`META_PATCH_EMPTY`).
- `body_markdown` / `body_patch`를 함께 보내면 `META_PATCH_HAS_BODY`.
- `claimed_done` 전이는 현재 body에 unchecked가 없을 때만 통과
  (preflight에서는 body가 비어 있으므로 handler가 재검).

### `acceptance_transition`
```json
{
  "shape": "acceptance_transition",
  "update_of": "task-slug",
  "expected_version": 3,
  "commit_msg": "mark step 2 partial — env setup covered",
  "acceptance_transition": {
    "checkbox_index": 2,
    "new_state": "[~]",
    "reason": "env setup covered; worker bootstrap separate PR"
  }
}
```
- `checkbox_index`는 4-state 전체에 걸친 document-order 인덱스 (0-base).
- `from_state`는 서버가 기록 — 요청에 포함 금지.
- Canonical-rewrite guard는 건너뛴다 (마커 1바이트 플립은 규범적 주장
  rewrite가 아님).

### `scope_defer`
```json
{
  "shape": "scope_defer",
  "update_of": "task-A",
  "expected_version": 5,
  "commit_msg": "move acceptance[2] to task-B",
  "scope_defer": {
    "checkbox_index": 2,
    "to_artifact": "task-B",      // slug 또는 pindoc:// URL
    "reason": "env setup 범위 밖"
  }
}
```
- 서버가 자동으로:
  1. body의 `acceptance[2]`를 `[-]`로 플립
  2. `artifact_scope_edges`에 `(from=task-A, from_item_ref=acceptance[2], to=task-B, reason=…)` insert
  3. shape_payload에 `to_artifact_id` / `to_artifact_slug` / `scope_defer_reason` 저장

---

## 3. revisions 테이블 변화 (migration 0017)

```sql
ALTER TABLE artifact_revisions
  ADD COLUMN revision_shape TEXT NOT NULL DEFAULT 'body_patch'
    CHECK (revision_shape IN ('body_patch','meta_patch','acceptance_transition','scope_defer')),
  ADD COLUMN shape_payload JSONB,
  ALTER COLUMN body_markdown DROP NOT NULL;
```

+ `0017`이 seed 경로(`0006 _template_*`, pre-fix `project_create.go`)로
  revision row 없이 남겨진 artifact 전체에 revision 1을 backfill. 이후
  `head()=0`은 legal state가 아니며 `expected_version=0`은 `FIELD_VALUE_
  RESERVED`로 거부.

---

## 4. 연관 기반 (Phase E·G·H)

- **Receipt** (`internal/pindoc/receipts`)은 이제 발급 시점의
  `(artifact_id, revision_number)` 리스트를 함께 저장. propose 시
  서버가 DB를 보고 "스냅샷 artifact 전부가 head를 넘어갔는가"를 판정
  — YES면 `RECEIPT_SUPERSEDED`, NO면 통과. 30분 clock TTL은 24h
  fallback으로 완화 (primary 신호가 revision move로 바뀜).
- **Warning severity** (`warning_severity.go`)은 응답의 `warnings[]`을
  error > warn > info 내림차순 정렬. 동일 severity 안에서는 emit order
  보존. 대응하는 `warning_severities[]`가 index-by-index 정렬된
  severity 문자열을 노출.
- **Toolset drift** — `pindoc.ping.toolset_version` 및
  `artifact.propose.toolset_version`이 `"<count>:<hash8>"` (sorted
  tool name SHA256 prefix) 반환. Claude Code schema cache가 세션 경계에
  묶여 있는 한계를 서버 쪽에서 직접 풀 수는 없지만, 값 변화를 보면
  "재접속 필요" 신호를 얻는다.

---

## 5. in-flight 질의 (Phase F 신규 tool)

`pindoc.scope.in_flight`는 project 전체에서 `[ ]` / `[~]` / (요청 시)
`[-]` 체크박스를 평탄화해 돌려준다. `[-]` 행은 `artifact_scope_edges`
조인으로 `forwarded_to_slug` + `forwarded_reason`을 같이 싣는다. Epic을
닫기 전에 "아직 안 끝난 항목 전부" 점검하거나, deferred trail을 따라갈
때 쓴다.

주의: 이 도구는 acceptance checkbox view다. Reader Task board의 대기열
(`task_meta.status` missing 또는 `open`)은 `pindoc.task.queue`가 canonical
source다. Agent가 "Task queue 완료"를 말하기 전에는 `pindoc.task.queue`
`pending_count == 0`을 확인해야 하며, `scope.in_flight` 결과만으로 완료를
판단하지 않는다.

필터:
- `state_filter`: `open` (기본, `[ ]` + `[~]`) / `unchecked` /
  `partial` / `all`
- `area_slug`: 특정 area로 제한
- `limit`: 기본 50, 최대 500. `truncated=true` 응답은 필터를 좁혀
  재호출.

`totals`는 **필터와 무관하게** 전체 카운트(`unchecked` / `partial` /
`deferred`)를 반환 — limit에 잘려도 denominator는 정확.

---

## 6. 지금 하면 안 되는 것

- `body_patch` 경로를 유지하면서 acceptance 박스를 body에서 **삭제**
  하지 말 것. `scope_defer`가 정직한 기록.
- `expected_version=0`을 seed artifact trick으로 쓰지 말 것. 0은 이제
  예약값 — `artifact.revisions`로 현재 head를 읽어 그 값을 넘긴다.
- `meta_patch`에 `body_markdown` / `body_patch`를 함께 싣지 말 것.
  의미상 충돌.
- `shape=scope_defer`로 자기 자신을 target으로 지정하지 말 것
  (`SCOPE_DEFER_SELF`).

---

## 7. 관련 코드

- 디스패치 + body 머터리얼라이즈: `internal/pindoc/mcp/tools/artifact_propose.go`
- meta_patch 핸들러: `internal/pindoc/mcp/tools/artifact_propose_meta_patch.go`
- 체크박스 파서 / transition: `internal/pindoc/mcp/tools/checkbox.go`
- scope_defer edge + in_flight tool: `internal/pindoc/mcp/tools/scope.go`
- receipt 스냅샷 / supersede check: `internal/pindoc/mcp/tools/receipt_snapshots.go`
- receipt store: `internal/pindoc/receipts/receipts.go`
- warning severity: `internal/pindoc/mcp/tools/warning_severity.go`
- toolset version: `internal/pindoc/mcp/tools/toolset.go`
