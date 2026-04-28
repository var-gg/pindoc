// Package titleguide carries the locale-aware quality rules artifact.propose
// runs against a proposed Title before commit. The package separates
// language-neutral META rules (kept as Go logic) from per-locale DATA
// (lengths and jargon token sets) so adding a new locale never requires a
// PINDOC.md rewrite — see docs/CONTRIBUTING_LOCALE.md for the contributor
// path. Falls back through a BCP47-ish chain ending at "en".
package titleguide

import "strings"

// LocaleData holds the per-locale knobs the propose preflight reads. The
// Go side never inlines locale strings; everything that varies by language
// lives in the LocaleDataFor map below or in a server_settings override.
type LocaleData struct {
	Locale   string
	MinRunes int
	MaxRunes int
	// JargonTokens is the case-insensitive substring set that flags
	// titles like "audit", "review", "general" — vague nouns that hurt
	// later retrieval. Matching is plain-text contains; semantic dedup
	// is the embedder's job, not this preflight's.
	JargonTokens []string
}

// localeDataFor is the embedded baseline. Operators extend this via
// server_settings (instance scope) or, eventually, project_settings.
// Adding a new locale = a new entry here + a CONTRIBUTING_LOCALE.md PR.
var localeDataFor = map[string]LocaleData{
	"en": {
		Locale:   "en",
		MinRunes: 15,
		MaxRunes: 80,
		JargonTokens: []string{
			"stuff", "thing", "things", "various",
			"misc", "etc", "general", "miscellaneous",
			"todo", "wip", "tmp", "temp", "draft draft",
		},
	},
	"ko": {
		Locale:   "ko",
		MinRunes: 8,
		MaxRunes: 60,
		JargonTokens: []string{
			"기타", "내용", "여러", "일반",
			"임시", "처리", "재판정", "수정 수정",
		},
	},
	"ja": {
		Locale:   "ja",
		MinRunes: 10,
		MaxRunes: 70,
		JargonTokens: []string{
			"その他", "一般", "雑多", "色々", "仮",
		},
	},
}

// Resolve returns the LocaleData for the given BCP47-ish locale tag,
// falling back through the language subtag and finally to "en". The
// returned struct is always populated — never nil — so callers can use
// it without guarding for a missing entry.
func Resolve(locale string) LocaleData {
	cleaned := strings.ToLower(strings.TrimSpace(locale))
	if cleaned == "" {
		return localeDataFor["en"]
	}
	if data, ok := localeDataFor[cleaned]; ok {
		return data
	}
	// Strip region/script: "ko-KR" → "ko", "zh-Hant-TW" → "zh".
	if dash := strings.IndexAny(cleaned, "-_"); dash > 0 {
		base := cleaned[:dash]
		if data, ok := localeDataFor[base]; ok {
			return data
		}
	}
	return localeDataFor["en"]
}

// SupportedLocales returns the set of locale keys the embedded baseline
// recognises. Test + tooling helper; not load-bearing at runtime.
func SupportedLocales() []string {
	out := make([]string, 0, len(localeDataFor))
	for k := range localeDataFor {
		out = append(out, k)
	}
	return out
}
