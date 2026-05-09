package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type projectInfo struct {
	ID                        string `json:"id"`
	Slug                      string `json:"slug"`
	OrgSlug                   string `json:"org_slug,omitempty"`
	OrganizationSlug          string `json:"organization_slug,omitempty"`
	Name                      string `json:"name"`
	Description               string `json:"description,omitempty"`
	Color                     string `json:"color,omitempty"`
	PrimaryLanguage           string `json:"primary_language"`
	Visibility                string `json:"visibility"`
	SensitiveOps              string `json:"sensitive_ops"`
	DefaultArtifactVisibility string `json:"default_artifact_visibility"`
	CurrentRole               string `json:"current_role,omitempty"`
	// Locale is a compatibility alias for PrimaryLanguage. Locale is no
	// longer part of project identity or Reader URLs after task-canonical-
	// locale-migration.
	Locale         string        `json:"locale"`
	AreasCount     int           `json:"areas_count"`
	ArtifactsCount int           `json:"artifacts_count"`
	CreatedAt      time.Time     `json:"created_at"`
	Rendering      RenderingCaps `json:"rendering"`
	Capabilities   ProjectCaps   `json:"capabilities"`
}

// RenderingCaps tells an agent which markdown features actually render in
// Pindoc's Wiki Reader. Anything not listed may round-trip correctly but
// will not visually render — agents should stick to this set when
// proposing artifact bodies.
type RenderingCaps struct {
	MarkdownFlavor string   `json:"markdown_flavor"`
	Extensions     []string `json:"extensions"`
	CodeLanguages  []string `json:"code_languages"`
	Notes          string   `json:"notes,omitempty"`
}

type ProjectCaps struct {
	ReviewQueueSupported bool `json:"review_queue_supported"`
}

var pindocRenderingCaps = RenderingCaps{
	MarkdownFlavor: "gfm",
	Extensions: []string{
		"tables",
		"task_lists",
		"strikethrough",
		"autolink",
		"footnotes",
		"soft_breaks",         // single newlines render as <br>
		"github_alerts",       // > [!NOTE] / [!TIP] / [!IMPORTANT] / [!WARNING] / [!CAUTION]
		"syntax_highlighting", // shiki via github-light/github-dark themes
		"code_block_copy",     // fenced code toolbar copies raw source
		"code_block_title",    // title="..." / filename="..." fence metadata
		"code_block_collapse", // long fenced code blocks can expand/collapse
		"diff_line_emphasis",  // fenced diff add/remove/hunk line styling
		"table_scroll",        // GFM tables render in a horizontal scroll wrapper
		"heading_anchors",     // #-anchor on H2–H4 hover
		"keyboard_tag",        // <kbd> rendered as keycap
		"mermaid",             // fenced ```mermaid blocks render as SVG
	},
	CodeLanguages: []string{
		"javascript", "typescript", "jsx", "tsx",
		"html", "css", "scss", "json",
		"go", "python", "rust", "java", "kotlin",
		"csharp", "php", "ruby", "swift", "c", "cpp",
		"bash", "shell", "powershell",
		"yaml", "toml", "ini", "sql", "xml",
		"markdown", "diff", "dockerfile", "graphql", "lua",
	},
	Notes: "Headings H1–H6 (H2–H4 expose hover anchors), ordered/unordered lists, GFM task lists, GFM tables with scroll containment, footnotes, blockquotes, soft line breaks, inline + fenced code with syntax highlighting (Shiki, GitHub themes), code copy/title/collapse controls, diff line emphasis, safe <kbd>, GitHub-style alert blockquotes, Mermaid via ```mermaid fence. Math/KaTeX not supported (M1.x).",
}

func (d Deps) handleConfig(w http.ResponseWriter, r *http.Request) {
	publicBase := ""
	if d.Settings != nil {
		publicBase = d.Settings.Get().PublicBaseURL
	}
	bindAddr := strings.TrimSpace(d.BindAddr)
	if bindAddr == "" {
		bindAddr = config.DefaultBindAddr
	}
	providers := append([]string(nil), d.AuthProviders...)
	if providers == nil {
		providers = []string{}
	}
	projectCaps := d.deriveMultiProjectCaps(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"default_project_slug":     d.DefaultProjectSlug,
		"default_project_locale":   d.DefaultProjectLocale,
		"multi_project":            projectCaps.MultiProjectSwitching,
		"multi_project_deprecated": "use multi_project_switching",
		"multi_project_switching":  projectCaps.MultiProjectSwitching,
		"project_create_allowed":   projectCaps.ProjectCreateAllowed,
		"public_base_url":          publicBase,
		"version":                  d.Version,
		// providers + bind_addr replace the deprecated auth_mode enum
		// (Decision `decision-auth-model-loopback-and-providers`).
		// Reader keys "is the operator the calling principal" off the
		// loopback judgement of the current request, not off this
		// instance-wide config.
		"providers": providers,
		"bind_addr": bindAddr,
		// identity_required surfaces the loopback identity gap so the
		// Reader can route fresh installs to the onboarding form
		// instead of attributing every revision to a NULL author. True
		// only when the daemon could not bind a user from settings,
		// env, or the lone-row backfill — i.e. truly fresh installs.
		"identity_required": d.identityRequired(),
		// onboarding_required tells the React app to redirect / →
		// new-project wizard. True when only the seed `pindoc`
		// project exists (Decision project-bootstrap-canonical-flow-
		// reader-ui-first-class). Independent from identity_required
		// — both can be true on first boot.
		"onboarding_required": d.checkOnboardingRequired(r.Context()),
	})
}

// isLoopbackOrTrustedProxy mirrors the same-host trust envelope
// PrincipalFromRequest applies — used by handlers that need a binary
// "is the caller the operator on this box" answer without going
// through the full principal resolver. Auth.PrincipalFromRequest
// returns nil for non-trusted-non-OAuth callers, but a few handlers
// (onboarding, providers admin) refuse outright with INSTANCE_OWNER_
// REQUIRED before consulting OAuth.
func (d Deps) isLoopbackOrTrustedProxy(r *http.Request) bool {
	if pauth.IsLoopbackRequest(r) {
		return true
	}
	return d.TrustedSameHostProxy
}

// identityRequired is true when the loopback principal has no bound
// users.id row. The Reader uses this to redirect a fresh install to
// the onboarding form before any project / artifact UI loads. Reads
// settings so it stays accurate after the operator submits the form
// without a daemon restart.
func (d Deps) identityRequired() bool {
	if d.Settings == nil {
		return false
	}
	return strings.TrimSpace(d.Settings.Get().DefaultLoopbackUserID) == ""
}

// deriveMultiProject is the HTTP-side mirror of mcp/tools.deriveMultiProject.
// Called once per /api/config, /api/projects, and /api/health response so
// the wire `multi_project` field tracks the real project row count
// without the operator flipping an env flag. Errors and a missing DB
// pool fall back to false — Reader chrome stays single-project rather
// than spuriously showing a switcher when the lookup hiccups.
func (d Deps) deriveMultiProject(r *http.Request) bool {
	return d.deriveMultiProjectCaps(r).MultiProjectSwitching
}

