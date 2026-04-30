package tools

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/var-gg/pindoc/internal/pindoc/i18n"
)

// AcceptanceTransitionInput is the agent-facing payload for shape=
// acceptance_transition. Toggles a single acceptance checkbox without
// re-sending the whole body. CheckboxIndex is 0-based across the full
// body, counting all 4-state markers ([ ] | [x] | [~] | [-]) in document
// order so transitions and future body_patch.checkbox_toggle calls agree
// on the same index space.
//
// NewState takes one of:
//
//	"[ ]" — reopen (clear a previously resolved item)
//	"[x]" — mark done
//	"[~]" — mark partial (item only partly addressed, reason required)
//	"[-]" — mark deferred (scope moved elsewhere, reason required)
//
// Reason is stored in the revision's shape_payload. It is REQUIRED for
// partial and deferred transitions (otherwise the handler rejects with
// ACCEPT_TRANSITION_REASON_REQUIRED) and OPTIONAL for done / reopen.
type AcceptanceTransitionInput struct {
	CheckboxIndex      *int   `json:"checkbox_index,omitempty" jsonschema:"0-based index across all 4-state acceptance checkboxes in document order"`
	CheckboxLabelMatch string `json:"checkbox_label_match,omitempty" jsonschema:"fuzzy label selector; case-insensitive substring/token match over unresolved checkbox labels; exactness-sensitive callers should use checkbox_index"`
	NewState           string `json:"new_state" jsonschema:"one of '[ ]' | '[x]' | '[~]' | '[-]'"`
	Reason             string `json:"reason,omitempty" jsonschema:"required for [~] and [-]; free-form justification stored on the revision"`
}

// ScopeDeferInput is the Phase F payload for shape=scope_defer. Moves an
// acceptance checkbox to another artifact: rewrites the source checkbox
// to [-] (synthesized AcceptanceTransition under the hood), records a
// row in artifact_scope_edges pointing at the target, and keeps both as
// one atomic revision so the graph never disagrees with the body.
//
// Reason is required — scope moves without explanation become noise in
// the in-flight query. The final acceptance marker's reason is composed
// server-side as "moved to <to_artifact_slug>: <reason>" so readers see
// both the destination and the rationale inline.
type ScopeDeferInput struct {
	CheckboxIndex *int   `json:"checkbox_index,omitempty" jsonschema:"0-based index across all 4-state acceptance checkboxes"`
	ToArtifact    string `json:"to_artifact" jsonschema:"slug or pindoc:// URL of the artifact that absorbs this acceptance item"`
	Reason        string `json:"reason" jsonschema:"short justification — why the item moved, not just where"`
}

// checkboxHit is a positional record for a 4-state checkbox found while
// walking the body. lineIndex is the line within strings.Split(body,"\n")
// containing the marker; markerByteOffset is the byte offset of '[' inside
// that line so the caller can rewrite the single character between
// the brackets without touching surrounding text.
type checkboxHit struct {
	lineIndex        int
	markerByteOffset int
	marker           byte
}

type AcceptanceLabelRef struct {
	Index       int    `json:"index"`
	State       string `json:"state"`
	Label       string `json:"label"`
	IndentLevel int    `json:"indent_level"`
}

// iterateCheckboxes returns every 4-state checkbox in the body in document
// order. Bullet markers accepted: `-`, `*`, `+`. Markers recognised:
// ' ', 'x', 'X', '~', '-'. Lines that don't match this shape are ignored
// — prose, code blocks, etc. don't inflate the index.
func iterateCheckboxes(body string) []checkboxHit {
	out := []checkboxHit{}
	lines := strings.Split(body, "\n")
	for li, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		leftPad := 0
		for leftPad < len(line) && (line[leftPad] == ' ' || line[leftPad] == '\t') {
			leftPad++
		}
		if leftPad >= len(line) {
			continue
		}
		bullet := line[leftPad]
		if bullet != '-' && bullet != '*' && bullet != '+' {
			continue
		}
		j := leftPad + 1
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		if j+2 >= len(line) || line[j] != '[' || line[j+2] != ']' {
			continue
		}
		m := line[j+1]
		switch m {
		case ' ', 'x', 'X', '~', '-':
			out = append(out, checkboxHit{
				lineIndex:        li,
				markerByteOffset: j,
				marker:           m,
			})
		}
	}
	return out
}

