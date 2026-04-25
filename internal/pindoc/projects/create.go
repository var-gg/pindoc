package projects

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SupportedLanguages is the V1 enum every entrypoint must enforce. The
// language is captured at create time and is immutable thereafter — no
// migration tooling exists for retroactive language switches, so picking
// the wrong one means recreating the project.
var SupportedLanguages = []string{"en", "ko", "ja"}

// projectSlugRe enforces the URL-safe shape every /p/{slug}/... route
// promises downstream. Leading letter keeps the slug from parsing as a
// number; the 2-40 character cap matches the Reader URL bar's comfortable
// width and the docs/03-architecture.md "kebab tied to repo or product
// name" guidance.
var projectSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

// reservedSlugs blocks slugs that would collide with routing, future
// sub-domains, or common admin paths on a self-host or hosted deployment.
// Kept conservative: anything an operator plausibly wants at /:slug on a
// typical web app gets reserved.
var reservedSlugs = map[string]struct{}{
	"admin": {}, "api": {}, "app": {}, "www": {}, "blog": {},
	"docs": {}, "help": {}, "mail": {}, "support": {}, "status": {},
	"billing": {}, "login": {}, "signup": {}, "logout": {},
	"dashboard": {}, "settings": {}, "public": {}, "static": {},
	"assets": {}, "auth": {}, "health": {}, "new": {},
	"about": {}, "terms": {}, "privacy": {}, "security": {},
	"pricing": {}, "contact": {}, "home": {}, "index": {},
	"p": {}, "wiki": {}, "tasks": {}, "graph": {}, "inbox": {},
	"design": {}, "ui": {}, "preview": {},
}

// Sentinel errors so callers can map to stable error codes
// (SLUG_INVALID / SLUG_RESERVED / SLUG_TAKEN / NAME_REQUIRED /
// LANG_REQUIRED / LANG_INVALID) without parsing error strings. Wrappers
// in mcp/tools, httpapi, and pindoc-admin all match these sentinels.
var (
	ErrSlugInvalid  = errors.New("SLUG_INVALID")
	ErrSlugReserved = errors.New("SLUG_RESERVED")
	ErrSlugTaken    = errors.New("SLUG_TAKEN")
	ErrNameRequired = errors.New("NAME_REQUIRED")
	ErrLangRequired = errors.New("LANG_REQUIRED")
	ErrLangInvalid  = errors.New("LANG_INVALID")
)

// CreateProjectInput is the entrypoint-agnostic projection of a "create
// project" request. MCP tool / REST handler / CLI all build one of these
// from their native input shape; UI hits the REST handler so it's
// transitively the same.
type CreateProjectInput struct {
	Slug            string
	Name            string
	Description     string // optional
	Color           string // optional CSS color
	PrimaryLanguage string // required, one of SupportedLanguages
	OwnerID         string // optional, defaults to "default"
}

// CreateProjectOutput carries the post-create facts every entrypoint
// needs. Entrypoint-specific framing (canonical URL, "reconnect required"
// guidance, agent-facing message) is built by the wrapper from these
// fields plus its own context.
type CreateProjectOutput struct {
	ID               string
	Slug             string
	Name             string
	PrimaryLanguage  string
	DefaultArea      string // always "misc" today; reserved for future override
	AreasCreated     int    // top-level + starter sub-area rows actually inserted
	TemplatesCreated int    // len(TemplateSeeds), 4 in V1
}

// ValidateProjectSlug runs the static checks (regex + reserved list) so
// callers can give live feedback on user input before paying for a tx.
// CreateProject re-runs the same check defensively, so a caller that
// skips this still gets the same outcome — just one round-trip later.
func ValidateProjectSlug(slug string) error {
	s := strings.ToLower(strings.TrimSpace(slug))
	if !projectSlugRe.MatchString(s) {
		return fmt.Errorf("%w: slug must be lowercase kebab-case (2-40 chars, starts with a letter): got %q", ErrSlugInvalid, slug)
	}
	if _, reserved := reservedSlugs[s]; reserved {
		return fmt.Errorf("%w: slug %q collides with common routes (/admin, /api, /docs, /wiki, ...). Pick something specific to this project", ErrSlugReserved, s)
	}
	return nil
}

