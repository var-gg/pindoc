package embed

import (
	"fmt"
	"strings"
	"time"
)

// Config picks which Provider to construct. Loaded from env by the server
// entrypoint. An empty Provider name resolves to gemma — the product
// default per docs/03's EC2-medium footprint target. stub is accepted
// only when set explicitly (PINDOC_EMBED_PROVIDER=stub) so missing envs
// can't silently downgrade semantic search to hash-ranked placeholders.
type Config struct {
	// Provider: "" (→ gemma default) | "gemma" | "embeddinggemma" | "http" | "stub"
	Provider string

	// Gemma-specific fields. Optional; empty triggers the default path.
	GemmaVariant string // "q4" (default) | "q4f16" | "quantized" | "fp16"
	ModelDir     string // override ~/.cache/pindoc/models/embeddinggemma-300m
	RuntimeDir   string // override ~/.cache/pindoc/runtime
	RuntimeLib   string // explicit onnxruntime.so/.dll/.dylib path

	// HTTP-specific fields (ignored for other providers).
	Endpoint     string
	APIKey       string
	Model        string
	Dimension    int
	MaxTokens    int
	Multilingual bool
	Timeout      time.Duration

	// PrefixQuery / PrefixDocument are prepended to each input text based
	// on Request.Kind. Needed for E5-family models which are trained with
	// "query: " / "passage: " prefixes. See embed/http.go for details.
	PrefixQuery    string
	PrefixDocument string
}

// Build returns the configured Provider.
//
// No silent fallback: an unknown provider name errors out immediately
// rather than downgrading to stub. stub itself is only built when the
// operator has explicitly opted in (useful for unit tests without any
// model assets on disk).
func Build(cfg Config) (Provider, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch name {
	case "", "gemma", "embeddinggemma":
		return NewGemma(GemmaConfig{
			Variant:    variantFromEnv(cfg.GemmaVariant),
			ModelDir:   cfg.ModelDir,
			RuntimeDir: cfg.RuntimeDir,
			RuntimeLib: cfg.RuntimeLib,
		})
	case "http":
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("http embedding provider requires PINDOC_EMBED_ENDPOINT")
		}
		if cfg.Dimension == 0 {
			return nil, fmt.Errorf("http embedding provider requires PINDOC_EMBED_DIM (model-specific)")
		}
		if cfg.Model == "" {
			cfg.Model = "default"
		}
		if cfg.MaxTokens == 0 {
			cfg.MaxTokens = 2048
		}
		return NewHTTP(HTTPConfig{
			Endpoint:       cfg.Endpoint,
			APIKey:         cfg.APIKey,
			Model:          cfg.Model,
			Dimension:      cfg.Dimension,
			MaxTokens:      cfg.MaxTokens,
			Multilingual:   cfg.Multilingual,
			Timeout:        cfg.Timeout,
			PrefixQuery:    cfg.PrefixQuery,
			PrefixDocument: cfg.PrefixDocument,
		}), nil
	case "stub":
		// Explicit opt-in only — used by unit tests and offline envs that
		// don't want to touch the network. Search quality is hash-based,
		// not semantic; artifact.propose's 0.85 conflict threshold is
		// meaningless under stub.
		dim := cfg.Dimension
		if dim == 0 {
			dim = 384
		}
		return NewStub(dim), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider %q (valid: gemma, http, stub)", cfg.Provider)
	}
}
