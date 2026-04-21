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
		"preflight.title_empty":       "✗ title is empty",
		"preflight.body_empty":        "✗ body_markdown is empty",
		"preflight.area_empty":        "✗ area_slug is empty — call pindoc.area.list and pick one",
		"preflight.author_empty":      "✗ author_id is empty (use 'claude-code', 'cursor', 'codex', ...)",
		"preflight.type_invalid":      "✗ type %q is not in the Tier A + Web SaaS pack whitelist",
		"preflight.completeness_invalid": "✗ completeness %q invalid; pick draft|partial|settled",
		"preflight.area_not_found":    "✗ area_slug %q does not exist in project %q",
		"preflight.conflict_exact":    "✗ An artifact with this exact title already exists (id=%s, slug=%s).",
		"preflight.task_acceptance":   "✗ Task body should include at least one acceptance criterion (a line matching '- [ ] ...').",
		"preflight.adr_sections":      "✗ Decision body should include 'Context' and 'Decision' sections (ADR convention).",

		"suggested.fix_all":           "Fix every item in the checklist.",
		"suggested.confirm_types":     "For type: call pindoc.project.current to confirm Tier A/B types your project accepts.",
		"suggested.use_misc":          "For area_slug: call pindoc.area.list and pick one; use 'misc' if nothing fits.",
		"suggested.list_areas":        "Call pindoc.area.list to see valid slugs.",
		"suggested.area_or_misc":      "If you truly need a new area, create it manually via the admin flow (Phase 3+) or use 'misc' for now.",
		"suggested.read_existing":     "Call pindoc.artifact.read with id_or_slug=%q to review the existing one.",
		"suggested.supersede":         "If this is an update, add a supersedes chain (Phase 2.x+, not yet implemented — for now archive the old one manually).",
		"suggested.pick_title":        "If this is a different document, pick a more specific title.",
	},
	"ko": {
		"preflight.title_empty":       "✗ title이 비어 있음",
		"preflight.body_empty":        "✗ body_markdown이 비어 있음",
		"preflight.area_empty":        "✗ area_slug가 비어 있음 — pindoc.area.list 호출 후 하나 고르기",
		"preflight.author_empty":      "✗ author_id가 비어 있음 ('claude-code', 'cursor', 'codex' 등 사용)",
		"preflight.type_invalid":      "✗ type %q 가 Tier A + Web SaaS pack 화이트리스트에 없음",
		"preflight.completeness_invalid": "✗ completeness %q 잘못됨; draft|partial|settled 중 선택",
		"preflight.area_not_found":    "✗ area_slug %q 는 프로젝트 %q 에 존재하지 않음",
		"preflight.conflict_exact":    "✗ 같은 제목의 artifact가 이미 존재함 (id=%s, slug=%s).",
		"preflight.task_acceptance":   "✗ Task body는 최소 1개의 acceptance criterion이 필요함 ('- [ ] ...' 라인 포함).",
		"preflight.adr_sections":      "✗ Decision body는 'Context' 와 'Decision' 섹션을 포함해야 함 (ADR 규약).",

		"suggested.fix_all":           "체크리스트의 모든 항목을 수정하세요.",
		"suggested.confirm_types":     "type 확인: pindoc.project.current를 호출해 프로젝트가 수용하는 Tier A/B 타입을 확인하세요.",
		"suggested.use_misc":          "area_slug 확인: pindoc.area.list를 호출해 하나 고르세요. 맞는 것이 없으면 'misc'를 사용.",
		"suggested.list_areas":        "pindoc.area.list를 호출해 유효한 slug를 확인하세요.",
		"suggested.area_or_misc":      "정말 새 area가 필요하면 admin 플로우(Phase 3+)로 생성하거나 지금은 'misc'를 사용하세요.",
		"suggested.read_existing":     "pindoc.artifact.read를 id_or_slug=%q 로 호출해 기존 artifact를 확인하세요.",
		"suggested.supersede":         "업데이트라면 supersede 체인을 연결하세요 (Phase 2.x+, 미구현 — 지금은 기존을 수동으로 archive).",
		"suggested.pick_title":        "다른 문서라면 더 구체적인 제목을 선택하세요.",
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
