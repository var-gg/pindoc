//go:build !cgo

package embed

import "fmt"

func NewGemma(cfg GemmaConfig) (Provider, error) {
	return nil, fmt.Errorf("%s embedding provider requires cgo; rebuild with CGO_ENABLED=1 and a C compiler, or set PINDOC_EMBED_PROVIDER=http", cfg.providerNameForError())
}

func (cfg GemmaConfig) providerNameForError() string {
	if cfg.Variant == "" {
		return "gemma"
	}
	return "gemma/" + string(cfg.Variant)
}
