package tools

import (
	"sort"
	"strings"
)

// Warning severities in priority order. Phase G classifies every emitted
// warning code so the response can sort them — agents reading warnings
// top-down hit the "error"-severity ones first and act on them, not on
// an info-level advisory that happened to be emitted first by the code
// path.
//
// Levels:
//
//	"error" — agent should address before continuing; likely indicates
//	          the write is subtly wrong (canonical claim rewritten
//	          without evidence, deferred acceptance without reason, etc.)
//	"warn"  — advisory that changes how a reader should interpret the
//	          artifact (source unclassified, consent pending, H2 missing)
//	"info"  — reminder / pointer that isn't a defect (new write but a
//	          close match existed and was skipped)
//
// Unknown warning codes default to "warn" — safer than demoting to info
// when we don't know the intent.
const (
	SeverityError = "error"
	SeverityWarn  = "warn"
	SeverityInfo  = "info"
)

// warningSeverityCatalog maps the code prefix (before any ':' suffix
// carrying parameters like section names) to a severity. Prefix match
// means "CANONICAL_REWRITE_WITHOUT_EVIDENCE:Root cause+Decision" still
// resolves to error via the base code.
var warningSeverityCatalog = map[string]string{
	// ERROR — agent likely got something substantively wrong.
	"CANONICAL_REWRITE_WITHOUT_EVIDENCE": SeverityError,
	"CLAIMED_DONE_INCOMPLETE":            SeverityError,
	"RECEIPT_SUPERSEDED":                 SeverityError,
	"FIELD_VALUE_RESERVED":               SeverityError,
	"PATCH_NOOP":                         SeverityError,

	// WARN — changes reader interpretation but write stands.
	"CONSENT_REQUIRED_FOR_USER_CHAT": SeverityWarn,
	"SOURCE_TYPE_UNCLASSIFIED":       SeverityWarn,
	"MISSING_H2":                     SeverityWarn,
	"BODY_HAS_H1_REDUNDANT":          SeverityWarn,
	"TITLE_LONG":                     SeverityWarn,
	"TITLE_VERY_LONG":                SeverityWarn,
	"PIN_PATH_NONEXISTENT":           SeverityWarn,
	"PIN_PATH_OUTSIDE_REPO":          SeverityWarn,

	// INFO — pointer / reminder.
	"RECOMMEND_READ_BEFORE_CREATE": SeverityInfo,
}

// warningSeverity resolves the severity for one warning code. Handles
// "PREFIX:params" by stripping the suffix before looking up the
// catalog. Unknown codes default to SeverityWarn.
func warningSeverity(code string) string {
	if code == "" {
		return SeverityWarn
	}
	prefix := code
	if i := strings.IndexByte(code, ':'); i > 0 {
		prefix = code[:i]
	}
	if s, ok := warningSeverityCatalog[prefix]; ok {
		return s
	}
	return SeverityWarn
}

// severityRank is the sort key — higher = more urgent.
func severityRank(s string) int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarn:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// sortWarningsBySeverity returns a new slice sorted by severity
// descending (error > warn > info), stable within the same severity so
// the emit order is preserved for equal-weight warnings. Nil input
// returns nil.
func sortWarningsBySeverity(warnings []string) []string {
	if len(warnings) == 0 {
		return warnings
	}
	out := make([]string, len(warnings))
	copy(out, warnings)
	sort.SliceStable(out, func(i, j int) bool {
		return severityRank(warningSeverity(out[i])) > severityRank(warningSeverity(out[j]))
	})
	return out
}
