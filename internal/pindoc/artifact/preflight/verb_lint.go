package preflight

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var taskListLineRe = regexp.MustCompile(`^\s*[-*]\s+\[[ xX~-]\]\s*(.*)$`)

type AcceptanceVerbFinding struct {
	LineNumber    int
	CheckboxIndex int
	Text          string
	Verb          string
	Variant       string
	ExampleBefore string
	ExampleAfter  string
}

func LintAcceptanceVerbs(body string) []AcceptanceVerbFinding {
	var out []AcceptanceVerbFinding
	checkboxIndex := 0
	for i, line := range strings.Split(body, "\n") {
		match := taskListLineRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		text := strings.TrimSpace(match[1])
		for _, hit := range DetectForbiddenAcceptanceVerbs(text) {
			out = append(out, AcceptanceVerbFinding{
				LineNumber:    i + 1,
				CheckboxIndex: checkboxIndex,
				Text:          text,
				Verb:          hit.Verb,
				Variant:       hit.Variant,
				ExampleBefore: hit.ExampleBefore,
				ExampleAfter:  hit.ExampleAfter,
			})
		}
		checkboxIndex++
	}
	return out
}

type VerbHit struct {
	Verb          string
	Variant       string
	ExampleBefore string
	ExampleAfter  string
}

func DetectForbiddenAcceptanceVerbs(text string) []VerbHit {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return nil
	}
	var out []VerbHit
	seen := map[string]struct{}{}
	for _, rule := range ForbiddenAcceptanceVerbs {
		for _, variant := range rule.Variants {
			if !containsBoundedKoreanPhrase(normalized, variant) {
				continue
			}
			if _, ok := seen[rule.Canonical]; ok {
				break
			}
			seen[rule.Canonical] = struct{}{}
			out = append(out, VerbHit{
				Verb:          rule.Canonical,
				Variant:       variant,
				ExampleBefore: rule.ExampleBefore,
				ExampleAfter:  rule.ExampleAfter,
			})
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Variant < out[j].Variant })
	return out
}

func containsBoundedKoreanPhrase(text, phrase string) bool {
	if phrase == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(text[start:], phrase)
		if idx < 0 {
			return false
		}
		idx += start
		if isBoundaryBefore(text, idx) && isBoundaryAfter(text, idx+len(phrase)) {
			return true
		}
		start = idx + len(phrase)
	}
}

func isBoundaryBefore(text string, byteIndex int) bool {
	if byteIndex <= 0 {
		return true
	}
	r, _ := lastRuneBefore(text[:byteIndex])
	return !isWordRune(r)
}

func isBoundaryAfter(text string, byteIndex int) bool {
	if byteIndex >= len(text) {
		return true
	}
	r, _ := firstRuneAt(text[byteIndex:])
	return !isWordRune(r)
}

func lastRuneBefore(text string) (rune, bool) {
	for i := len(text); i > 0; {
		r, size := rune(text[i-1]), 1
		if r >= 0x80 {
			r, size = utf8LastRune(text[:i])
		}
		return r, size > 0
	}
	return 0, false
}

func firstRuneAt(text string) (rune, bool) {
	for _, r := range text {
		return r, true
	}
	return 0, false
}

func utf8LastRune(text string) (rune, int) {
	for i := len(text) - 1; i >= 0; i-- {
		if text[i]&0xC0 != 0x80 {
			for _, r := range text[i:] {
				return r, len(text) - i
			}
		}
	}
	return 0, 0
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
