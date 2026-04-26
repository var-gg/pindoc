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
	"net/url"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/changegroup"
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

	// DefaultProjectLocale is a compatibility alias for the default
	// project's primary_language. Legacy clients may still read the field,
	// but canonical Reader URLs are /p/<slug>/...
	DefaultProjectLocale string

	Embedder    embed.Provider
	Settings    *settings.Store
	Telemetry   *telemetry.Store
	AuthMode    config.AuthMode
	Version     string
	BuildCommit string
	Summary     changegroup.SummaryConfig

	// StartTime stamps when the daemon process began running. Surfaced
	// via GET /health as uptime_sec so operators can spot-check that
	// the NSSM-managed service hasn't been silently restart-looped.
	// Zero value is OK — the health handler reports uptime_sec=0.
	StartTime time.Time

	// SPADistDir is the absolute filesystem path to the Reader UI build
	// output (web/dist). When set, the daemon serves /, /assets/...,
	// /p/{project}/... etc. as static files with a fallback to index.html
	// so React Router can pick up unknown paths client-side.
	// Empty disables SPA serving — useful in tests or when a Vite dev
	// server is fronting the daemon.
	SPADistDir string
}

func New(cfg *config.Config, d Deps) http.Handler {
	mux := http.NewServeMux()
	if cfg != nil && d.Summary.Endpoint == "" {
		d.Summary = changegroup.SummaryConfig{
			Endpoint:      cfg.Summary.Endpoint,
			APIKey:        cfg.Summary.APIKey,
			Model:         cfg.Summary.Model,
			Timeout:       cfg.Summary.Timeout,
			DailyTokenCap: cfg.Summary.DailyTokenCap,
			GroupCap:      cfg.Summary.GroupCap,
		}
	}
	if cfg != nil && d.AuthMode == "" {
		d.AuthMode = cfg.AuthMode
	}
	if d.AuthMode == "" {
		d.AuthMode = config.AuthModeTrustedLocal
	}

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
	mux.HandleFunc("GET /api/p/{project}/change-groups", d.handleChangeGroups)
	mux.HandleFunc("POST /api/p/{project}/read-mark", d.handleReadMark)
	mux.HandleFunc("GET /api/p/{project}/export", d.handleProjectExport)

	// Operational metadata edit — the one write surface the HTTP API
	// exposes. Scope is locked to task_meta.status / assignee / priority /
	// due_at / parent_slug per Decision agent-only-write-분할. The server
	// still gates status transitions here: verified remains verify-tool only
	// and claimed_done requires acceptance completion.
	mux.HandleFunc("POST /api/p/{project}/artifacts/{idOrSlug}/task-meta", d.handleTaskMetaPatch)
	mux.HandleFunc("POST /api/p/{project}/artifacts/{idOrSlug}/task-assign", d.handleTaskAssign)

	// Legacy Reader locale segment. Old Phase 18 shares used
	// /p/{project}/{locale}/{surface}/...; canonical-only routing removes
	// the locale segment, so the daemon issues a real 301 before the SPA
	// fallback handles the path.
	mux.HandleFunc("GET /p/{project}/{locale}/{view}", d.handleLegacyReaderLocaleRedirect)
	mux.HandleFunc("GET /p/{project}/{locale}/{view}/{rest...}", d.handleLegacyReaderLocaleRedirect)

	// Reader SPA. Catch-all `/` is the lowest-priority pattern in
	// Go 1.22's ServeMux, so /api/..., /mcp, /health all match
	// first. Disabled when SPADistDir is empty — typical in tests or
	// when the operator wants the daemon to be API-only and a Vite dev
	// server in front handles assets.
	if d.SPADistDir != "" {
		mux.HandleFunc("/", d.handleSPA)
	}

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

func (d Deps) handleLegacyReaderLocaleRedirect(w http.ResponseWriter, r *http.Request) {
	view := r.PathValue("view")
	if !isReaderSurface(view) {
		if d.SPADistDir != "" {
			d.handleSPA(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}

	target := "/p/" + url.PathEscape(r.PathValue("project")) + "/" + url.PathEscape(view)
	if rest := r.PathValue("rest"); rest != "" {
		target += "/" + escapePathSegments(rest)
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func isReaderSurface(view string) bool {
	switch view {
	case "wiki", "tasks", "graph", "inbox", "search", "today":
		return true
	default:
		return false
	}
}

func escapePathSegments(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