func (d Deps) deriveMultiProjectCaps(r *http.Request) projects.MultiProjectCaps {
	if d.DB == nil {
		return projects.CapabilitiesForVisibleCount(0)
	}
	n, err := projects.CountVisible(r.Context(), d.DB, d.viewerScopeForRequest(r))
	if err != nil {
		if d.Logger != nil {
			d.Logger.Warn("multi-project capability derivation failed; defaulting switching to false",
				"err", err,
			)
		}
		return projects.CapabilitiesForVisibleCount(0)
	}
	return projects.CapabilitiesForVisibleCount(n)
}

func (d Deps) viewerScopeForRequest(r *http.Request) projects.ViewerScope {
	principal := d.principalForRequest(r)
	if principal == nil || strings.TrimSpace(principal.UserID) == "" {
		return projects.ViewerScope{AnonymousOnly: true}
	}
	if principal.IsLoopback() {
		return projects.ViewerScope{UserID: principal.UserID, TrustedLocal: true}
	}
	return projects.ViewerScope{UserID: principal.UserID}
}

// checkOnboardingRequired returns true when the instance has no
// projects at all. Used by /api/config so the SPA can decide whether
// to redirect a fresh user into the wizard. Migration 0058 removes
// the empty `pindoc` bootstrap row on fresh installs, so the count
// directly reflects "operator has created any project yet?". Errors
// are logged and treated as "not required" — better to show the
// legacy landing than to dead-end on an unrelated DB hiccup. Cheap
// query (single COUNT) but skipped when DB pool is missing for
// tests that stub Deps.
func (d Deps) checkOnboardingRequired(ctx context.Context) bool {
	if d.DB == nil {
		return false
	}
	var count int
	if err := d.DB.QueryRow(ctx, `SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		d.Logger.Warn("onboarding check failed; defaulting to not required", "err", err)
		return false
	}
	return count == 0
}

// principalForRequest is the single helper every Reader-side HTTP
// handler calls to identify the calling user. Loopback addresses get
// auto-trusted owner principals (Decision § 2 Loopback Trust); non-
// loopback requests must present a valid OAuth browser session.
// Wraps auth.PrincipalFromRequest with the daemon's defaults so
// handlers don't repeat them on every call site.
//
// DefaultUserID is read from settings.DefaultLoopbackUserID at
// request time so the onboarding flow's identity binding takes
// effect immediately — no daemon restart required after a fresh
// operator submits the form. Falls back to Deps.DefaultUserID
// (env-derived bootstrap) when settings is unset (test fixtures).
func (d Deps) principalForRequest(r *http.Request) *pauth.Principal {
	defaultUserID := strings.TrimSpace(d.DefaultUserID)
	if d.Settings != nil {
		if uid := strings.TrimSpace(d.Settings.Get().DefaultLoopbackUserID); uid != "" {
			defaultUserID = uid
		}
	}
	return pauth.PrincipalFromRequest(r, pauth.HTTPDeps{
		OAuth:                d.OAuth,
		DefaultUserID:        defaultUserID,
		DefaultAgentID:       d.DefaultAgentID,
		TrustedSameHostProxy: d.TrustedSameHostProxy,
	})
}

type projectListRow struct {
	ID               string    `json:"id"`
	Slug             string    `json:"slug"`
	OrgSlug          string    `json:"org_slug,omitempty"`
	OrganizationSlug string    `json:"organization_slug,omitempty"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	Color            string    `json:"color,omitempty"`
	PrimaryLanguage  string    `json:"primary_language"`
	ArtifactsCount   int       `json:"artifacts_count"`
	CreatedAt        time.Time `json:"created_at"`
	ReaderHidden     bool      `json:"reader_hidden,omitempty"`
}

// userRow is the thin projection of users table rows TaskControls needs
// to build the assignee dropdown (Decision agent-only-write-분할 AC
// "users + agents"). email is intentionally omitted so a published
// Pindoc instance doesn't leak addresses in UI payloads.
type userRow struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	GithubHandle string `json:"github_handle,omitempty"`
	Source       string `json:"source"`
}

type currentUserRow struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email,omitempty"`
	GithubHandle string `json:"github_handle,omitempty"`
	Source       string `json:"source"`
	AuthMode     string `json:"auth_mode"`
}

type currentUserResponse struct {
	Status   string          `json:"status"`
	AuthMode string          `json:"auth_mode"`
	User     *currentUserRow `json:"user,omitempty"`
}