func acceptanceLabels(body string, unresolvedOnly bool) []AcceptanceLabelRef {
	hits := iterateCheckboxes(body)
	if len(hits) == 0 {
		return nil
	}
	lines := strings.Split(body, "\n")
	out := make([]AcceptanceLabelRef, 0, len(hits))
	for i, cb := range hits {
		state := markerState(cb.marker)
		if unresolvedOnly && state != "[ ]" && state != "[~]" {
			continue
		}
		label := ""
		indent := 0
		if cb.lineIndex < len(lines) {
			line := strings.TrimRight(lines[cb.lineIndex], "\r")
			labelStart := cb.markerByteOffset + 3
			if labelStart < len(line) {
				label = strings.TrimSpace(line[labelStart:])
			}
			indent = acceptanceIndentLevel(line)
		}
		out = append(out, AcceptanceLabelRef{
			Index:       i,
			State:       state,
			Label:       label,
			IndentLevel: indent,
		})
	}
	return out
}

func unresolvedAcceptanceLabels(body string) []AcceptanceLabelRef {
	return acceptanceLabels(body, true)
}

func acceptanceIndentLevel(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

func matchingAcceptanceLabels(body, query string) []AcceptanceLabelRef {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	labels := unresolvedAcceptanceLabels(body)
	matches := make([]AcceptanceLabelRef, 0, len(labels))
	for _, label := range labels {
		if acceptanceLabelMatches(query, label.Label) {
			matches = append(matches, label)
		}
	}
	return matches
}

func acceptanceLabelMatches(query, label string) bool {
	q := normalizeAcceptanceLabelText(query)
	l := normalizeAcceptanceLabelText(label)
	if q == "" || l == "" {
		return false
	}
	if strings.Contains(l, q) {
		return true
	}
	qTokens := strings.Fields(q)
	lTokens := strings.Fields(l)
	if len(qTokens) == 0 || len(lTokens) == 0 {
		return false
	}
	labelSet := map[string]struct{}{}
	for _, tok := range lTokens {
		labelSet[tok] = struct{}{}
	}
	for _, tok := range qTokens {
		if _, ok := labelSet[tok]; !ok {
			return false
		}
	}
	return true
}

func normalizeAcceptanceLabelText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func resolveAcceptanceLabelMatch(body string, label string) (int, []AcceptanceLabelRef, []AcceptanceLabelRef, string) {
	unresolved := unresolvedAcceptanceLabels(body)
	matches := make([]AcceptanceLabelRef, 0, len(unresolved))
	for _, candidate := range unresolved {
		if acceptanceLabelMatches(label, candidate.Label) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return 0, matches, unresolved, "ACCEPTANCE_LABEL_NOT_FOUND"
	case 1:
		return matches[0].Index, matches, unresolved, ""
	default:
		return 0, matches, unresolved, "ACCEPTANCE_LABEL_AMBIGUOUS"
	}
}

func labelMatchesCheckboxIndex(body string, index int, label string) bool {
	for _, candidate := range acceptanceLabels(body, false) {
		if candidate.Index == index {
			return acceptanceLabelMatches(label, candidate.Label)
		}
	}
	return false
}

func acceptanceLabelWarning(code, label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return code
	}
	return code + ":" + label
}

func uniqueSortedInts(values []int) []int {
	out := append([]int(nil), values...)
	sort.Ints(out)
	return out
}

// countAcceptanceResolution returns (resolved, total) where resolved counts
// checkboxes in any terminal state ([x] done, [~] partial, [-] deferred)
// and total is every 4-state checkbox. Used by the Task claimed_done
// evidence gate: an unchecked [ ] box blocks the transition, but a [~]
// or [-] counts as a judgment call the agent recorded via
// AcceptanceTransition and isn't blocking.
func countAcceptanceResolution(body string) (resolved, total int) {
	for _, cb := range iterateCheckboxes(body) {
		total++
		switch cb.marker {
		case 'x', 'X', '~', '-':
			resolved++
		}
	}
	return resolved, total
}

