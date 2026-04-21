package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPProvider talks to any HTTP service that accepts this body shape:
//
//	POST <endpoint>
//	{"model": "<id>", "input": ["text 1", "text 2"], "kind": "query|document"}
//
// and returns
//
//	{"data": [{"embedding": [0.1, 0.2, ...]}, ...]}
//
// This matches OpenAI's /v1/embeddings shape (minus the "kind" field which
// we add; servers are expected to ignore unknown fields). A reference
// Python sidecar lives at services/embed-sidecar/.
type HTTPProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
	info     Info
}

type HTTPConfig struct {
	Endpoint     string
	APIKey       string // optional; sent as Authorization: Bearer <key> if set
	Model        string
	Dimension    int
	MaxTokens    int
	Multilingual bool
	Distance     string // "cosine" default
	Timeout      time.Duration
}

func NewHTTP(c HTTPConfig) *HTTPProvider {
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Distance == "" {
		c.Distance = "cosine"
	}
	return &HTTPProvider{
		endpoint: c.Endpoint,
		apiKey:   c.APIKey,
		client:   &http.Client{Timeout: c.Timeout},
		info: Info{
			Name:         "http",
			ModelID:      c.Model,
			Dimension:    c.Dimension,
			MaxTokens:    c.MaxTokens,
			Multilingual: c.Multilingual,
			Distance:     c.Distance,
		},
	}
}

func (p *HTTPProvider) Info() Info { return p.info }

type httpReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
	Kind  string   `json:"kind,omitempty"`
}

type httpResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *HTTPProvider) Embed(ctx context.Context, req Request) (*Response, error) {
	buf, err := json.Marshal(httpReq{Model: p.info.ModelID, Input: req.Texts, Kind: string(req.Kind)})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embed http call: %w", err)
	}
	defer res.Body.Close()

	var decoded httpResp
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	if res.StatusCode >= 400 {
		msg := "status " + res.Status
		if decoded.Error != nil {
			msg += ": " + decoded.Error.Message
		}
		return nil, fmt.Errorf("embed service error: %s", msg)
	}
	if len(decoded.Data) != len(req.Texts) {
		return nil, fmt.Errorf("embed response length %d != input %d", len(decoded.Data), len(req.Texts))
	}
	vecs := make([][]float32, len(decoded.Data))
	for i, d := range decoded.Data {
		if p.info.Dimension > 0 && len(d.Embedding) != p.info.Dimension {
			return nil, fmt.Errorf("%w: got %d want %d", ErrDimensionMismatch, len(d.Embedding), p.info.Dimension)
		}
		vecs[i] = d.Embedding
	}
	return &Response{Vectors: vecs}, nil
}
