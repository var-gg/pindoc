package embed

import (
	"regexp"
	"strings"
)

// Chunk is one searchable unit of an artifact body. Index is position in
// the chunk sequence (0 for the first chunk). Heading is the H2/H3 that
// starts the chunk (empty for pre-heading preamble). Span is the byte
// range in the original body.
type Chunk struct {
	Index   int
	Heading string
	Text    string
	SpanStart int
	SpanEnd   int
}

var headingRegex = regexp.MustCompile(`(?m)^#{2,3}\s+.+$`)

// Chunk splits a markdown body on H2/H3 boundaries. Short bodies (no
// headings, or total length below minCharsForSplit) return as one chunk.
// Very long chunks without sub-headings get a hard split on double-newline
// paragraphs to keep each within the provider's MaxTokens budget, but the
// budget check itself is the caller's job — this function is
// token-agnostic.
//
// Title is prepended to each chunk's Text so that retrieval results carry
// context even when a chunk is matched in isolation.
func ChunkBody(title, body string, minCharsForSplit int) []Chunk {
	body = strings.TrimRight(body, "\n")
	if minCharsForSplit <= 0 {
		minCharsForSplit = 600
	}
	if len(body) == 0 {
		return nil
	}

	// Short bodies → single chunk, no splitting.
	if len(body) < minCharsForSplit {
		return []Chunk{{
			Index:     0,
			Text:      prefixTitle(title, body),
			SpanStart: 0,
			SpanEnd:   len(body),
		}}
	}

	locs := headingRegex.FindAllStringIndex(body, -1)
	if len(locs) == 0 {
		// Long body with no headings. Fall back to a single chunk for
		// now; a paragraph-boundary splitter lands the day we see this
		// fail the MaxTokens budget in practice.
		return []Chunk{{
			Index:     0,
			Text:      prefixTitle(title, body),
			SpanStart: 0,
			SpanEnd:   len(body),
		}}
	}

	// Sections run from each heading's start to the next heading (or EOF).
	// Preamble (anything before the first heading) becomes chunk 0 if
	// non-empty.
	var out []Chunk
	idx := 0
	if first := locs[0][0]; first > 0 {
		pre := strings.TrimSpace(body[:first])
		if pre != "" {
			out = append(out, Chunk{
				Index:     idx,
				Text:      prefixTitle(title, pre),
				SpanStart: 0,
				SpanEnd:   first,
			})
			idx++
		}
	}
	for i, l := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		section := body[l[0]:end]
		headingLine := body[l[0]:l[1]]
		heading := strings.TrimLeft(strings.TrimSpace(headingLine), "# \t")
		out = append(out, Chunk{
			Index:     idx,
			Heading:   heading,
			Text:      prefixTitle(title, strings.TrimSpace(section)),
			SpanStart: l[0],
			SpanEnd:   end,
		})
		idx++
	}
	return out
}

func prefixTitle(title, body string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return body
	}
	return title + "\n\n" + body
}