// NormalizeLanguage validates and normalizes a primary_language. Returns
// the lower-cased normalized form, or an error wrapping ErrLangRequired
// (empty input) or ErrLangInvalid (unsupported value). Default is
// forbidden by design: the language is immutable after create, so the
// agent or operator must pick deliberately.
func NormalizeLanguage(raw string) (string, error) {
	lang := strings.ToLower(strings.TrimSpace(raw))
	if lang == "" {
		return "", fmt.Errorf("%w: primary_language is required; default is forbidden. Ask the user before calling project.create. Supported languages: %s. primary_language is immutable after create; if wrong, recreate the project", ErrLangRequired, supportedLanguageList())
	}
	if !isSupportedLanguage(lang) {
		return "", fmt.Errorf("%w: unsupported primary_language %q. Supported languages: %s. primary_language is immutable after create; if wrong, recreate the project", ErrLangInvalid, raw, supportedLanguageList())
	}
	return lang, nil
}

func isSupportedLanguage(lang string) bool {
	for _, supported := range SupportedLanguages {
		if lang == supported {
			return true
		}
	}
	return false
}

func supportedLanguageList() string {
	return strings.Join(SupportedLanguages, ", ")
}

// CreateProject inserts a projects row, seeds the 9-area concern skeleton
// + starter sub-areas, and seeds the 4 _template_* artifacts under the
// 'misc' area — atomic via the caller-provided transaction. The caller
// is responsible for tx.Begin / tx.Commit / tx.Rollback. Common entry
// points pass tx straight through (MCP tool already in-tx; REST/CLI begin
// their own).
//
// Returned errors wrap one of the package's sentinel errors when the
// failure is user-visible (slug invalid / reserved / taken, name empty,
// language missing or unsupported); other errors are bubbled-up DB or
// internal failures and should surface as 500s.
func CreateProject(
	ctx context.Context,
	tx pgx.Tx,
	in CreateProjectInput,
) (CreateProjectOutput, error) {
	var zero CreateProjectOutput

	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	name := strings.TrimSpace(in.Name)
	desc := strings.TrimSpace(in.Description)
	color := strings.TrimSpace(in.Color)
	ownerID := strings.TrimSpace(in.OwnerID)
	if ownerID == "" {
		ownerID = "default"
	}

	if err := ValidateProjectSlug(slug); err != nil {
		return zero, err
	}
	if name == "" {
		return zero, fmt.Errorf("%w: name is required", ErrNameRequired)
	}
	lang, err := NormalizeLanguage(in.PrimaryLanguage)
	if err != nil {
		return zero, err
	}

	var descPtr, colorPtr *string
	if desc != "" {
		descPtr = &desc
	}
	if color != "" {
		colorPtr = &color
	}

	var projectID string
	err = tx.QueryRow(ctx, `
		INSERT INTO projects (owner_id, slug, name, description, color, primary_language, locale)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING id::text
	`, ownerID, slug, name, descPtr, colorPtr, lang).Scan(&projectID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return zero, fmt.Errorf("%w: project slug %q already exists under owner %q — pick a different slug", ErrSlugTaken, slug, ownerID)
		}
		return zero, fmt.Errorf("project insert: %w", err)
	}

	areasCreated, err := seedAreas(ctx, tx, projectID, lang)
	if err != nil {
		return zero, fmt.Errorf("seed default areas: %w", err)
	}

	templatesCreated, err := seedTemplates(ctx, tx, projectID)
	if err != nil {
		return zero, fmt.Errorf("seed templates: %w", err)
	}

	return CreateProjectOutput{
		ID:               projectID,
		Slug:             slug,
		Name:             name,
		PrimaryLanguage:  lang,
		DefaultArea:      "misc",
		AreasCreated:     areasCreated,
		TemplatesCreated: templatesCreated,
	}, nil
}

