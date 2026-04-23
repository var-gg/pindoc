package tools

import "strings"

// RevisionShape discriminates the kind of mutation an artifact revision
// records. Phase B introduces the enum + default dispatch; later phases
// light up their respective shapes:
//
//   - ShapeBodyPatch (Phase A/B) — section_replace / checkbox_toggle /
//     append, or the legacy "send body_markdown whole" path
//   - ShapeMetaPatch (Phase C) — status / completeness / tags / task_meta
//     without re-encoding body_markdown
//   - ShapeAcceptanceTransition (Phase D) — toggle a single acceptance
//     checkbox ([ ] → [x]/[~]/[-]) with reason, no full-body resend
//   - ShapeScopeDefer (Phase F) — move an acceptance item to a target
//     artifact and record the graph edge
//
// body_patch is the default so omitting shape on propose preserves the
// legacy "send body + meta together" agent contract.
type RevisionShape string

const (
	ShapeBodyPatch            RevisionShape = "body_patch"
	ShapeMetaPatch            RevisionShape = "meta_patch"
	ShapeAcceptanceTransition RevisionShape = "acceptance_transition"
	ShapeScopeDefer           RevisionShape = "scope_defer"
)

var validShapes = map[RevisionShape]struct{}{
	ShapeBodyPatch:            {},
	ShapeMetaPatch:            {},
	ShapeAcceptanceTransition: {},
	ShapeScopeDefer:           {},
}

// parseShape resolves the propose input's optional shape field. Empty
// input defaults to body_patch. Unknown values return ok=false so the
// caller can push a preflight failure with code SHAPE_INVALID.
func parseShape(raw string) (RevisionShape, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ShapeBodyPatch, true
	}
	shape := RevisionShape(s)
	if _, ok := validShapes[shape]; !ok {
		return "", false
	}
	return shape, true
}

// isTemplateArtifact returns true for artifacts whose slug marks them as
// type templates (migration 0006 seeds + project_create propagation).
// Used by the canonical-rewrite guard to suppress false positives: a
// template IS the source of truth for canonical sections, so editing
// ## Decision / ## Root cause on _template_* is the intended workflow,
// not a guarded rewrite of a verified claim.
func isTemplateArtifact(slug string) bool {
	return strings.HasPrefix(slug, "_template_")
}
