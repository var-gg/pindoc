// Package embed owns the pluggable embedding provider. All retrieval paths
// (artifact.search, context.for_task, conflict check) route through here so
// swapping models is a config change, not a code change.
package embed

import (
	"context"
	"fmt"
)

// Kind distinguishes queries (shorter, task-prefixed in Gemma-family models)
// from documents (longer, no prefix). Some providers ignore this; others
// inject different prefixes per the Sentence-Transformers convention.
type Kind string

const (
	KindQuery    Kind = "query"
	KindDocument Kind = "document"
)

// Request is one batch call. Texts all share the same Kind.
type Request struct {
	Texts []string
	Kind  Kind
}

// Response carries one vector per input text, in order. Dimension matches
// Provider.Info().Dimension.
type Response struct {
	Vectors [][]float32
}

// Info describes the provider's static properties. Server-side code uses
// MaxTokens for pre-flight body-length checks and Dimension for the
// artifact_chunks column width.
type Info struct {
	Name         string // "stub" | "http" | "embeddinggemma" | ...
	ModelID      string
	Dimension    int
	MaxTokens    int
	Distance     string // "cosine" | "dot"
	Multilingual bool
}

type Provider interface {
	Embed(ctx context.Context, req Request) (*Response, error)
	Info() Info
}

// ErrDimensionMismatch is returned when a provider yields vectors that
// don't match its own Info().Dimension. Worth a loud failure — the DB
// column width is sized for the declared dim.
var ErrDimensionMismatch = fmt.Errorf("embedding dimension mismatch")
