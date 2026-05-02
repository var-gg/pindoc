// Package artifactslug owns Pindoc artifact slug generation.
package artifactslug

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxSlugRunes = 60

// slugRegex replaces any run of characters that are NOT Unicode letters
// or numbers with a single hyphen. This preserves Hangul / Kana / CJK /
// Latin-ext / Cyrillic / Arabic verbatim.
var slugRegex = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Slugify produces a URL-safe, human-legible slug from a title.
//
// The cap is applied only at token boundaries. If the first token itself
// exceeds the cap, the token is preserved rather than being chopped in the
// middle of a word or Hangul phrase.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return trimAtWordBoundary(s, maxSlugRunes)
}

func trimAtWordBoundary(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	prefix := string(runes[:maxRunes])
	if idx := strings.LastIndex(prefix, "-"); idx > 0 {
		return strings.Trim(prefix[:idx], "-")
	}
	return s
}
