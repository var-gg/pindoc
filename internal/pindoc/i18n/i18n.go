// Package i18n is a flat-map translator keyed by language + message key.
// No ICU, no pluralization, no date formatting — just string swaps. Good
// enough for the ko/en set we ship and keeps the runtime dependency-free.
//
// Keys are stable; when a key needs a parameter, callers use fmt.Sprintf
// on the returned template. When a key is missing for the requested
// language the English fallback kicks in.
package i18n

import "strings"

const defaultLang = "en"

type Bundle map[string]map[string]string // lang → key → template

var bundle = Bundle{
	"en": {
		"preflight.title_empty":          "✗ title is empty",
		"preflight.body_empty":           "✗ body_markdown is empty",
		"preflight.area_empty":           "✗ area_slug is empty — call pindoc.area.list and pick one",
		"preflight.author_empty":         "✗ author_id is empty (use 'claude-code', 'cursor', 'codex', ...)",
		"preflight.type_invalid":         "✗ type %q is not in the Tier A + Web SaaS pack whitelist",
		"preflight.completeness_invalid": "✗ completeness %q invalid; pick draft|partial|settled",
		"preflight.area_not_found":       "✗ area_slug %q does not exist in project %q",
		"preflight.conflict_exact":       "✗ An artifact with this exact title already exists (id=%s, slug=%s).",
		"preflight.task_acceptance":      "✗ Task body should include at least one acceptance criterion (a line matching '- [ ] ...').",
		"preflight.adr_sections":         "✗ Decision body should include 'Context' and 'Decision' sections (ADR convention).",

		"suggested.fix_all":         "Fix every item in the checklist.",
		"suggested.confirm_types":   "For type: call pindoc.project.current to confirm Tier A/B types your project accepts.",
		"suggested.use_misc":        "For area_slug: call pindoc.area.list and pick one; use 'misc' if nothing fits.",
		"suggested.list_areas":      "Call pindoc.area.list to see valid slugs.",
		"suggested.area_or_misc":    "If you truly need a new area, create it manually via the admin flow (Phase 3+) or use 'misc' for now.",
		"suggested.read_existing":   "Call pindoc.artifact.read with id_or_slug=%q to review the existing one.",
		"suggested.update_of_hint":  "If this is an update, call pindoc.artifact.propose again with update_of=%q and commit_msg=\"<why>\" to write a new revision.",
		"suggested.pick_title":      "If this is a different document, pick a more specific title.",
		"suggested.commit_msg_hint": "Provide commit_msg as a one-line rationale (e.g. 'clarify trade-offs' or 'add 2026-04-22 decision').",
		"suggested.verify_diff":     "If the body differs from what you intended, recompute and retry; otherwise there is nothing to record.",

		"preflight.update_needs_commit":   "✗ commit_msg is required when update_of is set",
		"preflight.update_target_missing": "✗ update_of target %q not found in this project",
		"preflight.no_changes":            "✗ the submitted body and title match the current head — nothing to record",

		"preflight.pin_path_empty":                "✗ pins[%d].path is empty — every pin must reference a file path",
		"preflight.pin_lines_invalid":             "✗ pins[%d] lines_start/lines_end must be >= 1 when set",
		"preflight.pin_lines_range":               "✗ pins[%d] lines_end must be >= lines_start",
		"preflight.pin_kind_invalid":              "✗ pins[%d].kind %q is not one of code | resource | url",
		"preflight.pin_url_invalid":               "✗ pins[%d] kind=url but path doesn't look like an absolute URL (missing '://')",
		"preflight.task_meta_wrong_type":          "✗ task_meta is only valid when type='Task'; remove it or change the type",
		"preflight.task_status_invalid":           "✗ task_meta.status %q is not one of open | claimed_done | verified | blocked | cancelled",
		"preflight.verify_no_verdict":             "✗ VerificationReport body must contain a verdict keyword (pass | partial | fail | 합격 | 부분 | 불합격)",
		"preflight.verified_via_verify_tool_only": "✗ task_meta.status='verified' cannot be set via artifact.propose. Call pindoc.artifact.verify with a VerificationReport instead — direct transition would collapse the Implementer ≠ Verifier invariant.",
		"preflight.claimed_done_incomplete":       "✗ task_meta.status='claimed_done' requires every acceptance checkbox to be checked (currently %d/%d). Toggle the remaining boxes or pick status=open.",
		"preflight.task_priority_invalid":         "✗ task_meta.priority %q is not one of p0 | p1 | p2 | p3",
		"preflight.task_due_at_invalid":           "✗ task_meta.due_at %q must be RFC3339 (e.g. 2026-04-30T00:00:00Z)",
		"preflight.rel_target_empty":              "✗ relates_to[%d].target_id is empty",
		"preflight.rel_invalid":                   "✗ relates_to[%d].relation %q is not one of implements|references|blocks|relates_to",
		"preflight.rel_target_missing":            "✗ relates_to[%d] target %q not found in this project",
		"preflight.expected_version_negative":     "✗ expected_version cannot be negative",
		"preflight.expected_version_reserved":     "✗ expected_version=0 is reserved — every artifact has at least revision 1 (migration 0017 backfilled seed rows). Call pindoc.artifact.revisions and pass the current head.",
		"preflight.shape_invalid":                 "✗ shape %q is not one of body_patch | meta_patch | acceptance_transition | scope_defer",
		"preflight.shape_needs_update":            "✗ shape requires update_of: meta_patch / acceptance_transition / scope_defer all mutate an existing artifact; create path only accepts body_patch",
		"preflight.shape_not_implemented":         "✗ shape %q is declared but not yet implemented — Phase C/D/F will land it. Use body_patch (or omit) for now.",
		"preflight.ver_conflict":                  "✗ expected_version=%d but current head is %d — re-read and retry",
		"preflight.need_ver":                      "✗ expected_version is required on the update path (current head = %d). Pass it back via artifact.propose input.",
		"preflight.update_supersede_exclusive":    "✗ update_of and supersede_of are mutually exclusive; pick one",
		"preflight.supersede_target_missing":      "✗ supersede_of target %q not found in this project",

		"suggested.pick_one_mode":        "update_of appends a revision to the same artifact; supersede_of archives the old one and creates a replacement.",
		"suggested.read_existing_rel":    "Call pindoc.artifact.read on each relates_to target to confirm it exists and belongs to this project.",
		"suggested.reread_before_update": "Call pindoc.artifact.revisions, find the current max revision_number, and re-submit with expected_version set to it (or omit to disable optimistic lock).",

		"preflight.no_search_receipt":     "✗ basis.search_receipt is missing — a create path requires a valid receipt from pindoc.artifact.search or pindoc.context.for_task in the current session",
		"preflight.receipt_unknown":       "✗ basis.search_receipt is not recognised — the token may have been issued by a different MCP session or has been swept",
		"preflight.receipt_expired":       "✗ basis.search_receipt is expired (TTL 10 minutes) — re-run search/context and retry",
		"preflight.receipt_wrong_project": "✗ basis.search_receipt belongs to a different project",
		"preflight.possible_dup":          "✗ a near-duplicate artifact %q already exists (cosine distance %.3f) — read it and either update_of or prove the new one is distinct",
		"preflight.debug_no_repro":        "✗ Debug body should include reproduction or symptom info (keywords: reproduction / repro / symptom / 재현 / 증상)",
		"preflight.debug_no_resolution":   "✗ Debug body should include resolution or root cause (keywords: resolution / root cause / 원인 / 해결)",

		"suggested.call_search_first": "Call pindoc.artifact.search or pindoc.context.for_task (same project, same session) first; pass the returned search_receipt as basis.search_receipt.",
		"suggested.read_similar":      "Read the near-duplicate candidate(s) and either (a) update_of/supersede_of it, or (b) narrow the new artifact's scope so it isn't semantically covered.",
	},
	"ko": {
		"preflight.title_empty":          "✗ title이 비어 있음",
		"preflight.body_empty":           "✗ body_markdown이 비어 있음",
		"preflight.area_empty":           "✗ area_slug가 비어 있음 — pindoc.area.list 호출 후 하나 고르기",
		"preflight.author_empty":         "✗ author_id가 비어 있음 ('claude-code', 'cursor', 'codex' 등 사용)",
		"preflight.type_invalid":         "✗ type %q 가 Tier A + Web SaaS pack 화이트리스트에 없음",
		"preflight.completeness_invalid": "✗ completeness %q 잘못됨; draft|partial|settled 중 선택",
		"preflight.area_not_found":       "✗ area_slug %q 는 프로젝트 %q 에 존재하지 않음",
		"preflight.conflict_exact":       "✗ 같은 제목의 artifact가 이미 존재함 (id=%s, slug=%s).",
		"preflight.task_acceptance":      "✗ Task body는 최소 1개의 acceptance criterion이 필요함 ('- [ ] ...' 라인 포함).",
		"preflight.adr_sections":         "✗ Decision body는 'Context' 와 'Decision' 섹션을 포함해야 함 (ADR 규약).",

		"suggested.fix_all":         "체크리스트의 모든 항목을 수정하세요.",
		"suggested.confirm_types":   "type 확인: pindoc.project.current를 호출해 프로젝트가 수용하는 Tier A/B 타입을 확인하세요.",
		"suggested.use_misc":        "area_slug 확인: pindoc.area.list를 호출해 하나 고르세요. 맞는 것이 없으면 'misc'를 사용.",
		"suggested.list_areas":      "pindoc.area.list를 호출해 유효한 slug를 확인하세요.",
		"suggested.area_or_misc":    "정말 새 area가 필요하면 admin 플로우(Phase 3+)로 생성하거나 지금은 'misc'를 사용하세요.",
		"suggested.read_existing":   "pindoc.artifact.read를 id_or_slug=%q 로 호출해 기존 artifact를 확인하세요.",
		"suggested.update_of_hint":  "업데이트라면 pindoc.artifact.propose를 다시 호출하되 update_of=%q + commit_msg=\"<왜>\" 를 넘겨 새 revision을 작성하세요.",
		"suggested.pick_title":      "다른 문서라면 더 구체적인 제목을 선택하세요.",
		"suggested.commit_msg_hint": "commit_msg에 한 줄 사유를 넣으세요 (예: 'trade-off 명확화', '2026-04-22 결정 추가').",
		"suggested.verify_diff":     "body가 원래 의도와 다르면 다시 계산 후 재시도; 맞다면 기록할 것이 없습니다.",

		"preflight.update_needs_commit":   "✗ update_of 지정 시 commit_msg 필수",
		"preflight.update_target_missing": "✗ update_of 대상 %q 를 이 프로젝트에서 찾을 수 없음",
		"preflight.no_changes":            "✗ 제출된 body와 title이 현재 head와 동일 — 기록할 변경 없음",

		"preflight.pin_path_empty":                "✗ pins[%d].path가 비어 있음 — 모든 pin은 파일 경로가 필요함",
		"preflight.pin_lines_invalid":             "✗ pins[%d] lines_start/lines_end는 1 이상이어야 함",
		"preflight.pin_lines_range":               "✗ pins[%d] lines_end는 lines_start보다 크거나 같아야 함",
		"preflight.pin_kind_invalid":              "✗ pins[%d].kind %q 는 code | resource | url 중 하나여야 함",
		"preflight.pin_url_invalid":               "✗ pins[%d] kind=url 인데 path가 절대 URL 형식이 아님 ('://' 누락)",
		"preflight.task_meta_wrong_type":          "✗ task_meta는 type='Task' 에서만 유효. 제거하거나 type 변경",
		"preflight.task_status_invalid":           "✗ task_meta.status %q 는 open | claimed_done | verified | blocked | cancelled 중 하나여야 함",
		"preflight.verify_no_verdict":             "✗ VerificationReport body 는 판정 키워드(pass | partial | fail | 합격 | 부분 | 불합격) 중 하나를 포함해야 함",
		"preflight.verified_via_verify_tool_only": "✗ task_meta.status='verified' 는 artifact.propose로 설정할 수 없음. pindoc.artifact.verify를 VerificationReport와 함께 호출해야 함 — 직접 전이는 Implementer ≠ Verifier 원칙을 깨뜨림.",
		"preflight.claimed_done_incomplete":       "✗ task_meta.status='claimed_done' 전이는 acceptance checkbox 전부 체크 필요(현재 %d/%d). 남은 박스를 체크하거나 status=open 으로.",
		"preflight.task_priority_invalid":         "✗ task_meta.priority %q 는 p0 | p1 | p2 | p3 중 하나여야 함",
		"preflight.task_due_at_invalid":           "✗ task_meta.due_at %q 는 RFC3339 형식 필요 (예: 2026-04-30T00:00:00Z)",
		"preflight.rel_target_empty":              "✗ relates_to[%d].target_id가 비어 있음",
		"preflight.rel_invalid":                   "✗ relates_to[%d].relation %q 는 implements|references|blocks|relates_to 중 하나여야 함",
		"preflight.rel_target_missing":            "✗ relates_to[%d] 대상 %q 를 이 프로젝트에서 찾을 수 없음",
		"preflight.expected_version_negative":     "✗ expected_version은 음수일 수 없음",
		"preflight.expected_version_reserved":     "✗ expected_version=0은 예약값 — 모든 artifact는 revision 1 이상을 가짐 (migration 0017이 seed row도 backfill). pindoc.artifact.revisions로 현재 head 확인 후 전달.",
		"preflight.shape_invalid":                 "✗ shape %q 는 body_patch | meta_patch | acceptance_transition | scope_defer 중 하나여야 함",
		"preflight.shape_needs_update":            "✗ shape는 update_of가 필요함: meta_patch / acceptance_transition / scope_defer는 기존 artifact를 변형. create 경로는 body_patch만 허용",
		"preflight.shape_not_implemented":         "✗ shape %q 는 선언됐지만 아직 구현되지 않음 — Phase C/D/F에서 구현 예정. 지금은 body_patch 사용 (또는 생략).",
		"preflight.ver_conflict":                  "✗ expected_version=%d 이지만 현재 head는 %d — 다시 읽고 재시도",
		"preflight.need_ver":                      "✗ update 경로에는 expected_version 필수 (현재 head=%d). artifact.propose 입력에 포함하세요.",
		"preflight.update_supersede_exclusive":    "✗ update_of와 supersede_of는 동시 사용 불가 — 하나만 선택",
		"preflight.supersede_target_missing":      "✗ supersede_of 대상 %q 를 이 프로젝트에서 찾을 수 없음",

		"suggested.pick_one_mode":        "update_of는 같은 artifact에 revision 추가; supersede_of는 기존 artifact를 archive하고 새 artifact를 생성.",
		"suggested.read_existing_rel":    "각 relates_to 대상에 대해 pindoc.artifact.read를 호출해 이 프로젝트에 존재하는지 확인하세요.",
		"suggested.reread_before_update": "pindoc.artifact.revisions로 현재 max revision_number를 확인한 뒤 expected_version에 그 값을 넣고 재시도 (optimistic lock 비활성화하려면 필드 자체를 생략).",

		"preflight.no_search_receipt":     "✗ basis.search_receipt가 없음 — create 경로는 현재 세션에서 발급된 pindoc.artifact.search 또는 pindoc.context.for_task의 receipt 필수",
		"preflight.receipt_unknown":       "✗ basis.search_receipt를 인식하지 못함 — 다른 MCP 세션에서 발급됐거나 이미 sweep됨",
		"preflight.receipt_expired":       "✗ basis.search_receipt 만료 (TTL 10분) — search/context 재호출 후 재시도",
		"preflight.receipt_wrong_project": "✗ basis.search_receipt가 다른 프로젝트의 것임",
		"preflight.possible_dup":          "✗ 유사한 기존 artifact %q 존재 (cosine distance %.3f) — 읽어본 뒤 update_of 하거나 새 artifact가 다르다는 근거를 제시",
		"preflight.debug_no_repro":        "✗ Debug body에 재현/증상 정보 필요 (키워드: reproduction / repro / symptom / 재현 / 증상)",
		"preflight.debug_no_resolution":   "✗ Debug body에 해결/원인 정보 필요 (키워드: resolution / root cause / 원인 / 해결)",

		"suggested.call_search_first": "먼저 pindoc.artifact.search 또는 pindoc.context.for_task를 (같은 프로젝트, 같은 세션) 호출하고 반환된 search_receipt를 basis.search_receipt로 전달하세요.",
		"suggested.read_similar":      "유사 후보를 읽어본 뒤 (a) update_of/supersede_of 하거나 (b) 새 artifact의 범위를 좁혀 기존 것에 포괄되지 않도록 하세요.",
	},
}

// T looks up a translation; fmt.Sprintf-style callers still call
// fmt.Sprintf on the returned template. Unknown keys fall through to the
// English bundle, then to the literal key string.
func T(lang, key string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if m, ok := bundle[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := bundle[defaultLang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}