// parseAcceptanceState maps the agent-facing marker string (`[ ]`, `[x]`,
// `[~]`, `[-]`) to the single marker byte stored between the brackets in
// body text. Returns ok=false for unknown shapes so the caller can emit
// ACCEPT_TRANSITION_STATE_INVALID. Case-sensitive on the 'x' per markdown
// convention — `[x]` is canonical, `[X]` is tolerated on read only.
func parseAcceptanceState(s string) (byte, bool) {
	switch strings.TrimSpace(s) {
	case "[ ]":
		return ' ', true
	case "[x]", "[X]":
		return 'x', true
	case "[~]":
		return '~', true
	case "[-]":
		return '-', true
	}
	return 0, false
}

// applyAcceptanceTransition rewrites a single checkbox marker in the body
// and returns (newBody, fromMarker, errCode). fromMarker is the marker
// that was replaced, stored on the revision's shape_payload so audit
// queries can show "[ ] → [-]" without re-diffing bodies. errCode is
// populated on validation failure; the caller translates it into a
// not_ready response.
func applyAcceptanceTransition(prev string, in *AcceptanceTransitionInput) (string, byte, string) {
	if in == nil {
		return "", 0, "ACCEPT_TRANSITION_REQUIRED"
	}
	if in.CheckboxIndex == nil {
		return "", 0, "ACCEPT_TRANSITION_INDEX_REQUIRED"
	}
	target := *in.CheckboxIndex
	if target < 0 {
		return "", 0, "ACCEPT_TRANSITION_INDEX_NEGATIVE"
	}
	newMarker, ok := parseAcceptanceState(in.NewState)
	if !ok {
		return "", 0, "ACCEPT_TRANSITION_STATE_INVALID"
	}
	if (newMarker == '~' || newMarker == '-') && strings.TrimSpace(in.Reason) == "" {
		return "", 0, "ACCEPT_TRANSITION_REASON_REQUIRED"
	}

	hits := iterateCheckboxes(prev)
	if target >= len(hits) {
		return "", 0, "ACCEPT_TRANSITION_INDEX_OUT_OF_RANGE"
	}
	cb := hits[target]
	from := cb.marker
	// Canonicalise 'X' → 'x' when comparing; we never emit 'X' ourselves.
	fromCanonical := from
	if fromCanonical == 'X' {
		fromCanonical = 'x'
	}
	if fromCanonical == newMarker {
		return "", 0, "ACCEPT_TRANSITION_NOOP"
	}

	lines := strings.Split(prev, "\n")
	line := lines[cb.lineIndex]
	// Rewrite single marker byte between the brackets. markerByteOffset
	// points at '['; the marker lives at offset+1.
	mo := cb.markerByteOffset + 1
	lines[cb.lineIndex] = line[:mo] + string(newMarker) + line[mo+1:]
	return strings.Join(lines, "\n"), fromCanonical, ""
}

// acceptanceTransitionChecklist maps the errCode emitted by
// applyAcceptanceTransition to a human-readable preflight-style message.
// Keeps the handler thin — callers just look up the code and push.
func acceptanceTransitionChecklist(lang, code string) string {
	key := ""
	switch code {
	case "ACCEPT_TRANSITION_REQUIRED":
		key = "preflight.accept_transition_required"
	case "ACCEPT_TRANSITION_INDEX_REQUIRED":
		key = "preflight.accept_transition_index_required"
	case "ACCEPT_TRANSITION_INDEX_NEGATIVE":
		key = "preflight.accept_transition_index_negative"
	case "ACCEPT_TRANSITION_INDEX_OUT_OF_RANGE":
		key = "preflight.accept_transition_index_range"
	case "ACCEPT_TRANSITION_STATE_INVALID":
		key = "preflight.accept_transition_state_invalid"
	case "ACCEPT_TRANSITION_REASON_REQUIRED":
		key = "preflight.accept_transition_reason_required"
	case "ACCEPT_TRANSITION_NOOP":
		key = "preflight.accept_transition_noop"
	case "ACCEPTANCE_LABEL_NOT_FOUND":
		return "✗ checkbox_label_match did not match any unresolved acceptance label."
	case "ACCEPTANCE_LABEL_AMBIGUOUS":
		return "✗ checkbox_label_match matched more than one unresolved acceptance label; use checkbox_index or a more specific label."
	default:
		return fmt.Sprintf("✗ acceptance transition rejected: %s", code)
	}
	return i18n.T(lang, key)
}