// seedAreas inserts the fixed 9-row top-level skeleton and the depth-1
// starter sub-area rows. ON CONFLICT DO NOTHING guards re-runs in the
// rare case a caller seeds twice (not expected — kept defensive). The
// returned count reflects the rows the caller asked us to attempt
// (deterministic per V1) so wrappers can surface "9 areas + N sub-areas"
// without an extra SELECT.
func seedAreas(ctx context.Context, tx pgx.Tx, projectID, lang string) (int, error) {
	count := 0
	for _, seed := range TopLevelAreaSeed {
		if _, err := tx.Exec(ctx, `
			INSERT INTO areas (project_id, slug, name, description, is_cross_cutting)
			VALUES ($1::uuid, $2, $3, $4, $5)
			ON CONFLICT (project_id, slug) DO NOTHING
		`, projectID, seed.Slug, seed.Name, LocalizedAreaDescription(seed.DescriptionEN, seed.DescriptionKO, lang), seed.IsCrossCutting); err != nil {
			return count, fmt.Errorf("seed area %s: %w", seed.Slug, err)
		}
		count++
	}
	for _, seed := range StarterSubAreaSeeds {
		if _, err := tx.Exec(ctx, `
			INSERT INTO areas (project_id, parent_id, slug, name, description, is_cross_cutting)
			SELECT $1::uuid, parent.id, $3, $4, $5, $6
			FROM areas parent
			WHERE parent.project_id = $1::uuid AND parent.slug = $2
			ON CONFLICT (project_id, slug) DO NOTHING
		`, projectID, seed.ParentSlug, seed.Slug, seed.Name, seed.Description, seed.IsCrossCutting); err != nil {
			return count, fmt.Errorf("seed area %s/%s: %w", seed.ParentSlug, seed.Slug, err)
		}
		count++
	}
	return count, nil
}

// seedTemplates inserts the 4 _template_* artifacts under the project's
// 'misc' area, plus revision 1 for each (artifact_revisions is the source
// of truth for diff/history). Mirrors migration 0006_template_artifacts
// for pre-existing projects so behavior stays identical whether a project
// was created via raw migration seed or via this function.
func seedTemplates(ctx context.Context, tx pgx.Tx, projectID string) (int, error) {
	var miscID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM areas WHERE project_id = $1::uuid AND slug = 'misc'
	`, projectID).Scan(&miscID); err != nil {
		return 0, fmt.Errorf("resolve misc area: %w", err)
	}
	for _, t := range TemplateSeeds {
		var templateID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO artifacts (
				project_id, area_id, slug, type, title, body_markdown, tags,
				completeness, status, review_state,
				author_kind, author_id, author_version, published_at
			) VALUES (
				$1::uuid, $2::uuid, $3, $4, $5, $6, ARRAY['_template'],
				'partial', 'published', 'auto_published',
				'system', 'pindoc-seed', '0.0.1', now()
			)
			RETURNING id::text
		`, projectID, miscID, t.Slug, t.Type, t.Title, t.Body).Scan(&templateID); err != nil {
			return 0, fmt.Errorf("seed template %s: %w", t.Slug, err)
		}
		// Phase A revision-shapes refactor: every artifact must have
		// revision 1 (head() = 0 is no longer legal — see migration
		// 0017). Templates seeded via raw INSERT historically skipped
		// this; doing it in the same tx keeps create atomic.
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_revisions (
				artifact_id, revision_number, title, body_markdown, body_hash, tags,
				completeness, author_kind, author_id, author_version, commit_msg,
				revision_shape
			) VALUES ($1::uuid, 1, $2, $3, encode(sha256(convert_to($3, 'UTF8')), 'hex'),
			          ARRAY['_template'], 'partial', 'system', 'pindoc-seed', '0.0.1',
			          'seed: template artifact', 'body_patch')
		`, templateID, t.Title, t.Body); err != nil {
			return 0, fmt.Errorf("seed template revision %s: %w", t.Slug, err)
		}
	}
	return len(TemplateSeeds), nil
}
