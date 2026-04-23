package tools

import (
	"fmt"
	"strings"

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
	CheckboxIndex *int   `json:"checkbox_index,omitempty" jsonschema:"0-based index across all 4-state acceptance checkboxes in document order"`
	NewState      string `json:"new_state" jsonschema:"one of '[ ]' | '[x]' | '[~]' | '[-]'"`
	Reason        string `json:"reason,omitempty" jsonschema:"required for [~] and [-]; free-form justification stored on the revision"`
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
	default:
		return fmt.Sprintf("✗ acceptance transition rejected: %s", code)
	}
	return i18n.T(lang, key)
}
