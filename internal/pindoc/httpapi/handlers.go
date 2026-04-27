package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type projectInfo struct {
	ID              string `json:"id"`
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	OwnerID         string `json:"owner_id"`
	Description     string `json:"description,omitempty"`
	Color           string `json:"color,omitempty"`
	PrimaryLanguage string `json:"primary_language"`
	SensitiveOps    string `json:"sensitive_ops"`
	CurrentRole     string `json:"current_role,omitempty"`
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
		"mermaid", // fenced ```mermaid blocks render as SVG
	},
	CodeLanguages: []string{"any"}, // plain monospace rendering for all
	Notes:         "Headings H1–H6, ordered/unordered lists, blockquotes, inline code, fenced code, links. Mermaid via ```mermaid fence. Math/KaTeX not supported (M1.x).",
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
	writeJSON(w, http.StatusOK, map[string]any{
		"default_project_slug":   d.DefaultProjectSlug,
		"default_project_locale": d.DefaultProjectLocale,
		"multi_project":          d.deriveMultiProject(r.Context()),
		"public_base_url":        publicBase,
		"version":                d.Version,
		// providers + bind_addr replace the deprecated auth_mode enum
		// (Decision `decision-auth-model-loopback-and-providers`).
		// Reader keys "is the operator the calling principal" off the
		// loopback judgement of the current request, not off this
		// instance-wide config.
		"providers": providers,
		"bind_addr": bindAddr,
		// onboarding_required tells the React app to redirect / →
		// wizard. True when only the seed `pindoc` project exists
		// (Decision project-bootstrap-canonical-flow-reader-ui-first-
		// class). The react app reads this on mount and, if true,
		// sends the user to /projects/new?welcome=1 instead of the
		// legacy redirect. Self-correcting — if the user later
		// deletes their projects, the wizard returns.
		"onboarding_required": d.checkOnboardingRequired(r.Context()),
	})
}

// deriveMultiProject is the HTTP-side mirror of mcp/tools.deriveMultiProject.
// Called once per /api/config, /api/projects, and /api/health response so
// the wire `multi_project` field tracks the real project row count
// without the operator flipping an env flag. Errors and a missing DB
// pool fall back to false — Reader chrome stays single-project rather
// than spuriously showing a switcher when the lookup hiccups.
func (d Deps) deriveMultiProject(ctx context.Context) bool {
	if d.DB == nil {
		return false
	}
	n, err := projects.CountVisible(ctx, d.DB, "")
	if err != nil {
		if d.Logger != nil {
			d.Logger.Warn("multi_project derivation failed; defaulting to false",
				"err", err,
			)
		}
		return false
	}
	return projects.IsMultiProject(n)
}

