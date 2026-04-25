// Package httpapi exposes the HTTP endpoints the web UI uses.
//
// Semantic writes (body / title / relations) stay on the MCP side —
// agents write through pindoc.artifact.propose over stdio, UI reads.
// Operational-metadata writes (task_meta assignee / priority / due_at
// per Decision agent-only-write-분할) are the one exception and live on
// POST /api/p/{project}/artifacts/{idOrSlug}/task-meta. That lane is
// gated on auth_mode=trusted_local today; V1.5+ ACLs will extend the
// check rather than reopening the MCP-only invariant for other fields.
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
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
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
	Telemetry   *telemetry.Store
	Version     string
	BuildCommit string

	// StartTime stamps when the daemon process began running. Surfaced
	// via GET /health as uptime_sec so operators can spot-check that
	// the NSSM-managed service hasn't been silently restart-looped.
	// Zero value is OK — the health handler reports uptime_sec=0.
	StartTime time.Time
}

func New(cfg *config.Config, d Deps) http.Handler {
	mux := http.NewServeMux()

	// Unscoped reads — apply to the whole instance.
	// /health is the lightweight liveness probe (NSSM / external
	// monitor); /api/health is the verbose embedder-aware variant the
	// Reader uses internally.
	mux.HandleFunc("GET /health", d.handleSimpleHealth)
	mux.HandleFunc("GET /api/health", d.handleHealth)
	mux.HandleFunc("GET /api/config", d.handleConfig)
	mux.HandleFunc("GET /api/projects", d.handleProjectList)
	// POST /api/projects creates a new project (Decision
	// project-bootstrap-canonical-flow-reader-ui-first-class). Behind the
	// wire it calls projects.CreateProject — same source of truth as the
	// MCP tool and the pindoc-admin CLI. Locked to trusted_local via the
	// pindoc-api 127.0.0.1 bind today; OAuth comes with V1.5.
	mux.HandleFunc("POST /api/projects", d.handleProjectCreate)
	// users is an instance-wide table (migration 0014). Surfaced read-only
	// so Reader TaskControls can offer a real assignee dropdown next to
	// the agents aggregate (Decision agent-only-write-분할 AC).
	mux.HandleFunc("GET /api/users", d.handleUserList)

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

	// Operational metadata edit — the one write surface the HTTP API
	// exposes. Scope is locked to task_meta.status / assignee / priority /
	// due_at / parent_slug per Decision agent-only-write-분할. The server
	// still gates status transitions here: verified remains verify-tool only
	// and claimed_done requires acceptance completion.
	mux.HandleFunc("POST /api/p/{project}/artifacts/{idOrSlug}/task-meta", d.handleTaskMetaPatch)
	mux.HandleFunc("POST /api/p/{project}/artifacts/{idOrSlug}/task-assign", d.handleTaskAssign)

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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
