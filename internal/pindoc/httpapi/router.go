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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/changegroup"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/providers"
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
	// UserLanguage is the operator-selected fallback language from
	// PINDOC_USER_LANGUAGE. Fresh installs can lack a default project row,
	// so onboarding uses this when it has to create that project.
	UserLanguage string

	Embedder  embed.Provider
	Settings  *settings.Store
	Telemetry *telemetry.Store
	OAuth     *pauth.OAuthService
	// Providers is the runtime IdP registry (`instance_providers`
	// table). Nil-safe — admin endpoints respond 503 when the store
	// hasn't been wired (test fixtures).
	Providers     *providers.Store
	AuthProviders []string
	BindAddr      string
	// DefaultUserID / DefaultAgentID are stamped on loopback
	// Principals so MCP and HTTP layers attribute writes to the same
	// (users, agents) row even when the caller never authenticated.
	// Empty UserID is the "operator skipped PINDOC_USER_NAME" case —
	// handlers fall back to anonymous attribution.
	DefaultUserID  string
	DefaultAgentID string

	// TrustedSameHostProxy widens the loopback principal trust to
	// any source IP when the daemon is behind a same-host proxy
	// (docker port forwarding, NSSM reverse proxy). Set true at boot
	// when (a) the daemon detects it is running in a container and
	// (b) operator intent is loopback-only (cfg.IsLoopbackBind). See
	// auth.HTTPDeps.TrustedSameHostProxy for the security envelope.
	TrustedSameHostProxy bool
	Version              string
	BuildCommit          string
	RepoRoot             string
	Summary              changegroup.SummaryConfig

	// StartTime stamps when the daemon process began running. Surfaced
	// via GET /health as uptime_sec so operators can spot-check that
	// the NSSM-managed service hasn't been silently restart-looped.
	// Zero value is OK — the health handler reports uptime_sec=0.
	StartTime time.Time

	// AssetRoot is the LocalFS blob root used by /api/p/{project}/assets
	// routes. Empty falls back to /var/lib/pindoc/assets.
	AssetRoot string

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
	if cfg != nil {
		if d.AuthProviders == nil {
			d.AuthProviders = cfg.AuthProviders
		}
		if d.BindAddr == "" {
			d.BindAddr = cfg.BindAddr
		}
	}
	if d.BindAddr == "" {
		d.BindAddr = config.DefaultBindAddr
	}
	if d.OAuth != nil {
		d.OAuth.RegisterRoutes(mux)
	} else {
		pauth.RegisterUnavailableOAuthRoutes(mux)
	}

	// Unscoped reads — apply to the whole instance.
	// /health is the lightweight liveness probe (NSSM / external
	// monitor); /api/health is the verbose embedder-aware variant the
	// Reader uses internally.
	mux.HandleFunc("GET /health", d.handleSimpleHealth)
	mux.HandleFunc("GET /api/health", d.handleHealth)
	mux.HandleFunc("GET /api/config", d.handleConfig)
	mux.HandleFunc("GET /api/projects", d.handleProjectList)
	mux.HandleFunc("GET /join", d.handleInviteJoinInfo)
	mux.HandleFunc("POST /join", d.handleInviteJoin)
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
	mux.HandleFunc("GET /api/user/current", d.handleCurrentUser)

	// Agent-era first-time identity flow. POST creates / rebinds the
	// loopback principal's users.id row + sets server_settings.
	// default_loopback_user_id atomically. Loopback only.
	mux.HandleFunc("POST /api/onboarding/identity", d.handleOnboardingIdentity)

	// Ops surface (Phase J UI). Instance-wide telemetry aggregation —
	// per-tool averages + recent call timeline so operators can see
	// "which tool is a token hog this week" in the Reader without
	// dropping into psql. Read-only, same convention as the rest of
	// httpapi.
	mux.HandleFunc("GET /api/ops/telemetry", d.handleTelemetry)

	// Instance-level admin: identity provider registry. task-providers-
	// admin-ui — env seeds defaults, this surface mutates the DB row at
	// runtime so credential rotation / IdP toggling works without a
	// daemon restart. Loopback principal only (instance owner).
	mux.HandleFunc("GET /api/instance/providers", d.handleInstanceProvidersList)
	mux.HandleFunc("POST /api/instance/providers", d.handleInstanceProvidersUpsert)
	mux.HandleFunc("DELETE /api/instance/providers/{idOrName}", d.handleInstanceProvidersDelete)
	mux.HandleFunc("GET /api/instance/oauth-clients", d.handleOAuthClientsList)
	mux.HandleFunc("POST /api/instance/oauth-clients", d.handleOAuthClientsCreate)
	mux.HandleFunc("PATCH /api/instance/oauth-clients/dcr-mode", d.handleOAuthClientsDCRModePatch)
	mux.HandleFunc("DELETE /api/instance/oauth-clients/{clientID}", d.handleOAuthClientsDelete)

	// Project-scoped reads. The {project} path segment resolves a row in
	// projects.slug; 404 if missing so URL shares fail loudly rather than
	// silently leaking to the caller's current project.
	mux.HandleFunc("GET /api/p/{project}", d.handleProjectCurrent)
	mux.HandleFunc("PATCH /api/p/{project}/settings", d.handleProjectSettingsPatch)
	mux.HandleFunc("GET /api/p/{project}/areas", d.handleAreas)
	mux.HandleFunc("GET /api/p/{project}/artifacts", d.handleArtifactList)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}", d.handleArtifactGet)
	mux.HandleFunc("PATCH /api/p/{project}/artifacts/{idOrSlug}", d.handleArtifactPatch)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}/revisions", d.handleArtifactRevisions)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}/diff", d.handleArtifactDiff)
	mux.HandleFunc("GET /api/p/{project}/task-flow", d.handleTaskFlow)
	mux.HandleFunc("GET /api/p/{project}/search", d.handleSearch)
	mux.HandleFunc("GET /api/p/{project}/change-groups", d.handleChangeGroups)
	mux.HandleFunc("GET /api/p/{project}/inbox", d.handleInbox)
	mux.HandleFunc("POST /api/p/{project}/inbox/{idOrSlug}/review", d.handleInboxReview)
	mux.HandleFunc("POST /api/p/{project}/invite", d.handleInviteIssue)
	// Phase D — permission management plane.
	mux.HandleFunc("GET /api/p/{project}/members", d.handleMembersList)
	mux.HandleFunc("DELETE /api/p/{project}/members/{user_id}", d.handleMemberRemove)
	mux.HandleFunc("GET /api/p/{project}/invites", d.handleInvitesList)
	mux.HandleFunc("POST /api/p/{project}/invites/{token_hash}/extend", d.handleInviteExtend)
	mux.HandleFunc("DELETE /api/p/{project}/invites/{token_hash}", d.handleInviteRevoke)
	mux.HandleFunc("POST /api/p/{project}/read-mark", d.handleReadMark)
	mux.HandleFunc("POST /api/p/{project}/read-events", d.handleReadEvent)
	mux.HandleFunc("GET /api/p/{project}/read-states", d.handleReadStates)
	mux.HandleFunc("GET /api/p/{project}/artifacts/{idOrSlug}/read-state", d.handleArtifactReadState)
	mux.HandleFunc("GET /api/p/{project}/export", d.handleProjectExport)
	mux.HandleFunc("GET /api/p/{project}/git/repos", d.handleGitRepos)
	mux.HandleFunc("GET /api/p/{project}/git/changed-files", d.handleGitChangedFiles)
	mux.HandleFunc("GET /api/p/{project}/git/commit", d.handleGitCommit)
	mux.HandleFunc("GET /api/p/{project}/git/commits/{sha}/referencing-artifacts", d.handleGitCommitReferences)
	mux.HandleFunc("GET /api/p/{project}/git/blob", d.handleGitBlob)
	mux.HandleFunc("GET /api/p/{project}/git/diff", d.handleGitDiff)
	mux.HandleFunc("GET /api/p/{project}/assets/{assetID}/blob", d.handleAssetBlob)

	// Org-scoped project routes. Public-safe reads resolve
	// (organization_slug, project_slug) first, then apply artifact
	// visibility filters in the handlers below. Rich operational surfaces
	// stay member-only until each endpoint has an explicit public
	// projection, so the cascade route exists without accidentally
	// widening data exposure.
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}", d.handleOrgProjectPublic(d.handleProjectCurrent))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/areas", d.handleOrgProjectPublic(d.handleAreas))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/artifacts", d.handleOrgProjectPublic(d.handleArtifactList))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}", d.handleOrgProjectPublic(d.handleArtifactGet))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/assets/{assetID}/blob", d.handleOrgProjectPublic(d.handleAssetBlob))
	mux.HandleFunc("PATCH /api/orgs/{org}/p/{project}/settings", d.handleOrgProjectMember(d.handleProjectSettingsPatch))
	mux.HandleFunc("PATCH /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}", d.handleOrgProjectMember(d.handleArtifactPatch))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}/revisions", d.handleOrgProjectMember(d.handleArtifactRevisions))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}/diff", d.handleOrgProjectMember(d.handleArtifactDiff))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/task-flow", d.handleOrgProjectMember(d.handleTaskFlow))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/search", d.handleOrgProjectMember(d.handleSearch))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/change-groups", d.handleOrgProjectMember(d.handleChangeGroups))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/inbox", d.handleOrgProjectMember(d.handleInbox))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/inbox/{idOrSlug}/review", d.handleOrgProjectMember(d.handleInboxReview))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/invite", d.handleOrgProjectMember(d.handleInviteIssue))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/members", d.handleOrgProjectMember(d.handleMembersList))
	mux.HandleFunc("DELETE /api/orgs/{org}/p/{project}/members/{user_id}", d.handleOrgProjectMember(d.handleMemberRemove))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/invites", d.handleOrgProjectMember(d.handleInvitesList))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/invites/{token_hash}/extend", d.handleOrgProjectMember(d.handleInviteExtend))
	mux.HandleFunc("DELETE /api/orgs/{org}/p/{project}/invites/{token_hash}", d.handleOrgProjectMember(d.handleInviteRevoke))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/read-mark", d.handleOrgProjectMember(d.handleReadMark))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/read-events", d.handleOrgProjectMember(d.handleReadEvent))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/read-states", d.handleOrgProjectMember(d.handleReadStates))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}/read-state", d.handleOrgProjectMember(d.handleArtifactReadState))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/export", d.handleOrgProjectMember(d.handleProjectExport))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/git/repos", d.handleOrgProjectMember(d.handleGitRepos))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/git/changed-files", d.handleOrgProjectMember(d.handleGitChangedFiles))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/git/commit", d.handleOrgProjectMember(d.handleGitCommit))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/git/commits/{sha}/referencing-artifacts", d.handleOrgProjectMember(d.handleGitCommitReferences))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/git/blob", d.handleOrgProjectMember(d.handleGitBlob))
	mux.HandleFunc("GET /api/orgs/{org}/p/{project}/git/diff", d.handleOrgProjectMember(d.handleGitDiff))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}/task-meta", d.handleOrgProjectMember(d.handleTaskMetaPatch))
	mux.HandleFunc("POST /api/orgs/{org}/p/{project}/artifacts/{idOrSlug}/task-assign", d.handleOrgProjectMember(d.handleTaskAssign))

	// Operational metadata edit — the one write surface the HTTP API
	// exposes. Scope is locked to task_meta.status / assignee / priority /
	// due_at / parent_slug per Decision agent-only-write-분할. The server
	// still gates status transitions here: claimed_done requires acceptance
	// completion.
	mux.HandleFunc("POST /api/p/{project}/artifacts/{idOrSlug}/task-meta", d.handleTaskMetaPatch)
	mux.HandleFunc("POST /api/p/{project}/artifacts/{idOrSlug}/task-assign", d.handleTaskAssign)

	// Legacy Reader locale segment. Old Phase 18 shares used
	// /p/{project}/{locale}/{surface}/...; canonical-only routing removes
	// the locale segment, so the daemon issues a real 301 before the SPA
	// fallback handles the path.
	mux.HandleFunc("GET /p/{project}/{locale}/{view}", d.handleLegacyReaderLocaleRedirect)
	mux.HandleFunc("GET /p/{project}/{locale}/{view}/{rest...}", d.handleLegacyReaderLocaleRedirect)
	mux.HandleFunc("GET /p/{project}/wiki/{idOrSlug}", d.handleReaderWikiAliasRedirect)

	// Reader SPA. Catch-all `/` is the lowest-priority pattern in
	// Go 1.22's ServeMux, so /api/..., /mcp, /health all match
	// first. Disabled when SPADistDir is empty — typical in tests or
	// when the operator wants the daemon to be API-only and a Vite dev
	// server in front handles assets.
	if d.SPADistDir != "" {
		mux.HandleFunc("/", d.handleSPA)
	}

	return withSecurityHeaders(withCORS(cfg, withRecover(mux, d.Logger)))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

const spaBaselineCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self' http://localhost:* http://127.0.0.1:*; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

func withSecurityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.ServeHTTP(w, r)
	})
}

type corsPolicy struct {
	allowAll bool
	allowed  map[string]bool
}

func corsPolicyFromConfig(cfg *config.Config) corsPolicy {
	if cfg != nil && cfg.DevMode {
		return corsPolicy{allowAll: true}
	}
	policy := corsPolicy{allowed: map[string]bool{}}
	if cfg != nil {
		for _, origin := range cfg.AllowedOrigins {
			origin = normalizeCORSOrigin(origin)
			if origin != "" {
				policy.allowed[origin] = true
			}
		}
	}
	return policy
}

func normalizeCORSOrigin(origin string) string {
	return strings.TrimRight(strings.TrimSpace(origin), "/")
}

func (p corsPolicy) allow(origin string) (string, bool) {
	origin = normalizeCORSOrigin(origin)
	if origin == "" {
		return "", false
	}
	if p.allowAll {
		return "*", true
	}
	if p.allowed[origin] {
		return origin, true
	}
	return "", false
}

// withCORS is default-deny: same-origin requests need no CORS header,
// configured origins are echoed, and wildcard is restricted to DevMode.
func withCORS(cfg *config.Config, h http.Handler) http.Handler {
	policy := corsPolicyFromConfig(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed, ok := policy.allow(r.Header.Get("Origin")); ok {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			if allowed != "*" {
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
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
	if scope, ok := projectRouteContextFrom(r); ok && scope.ProjectSlug != "" {
		return scope.ProjectSlug
	}
	return r.PathValue("project")
}

type orgProjectRouteAccess string

const (
	orgProjectRouteAccessPublic  orgProjectRouteAccess = "public"
	orgProjectRouteAccessMember  orgProjectRouteAccess = "member"
	orgProjectRouteAccessTrusted orgProjectRouteAccess = "trusted"
)

type projectRouteContextKey struct{}

type projectRouteContext struct {
	OrgScoped   bool
	OrgID       string
	OrgSlug     string
	ProjectID   string
	ProjectSlug string
	UserID      string
	Access      orgProjectRouteAccess
}

func projectRouteContextFrom(r *http.Request) (projectRouteContext, bool) {
	if r == nil {
		return projectRouteContext{}, false
	}
	scope, ok := r.Context().Value(projectRouteContextKey{}).(projectRouteContext)
	return scope, ok
}

func orgSlugFrom(r *http.Request) string {
	if scope, ok := projectRouteContextFrom(r); ok {
		return scope.OrgSlug
	}
	return r.PathValue("org")
}

func projectLookupPredicate(r *http.Request, alias string, placeholder int) (string, any) {
	if placeholder <= 0 {
		placeholder = 1
	}
	projectCol := sqlColumn(alias, "slug")
	if scope, ok := projectRouteContextFrom(r); ok && scope.ProjectID != "" {
		return fmt.Sprintf("%s = $%d::uuid", sqlColumn(alias, "id"), placeholder), scope.ProjectID
	}
	return fmt.Sprintf("%s = $%d", projectCol, placeholder), projectSlugFrom(r)
}

func (d Deps) artifactVisibilityPredicate(r *http.Request, alias string, startPlaceholder int) (string, []any) {
	scope, ok := projectRouteContextFrom(r)
	if !ok || !scope.OrgScoped || scope.Access == orgProjectRouteAccessTrusted {
		if principal := d.principalForRequest(r); principal != nil && principal.IsLoopback() {
			return "TRUE", nil
		}
		if ok && scope.Access == orgProjectRouteAccessTrusted {
			return "TRUE", nil
		}
		if principal := d.principalForRequest(r); principal != nil && principal.IsOAuth() && strings.TrimSpace(principal.UserID) != "" {
			return oauthArtifactVisibilityPredicate(alias, startPlaceholder, principal.UserID)
		}
		return publicArtifactVisibilityPredicate(alias, startPlaceholder)
	}
	if startPlaceholder <= 0 {
		startPlaceholder = 1
	}
	visibilityCol := sqlColumn(alias, "visibility")
	authorCol := sqlColumn(alias, "author_user_id")
	switch scope.Access {
	case orgProjectRouteAccessMember:
		next := startPlaceholder
		parts := []string{
			fmt.Sprintf("%s = $%d", visibilityCol, next),
			fmt.Sprintf("%s = $%d", visibilityCol, next+1),
		}
		args := []any{projects.VisibilityPublic, projects.VisibilityOrg}
		next += 2
		if strings.TrimSpace(scope.UserID) != "" {
			ownerPredicate := fmt.Sprintf(`EXISTS (
				SELECT 1
				  FROM project_members pm
				 WHERE pm.project_id = %s
				   AND pm.user_id::text = $%d
				   AND pm.role = 'owner'
			)`, sqlColumn(alias, "project_id"), next+1)
			parts = append(parts, fmt.Sprintf("(%s = $%d AND (%s::text = $%d OR %s))", visibilityCol, next, authorCol, next+1, ownerPredicate))
			args = append(args, projects.VisibilityPrivate, scope.UserID)
		}
		return "(" + strings.Join(parts, " OR ") + ")", args
	default:
		return publicArtifactVisibilityPredicate(alias, startPlaceholder)
	}
}

func publicArtifactVisibilityPredicate(alias string, startPlaceholder int) (string, []any) {
	if startPlaceholder <= 0 {
		startPlaceholder = 1
	}
	return fmt.Sprintf("%s = $%d", sqlColumn(alias, "visibility"), startPlaceholder), []any{projects.VisibilityPublic}
}

func oauthArtifactVisibilityPredicate(alias string, startPlaceholder int, userID string) (string, []any) {
	if startPlaceholder <= 0 {
		startPlaceholder = 1
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return publicArtifactVisibilityPredicate(alias, startPlaceholder)
	}
	visibilityCol := sqlColumn(alias, "visibility")
	projectCol := sqlColumn(alias, "project_id")
	authorCol := sqlColumn(alias, "author_user_id")
	memberPredicate := fmt.Sprintf(`(
		EXISTS (
			SELECT 1
			  FROM project_members pm
			 WHERE pm.project_id = %s
			   AND pm.user_id::text = $%d
		)
		OR EXISTS (
			SELECT 1
			  FROM projects vp
			  JOIN organization_members om ON om.organization_id = vp.organization_id
			 WHERE vp.id = %s
			   AND om.user_id::text = $%d
		)
	)`, projectCol, startPlaceholder+3, projectCol, startPlaceholder+3)
	ownerPredicate := fmt.Sprintf(`EXISTS (
		SELECT 1
		  FROM project_members pm
		 WHERE pm.project_id = %s
		   AND pm.user_id::text = $%d
		   AND pm.role = 'owner'
	)`, projectCol, startPlaceholder+3)
	predicate := fmt.Sprintf(
		"(%s = $%d OR (%s AND (%s = $%d OR (%s = $%d AND (%s::text = $%d OR %s)))))",
		visibilityCol, startPlaceholder,
		memberPredicate,
		visibilityCol, startPlaceholder+1,
		visibilityCol, startPlaceholder+2,
		authorCol, startPlaceholder+3,
		ownerPredicate,
	)
	return predicate, []any{projects.VisibilityPublic, projects.VisibilityOrg, projects.VisibilityPrivate, userID}
}

func sqlColumn(alias, column string) string {
	alias = strings.TrimSpace(alias)
	column = strings.TrimSpace(column)
	if alias == "" {
		return column
	}
	return alias + "." + column
}

func (d Deps) handleOrgProjectPublic(next http.HandlerFunc) http.HandlerFunc {
	return d.handleOrgProject(next, false)
}

func (d Deps) handleOrgProjectMember(next http.HandlerFunc) http.HandlerFunc {
	return d.handleOrgProject(next, true)
}

func (d Deps) handleOrgProject(next http.HandlerFunc, requireMember bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.DB == nil {
			writeError(w, http.StatusInternalServerError, "database unavailable")
			return
		}
		orgSlug := strings.TrimSpace(r.PathValue("org"))
		projectSlug := strings.TrimSpace(r.PathValue("project"))
		resolved, err := projects.ResolveByOrgAndSlug(r.Context(), d.DB, orgSlug, projectSlug)
		if errors.Is(err, projects.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		if err != nil {
			if d.Logger != nil {
				d.Logger.Error("org project resolve", "err", err, "org", orgSlug, "project", projectSlug)
			}
			writeError(w, http.StatusInternalServerError, "project lookup failed")
			return
		}

		access, userID, canRead, err := d.resolveOrgProjectAccess(r.Context(), r, resolved)
		if err != nil {
			if d.Logger != nil {
				d.Logger.Warn("org project access resolve", "err", err, "org", orgSlug, "project", projectSlug)
			}
			writeError(w, http.StatusInternalServerError, "project access check failed")
			return
		}
		if !canRead || (requireMember && access == orgProjectRouteAccessPublic) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}

		scope := projectRouteContext{
			OrgScoped:   true,
			OrgID:       resolved.OrgID,
			OrgSlug:     resolved.OrgSlug,
			ProjectID:   resolved.ProjectID,
			ProjectSlug: resolved.ProjectSlug,
			UserID:      userID,
			Access:      access,
		}
		ctx := context.WithValue(r.Context(), projectRouteContextKey{}, scope)
		next(w, r.WithContext(ctx))
	}
}

func (d Deps) resolveOrgProjectAccess(ctx context.Context, r *http.Request, resolved *projects.ResolveResult) (orgProjectRouteAccess, string, bool, error) {
	principal := d.principalForRequest(r)
	if principal == nil {
		return orgProjectRouteAccessPublic, "", resolved.IsAccessibleAnonymously(), nil
	}
	userID := strings.TrimSpace(principal.UserID)
	if principal.IsLoopback() {
		return orgProjectRouteAccessTrusted, userID, true, nil
	}

	projectMember, orgMember, err := d.orgProjectMembership(ctx, userID, resolved.ProjectID, resolved.OrgID)
	if err != nil {
		return orgProjectRouteAccessPublic, userID, false, err
	}
	memberAccess := projectMember || orgMember
	switch resolved.ProjectVisibility {
	case projects.VisibilityPublic:
		if memberAccess {
			return orgProjectRouteAccessMember, userID, true, nil
		}
		return orgProjectRouteAccessPublic, userID, true, nil
	case projects.VisibilityOrg:
		if memberAccess {
			return orgProjectRouteAccessMember, userID, true, nil
		}
	case projects.VisibilityPrivate:
		if projectMember {
			return orgProjectRouteAccessMember, userID, true, nil
		}
	}
	return orgProjectRouteAccessPublic, userID, false, nil
}

func (d Deps) orgProjectMembership(ctx context.Context, userID, projectID, orgID string) (bool, bool, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	orgID = strings.TrimSpace(orgID)
	if userID == "" {
		return false, false, nil
	}

	var projectMember bool
	if projectID != "" {
		err := d.DB.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				  FROM project_members
				 WHERE project_id = $1::uuid
				   AND user_id = $2::uuid
			)
		`, projectID, userID).Scan(&projectMember)
		if err != nil {
			return false, false, err
		}
	}

	var orgMember bool
	if orgID != "" {
		err := d.DB.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				  FROM organization_members
				 WHERE organization_id = $1::uuid
				   AND user_id = $2::uuid
			)
		`, orgID, userID).Scan(&orgMember)
		if err != nil {
			return false, false, err
		}
	}
	return projectMember, orgMember, nil
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

func (d Deps) handleReaderWikiAliasRedirect(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		d.handleSPA(w, r)
		return
	}
	projectSlug := r.PathValue("project")
	ref := r.PathValue("idOrSlug")
	var canonical string
	err := d.DB.QueryRow(r.Context(), `
		SELECT a.slug
		  FROM artifact_slug_aliases asa
		  JOIN projects p ON p.id = asa.project_id
		  JOIN artifacts a ON a.id = asa.artifact_id
		 WHERE p.slug = $1
		   AND asa.old_slug = $2
		   AND a.status <> 'archived'
		 LIMIT 1
	`, projectSlug, ref).Scan(&canonical)
	if errors.Is(err, pgx.ErrNoRows) {
		d.handleSPA(w, r)
		return
	}
	if err != nil {
		if d.Logger != nil {
			d.Logger.Warn("artifact slug alias lookup failed", "project", projectSlug, "ref", ref, "err", err)
		}
		d.handleSPA(w, r)
		return
	}
	if canonical == "" || canonical == ref {
		d.handleSPA(w, r)
		return
	}
	target := "/p/" + url.PathEscape(projectSlug) + "/wiki/" + url.PathEscape(canonical)
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}
