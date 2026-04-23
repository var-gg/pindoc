// Package httpapi exposes read-only HTTP endpoints the web UI uses.
//
// Writes stay on the MCP side — agents write through pindoc.artifact.propose
// over stdio, UI just reads. Keeping the HTTP surface read-only is a
// deliberate design choice: it means a deployment can ship the web UI
// behind read-only auth (GitHub OAuth, for instance) without the web
// layer needing to mint agent tokens.
//
// URL convention: every project-scoped read lives under /api/p/{project}/…
// so a URL is shareable without ambiguity. Unscoped reads (config, health,
// projects list) stay at /api/… The web UI mirrors this: /p/{project}/wiki
// etc. See docs/03-architecture.md for the full URL convention.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

type Deps struct {
	DB     *db.Pool
	Logger *slog.Logger

	// DefaultProjectSlug is the project the root URL redirects to when
	// the URL has no /p/{project} prefix (legacy shares, cold open).
	// Resolved once at startup from PINDOC_PROJECT.
	DefaultProjectSlug string

	// DefaultProjectLocale pairs with DefaultProjectSlug to rebuild
	// pre-Phase-18 URLs (`/wiki/...` legacy shares) into their new
	// `/p/<slug>/<locale>/...` canonical shape. Resolved once at startup
	// by querying `projects.locale WHERE slug = DefaultProjectSlug`.
	// Empty falls back to "en" client-side (see ServerConfig in
	// web/src/api/client.ts).
	DefaultProjectLocale string

	// MultiProject toggles UI switcher visibility. Read: does this
	// instance expect to host >1 project? False keeps the switcher
	// hidden even if extra rows exist in the projects table.
	MultiProject bool

	Embedder    embed.Provider
	Settings    *settings.Store
	Version     string
	BuildCommit string
}

func New(cfg *config.Config, d Deps) http.Handler {
	mux := http.NewServeMux()

	// Unscoped reads — apply to the whole instance.
	mux.HandleFunc("GET /api/health", d.handleHealth)
	mux.HandleFunc("GET /api/config", d.handleConfig)
	mux.HandleFunc("GET /api/projects", d.handleProjectList)

	// Ops surface (Phase J UI). Instance-wide telemetry aggregation —
	// per-tool averages + recent call timeline so operators can see
	// "which tool is a token hog this week" in the Reader without
	// dropping into psql. Read-only, same convention as the rest of
	// httpapi.
	mux.HandleFunc("GET /api/ops/telemetry", d.handleTelemetry)

	// Project-scoped reads. The {project} path segment resolves a row in
	// projects.slug; 404 if missing so URL shares fail loudly rather than
	// silently leaking to the caller's current project.
	mux.HandleFunc("GET /api/p/{project}", d.handleProjectCurrent)
	mux.HandleFunc("GET /api/p/{project}/areas", d.handleAreas)
	mux.HandleFunc("GET /api/p/{project}/artifacts", d.handleArtifactList)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}", d.handleArtifactGet)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}/revisions", d.handleArtifactRevisions)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}/diff", d.handleArtifactDiff)
	mux.HandleFunc("GET /api/p/{project}/search", d.handleSearch)

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

// projectSlugFrom extracts the {project} path value. Returns empty string
// if the route had no project segment (shouldn't happen for scoped routes
// but keeps the helper crash-safe).
func projectSlugFrom(r *http.Request) string {
	return r.PathValue("project")
}
