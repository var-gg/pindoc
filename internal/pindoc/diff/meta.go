package diff

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	RevisionTypeTextEdit         = "text_edit"
	RevisionTypeAcceptanceToggle = "acceptance_toggle"
	RevisionTypeMetaChange       = "meta_change"
	RevisionTypeSystemAuto       = "system_auto"
	RevisionTypeMixed            = "mixed"
)

// MetaDeltaEntry is one key-level metadata movement between two revisions.
// Before/After intentionally stay JSON-shaped so callers can render strings,
// arrays, null clears, and future structured metadata without another API
// revision.
type MetaDeltaEntry struct {
	Key    string `json:"key"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// RevisionMetaSnapshot is the metadata subset available from
// artifact_revisions. It is enough to classify revision history rows and to
// build a best-effort meta_delta for shape_payload-backed metadata changes.
type RevisionMetaSnapshot struct {
	RevisionNumber int
	Tags           []string
	Completeness   string
	Shape          string
	ShapePayload   json.RawMessage
}

// MetaDeltaForRange folds revision metadata from rev 1 through toRev and
// returns the fields that moved after fromRev. task_meta/artifact_meta are
// reconstructed from shape_payload, so legacy body_patch revisions that
// updated those JSONB columns before shape_payload existed remain unknown.
func MetaDeltaForRange(fromRev, toRev int, snapshots []RevisionMetaSnapshot) []MetaDeltaEntry {
	if len(snapshots) == 0 || toRev < fromRev {
		return []MetaDeltaEntry{}
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].RevisionNumber < snapshots[j].RevisionNumber
	})

	state := map[string]any{}
	changes := map[string]MetaDeltaEntry{}

	apply := func(key string, next any, inRange bool) {
		prev, hadPrev := state[key]
		if !hadPrev {
			prev = nil
		}
		if valueEqual(prev, next) {
			state[key] = cloneMetaValue(next)
			return
		}
		if inRange {
			if existing, ok := changes[key]; ok {
				existing.After = cloneMetaValue(next)
				changes[key] = existing
			} else {
				changes[key] = MetaDeltaEntry{
					Key:    key,
					Before: cloneMetaValue(prev),
					After:  cloneMetaValue(next),
				}
			}
		}
		state[key] = cloneMetaValue(next)
	}

	for _, snap := range snapshots {
		if snap.RevisionNumber > toRev {
			break
		}
		inRange := snap.RevisionNumber > fromRev
		apply("tags", cloneStringSlice(snap.Tags), inRange)
		if snap.Completeness != "" {
			apply("completeness", snap.Completeness, inRange)
		}
		for key, value := range FlattenMetaPayload(snap.ShapePayload) {
			apply(key, value, inRange)
		}
	}

	out := make([]MetaDeltaEntry, 0, len(changes))
	for _, entry := range changes {
		if !valueEqual(entry.Before, entry.After) {
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

// MetaChangedBetween classifies a single row relative to its predecessor for
// history badges. It uses revision snapshots plus shape_payload; body changes
// are supplied separately by the caller through body_hash comparison.
func MetaChangedBetween(previous, current RevisionMetaSnapshot) bool {
	if previous.RevisionNumber > 0 {
		if !valueEqual(cloneStringSlice(previous.Tags), cloneStringSlice(current.Tags)) {
			return true
		}
		if previous.Completeness != current.Completeness {
			return true
		}
	}
	if len(FlattenMetaPayload(current.ShapePayload)) > 0 {
		return true
	}
	return strings.TrimSpace(current.Shape) == "meta_patch"
}

// FlattenMetaPayload returns only metadata keys from a revision
// shape_payload. Acceptance-transition payload fields are intentionally
// ignored here; T2 owns semantic checklist rendering.
func FlattenMetaPayload(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return out
	}
	for key, value := range payload {
		switch key {
		case "tags", "completeness":
			out[key] = value
		case "task_meta", "artifact_meta":
			if fields, ok := value.(map[string]any); ok {
				for field, fieldValue := range fields {
					out[key+"."+field] = fieldValue
				}
			}
		}
	}
	return out
}

// ClassifyRevisionType maps the low-level revision shape plus detected body
// and metadata movement into the compact UI vocabulary.
func ClassifyRevisionType(shape, commitMsg string, bodyChanged, metaChanged bool) string {
	switch strings.TrimSpace(shape) {
	case "acceptance_transition", "scope_defer":
		return RevisionTypeAcceptanceToggle
	}
	if bodyChanged && metaChanged {
		return RevisionTypeMixed
	}
	if metaChanged {
		if hasSystemCommitPrefix(commitMsg) {
			return RevisionTypeSystemAuto
		}
		return RevisionTypeMetaChange
	}
	return RevisionTypeTextEdit
}

func hasSystemCommitPrefix(commitMsg string) bool {
	s := strings.TrimSpace(commitMsg)
	return strings.HasPrefix(s, "Repair:") || strings.HasPrefix(s, "Auto:")
}

func valueEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneMetaValue(v any) any {
	switch t := v.(type) {
	case []string:
		return cloneStringSlice(t)
	case nil:
		return nil
	default:
		buf, err := json.Marshal(t)
		if err != nil {
			return t
		}
		var out any
		if err := json.Unmarshal(buf, &out); err != nil {
			return t
		}
		return out
	}
}