// handleUserList returns all rows in `users` ordered by display_name.
// Unscoped on purpose — the users table is instance-wide (migration 0014).
// Reader's TaskControls fetches this once per shell load to populate the
// assignee dropdown alongside the project's agents aggregate.
func (d Deps) handleUserList(w http.ResponseWriter, r *http.Request) {
	rows, err := d.DB.Query(r.Context(), `
		SELECT id::text, display_name, github_handle, source
		FROM users
		WHERE deleted_at IS NULL
		ORDER BY display_name
	`)
	if err != nil {
		d.Logger.Error("user list", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []userRow{}
	for rows.Next() {
		var u userRow
		var gh *string
		if err := rows.Scan(&u.ID, &u.DisplayName, &gh, &u.Source); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if gh != nil {
			u.GithubHandle = *gh
		}
		out = append(out, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (d Deps) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	principal := d.principalForRequest(r)
	if principal == nil {
		writeJSON(w, http.StatusOK, currentUserResponse{
			Status:   "not_authenticated",
			AuthMode: "unknown",
		})
		return
	}
	authMode := "trusted_local"
	if principal.Source == pauth.SourceOAuth {
		authMode = "oauth_github"
	}
	if strings.TrimSpace(principal.UserID) == "" {
		writeJSON(w, http.StatusOK, currentUserResponse{
			Status:   "informational",
			AuthMode: authMode,
		})
		return
	}
	var u currentUserRow
	var email, github *string
	err := d.DB.QueryRow(r.Context(), `
		SELECT id::text, display_name, email, github_handle, source
		  FROM users
		 WHERE id = $1 AND deleted_at IS NULL
	`, principal.UserID).Scan(&u.ID, &u.DisplayName, &email, &github, &u.Source)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, currentUserResponse{
			Status:   "informational",
			AuthMode: authMode,
		})
		return
	}
	if err != nil {
		d.Logger.Error("current user", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if email != nil {
		u.Email = *email
	}
	if github != nil {
		u.GithubHandle = *github
	}
	u.AuthMode = authMode
	writeJSON(w, http.StatusOK, currentUserResponse{
		Status:   "ok",
		AuthMode: authMode,
		User:     &u,
	})
}

func (d Deps) handleProjectList(w http.ResponseWriter, r *http.Request) {
	principal := d.principalForRequest(r)
	rows, err := projects.ListVisible(r.Context(), d.DB, d.viewerScopeForRequest(r))
	if err != nil {
		d.Logger.Error("project list", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	out := []projectListRow{}
	for _, row := range rows {
		p := projectListRow{
			ID:               row.ID,
			Slug:             row.Slug,
			OrganizationSlug: row.OrganizationSlug,
			Name:             row.Name,
			Description:      row.Description,
			Color:            row.Color,
			PrimaryLanguage:  row.PrimaryLanguage,
			ArtifactsCount:   row.ArtifactsCount,
			CreatedAt:        row.CreatedAt,
		}
		p.OrgSlug = p.OrganizationSlug
		p.ReaderHidden = readerHiddenProjectSlug(p.Slug)
		scope := &pauth.ProjectScope{Role: row.Role}
		if principal != nil && principal.IsLoopback() {
			scope.Role = pauth.RoleOwner
		}
		if p.ReaderHidden && !includeReaderHiddenProjects(r, scope) {
			continue
		}
		out = append(out, p)
	}
	projectCaps := projects.CapabilitiesForVisibleCount(len(out))
	writeJSON(w, http.StatusOK, map[string]any{
		"projects":                out,
		"default_project_slug":    d.DefaultProjectSlug,
		"multi_project":           projectCaps.MultiProjectSwitching,
		"multi_project_switching": projectCaps.MultiProjectSwitching,
		"project_create_allowed":  projectCaps.ProjectCreateAllowed,
	})
}

func (d Deps) handleProjectCurrent(w http.ResponseWriter, r *http.Request) {
	projectPredicate, projectArg := projectLookupPredicate(r, "p", 1)
	var out projectInfo
	var desc, color *string
	err := d.DB.QueryRow(r.Context(), fmt.Sprintf(`
		SELECT
			p.id::text, p.slug, o.slug, p.name, p.description, p.color,
			p.primary_language, p.primary_language, COALESCE(NULLIF(p.sensitive_ops, ''), 'auto'),
			COALESCE(NULLIF(p.visibility, ''), 'org'),
			COALESCE(NULLIF(p.default_artifact_visibility, ''), 'org'), p.created_at,
			(SELECT count(*) FROM areas     WHERE project_id = p.id),
			(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
		FROM projects p
		LEFT JOIN organizations o ON o.id = p.organization_id
		WHERE %s
	`, projectPredicate), projectArg).Scan(
		&out.ID, &out.Slug, &out.OrganizationSlug, &out.Name, &desc, &color,
		&out.PrimaryLanguage, &out.Locale, &out.SensitiveOps,
		&out.Visibility, &out.DefaultArtifactVisibility, &out.CreatedAt,
		&out.AreasCount, &out.ArtifactsCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		d.Logger.Error("project query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if desc != nil {
		out.Description = *desc
	}
	if color != nil {
		out.Color = *color
	}
	out.OrgSlug = out.OrganizationSlug
	out.Rendering = pindocRenderingCaps
	out.Capabilities = ProjectCaps{ReviewQueueSupported: true}
	out.CurrentRole = d.currentProjectRole(r.Context(), r, out.Slug)
	writeJSON(w, http.StatusOK, out)
}

// currentProjectRole returns the calling caller's role on the named
// project. Loopback callers — the operator on their own box — see
// every project as owner so the Reader's role chip lights up
// correctly. OAuth callers consult project_members via
// auth.ResolveProject. Empty string means "no role" (anonymous or
// project not found) — the Reader treats that as read-only.
func (d Deps) currentProjectRole(ctx context.Context, r *http.Request, projectSlug string) string {
	if d.DB == nil {
		return ""
	}
	principal := d.principalForRequest(r)
	if principal == nil {
		return ""
	}
	if principal.IsLoopback() {
		return pauth.RoleOwner
	}
	scope, err := pauth.ResolveProject(ctx, d.DB, principal, projectSlug)
	if err != nil {
		return ""
	}
	return scope.Role
}

type areaRow struct {
	ID             string   `json:"id"`
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	ParentSlug     string   `json:"parent_slug,omitempty"`
	IsCrossCutting bool     `json:"is_cross_cutting"`
	ArtifactCount  int      `json:"artifact_count"`
	ChildrenSlugs  []string `json:"children_slugs,omitempty"`
}

func (d Deps) handleAreas(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	projectPredicate, projectArg := projectLookupPredicate(r, "projects", 1)
	visibilityPredicate, visibilityArgs := d.artifactVisibilityPredicate(r, "x", 3)
	// include_templates=true counts _template_* artifacts in artifact_count.
	// Must stay in lockstep with handleArtifactList's filter so Sidebar
	// counts == list cardinality. Fixing the Phase 13 regression where
	// counts included templates but the list excluded them.
	includeTemplates := r.URL.Query().Get("include_templates") == "true"
	args := append([]any{projectArg, includeTemplates}, visibilityArgs...)
	rows, err := d.DB.Query(r.Context(), fmt.Sprintf(`
		WITH p AS (SELECT id FROM projects WHERE %s)
		SELECT
			a.id::text, a.slug, a.name, a.description,
			parent.slug, a.is_cross_cutting,
			(SELECT count(*) FROM artifacts x
			  WHERE x.area_id = a.id
			    AND x.status <> 'archived'
			    AND ($2::bool OR NOT starts_with(x.slug, '_template_'))
			    AND %s),
			COALESCE(ARRAY(
			  SELECT c.slug FROM areas c
			  WHERE c.parent_id = a.id
			  ORDER BY c.slug
			), ARRAY[]::text[])
		FROM areas a
		JOIN p ON a.project_id = p.id
		LEFT JOIN areas parent ON parent.id = a.parent_id
		ORDER BY a.is_cross_cutting, a.slug
	`, projectPredicate, visibilityPredicate), args...)
	if err != nil {
		d.Logger.Error("areas query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []areaRow{}
	for rows.Next() {
		var a areaRow
		var desc, parent *string
		if err := rows.Scan(
			&a.ID, &a.Slug, &a.Name, &desc,
			&parent, &a.IsCrossCutting,
			&a.ArtifactCount, &a.ChildrenSlugs,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if desc != nil {
			a.Description = *desc
		}
		if parent != nil {
			a.ParentSlug = *parent
		}
		out = append(out, a)
	}
	body := map[string]any{
		"project_slug": slug,
		"areas":        out,
	}
	if org := orgSlugFrom(r); org != "" {
		body["org_slug"] = org
		body["organization_slug"] = org
	}
	writeJSON(w, http.StatusOK, body)
}

type artifactRow struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	Type         string    `json:"type"`
	Title        string    `json:"title"`
	AreaSlug     string    `json:"area_slug"`
	BodyLocale   string    `json:"body_locale,omitempty"`
	Visibility   string    `json:"visibility"`
	Completeness string    `json:"completeness"`
	Status       string    `json:"status"`
	ReviewState  string    `json:"review_state"`
	AuthorID     string    `json:"author_id"`
	PublishedAt  time.Time `json:"published_at,omitzero"`
	UpdatedAt    time.Time `json:"updated_at"`
	// TaskMeta is raw JSONB for Task artifacts (Phase 15b). Pass-through
	// to the client — Reader parses it to lay out the Tasks view as a
	// kanban-lite grouped by status. null / omitted for non-Task.
	TaskMeta json.RawMessage `json:"task_meta,omitempty"`
	// ArtifactMeta is raw JSONB carrying the epistemic axes (migration
	// 0012). Pass-through so the Reader Trust Card component narrows the
	// fields it cares about client-side. Empty object for rows that
	// predate migration 0012.
	ArtifactMeta json.RawMessage `json:"artifact_meta,omitempty"`
	// AuthorUser joins the user row pointed at by artifacts.author_user_id
	// (migration 0014). Null when the artifact predates identity-dual
	// (D-slug backfill rule: existing rows stay null) or when the MCP
	// server ran without PINDOC_USER_NAME. Reader falls back to
	// "(unknown) via {author_id}" byline in that case.
	AuthorUser *authorUserRef `json:"author_user,omitempty"`
	// RecentWarnings mirrors the detail payload's current warning chips so
	// the Reader can filter list views by warning badges without fetching
	// every artifact detail.
	RecentWarnings []recentWarningRow `json:"recent_warnings,omitempty"`
}

const (
	artifactListDefaultLimit = 100
	artifactListMaxLimit     = 200
)

type artifactListCursor struct {
	TaskRank  int       `json:"task_rank"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
}

func artifactListTaskRank(artifactType string) int {
	if artifactType == "Task" {
		return 1
	}
	return 0
}

func parseArtifactListLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return artifactListDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid limit")
	}
	if n > artifactListMaxLimit {
		return artifactListMaxLimit, nil
	}
	return n, nil
}

func encodeArtifactListCursor(a artifactRow) (string, error) {
	payload, err := json.Marshal(artifactListCursor{
		TaskRank:  artifactListTaskRank(a.Type),
		UpdatedAt: a.UpdatedAt,
		ID:        a.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeArtifactListCursor(raw string) (*artifactListCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var cursor artifactListCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return nil, err
	}
	if cursor.TaskRank < 0 || cursor.TaskRank > 1 || cursor.UpdatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &cursor, nil
}

const maxGraphEdges = 5000

type graphEdgesResponse struct {
	ProjectSlug string         `json:"project_slug"`
	Edges       []graphEdgeRow `json:"edges"`
	Truncated   bool           `json:"truncated,omitempty"`
}

type graphEdgeRow struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Relation string `json:"relation"`
}

func (d Deps) handleGraphEdges(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	projectPredicate, projectArg := projectLookupPredicate(r, "p", 1)
	sourceVisibilityPredicate, sourceVisibilityArgs := d.artifactVisibilityPredicate(r, "s", 2)
	targetVisibilityPredicate, targetVisibilityArgs := d.artifactVisibilityPredicate(r, "t", 2+len(sourceVisibilityArgs))
	includeTemplates := r.URL.Query().Get("include_templates") == "true"

	args := append([]any{projectArg}, sourceVisibilityArgs...)
	args = append(args, targetVisibilityArgs...)
	includeTemplatesArg := len(args) + 1
	args = append(args, includeTemplates)
	limitArg := len(args) + 1
	args = append(args, maxGraphEdges+1)

	rows, err := d.DB.Query(r.Context(), fmt.Sprintf(`
		SELECT e.source_id::text, e.target_id::text, e.relation
		FROM artifact_edges e
		JOIN artifacts s ON s.id = e.source_id
		JOIN artifacts t ON t.id = e.target_id
		JOIN projects p ON p.id = s.project_id AND p.id = t.project_id
		WHERE %s
		  AND s.status <> 'archived'
		  AND t.status <> 'archived'
		  AND ($%d::bool OR (NOT starts_with(s.slug, '_template_') AND NOT starts_with(t.slug, '_template_')))
		  AND %s
		  AND %s
		ORDER BY e.created_at, e.id
		LIMIT $%d
	`, projectPredicate, includeTemplatesArg, sourceVisibilityPredicate, targetVisibilityPredicate, limitArg), args...)
	if err != nil {
		d.Logger.Error("graph edges query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	edges := []graphEdgeRow{}
	for rows.Next() {
		var edge graphEdgeRow
		if err := rows.Scan(&edge.SourceID, &edge.TargetID, &edge.Relation); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	truncated := len(edges) > maxGraphEdges
	if truncated {
		edges = edges[:maxGraphEdges]
	}
	writeJSON(w, http.StatusOK, graphEdgesResponse{
		ProjectSlug: slug,
		Edges:       edges,
		Truncated:   truncated,
	})
}

// authorUserRef is the thin join projection of users → artifact list.
// We intentionally omit email so Pindoc-self publication doesn't leak
// addresses; display_name + github_handle is what Reader needs for the
// avatar + byline.
type authorUserRef struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	GithubHandle string `json:"github_handle,omitempty"`
}

func (d Deps) handleArtifactList(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	projectPredicate, projectArg := projectLookupPredicate(r, "p", 1)
	visibilityPredicate, visibilityArgs := d.artifactVisibilityPredicate(r, "a", 5)
	areaSlug := r.URL.Query().Get("area")
	typeFilter := r.URL.Query().Get("type")
	limit, err := parseArtifactListLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	cursor, err := decodeArtifactListCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	// include_templates=true surfaces _template_* artifacts (Phase 13).
	// Default hides them so the reader list stays focused on real docs.
	includeTemplates := r.URL.Query().Get("include_templates") == "true"
	args := append([]any{projectArg, areaSlug, typeFilter, includeTemplates}, visibilityArgs...)

	cursorPredicate := "TRUE"
	if cursor != nil {
		rankArg := len(args) + 1
		args = append(args, cursor.TaskRank)
		updatedArg := len(args) + 1
		args = append(args, cursor.UpdatedAt)
		idArg := len(args) + 1
		args = append(args, cursor.ID)
		cursorPredicate = fmt.Sprintf(`(
			(CASE WHEN a.type = 'Task' THEN 1 ELSE 0 END) > $%d::int
			OR (
				(CASE WHEN a.type = 'Task' THEN 1 ELSE 0 END) = $%d::int
				AND (
					a.updated_at < $%d::timestamptz
					OR (a.updated_at = $%d::timestamptz AND a.id < $%d::uuid)
				)
			)
		)`, rankArg, rankArg, updatedArg, updatedArg, idArg)
	}
	limitArg := len(args) + 1
	args = append(args, limit+1)

	rows, err := d.DB.Query(r.Context(), fmt.Sprintf(`
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(NULLIF(a.body_locale, ''), NULLIF(p.primary_language, ''), 'en'),
			a.visibility,
			a.completeness, a.status, a.review_state,
			a.author_id, a.published_at, a.updated_at, a.task_meta, a.artifact_meta,
			'[]'::jsonb AS recent_warnings,
			u.id::text, u.display_name, u.github_handle
		FROM artifacts a
		JOIN projects p  ON p.id  = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		LEFT JOIN users u ON u.id = a.author_user_id
		WHERE %s
		  AND a.status <> 'archived'
		  AND ($2 = '' OR ar.slug = $2)
		  AND ($3 = '' OR a.type  = $3)
		  AND ($4::bool OR NOT starts_with(a.slug, '_template_'))
		  AND %s
		  AND %s
		-- Tasks churn far more than wiki bodies (Decision/Glossary/DataModel/...);
		-- without this Tasks alone fill the first page and wiki docs vanish from the list.
		ORDER BY (CASE WHEN a.type = 'Task' THEN 1 ELSE 0 END) ASC, a.updated_at DESC, a.id DESC
		LIMIT $%d
	`, projectPredicate, visibilityPredicate, cursorPredicate, limitArg), args...)
	if err != nil {
		d.Logger.Error("artifact list", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []artifactRow{}
	for rows.Next() {
		var a artifactRow
		var publishedAt *time.Time
		var taskMeta, artifactMeta, recentWarnings []byte
		var userID, userDisplay, userGithub *string
		if err := rows.Scan(
			&a.ID, &a.Slug, &a.Type, &a.Title, &a.AreaSlug,
			&a.BodyLocale, &a.Visibility,
			&a.Completeness, &a.Status, &a.ReviewState,
			&a.AuthorID, &publishedAt, &a.UpdatedAt, &taskMeta, &artifactMeta,
			&recentWarnings,
			&userID, &userDisplay, &userGithub,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if publishedAt != nil {
			a.PublishedAt = *publishedAt
		}
		if len(taskMeta) > 0 {
			a.TaskMeta = json.RawMessage(taskMeta)
		}
		if len(artifactMeta) > 0 {
			a.ArtifactMeta = json.RawMessage(artifactMeta)
		}
		if len(recentWarnings) > 0 && string(recentWarnings) != "[]" {
			if err := json.Unmarshal(recentWarnings, &a.RecentWarnings); err != nil {
				d.Logger.Warn("artifact list warning payload unmarshal failed",
					"artifact_id", a.ID, "err", err)
			}
		}
		if userID != nil && userDisplay != nil {
			ref := &authorUserRef{ID: *userID, DisplayName: *userDisplay}
			if userGithub != nil {
				ref.GithubHandle = *userGithub
			}
			a.AuthorUser = ref
		}
		out = append(out, a)
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	if warnings, err := d.loadRecentWarningsBatch(r.Context(), artifactRowIDs(out)); err != nil {
		d.Logger.Warn("artifact list warning batch lookup failed", "err", err)
	} else {
		for i := range out {
			out[i].RecentWarnings = warnings[out[i].ID]
		}
	}
	body := map[string]any{
		"project_slug": slug,
		"artifacts":    out,
		"has_more":     hasMore,
	}
	if hasMore && len(out) > 0 {
		nextCursor, err := encodeArtifactListCursor(out[len(out)-1])
		if err != nil {
			d.Logger.Error("artifact list cursor encode", "err", err)
			writeError(w, http.StatusInternalServerError, "cursor encode failed")
			return
		}
		body["next_cursor"] = nextCursor
	}
	if org := orgSlugFrom(r); org != "" {
		body["org_slug"] = org
		body["organization_slug"] = org
	}
	writeJSON(w, http.StatusOK, body)
}

type artifactDetail struct {
	artifactRow
	BodyMarkdown  string    `json:"body_markdown"`
	Tags          []string  `json:"tags"`
	AuthorVersion string    `json:"author_version,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	// RevisionNumber is the current head revision. Surfaced so the Reader
	// TaskControls can pass it back as expected_version on
	// POST .../task-meta without a second round-trip to /revisions.
	// Zero is never legal (migration 0017 backfills rev 1 for every row).
	RevisionNumber int `json:"revision_number"`
	// Relates and RelatedBy (Phase 15b) surface artifact_edges to the
	// Reader's Sidecar so users opening a Task/Decision see their
	// connected artifacts as one-click cards instead of hunting through
	// markdown references.
	Relates   []edgeRef `json:"relates_to,omitempty"`
	RelatedBy []edgeRef `json:"related_by,omitempty"`
	// Pins and SourceSessionRef feed the Reader Trust Card + Sidecar
	// provenance section (Task `reader-trust-card-...`). Pins let the
	// user confirm the code substrate behind an artifact; source_session
	// ref records the agent_id that authored the latest revision. Both
	// empty when the artifact has no pins / the agent didn't report a
	// session.
	Pins                 []pinRow              `json:"pins,omitempty"`
	Assets               []assetDetailRow      `json:"assets,omitempty"`
	SourceSessionRef     json.RawMessage       `json:"source_session_ref,omitempty"`
	VerificationNotes    []verificationNoteRow `json:"verification_notes,omitempty"`
	VerificationReceipts []string              `json:"verification_receipts,omitempty"`

	// RecentWarnings projects events.artifact.warning_raised rows for
	// the Reader Trust Card (Task propose-경로-warning-영속화). The
	// server returns up to 5 most-recent warning events so the Trust
	// Card can highlight the latest-revision advisories and older ones
	// stay accessible via the revision history. Empty when the artifact
	// has never raised a warning.
	RecentWarnings []recentWarningRow `json:"recent_warnings,omitempty"`
	// CanEditVisibility lets the Reader render the visibility chip as a
	// dropdown only for callers that can mutate project-level exposure.
	CanEditVisibility bool `json:"can_edit_visibility,omitempty"`
}

// recentWarningRow is the Reader-facing projection of one
// events.artifact.warning_raised row. `Codes` carries the raw stable
// codes ("CANONICAL_REWRITE_WITHOUT_EVIDENCE" etc.) so the client can
// decide which chip tone to render; RevisionNumber lets the Trust Card
// suppress badges from older revisions when the latest revision is
// clean.
type recentWarningRow struct {
	Codes                           []string  `json:"codes"`
	RevisionNumber                  int       `json:"revision_number"`
	AuthorID                        string    `json:"author_id,omitempty"`
	CanonicalRewriteWithoutEvidence bool      `json:"canonical_rewrite_without_evidence,omitempty"`
	CreatedAt                       time.Time `json:"created_at"`
}

type edgeRef struct {
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Relation   string `json:"relation"`
	// BodyLocale is the BCP47 locale of the target artifact's body. The
	// Reader uses it to decorate translation_of edges with a language
	// chip ("EN" / "KO") so the user knows which translation each link
	// leads to without clicking through.
	BodyLocale string `json:"body_locale,omitempty"`
}

// pinRow mirrors artifact_pins. Reader Sidecar groups by Kind for the
// provenance block. Empty fields dropped by omitempty so the wire payload
// stays compact on resource/url pins that don't carry line ranges.
type pinRow struct {
	Kind       string `json:"kind"`
	RepoID     string `json:"repo_id,omitempty"`
	Repo       string `json:"repo,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Path       string `json:"path"`
	LinesStart int    `json:"lines_start,omitempty"`
	LinesEnd   int    `json:"lines_end,omitempty"`
}

type verificationNoteRow struct {
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Command string `json:"command,omitempty"`
}

func (d Deps) handleArtifactGet(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	projectPredicate, projectArg := projectLookupPredicate(r, "p", 1)
	visibilityPredicate, visibilityArgs := d.artifactVisibilityPredicate(r, "a", 3)
	ref := r.PathValue("idOrSlug")
	if ref == "" {
		writeError(w, http.StatusBadRequest, "missing id or slug")
		return
	}

	var a artifactDetail
	var publishedAt *time.Time
	var authorVer *string
	var taskMeta, artifactMeta, sourceSessionRef []byte
	var userID, userDisplay, userGithub *string
	args := append([]any{projectArg, ref}, visibilityArgs...)
	err := d.DB.QueryRow(r.Context(), fmt.Sprintf(`
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(NULLIF(a.body_locale, ''), NULLIF(p.primary_language, ''), 'en'),
			a.visibility,
			a.completeness, a.status, a.review_state,
			a.author_id, a.published_at, a.updated_at,
			a.body_markdown, a.tags, a.author_version, a.created_at,
			a.task_meta, a.artifact_meta, a.source_session_ref,
			u.id::text, u.display_name, u.github_handle,
			COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p  ON p.id  = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		LEFT JOIN users u ON u.id = a.author_user_id
		WHERE %s AND (
			a.id::text = $2 OR a.slug = $2 OR
			a.id = (
				SELECT asa.artifact_id
				  FROM artifact_slug_aliases asa
				 WHERE asa.project_id = p.id AND asa.old_slug = $2
				 LIMIT 1
			)
		)
		  AND %s
		LIMIT 1
	`, projectPredicate, visibilityPredicate), args...).Scan(
		&a.ID, &a.Slug, &a.Type, &a.Title, &a.AreaSlug,
		&a.BodyLocale, &a.Visibility,
		&a.Completeness, &a.Status, &a.ReviewState,
		&a.AuthorID, &publishedAt, &a.UpdatedAt,
		&a.BodyMarkdown, &a.Tags, &authorVer, &a.CreatedAt,
		&taskMeta, &artifactMeta, &sourceSessionRef,
		&userID, &userDisplay, &userGithub,
		&a.RevisionNumber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		d.Logger.Error("artifact get", "err", err, "ref", ref)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if publishedAt != nil {
		a.PublishedAt = *publishedAt
	}
	if authorVer != nil {
		a.AuthorVersion = *authorVer
	}
	if len(taskMeta) > 0 {
		a.TaskMeta = json.RawMessage(taskMeta)
	}
	if len(artifactMeta) > 0 {
		a.ArtifactMeta = json.RawMessage(artifactMeta)
	}
	if len(sourceSessionRef) > 0 {
		a.SourceSessionRef = json.RawMessage(sourceSessionRef)
	}
	if userID != nil && userDisplay != nil {
		ref := &authorUserRef{ID: *userID, DisplayName: *userDisplay}
		if userGithub != nil {
			ref.GithubHandle = *userGithub
		}
		a.AuthorUser = ref
	}
	if principal := d.principalForRequest(r); principal != nil {
		if scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, slug); err == nil && scope.Can("write.project") {
			a.CanEditVisibility = true
		}
	}

	// Load edges (best-effort — failure leaves the slices empty, artifact
	// still renders). Outgoing (this → others) goes in Relates; incoming
	// (others → this) in RelatedBy.
	if outEdges, err := d.loadEdges(r.Context(), a.ID, "out"); err != nil {
		d.Logger.Warn("edge outgoing lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.Relates = outEdges
	}
	if inEdges, err := d.loadEdges(r.Context(), a.ID, "in"); err != nil {
		d.Logger.Warn("edge incoming lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.RelatedBy = inEdges
	}
	if pins, err := d.loadArtifactPins(r.Context(), a.ID); err != nil {
		d.Logger.Warn("pins lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.Pins = pins
	}
	if assetRows, err := d.loadArtifactAssets(r.Context(), slug, a.ID, a.RevisionNumber); err != nil {
		d.Logger.Warn("asset relation lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.Assets = assetRows
	}
	if warnings, err := d.loadRecentWarnings(r.Context(), a.ID); err != nil {
		d.Logger.Warn("recent warnings lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.RecentWarnings = warnings
	}
	if notes, receipts, err := d.loadLatestVerificationEvidence(r.Context(), a.ID); err != nil {
		d.Logger.Warn("verification evidence lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.VerificationNotes = notes
		a.VerificationReceipts = receipts
	}

	writeJSON(w, http.StatusOK, a)
}

func (d Deps) loadLatestVerificationEvidence(ctx context.Context, artifactID string) ([]verificationNoteRow, []string, error) {
	var raw []byte
	err := d.DB.QueryRow(ctx, `
		SELECT shape_payload
		  FROM artifact_revisions
		 WHERE artifact_id = $1
		   AND shape_payload->>'kind' = 'claim_done'
		   AND (
		        shape_payload ? 'verification_notes'
		     OR shape_payload ? 'verification_receipts'
		   )
		 ORDER BY revision_number DESC
		 LIMIT 1
	`, artifactID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		VerificationNotes    []verificationNoteRow `json:"verification_notes"`
		VerificationReceipts []string              `json:"verification_receipts"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, nil, err
	}
	return parsed.VerificationNotes, normalizeStringRefs(parsed.VerificationReceipts), nil
}

func normalizeStringRefs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "pindoc://"))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// loadRecentWarnings returns up to 5 most-recent
// `events.artifact.warning_raised` rows for an artifact, newest first.
// Reader Trust Card renders badges from this list (Task propose-경로-
// warning-영속화). Missing events.subject_id ↔ artifact.id join is
// expected for artifacts created before the persistence hook landed —
// the slice just comes back empty.
func (d Deps) loadRecentWarnings(ctx context.Context, artifactID string) ([]recentWarningRow, error) {
	rows, err := d.DB.Query(ctx, `
		SELECT payload, created_at
		  FROM events
		 WHERE subject_id = $1 AND kind = 'artifact.warning_raised'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 5
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []recentWarningRow
	for rows.Next() {
		var payload []byte
		var createdAt time.Time
		if err := rows.Scan(&payload, &createdAt); err != nil {
			return nil, err
		}
		row, ok := parseRecentWarningRow(payload, createdAt)
		if !ok {
			d.Logger.Warn("warning event payload unmarshal failed", "artifact_id", artifactID)
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func artifactRowIDs(rows []artifactRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" {
			out = append(out, row.ID)
		}
	}
	return out
}

func (d Deps) loadRecentWarningsBatch(ctx context.Context, artifactIDs []string) (map[string][]recentWarningRow, error) {
	out := map[string][]recentWarningRow{}
	if len(artifactIDs) == 0 {
		return out, nil
	}
	rows, err := d.DB.Query(ctx, `
		WITH ranked AS (
			SELECT
				subject_id::text AS artifact_id,
				payload,
				created_at,
				row_number() OVER (
					PARTITION BY subject_id
					ORDER BY created_at DESC, id DESC
				) AS rn
			FROM events
			WHERE kind = 'artifact.warning_raised'
			  AND subject_id = ANY($1::uuid[])
		)
		SELECT artifact_id, payload, created_at
		FROM ranked
		WHERE rn <= 5
		ORDER BY artifact_id, created_at DESC
	`, artifactIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var artifactID string
		var payload []byte
		var createdAt time.Time
		if err := rows.Scan(&artifactID, &payload, &createdAt); err != nil {
			return nil, err
		}
		row, ok := parseRecentWarningRow(payload, createdAt)
		if !ok {
			d.Logger.Warn("warning event payload unmarshal failed", "artifact_id", artifactID)
			continue
		}
		out[artifactID] = append(out[artifactID], row)
	}
	return out, rows.Err()
}

func parseRecentWarningRow(payload []byte, createdAt time.Time) (recentWarningRow, bool) {
	var parsed struct {
		Codes                           []string `json:"codes"`
		RevisionNumber                  int      `json:"revision_number"`
		AuthorID                        string   `json:"author_id"`
		CanonicalRewriteWithoutEvidence bool     `json:"canonical_rewrite_without_evidence"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return recentWarningRow{}, false
	}
	return recentWarningRow{
		Codes:                           parsed.Codes,
		RevisionNumber:                  parsed.RevisionNumber,
		AuthorID:                        parsed.AuthorID,
		CanonicalRewriteWithoutEvidence: parsed.CanonicalRewriteWithoutEvidence,
		CreatedAt:                       createdAt,
	}, true
}

// loadArtifactPins returns artifact_pins rows sorted by insertion order.
// The Reader Sidecar renders them grouped by kind so provenance inspection
// lands on the most likely evidence type without extra clicks.
func (d Deps) loadArtifactPins(ctx context.Context, artifactID string) ([]pinRow, error) {
	rows, err := d.DB.Query(ctx, `
		SELECT kind, repo_id::text, repo, commit_sha, path, lines_start, lines_end
		FROM artifact_pins
		WHERE artifact_id = $1
		ORDER BY id
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pinRow
	for rows.Next() {
		var p pinRow
		var repoID, commitSHA *string
		var ls, le *int
		if err := rows.Scan(&p.Kind, &repoID, &p.Repo, &commitSHA, &p.Path, &ls, &le); err != nil {
			return nil, err
		}
		if repoID != nil {
			p.RepoID = *repoID
		}
		if commitSHA != nil {
			p.CommitSHA = *commitSHA
		}
		if ls != nil {
			p.LinesStart = *ls
		}
		if le != nil {
			p.LinesEnd = *le
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadEdges returns artifact_edges rows from the perspective of the given
// artifact ID. direction="out" lists outgoing edges (this artifact's
// relates_to); direction="in" lists incoming (others pointing at this).
// The target/source join resolves slug+type+title so the Reader can
// render cards without a second fetch.
func (d Deps) loadEdges(ctx context.Context, artifactID, direction string) ([]edgeRef, error) {
	var sql string
	switch direction {
	case "out":
		sql = `SELECT e.target_id::text, a.slug, a.type, a.title, e.relation, COALESCE(a.body_locale, '')
			FROM artifact_edges e
			JOIN artifacts a ON a.id = e.target_id
			WHERE e.source_id = $1
			ORDER BY e.created_at`
	case "in":
		sql = `SELECT e.source_id::text, a.slug, a.type, a.title, e.relation, COALESCE(a.body_locale, '')
			FROM artifact_edges e
			JOIN artifacts a ON a.id = e.source_id
			WHERE e.target_id = $1
			ORDER BY e.created_at`
	default:
		return nil, errors.New("bad direction")
	}
	rows, err := d.DB.Query(ctx, sql, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []edgeRef
	for rows.Next() {
		var e edgeRef
		if err := rows.Scan(&e.ArtifactID, &e.Slug, &e.Type, &e.Title, &e.Relation, &e.BodyLocale); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d Deps) handleSearch(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	// include_templates=true surfaces _template_* artifacts. Default false
	// matches MCP artifact.search / context.for_task / artifact list for
	// the "sidebar count == list.length" invariant per the Phase 17 follow-up.
	includeTemplates := r.URL.Query().Get("include_templates") == "true"
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	crossProject := parseSearchCrossProject(r.URL.Query().Get("cross_project"))
	if d.Embedder == nil {
		body := map[string]any{
			"query":        q,
			"project_slug": slug,
			"hits":         []any{},
			"notice":       "embedder not configured",
		}
		if org := orgSlugFrom(r); org != "" {
			body["org_slug"] = org
			body["organization_slug"] = org
		}
		if crossProject {
			body["cross_project"] = true
		}
		writeJSON(w, http.StatusOK, body)
		return
	}
	res, err := d.Embedder.Embed(r.Context(), embed.Request{Texts: []string{q}, Kind: embed.KindQuery})
	if err != nil {
		d.Logger.Error("embed query", "err", err)
		writeError(w, http.StatusInternalServerError, "embed failed")
		return
	}
	qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))

	projectPredicate, projectArg := projectLookupPredicate(r, "p", 2)
	projectFilter := projectPredicate
	args := []any{qVec, projectArg, includeTemplates, typeFilter}
	limit := 10
	if crossProject {
		visibleProjects, err := projects.ListVisible(r.Context(), d.DB, d.viewerScopeForRequest(r))
		if err != nil {
			d.Logger.Error("search visible projects", "err", err)
			writeError(w, http.StatusInternalServerError, "search failed")
			return
		}
		visibleProjectIDs := make([]string, 0, len(visibleProjects))
		for _, row := range visibleProjects {
			visibleProjectIDs = append(visibleProjectIDs, row.ID)
		}
		if len(visibleProjectIDs) == 0 {
			info := d.Embedder.Info()
			writeJSON(w, http.StatusOK, map[string]any{
				"query":         q,
				"project_slug":  slug,
				"cross_project": true,
				"hits":          []any{},
				"embedder_used": map[string]any{
					"name":      info.Name,
					"model_id":  info.ModelID,
					"dimension": info.Dimension,
				},
			})
			return
		}
		projectFilter = "p.id::text = ANY($2::text[])"
		args = []any{qVec, visibleProjectIDs, includeTemplates, typeFilter}
		limit = 20
	}
	visibilityPredicate, visibilityArgs := d.artifactVisibilityPredicate(r, "a", 5)
	args = append(args, visibilityArgs...)
	candidateLimitArg := len(args) + 1
	args = append(args, searchCandidateLimit(limit))
	limitArg := len(args) + 1
	args = append(args, limit)

	rows, err := d.DB.Query(r.Context(), fmt.Sprintf(`
		WITH nearest AS MATERIALIZED (
			SELECT
				c.id AS chunk_id, c.artifact_id,
				c.embedding <=> $1::vector AS distance
			FROM artifact_chunks c
			JOIN artifacts a ON a.id = c.artifact_id
			JOIN projects p ON p.id = a.project_id
			WHERE %s
			  AND a.status <> 'archived'
			  AND ($3::bool OR NOT starts_with(a.slug, '_template_'))
			  AND ($4::text = '' OR lower(a.type) = lower($4::text))
			  AND %s
			ORDER BY c.embedding <=> $1::vector
			LIMIT $%d
		),
		scored AS (
			SELECT DISTINCT ON (artifact_id)
				chunk_id, artifact_id, distance
			FROM nearest
			ORDER BY artifact_id, distance
		)
		SELECT
			s.artifact_id::text, p.slug, COALESCE(o.slug, ''),
			a.slug, a.type, a.title, ar.slug,
			COALESCE(c.heading, '') , c.text, s.distance,
			a.updated_at, a.status, a.completeness,
			COALESCE(a.task_meta->>'status', ''),
			COALESCE(a.task_meta->>'priority', '')
		FROM scored s
		JOIN artifact_chunks c ON c.id = s.chunk_id
		JOIN artifacts a  ON a.id  = s.artifact_id
		JOIN areas     ar ON ar.id = a.area_id
		JOIN projects  p  ON p.id  = a.project_id
		LEFT JOIN organizations o ON o.id = p.organization_id
		ORDER BY s.distance
		LIMIT $%d
	`, projectFilter, visibilityPredicate, candidateLimitArg, limitArg), args...)
	if err != nil {
		d.Logger.Error("search", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	type hit struct {
		ArtifactID   string    `json:"artifact_id"`
		ProjectSlug  string    `json:"project_slug"`
		OrgSlug      string    `json:"org_slug"`
		Slug         string    `json:"slug"`
		Type         string    `json:"type"`
		Title        string    `json:"title"`
		AreaSlug     string    `json:"area_slug"`
		Heading      string    `json:"heading,omitempty"`
		Snippet      string    `json:"snippet"`
		Distance     float64   `json:"distance"`
		UpdatedAt    time.Time `json:"updated_at"`
		Status       string    `json:"status"`
		Completeness string    `json:"completeness"`
		TaskStatus   string    `json:"task_status,omitempty"`
		TaskPriority string    `json:"task_priority,omitempty"`
	}
	out := []hit{}
	for rows.Next() {
		var h hit
		if err := rows.Scan(
			&h.ArtifactID, &h.ProjectSlug, &h.OrgSlug,
			&h.Slug, &h.Type, &h.Title, &h.AreaSlug,
			&h.Heading, &h.Snippet, &h.Distance,
			&h.UpdatedAt, &h.Status, &h.Completeness, &h.TaskStatus, &h.TaskPriority,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if len(h.Snippet) > 280 {
			h.Snippet = h.Snippet[:280] + "..."
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	info := d.Embedder.Info()
	body := map[string]any{
		"query":        q,
		"project_slug": slug,
		"hits":         out,
		"embedder_used": map[string]any{
			"name":      info.Name,
			"model_id":  info.ModelID,
			"dimension": info.Dimension,
		},
	}
	if org := orgSlugFrom(r); org != "" {
		body["org_slug"] = org
		body["organization_slug"] = org
	}
	if crossProject {
		body["cross_project"] = true
	}
	writeJSON(w, http.StatusOK, body)
}

func parseSearchCrossProject(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func searchCandidateLimit(finalLimit int) int {
	if finalLimit <= 0 {
		return 200
	}
	n := finalLimit * 50
	if n < 200 {
		return 200
	}
	if n > 2000 {
		return 2000
	}
	return n
}

func (d Deps) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := d.DB.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db unreachable")
		return
	}
	info := map[string]any{
		"ok":                   true,
		"version":              d.Version,
		"default_project_slug": d.DefaultProjectSlug,
		"multi_project":        d.deriveMultiProject(r),
	}
	if d.Embedder != nil {
		ei := d.Embedder.Info()
		info["embedder"] = map[string]any{"name": ei.Name, "model": ei.ModelID, "dim": ei.Dimension}
	}
	writeJSON(w, http.StatusOK, info)
}

// handleSPA serves the Reader UI build output as static files with a
// client-side-routing fallback. Anything that resolves to an existing
// file under d.SPADistDir (assets/, favicon, etc.) is served directly;
// every other path returns index.html so React Router can render the
// page client-side. d.SPADistDir is trusted to be an absolute path —
// the resolver guards against `..` traversal anyway since the daemon
// is exposed on loopback only and we'd rather refuse than guess.
func (d Deps) handleSPA(w http.ResponseWriter, r *http.Request) {
	if d.SPADistDir == "" {
		http.NotFound(w, r)
		return
	}
	distAbs, err := filepath.Abs(d.SPADistDir)
	if err != nil {
		d.Logger.Error("spa dist abs failed", "err", err, "dir", d.SPADistDir)
		http.Error(w, "spa misconfigured", http.StatusInternalServerError)
		return
	}
	// Reject obvious traversal before joining; filepath.Clean("/foo/../bar")
	// collapses but a leading `..` segment is the easiest tell.
	rel := strings.TrimPrefix(filepath.Clean("/"+strings.TrimPrefix(r.URL.Path, "/")), "/")
	if strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	candidate := filepath.Join(distAbs, rel)
	if !strings.HasPrefix(candidate, distAbs) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		if strings.EqualFold(filepath.Ext(candidate), ".html") {
			w.Header().Set("Content-Security-Policy", spaBaselineCSP)
		}
		http.ServeFile(w, r, candidate)
		return
	}
	// Fallback — let React Router resolve the path. index.html itself
	// covers `/`, `/p/...`, `/wiki/...`, etc.
	w.Header().Set("Content-Security-Policy", spaBaselineCSP)
	http.ServeFile(w, r, filepath.Join(distAbs, "index.html"))
}

// handleSimpleHealth is the minimal liveness probe NSSM (and any external
// monitor) hits — process-up + DB reachable, no embedder spinup, no
// project lookup. Always returns 200 so a transient DB blip surfaces as
// `db: degraded` in the body rather than tripping monitor alerts;
// process-down is what those monitors should treat as failure.
func (d Deps) handleSimpleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if d.DB == nil {
		dbStatus = "degraded"
	} else if err := d.DB.Ping(r.Context()); err != nil {
		dbStatus = "degraded"
	}
	uptime := int64(0)
	if !d.StartTime.IsZero() {
		uptime = int64(time.Since(d.StartTime).Seconds())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"version":    d.Version,
		"uptime_sec": uptime,
		"db":         dbStatus,
	})
}
