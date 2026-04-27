// Package readstate computes per-artifact reading state from raw read_events.
//
// Layer model (see docs/02-concepts.md "Read state vs Acknowledgement vs
// Verification"): read_events stores raw per-session facts; this package
// rolls those up into a per-(artifact,user) classification used by the
// Reader UI, Today fallback ordering, and the verification bridge that
// promotes AI-authored revisions once a human has actually read them.
//
// Classification is intentionally pure — it takes an Aggregate plus the
// artifact body + locale and returns a State. DB-side aggregation lives
// in query.go and the artifact_read_states view (migration 0040).
package readstate

import (
	"strings"
	"time"
	"unicode/utf8"
)

// ReadState is the application-level classification surfaced to the UI and
// to MCP tools. The view artifact_read_states never materializes this — it
// is computed against the artifact body so the threshold can evolve
// without touching SQL.
type ReadState string

const (
	StateUnseen     ReadState = "unseen"
	StateGlanced    ReadState = "glanced"
	StateRead       ReadState = "read"
	StateDeeplyRead ReadState = "deeply_read"
)

// Thresholds. Decided 2026-04-27: completion_pct = active_ratio * scroll_max,
// active_ratio = min(active_seconds / expected_seconds, 1). 'read' >= 0.5,
// 'deeply_read' >= 0.8. 'glanced' is "any signal but below read threshold".
const (
	ThresholdRead       = 0.5
	ThresholdDeeplyRead = 0.8
)

// Reading speed defaults. Korean technical prose: ~600 chars/min. English:
// 250 wpm × 5 chars/word ≈ 1250 chars/min. The numbers are conservative on
// purpose — we'd rather call something "read" too readily than gate the
// verification bridge behind a too-strict bar that nobody hits.
const (
	korCharsPerMin = 600.0
	enCharsPerMin  = 1250.0
	minExpectedSec = 10.0
)

// Aggregate mirrors a single row of artifact_read_states (raw view). All
// time-ish fields are summed across every read_event row for the
// (artifact_id, user_key) pair.
type Aggregate struct {
	ArtifactID         string
	UserKey            string
	FirstSeenAt        time.Time
	LastSeenAt         time.Time
	TotalActiveSeconds float64
	TotalIdleSeconds   float64
	MaxScrollPct       float64
	EventCount         int
}

// State is the application-level read state surfaced to UI/MCP. It is
// derived from an Aggregate plus the artifact body + locale; nothing
// is persisted at this layer (the view is the source of truth).
type State struct {
	ArtifactID    string     `json:"artifact_id"`
	UserKey       string     `json:"user_key,omitempty"`
	ReadState     ReadState  `json:"read_state"`
	CompletionPct float64    `json:"completion_pct"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	EventCount    int        `json:"event_count"`
}

// ExpectedSeconds returns how long, in seconds, an attentive reader should
// take to read this body once. Locale changes the chars-per-minute baseline;
// a floor (minExpectedSec) keeps very short bodies from classifying as
// 'deeply_read' after a 1-second glance.
func ExpectedSeconds(body, locale string) float64 {
	return ExpectedSecondsFromChars(utf8.RuneCountInString(body), locale)
}

// ExpectedSecondsFromChars is the rune-count variant. The HTTP/MCP path
// reads char_length(body_markdown) directly from Postgres so it can avoid
// shipping the full body to the application just to count runes.
func ExpectedSecondsFromChars(chars int, locale string) float64 {
	if chars <= 0 {
		return minExpectedSec
	}
	cpm := enCharsPerMin
	if isKoreanLocale(locale) {
		cpm = korCharsPerMin
	}
	expected := float64(chars) / cpm * 60.0
	if expected < minExpectedSec {
		return minExpectedSec
	}
	return expected
}

// Completion returns the 0..1 completion ratio. The product form
// (active_ratio * scroll_max) means BOTH dwell AND scroll have to land
// before completion saturates — a reader who leaves the tab open without
// scrolling does not earn 'deeply_read', and a reader who scrolls to the
// bottom in 2 seconds does not either.
func Completion(activeSeconds, scrollMax, expectedSeconds float64) float64 {
	if expectedSeconds <= 0 {
		return 0
	}
	activeRatio := activeSeconds / expectedSeconds
	if activeRatio > 1.0 {
		activeRatio = 1.0
	}
	if activeRatio < 0 {
		activeRatio = 0
	}
	if scrollMax > 1.0 {
		scrollMax = 1.0
	}
	if scrollMax < 0 {
		scrollMax = 0
	}
	return activeRatio * scrollMax
}

// Classify returns the ReadState given the raw signals plus the body
// length basis. It is the single entry point the rest of the system
// uses; do not call Completion + threshold-compare inline.
func Classify(activeSeconds, scrollMax, expectedSeconds float64) ReadState {
	if activeSeconds <= 0 && scrollMax <= 0 {
		return StateUnseen
	}
	c := Completion(activeSeconds, scrollMax, expectedSeconds)
	switch {
	case c >= ThresholdDeeplyRead:
		return StateDeeplyRead
	case c >= ThresholdRead:
		return StateRead
	default:
		return StateGlanced
	}
}

// ClassifyAggregate is the convenience that the HTTP/MCP layers call.
// agg may be nil (no read_events for the (artifact, user) pair) — that
// case maps to StateUnseen with a zero CompletionPct.
func ClassifyAggregate(agg *Aggregate, body, locale string) State {
	return ClassifyAggregateFromChars(agg, utf8.RuneCountInString(body), locale)
}

// ClassifyAggregateFromChars takes the rune-count of the body directly,
// matching the SQL char_length() output. Used by ProjectStates so the
// query can avoid materializing every body in memory.
func ClassifyAggregateFromChars(agg *Aggregate, bodyChars int, locale string) State {
	if agg == nil || agg.EventCount == 0 {
		return State{ReadState: StateUnseen}
	}
	expected := ExpectedSecondsFromChars(bodyChars, locale)
	completion := Completion(agg.TotalActiveSeconds, agg.MaxScrollPct, expected)
	out := State{
		ArtifactID:    agg.ArtifactID,
		UserKey:       agg.UserKey,
		ReadState:     Classify(agg.TotalActiveSeconds, agg.MaxScrollPct, expected),
		CompletionPct: completion,
		EventCount:    agg.EventCount,
	}
	if !agg.LastSeenAt.IsZero() {
		ls := agg.LastSeenAt
		out.LastSeenAt = &ls
	}
	return out
}

func isKoreanLocale(locale string) bool {
	lower := strings.ToLower(strings.TrimSpace(locale))
	return lower == "ko" || strings.HasPrefix(lower, "ko-") || strings.HasPrefix(lower, "ko_")
}
