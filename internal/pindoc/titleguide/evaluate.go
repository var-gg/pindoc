package titleguide

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Warning is one finding from EvaluateTitle. Code is the stable identifier
// preflight surfaces in the not_ready envelope; Detail is the
// human-readable suffix appended to logs and chat. Severity is decided in
// warning_severity.go on the propose side, not here, so the package stays
// data-only.
type Warning struct {
	Code   string
	Detail string
}

// ProjectOverride lets a project (or instance settings) extend the
// embedded jargon set without forking locale_data.go. Empty fields fall
// through to the embedded baseline.
type ProjectOverride struct {
	// ExtraJargon is appended to LocaleData.JargonTokens before the
	// match runs. Values are matched case-insensitively, same as the
	// baseline tokens.
	ExtraJargon []string
}

// EvaluateTitle returns every quality warning the proposed title trips,
// scoped to the artifact body_locale plus any project-side override. The
// preflight pipeline maps each Code to a stable severity (warn/info) on
// the propose side; this package only emits the findings.
//
// Findings:
//   - TITLE_TOO_SHORT:N_runes — below the locale's MinRunes
//   - TITLE_TOO_LONG:N_runes  — above the locale's MaxRunes
//   - TITLE_GENERIC_TOKENS:tok1,tok2 — title contains tokens from the
//     locale's JargonTokens set (case-insensitive contains). The matched
//     tokens are appended so the agent can edit the right words rather
//     than guessing what triggered the warning.
func EvaluateTitle(title, locale string, override ProjectOverride) []Warning {
	trimmed := strings.TrimSpace(title)
	data := Resolve(locale)

	var out []Warning
	n := utf8.RuneCountInString(trimmed)
	if n > 0 && n < data.MinRunes {
		out = append(out, Warning{
			Code:   fmt.Sprintf("TITLE_TOO_SHORT:%d_runes", n),
			Detail: fmt.Sprintf("locale=%s min=%d", data.Locale, data.MinRunes),
		})
	}
	if n > data.MaxRunes {
		out = append(out, Warning{
			Code:   fmt.Sprintf("TITLE_TOO_LONG:%d_runes", n),
			Detail: fmt.Sprintf("locale=%s max=%d", data.Locale, data.MaxRunes),
		})
	}

	matched := matchJargonTokens(trimmed, data.JargonTokens, override.ExtraJargon)
	if len(matched) > 0 {
		sort.Strings(matched)
		out = append(out, Warning{
			Code:   "TITLE_GENERIC_TOKENS:" + strings.Join(matched, ","),
			Detail: fmt.Sprintf("locale=%s tokens=%s", data.Locale, strings.Join(matched, ",")),
		})
	}
	return out
}

func matchJargonTokens(title string, baseline, extra []string) []string {
	lowerTitle := strings.ToLower(title)
	seen := map[string]struct{}{}
	var matched []string
	for _, tok := range append(append([]string{}, baseline...), extra...) {
		t := strings.ToLower(strings.TrimSpace(tok))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		// Use a Unicode-safe contains. Tokens are short; full title is
		// short; the cost of a per-token Contains is fine.
		if strings.Contains(lowerTitle, t) {
			seen[t] = struct{}{}
			matched = append(matched, t)
		}
	}
	return matched
}

// SlugVerboseThreshold is the rune count above which the auto-generated
// slug surfaces SLUG_VERBOSE. The threshold is locale-driven (mirrors
// MaxRunes / 1.7 floor) so a verbose slug warning never fires on a title
// that already passed the length gate. Hardened from the previous global
// 47 runes after the dogfood feedback on 2026-04-28.
func SlugVerboseThreshold(locale string) int {
	data := Resolve(locale)
	t := data.MaxRunes / 2
	if t < 30 {
		t = 30
	}
	return t
}
