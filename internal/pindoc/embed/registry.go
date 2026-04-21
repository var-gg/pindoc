package embed

import (
	"fmt"
	"strings"
	"time"
)

// Config picks which Provider to construct. Loaded from env by the server
// entrypoint. The default (empty Provider name) is the stub.
type Config struct {
	// Provider: "stub" (default) | "http"
	Provider string

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

func Build(cfg Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "stub":
		// Default dim 384 matches paraphrase-multilingual-MiniLM-L12-v2,
		// the Python sidecar's default model.
		dim := cfg.Dimension
		if dim == 0 {
			dim = 384
		}
		return NewStub(dim), nil
	case "http":
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("http embedding provider requires endpoint")
		}
		if cfg.Dimension == 0 {
			return nil, fmt.Errorf("http embedding provider requires dimension (model-specific)")
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
	default:
		return nil, fmt.Errorf("unknown embedding provider %q (valid: stub, http)", cfg.Provider)
	}
}
