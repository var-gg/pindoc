package embed

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// StubProvider produces deterministic hash-based pseudo-embeddings. It is
// the default when no real provider is configured, so the search/chunking
// pipeline is exercised end-to-end on a fresh install even if the user
// hasn't set up an embedding service yet.
//
// Search quality under Stub is only meaningful for exact-string matches
// in titles (because the hash is deterministic per input). Real retrieval
// quality requires swapping in HTTPProvider pointed at a real model.
type StubProvider struct {
	dim int
}

func NewStub(dim int) *StubProvider {
	if dim <= 0 {
		dim = 384
	}
	return &StubProvider{dim: dim}
}

func (s *StubProvider) Info() Info {
	return Info{
		Name:         "stub",
		ModelID:      "sha256-unit-hash",
		Dimension:    s.dim,
		MaxTokens:    8192, // no real limit; text just folds into a hash
		Distance:     "cosine",
		Multilingual: true,
	}
}

func (s *StubProvider) Embed(_ context.Context, req Request) (*Response, error) {
	out := make([][]float32, len(req.Texts))
	for i, t := range req.Texts {
		out[i] = hashToUnitVec(t, s.dim)
	}
	return &Response{Vectors: out}, nil
}

// hashToUnitVec turns a string into a unit-length float32 vector of length
// dim. SHA-256 is reused across chunks of 8 bytes to seed successive dims
// until we fill the vector, then we L2-normalize. Deterministic, fast,
// obviously not semantic.
func hashToUnitVec(text string, dim int) []float32 {
	vec := make([]float32, dim)
	// Roll the hash forward so each 8-byte window feeds one dimension.
	seed := []byte(text)
	h := sha256.Sum256(seed)
	for i := 0; i < dim; i++ {
		// Every 4 dims, reshuffle the 32-byte digest.
		if i > 0 && i%4 == 0 {
			h = sha256.Sum256(h[:])
		}
		off := (i % 4) * 8
		u := binary.BigEndian.Uint64(h[off : off+8])
		// Map uint64 → [-1, 1].
		vec[i] = float32(float64(int64(u))/float64(1<<63)) - 0.5
	}
	// L2 normalize so cosine == dot.
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	norm := float32(math.Sqrt(sum))
	if norm == 0 {
		return vec
	}
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}
