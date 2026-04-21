// Package httpapi exposes read-only HTTP endpoints the web UI uses.
//
// Writes stay on the MCP side — agents write through pindoc.artifact.propose
// over stdio, UI just reads. Keeping the HTTP surface read-only is a
// deliberate design choice: it means a deployment can ship the web UI
// behind read-only auth (GitHub OAuth, for instance) without the web
// layer needing to mint agent tokens.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type Deps struct {
	DB          *db.Pool
	Logger      *slog.Logger
	ProjectSlug string
	Embedder    embed.Provider
	Version     string
	BuildCommit string
}

func New(cfg *config.Config, d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/current", d.handleProjectCurrent)
	mux.HandleFunc("GET /api/areas", d.handleAreas)
	mux.HandleFunc("GET /api/artifacts", d.handleArtifactList)
	mux.HandleFunc("GET /api/artifacts/{idOrSlug}", d.handleArtifactGet)
	mux.HandleFunc("GET /api/search", d.handleSearch)
	mux.HandleFunc("GET /api/health", d.handleHealth)
	return withCORS(withRecover(mux, d.Logger))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// withCORS permits the Vite dev server (same origin via its proxy is the
// normal path, but if the UI is served from a different origin during
// dev we still accept reads). Production locks this down via reverse
// proxy config anyway.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func withRecover(h http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				logger.Error("panic in http handler", "panic", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		h.ServeHTTP(w, r)
	})
}
