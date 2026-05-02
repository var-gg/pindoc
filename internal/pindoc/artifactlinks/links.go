// Package artifactlinks rewrites agent-facing artifact references inside
// markdown bodies.
package artifactlinks

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const prefix = "pindoc://"

// LinkError identifies the unresolved pindoc:// reference that stopped a
// rewrite.
type LinkError struct {
	Ref string
	Err error
}

func (e *LinkError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("pindoc link %q: %v", e.Ref, e.Err)
}

func (e *LinkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NormalizeRef trims and path-decodes a ref captured after pindoc://.
func NormalizeRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	return strings.TrimSpace(ref)
}

// RewritePindocLinks replaces pindoc://<slug> references with URLs returned
// by resolve. Fenced code blocks and inline code spans are left untouched so
// examples can still mention the raw agent ref syntax.
func RewritePindocLinks(body string, resolve func(ref string) (string, error)) (string, []string, error) {
	if !strings.Contains(body, prefix) {
		return body, nil, nil
	}
	lines := strings.SplitAfter(body, "\n")
	var out strings.Builder
	out.Grow(len(body))
	var refs []string
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			out.WriteString(line)
			inFence = !inFence
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}
		rewritten, lineRefs, err := rewriteLine(line, resolve)
		if err != nil {
			return body, refs, err
		}
		out.WriteString(rewritten)
		refs = append(refs, lineRefs...)
	}
	if len(refs) == 0 {
		return body, nil, nil
	}
	return out.String(), refs, nil
}

func rewriteLine(line string, resolve func(ref string) (string, error)) (string, []string, error) {
	var out strings.Builder
	out.Grow(len(line))
	var refs []string
	inCode := false
	for i := 0; i < len(line); {
		if line[i] == '`' {
			n := repeatedBackticks(line[i:])
			out.WriteString(line[i : i+n])
			i += n
			inCode = !inCode
			continue
		}
		if !inCode && strings.HasPrefix(line[i:], prefix) {
			ref, consumed, trailing := consumeRef(line[i+len(prefix):])
			if ref == "" {
				out.WriteString(prefix)
				i += len(prefix)
				continue
			}
			url, err := resolve(ref)
			if err != nil {
				return "", refs, &LinkError{Ref: ref, Err: err}
			}
			out.WriteString(url)
			out.WriteString(trailing)
			refs = append(refs, ref)
			i += len(prefix) + consumed
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if r == utf8.RuneError && size == 1 {
			out.WriteByte(line[i])
			i++
			continue
		}
		out.WriteString(line[i : i+size])
		i += size
	}
	return out.String(), refs, nil
}

func repeatedBackticks(s string) int {
	n := 0
	for n < len(s) && s[n] == '`' {
		n++
	}
	return n
}

func consumeRef(s string) (ref string, consumed int, trailing string) {
	end := 0
	for end < len(s) {
		r, size := utf8.DecodeRuneInString(s[end:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		if r == '/' {
			return "", 0, ""
		}
		if isTerminator(r) {
			break
		}
		end += size
	}
	raw := s[:end]
	ref = strings.TrimRight(raw, ".,;:!?")
	trailing = raw[len(ref):]
	return ref, end, trailing
}

func isTerminator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '`', '<', '>', '"', '\'', '(', ')', '[', ']', '{', '}', '#', '?':
		return true
	default:
		return false
	}
}
