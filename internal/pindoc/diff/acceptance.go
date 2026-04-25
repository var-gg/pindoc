package diff

import (
	"encoding/json"
	"regexp"
	"strings"
)

var acceptanceLineRe = regexp.MustCompile(`^\s*[-*+]\s+\[([ xX~-])\]\s*(.*)$`)

type AcceptanceChecklist struct {
	Items        []AcceptanceItem `json:"items"`
	HasChange    bool             `json:"has_change"`
	ChangedIndex *int             `json:"changed_index,omitempty"`
	Reason       string           `json:"reason,omitempty"`
}

type AcceptanceItem struct {
	Index     int    `json:"index"`
	State     string `json:"state"`
	Text      string `json:"text"`
	Changed   bool   `json:"changed,omitempty"`
	FromState string `json:"from_state,omitempty"`
	ToState   string `json:"to_state,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type acceptancePayload struct {
	CheckboxIndex *int   `json:"checkbox_index"`
	FromState     string `json:"from_state"`
	NewState      string `json:"new_state"`
	Reason        string `json:"reason"`
}

// AcceptanceChecklistSummary returns the checklist state at the "after"
// revision plus semantic markers for items changed by this revision. The
// shape_payload path is authoritative for acceptance_transition/scope_defer;
// the fallback body comparison keeps mixed/manual checkbox edits visible.
func AcceptanceChecklistSummary(before, after, shape string, payload json.RawMessage) AcceptanceChecklist {
	beforeItems := ParseAcceptanceItems(before)
	afterItems := ParseAcceptanceItems(after)
	out := AcceptanceChecklist{Items: afterItems}
	if len(afterItems) == 0 {
		return out
	}

	transition := parseAcceptancePayload(payload)
	if transition.CheckboxIndex != nil && *transition.CheckboxIndex >= 0 && *transition.CheckboxIndex < len(out.Items) {
		idx := *transition.CheckboxIndex
		markAcceptanceItem(&out, idx, markerToState(transition.FromState), normalizeState(transition.NewState), transition.Reason)
		return out
	}

	// Fallback: detect state changes by document-order index. This covers
	// body_patch.checkbox_toggle and mixed revisions that predate the typed
	// acceptance_transition shape.
	for i := range out.Items {
		if i >= len(beforeItems) {
			continue
		}
		if beforeItems[i].State != out.Items[i].State {
			markAcceptanceItem(&out, i, beforeItems[i].State, out.Items[i].State, "")
		}
	}
	_ = shape // reserved for future shape-specific fallbacks.
	return out
}

func ParseAcceptanceItems(body string) []AcceptanceItem {
	lines := strings.Split(body, "\n")
	items := []AcceptanceItem{}
	for _, line := range lines {
		match := acceptanceLineRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if len(match) != 3 {
			continue
		}
		items = append(items, AcceptanceItem{
			Index: len(items),
			State: markerToState(match[1]),
			Text:  strings.TrimSpace(match[2]),
		})
	}
	return items
}

func parseAcceptancePayload(raw json.RawMessage) acceptancePayload {
	var payload acceptancePayload
	if len(raw) == 0 {
		return payload
	}
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func markAcceptanceItem(out *AcceptanceChecklist, idx int, fromState, toState, reason string) {
	out.HasChange = true
	out.ChangedIndex = &idx
	out.Reason = strings.TrimSpace(reason)
	out.Items[idx].Changed = true
	out.Items[idx].FromState = fromState
	out.Items[idx].ToState = toState
	out.Items[idx].Reason = out.Reason
}

func markerToState(marker string) string {
	switch strings.TrimSpace(marker) {
	case "x", "X", "[x]", "[X]":
		return "[x]"
	case "~", "[~]":
		return "[~]"
	case "-", "[-]":
		return "[-]"
	default:
		return "[ ]"
	}
}

func normalizeState(state string) string {
	return markerToState(state)
}
