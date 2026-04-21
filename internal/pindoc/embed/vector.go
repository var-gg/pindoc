package embed

import (
	"strconv"
	"strings"
)

// VectorString formats a []float32 as the pgvector literal Postgres wants:
//
//	"[0.1,0.2,...]"
//
// Avoids the pgvector-go driver dependency for Phase 3; we only need one
// direction (write) because search queries get distances back, not the
// vectors themselves.
func VectorString(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 10)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(x), 'f', 6, 32))
	}
	sb.WriteByte(']')
	return sb.String()
}

// PadTo768 pads a vector with zeros up to 768 dims (or truncates if longer).
// The artifact_chunks.embedding column is fixed at vector(768); uniform
// zero-padding preserves cosine similarity between same-provider vectors.
// Mixing provider vectors in the same column invalidates comparisons —
// document as "switching providers requires re-embed" in PINDOC.md.
func PadTo768(v []float32) []float32 {
	const target = 768
	if len(v) == target {
		return v
	}
	out := make([]float32, target)
	n := len(v)
	if n > target {
		n = target
	}
	copy(out, v[:n])
	return out
}
