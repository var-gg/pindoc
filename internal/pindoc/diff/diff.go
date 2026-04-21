// Package diff computes artifact revision diffs. Two views:
//
//   Unified(a, b)    — classic unified diff text, for human eyes.
//   SectionDeltas(a, b) — heading-scoped change summary so an agent can
//                         decide whether to fetch the full unified diff.
//
// Kept out of internal/pindoc/mcp/tools so both the MCP tool and the HTTP
// API can import it.
package diff

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

// Stats summarises byte/line-level change magnitude.
type Stats struct {
	LinesAdded   int `json:"lines_added"`
	LinesRemoved int `json:"lines_removed"`
	BytesAdded   int `json:"bytes_added"`
	BytesRemoved int `json:"bytes_removed"`
}

// SectionDelta groups diff by H1/H2/H3 heading so a reviewer can scan.
type SectionDelta struct {
	Heading string `json:"heading"` // empty = pre-heading preamble
	Change  string `json:"change"`  // "unchanged" | "modified" | "added" | "removed"
	Before  string `json:"excerpt_before,omitempty"`
	After   string `json:"excerpt_after,omitempty"`
	// Line-level stats scoped to this section.
	Added   int `json:"lines_added"`
	Removed int `json:"lines_removed"`
}

// Unified returns a standard unified diff with `--- a/uri` / `+++ b/uri`
// headers. uri is a display-only label.
func Unified(uri, before, after string) string {
	edits := myers.ComputeEdits(span.URIFromPath(uri), before, after)
	return fmt.Sprint(gotextdiff.ToUnified("a/"+uri, "b/"+uri, before, edits))
}

// Summary computes line-level stats + per-section deltas. Stats are
// derived from the unified diff so we don't depend on gotextdiff's
// offset fields being populated (they aren't for myers output).
func Summary(before, after string) (Stats, []SectionDelta) {
	return lineStats(before, after), sectionDeltas(before, after)
}

func lineStats(before, after string) Stats {
	u := Unified("body", before, after)
	var s Stats
	for _, line := range strings.Split(u, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "@@ "):
			continue
		case strings.HasPrefix(line, "+"):
			s.LinesAdded++
			s.BytesAdded += len(line) - 1
		case strings.HasPrefix(line, "-"):
			s.LinesRemoved++
			s.BytesRemoved += len(line) - 1
		}
	}
	return s
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,3})\s+(.+)$`)

// section walks a markdown body and returns the slice of (heading, text)
// pairs in order. Preamble before the first heading is recorded with an
// empty heading string.
func splitSections(body string) []section {
	body = strings.TrimRight(body, "\n") + "\n"
	locs := headingRe.FindAllStringIndex(body, -1)
	var out []section
	if len(locs) == 0 {
		return []section{{Heading: "", Text: body}}
	}
	if locs[0][0] > 0 {
		pre := strings.TrimSpace(body[:locs[0][0]])
		if pre != "" {
			out = append(out, section{Heading: "", Text: pre})
		}
	}
	for i, loc := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		heading := strings.TrimSpace(strings.TrimLeft(body[loc[0]:loc[1]], "# \t"))
		out = append(out, section{
			Heading: heading,
			Text:    strings.TrimSpace(body[loc[0]:end]),
		})
	}
	return out
}

type section struct {
	Heading string
	Text    string
}

func sectionDeltas(before, after string) []SectionDelta {
	sa := splitSections(before)
	sb := splitSections(after)

	// Map heading → text per side. Deterministic headings win simple
	// pair-up; same-name sections in different positions still pair.
	aMap := map[string]section{}
	bMap := map[string]section{}
	var order []string
	seen := map[string]bool{}
	push := func(k string) {
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}
	for _, s := range sa {
		aMap[s.Heading] = s
		push(s.Heading)
	}
	for _, s := range sb {
		bMap[s.Heading] = s
		push(s.Heading)
	}

	var out []SectionDelta
	for _, h := range order {
		a, inA := aMap[h]
		b, inB := bMap[h]
		switch {
		case inA && !inB:
			stats := lineStats(a.Text, "")
			out = append(out, SectionDelta{
				Heading: h, Change: "removed",
				Before:   excerpt(a.Text),
				Added:    0,
				Removed:  stats.LinesRemoved,
			})
		case !inA && inB:
			stats := lineStats("", b.Text)
			out = append(out, SectionDelta{
				Heading: h, Change: "added",
				After:    excerpt(b.Text),
				Added:    stats.LinesAdded,
				Removed:  0,
			})
		default:
			if a.Text == b.Text {
				out = append(out, SectionDelta{
					Heading: h, Change: "unchanged",
				})
				continue
			}
			stats := lineStats(a.Text, b.Text)
			out = append(out, SectionDelta{
				Heading: h, Change: "modified",
				Before:   excerpt(a.Text),
				After:    excerpt(b.Text),
				Added:    stats.LinesAdded,
				Removed:  stats.LinesRemoved,
			})
		}
	}
	return out
}

func excerpt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 200 {
		return s
	}
	// Cut at word boundary near 200 chars.
	cut := 200
	if i := strings.LastIndexAny(s[:cut], " \n\t"); i > 80 {
		cut = i
	}
	return s[:cut] + "..."
}