// checkOnboardingRequired returns true when the instance has no projects
// other than the seed `pindoc` row. Used by /api/config so the SPA can
// decide whether to redirect a fresh user into the wizard. Errors are
// logged and treated as "not required" — better to show the legacy
// landing than to dead-end on an unrelated DB hiccup. Cheap query
// (single COUNT) but skipped when DB pool is missing for tests that
// stub Deps.
func (d Deps) checkOnboardingRequired(ctx context.Context) bool {
	if d.DB == nil {
		return false
	}
	var count int
	if err := d.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE slug != 'pindoc'`,
	).Scan(&count); err != nil {
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
func (d Deps) principalForRequest(r *http.Request) *pauth.Principal {
	return pauth.PrincipalFromRequest(r, pauth.HTTPDeps{
		OAuth:          d.OAuth,
		DefaultUserID:  d.DefaultUserID,
		DefaultAgentID: d.DefaultAgentID,
	})
}

type projectListRow struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	OwnerID         string    `json:"owner_id"`
	Description     string    `json:"description,omitempty"`
	Color           string    `json:"color,omitempty"`
	PrimaryLanguage string    `json:"primary_language"`
	ArtifactsCount  int       `json:"artifacts_count"`
	CreatedAt       time.Time `json:"created_at"`
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

func (d Deps) handleProjectList(w http.ResponseWriter, r *http.Request) {
	rows, err := d.DB.Query(r.Context(), `
		SELECT
			p.id::text, p.slug, p.name, p.owner_id, p.description, p.color,
			p.primary_language, p.created_at,
			(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
		FROM projects p
		ORDER BY p.created_at
	`)
	if err != nil {
		d.Logger.Error("project list", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []projectListRow{}
	for rows.Next() {
		var p projectListRow
		var desc, color *string
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Name, &p.OwnerID, &desc, &color,
			&p.PrimaryLanguage, &p.CreatedAt, &p.ArtifactsCount,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if desc != nil {
			p.Description = *desc
		}
		if color != nil {
			p.Color = *color
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"projects":             out,
		"default_project_slug": d.DefaultProjectSlug,
		"multi_project":        d.deriveMultiProject(r.Context()),
	})
}

func (d Deps) handleProjectCurrent(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	var out projectInfo
	var desc, color *string
	err := d.DB.QueryRow(r.Context(), `
		SELECT
			p.id::text, p.slug, p.name, p.owner_id, p.description, p.color,
			p.primary_language, p.primary_language, COALESCE(NULLIF(p.sensitive_ops, ''), 'auto'), p.created_at,
			(SELECT count(*) FROM areas     WHERE project_id = p.id),
			(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
		FROM projects p WHERE p.slug = $1
	`, slug).Scan(
		&out.ID, &out.Slug, &out.Name, &out.OwnerID, &desc, &color,
		&out.PrimaryLanguage, &out.Locale, &out.SensitiveOps, &out.CreatedAt,
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
	// include_templates=true counts _template_* artifacts in artifact_count.
	// Must stay in lockstep with handleArtifactList's filter so Sidebar
	// counts == list cardinality. Fixing the Phase 13 regression where
	// counts included templates but the list excluded them.
	includeTemplates := r.URL.Query().Get("include_templates") == "true"
	rows, err := d.DB.Query(r.Context(), `
		WITH p AS (SELECT id FROM projects WHERE slug = $1)
		SELECT
			a.id::text, a.slug, a.name, a.description,
			parent.slug, a.is_cross_cutting,
			(SELECT count(*) FROM artifacts x
			  WHERE x.area_id = a.id
			    AND x.status <> 'archived'
			    AND ($2::bool OR NOT starts_with(x.slug, '_template_'))),
			COALESCE(ARRAY(
			  SELECT c.slug FROM areas c
			  WHERE c.parent_id = a.id
			  ORDER BY c.slug
			), ARRAY[]::text[])
		FROM areas a
		JOIN p ON a.project_id = p.id
		LEFT JOIN areas parent ON parent.id = a.parent_id
		ORDER BY a.is_cross_cutting, a.slug
	`, slug, includeTemplates)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"project_slug": slug,
		"areas":        out,
	})
}

type artifactRow struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	Type         string    `json:"type"`
	Title        string    `json:"title"`
	AreaSlug     string    `json:"area_slug"`
	BodyLocale   string    `json:"body_locale,omitempty"`
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
	areaSlug := r.URL.Query().Get("area")
	typeFilter := r.URL.Query().Get("type")
	// include_templates=true surfaces _template_* artifacts (Phase 13).
	// Default hides them so the reader list stays focused on real docs.
	includeTemplates := r.URL.Query().Get("include_templates") == "true"

	rows, err := d.DB.Query(r.Context(), `
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(NULLIF(a.body_locale, ''), NULLIF(p.primary_language, ''), 'en'),
			a.completeness, a.status, a.review_state,
			a.author_id, a.published_at, a.updated_at, a.task_meta, a.artifact_meta,
			wr.recent_warnings,
			u.id::text, u.display_name, u.github_handle
		FROM artifacts a
		JOIN projects p  ON p.id  = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		LEFT JOIN users u ON u.id = a.author_user_id
		LEFT JOIN LATERAL (
			SELECT COALESCE(jsonb_agg(jsonb_build_object(
				'codes', COALESCE(e.payload->'codes', '[]'::jsonb),
				'revision_number', COALESCE(NULLIF(e.payload->>'revision_number', '')::int, 0),
				'author_id', NULLIF(e.payload->>'author_id', ''),
				'canonical_rewrite_without_evidence',
					COALESCE((e.payload->>'canonical_rewrite_without_evidence')::boolean, false),
				'created_at', e.created_at
			) ORDER BY e.created_at DESC), '[]'::jsonb) AS recent_warnings
			FROM (
				SELECT payload, created_at
				  FROM events
				 WHERE subject_id = a.id AND kind = 'artifact.warning_raised'
				 ORDER BY created_at DESC
				 LIMIT 5
			) e
		) wr ON true
		WHERE p.slug = $1
		  AND a.status <> 'archived'
		  AND ($2 = '' OR ar.slug = $2)
		  AND ($3 = '' OR a.type  = $3)
		  AND ($4::bool OR NOT starts_with(a.slug, '_template_'))
		ORDER BY a.updated_at DESC
		LIMIT 200
	`, slug, areaSlug, typeFilter, includeTemplates)
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
			&a.BodyLocale,
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
	writeJSON(w, http.StatusOK, map[string]any{
		"project_slug": slug,
		"artifacts":    out,
	})
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
	Pins             []pinRow        `json:"pins,omitempty"`
	SourceSessionRef json.RawMessage `json:"source_session_ref,omitempty"`

	// RecentWarnings projects events.artifact.warning_raised rows for
	// the Reader Trust Card (Task propose-경로-warning-영속화). The
	// server returns up to 5 most-recent warning events so the Trust
	// Card can highlight the latest-revision advisories and older ones
	// stay accessible via the revision history. Empty when the artifact
	// has never raised a warning.
	RecentWarnings []recentWarningRow `json:"recent_warnings,omitempty"`
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
	Repo       string `json:"repo,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Path       string `json:"path"`
	LinesStart int    `json:"lines_start,omitempty"`
	LinesEnd   int    `json:"lines_end,omitempty"`
}

func (d Deps) handleArtifactGet(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
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
	err := d.DB.QueryRow(r.Context(), `
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(NULLIF(a.body_locale, ''), NULLIF(p.primary_language, ''), 'en'),
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
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, slug, ref).Scan(
		&a.ID, &a.Slug, &a.Type, &a.Title, &a.AreaSlug,
		&a.BodyLocale,
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
	if warnings, err := d.loadRecentWarnings(r.Context(), a.ID); err != nil {
		d.Logger.Warn("recent warnings lookup failed", "artifact_id", a.ID, "err", err)
	} else {
		a.RecentWarnings = warnings
	}

	writeJSON(w, http.StatusOK, a)
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
		 ORDER BY created_at DESC
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
		var parsed struct {
			Codes                           []string `json:"codes"`
			RevisionNumber                  int      `json:"revision_number"`
			AuthorID                        string   `json:"author_id"`
			CanonicalRewriteWithoutEvidence bool     `json:"canonical_rewrite_without_evidence"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			// Skip malformed rows rather than 500 — an older payload
			// shape shouldn't break the whole artifact detail call.
			d.Logger.Warn("warning event payload unmarshal failed",
				"artifact_id", artifactID, "err", err)
			continue
		}
		out = append(out, recentWarningRow{
			Codes:                           parsed.Codes,
			RevisionNumber:                  parsed.RevisionNumber,
			AuthorID:                        parsed.AuthorID,
			CanonicalRewriteWithoutEvidence: parsed.CanonicalRewriteWithoutEvidence,
			CreatedAt:                       createdAt,
		})
	}
	return out, nil
}

// loadArtifactPins returns artifact_pins rows sorted by insertion order.
// The Reader Sidecar renders them grouped by kind so provenance inspection
// lands on the most likely evidence type without extra clicks.
func (d Deps) loadArtifactPins(ctx context.Context, artifactID string) ([]pinRow, error) {
	rows, err := d.DB.Query(ctx, `
		SELECT kind, repo, commit_sha, path, lines_start, lines_end
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
		var commitSHA *string
		var ls, le *int
		if err := rows.Scan(&p.Kind, &p.Repo, &commitSHA, &p.Path, &ls, &le); err != nil {
			return nil, err
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
	if d.Embedder == nil {
		writeJSON(w, http.StatusOK, map[string]any{"query": q, "hits": []any{}, "notice": "embedder not configured"})
		return
	}
	res, err := d.Embedder.Embed(r.Context(), embed.Request{Texts: []string{q}, Kind: embed.KindQuery})
	if err != nil {
		d.Logger.Error("embed query", "err", err)
		writeError(w, http.StatusInternalServerError, "embed failed")
		return
	}
	qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))

	rows, err := d.DB.Query(r.Context(), `
		WITH scored AS (
			SELECT DISTINCT ON (c.artifact_id)
				c.artifact_id, c.heading, c.text,
				c.embedding <=> $1::vector AS distance
			FROM artifact_chunks c
			JOIN artifacts a ON a.id = c.artifact_id
			JOIN projects p ON p.id = a.project_id
			WHERE p.slug = $2
			  AND a.status <> 'archived'
			  AND ($3::bool OR NOT starts_with(a.slug, '_template_'))
			ORDER BY c.artifact_id, distance
		)
		SELECT
			s.artifact_id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(s.heading, '') , s.text, s.distance
		FROM scored s
		JOIN artifacts a  ON a.id  = s.artifact_id
		JOIN areas     ar ON ar.id = a.area_id
		ORDER BY s.distance
		LIMIT 10
	`, qVec, slug, includeTemplates)
	if err != nil {
		d.Logger.Error("search", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	type hit struct {
		ArtifactID string  `json:"artifact_id"`
		Slug       string  `json:"slug"`
		Type       string  `json:"type"`
		Title      string  `json:"title"`
		AreaSlug   string  `json:"area_slug"`
		Heading    string  `json:"heading,omitempty"`
		Snippet    string  `json:"snippet"`
		Distance   float64 `json:"distance"`
	}
	out := []hit{}
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.ArtifactID, &h.Slug, &h.Type, &h.Title, &h.AreaSlug, &h.Heading, &h.Snippet, &h.Distance); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if len(h.Snippet) > 280 {
			h.Snippet = h.Snippet[:280] + "..."
		}
		out = append(out, h)
	}
	info := d.Embedder.Info()
	writeJSON(w, http.StatusOK, map[string]any{
		"query":        q,
		"project_slug": slug,
		"hits":         out,
		"embedder_used": map[string]any{
			"name":      info.Name,
			"model_id":  info.ModelID,
			"dimension": info.Dimension,
		},
	})
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
		"multi_project":        d.deriveMultiProject(r.Context()),
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
		http.ServeFile(w, r, candidate)
		return
	}
	// Fallback — let React Router resolve the path. index.html itself
	// covers `/`, `/p/...`, `/wiki/...`, etc.
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
