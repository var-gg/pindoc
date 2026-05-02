package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/artifacts"
	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type artifactTranslateInput struct {
	ProjectSlug  string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	ArtifactSlug string `json:"artifact_slug" jsonschema:"artifact slug, UUID, pindoc:// ref, or Reader share URL to translate"`
	TargetLocale string `json:"target_locale" jsonschema:"target BCP 47 language tag, e.g. en | ko | ja | hi"`
	// UseCache defaults to true. Pointer keeps omitted distinct from
	// explicit false.
	UseCache *bool `json:"use_cache,omitempty" jsonschema:"look up cached translation_of artifact first; default true"`
}

type artifactTranslateOutput struct {
	Status         string `json:"status"`
	SourceMarkdown string `json:"source_markdown"`
	SourceLocale   string `json:"source_locale"`
	TargetLocale   string `json:"target_locale"`
	ArtifactID     string `json:"artifact_id"`
	ArtifactSlug   string `json:"artifact_slug"`
	ArtifactTitle  string `json:"artifact_title"`
	ArtifactType   string `json:"artifact_type"`
	AreaSlug       string `json:"area_slug"`
	AgentRef       string `json:"agent_ref"`
	HumanURL       string `json:"human_url"`
	HumanURLAbs    string `json:"human_url_abs,omitempty"`

	CachedMarkdown   string `json:"cached_translation_markdown,omitempty"`
	CachedArtifactID string `json:"cached_translation_artifact_id,omitempty"`
	CachedSlug       string `json:"cached_translation_slug,omitempty"`
	CachedAt         string `json:"cached_at,omitempty"`
	CachedStale      bool   `json:"cached_stale,omitempty"`

	SaveHint       string `json:"save_hint,omitempty"`
	ToolsetVersion string `json:"toolset_version,omitempty"`
}

type translateSourceArtifact struct {
	ID           string
	Slug         string
	Title        string
	Type         string
	AreaSlug     string
	BodyMarkdown string
	BodyLocale   string
	UpdatedAt    time.Time
}

// RegisterArtifactTranslate wires pindoc.artifact.translate. The server does
// not run an LLM; it returns source markdown, locale metadata, and an optional
// cached translation artifact so the calling agent can translate or reuse.
func RegisterArtifactTranslate(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.translate",
			Description: "Return source markdown and translation cache metadata for an artifact. The server does not translate; the calling agent translates source_markdown to target_locale and may cache via a translation_of artifact.",
		},
		func(ctx context.Context, p *auth.Principal, in artifactTranslateInput) (*sdk.CallToolResult, artifactTranslateOutput, error) {
			readScope, err := resolveMCPReadProjectScope(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactTranslateOutput{}, fmt.Errorf("artifact.translate: %w", err)
			}
			scope := readScope.ProjectScope
			targetLocale := normalizeBodyLocale(in.TargetLocale)
			if targetLocale == "" {
				return nil, artifactTranslateOutput{}, errors.New("target_locale is required")
			}
			ref := normalizeArtifactReadRef(in.ArtifactSlug, scope.ProjectSlug)
			if ref.Value == "" {
				return nil, artifactTranslateOutput{}, errors.New("artifact_slug is required")
			}
			if ref.ScopeMismatch {
				return nil, artifactTranslateOutput{}, artifactReadNotFoundError(in.ArtifactSlug, scope, ref)
			}

			source, err := loadTranslateSource(ctx, deps, readScope, ref.Value)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, artifactTranslateOutput{}, artifactReadNotFoundError(in.ArtifactSlug, scope, ref)
				}
				return nil, artifactTranslateOutput{}, fmt.Errorf("artifact.translate source lookup: %w", err)
			}

			useCache := true
			if in.UseCache != nil {
				useCache = *in.UseCache
			}

			out := artifactTranslateOutput{
				Status:         "ok",
				SourceMarkdown: source.BodyMarkdown,
				SourceLocale:   source.BodyLocale,
				TargetLocale:   targetLocale,
				ArtifactID:     source.ID,
				ArtifactSlug:   source.Slug,
				ArtifactTitle:  source.Title,
				ArtifactType:   source.Type,
				AreaSlug:       source.AreaSlug,
				AgentRef:       "pindoc://" + source.Slug,
				HumanURL:       HumanURL(scope.ProjectSlug, scope.ProjectLocale, source.Slug),
				HumanURLAbs:    AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, source.Slug),
			}
			out.SaveHint = fmt.Sprintf(
				"Translate source_markdown from %s to %s. To cache the result, call pindoc.artifact.propose with body_locale=%q and relates_to=[{target_id:%q, relation:\"translation_of\"}].",
				source.BodyLocale, targetLocale, targetLocale, source.Slug,
			)

			if useCache {
				cached, err := findVisibleCachedTranslation(ctx, deps, readScope, source.ID, targetLocale)
				if err != nil && !artifacts.IsNoCachedTranslation(err) {
					return nil, artifactTranslateOutput{}, fmt.Errorf("translation cache lookup: %w", err)
				}
				if cached != nil {
					out.CachedMarkdown = cached.BodyMarkdown
					out.CachedArtifactID = cached.ID
					out.CachedSlug = cached.Slug
					out.CachedAt = cached.UpdatedAt.Format(time.RFC3339)
					out.CachedStale = cached.UpdatedAt.Before(source.UpdatedAt)
				}
			}

			return nil, out, nil
		},
	)
}

func loadTranslateSource(ctx context.Context, deps Deps, readScope *mcpReadProjectScope, idOrSlug string) (translateSourceArtifact, error) {
	var out translateSourceArtifact
	args := []any{readScope.ProjectSlug, idOrSlug}
	visibilityWhere, visibilityArgs := mcpReadArtifactVisibilityWhere(readScope, "a", len(args)+1)
	args = append(args, visibilityArgs...)
	err := deps.DB.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			a.id::text,
			a.slug,
			a.title,
			a.type,
			area.slug,
			a.body_markdown,
			COALESCE(NULLIF(a.body_locale, ''), NULLIF(proj.primary_language, ''), 'en'),
			a.updated_at
		FROM artifacts a
		JOIN projects proj ON proj.id = a.project_id
		JOIN areas area ON area.id = a.area_id
		WHERE proj.slug = $1
		  AND a.status <> 'archived'
		  AND (a.id::text = $2 OR a.slug = $2)
		  AND %s
		LIMIT 1
	`, visibilityWhere), args...).Scan(
		&out.ID,
		&out.Slug,
		&out.Title,
		&out.Type,
		&out.AreaSlug,
		&out.BodyMarkdown,
		&out.BodyLocale,
		&out.UpdatedAt,
	)
	out.BodyLocale = normalizeBodyLocale(out.BodyLocale)
	return out, err
}

func findVisibleCachedTranslation(ctx context.Context, deps Deps, readScope *mcpReadProjectScope, sourceArtifactID, targetLocale string) (*artifacts.CachedTranslation, error) {
	args := []any{sourceArtifactID, targetLocale}
	visibilityWhere, visibilityArgs := mcpReadArtifactVisibilityWhere(readScope, "t", len(args)+1)
	args = append(args, visibilityArgs...)
	rows, err := deps.DB.Query(ctx, fmt.Sprintf(`
		WITH candidates AS (
			SELECT t.id::text, t.slug, t.title, t.body_markdown, t.body_locale, t.updated_at
			  FROM artifact_edges e
			  JOIN artifacts t ON t.id = e.source_id
			 WHERE e.relation = 'translation_of'
			   AND e.target_id = $1::uuid
			   AND lower(t.body_locale) = lower($2)
			   AND t.status <> 'archived'
			   AND %s
			UNION
			SELECT t.id::text, t.slug, t.title, t.body_markdown, t.body_locale, t.updated_at
			  FROM artifact_edges e
			  JOIN artifacts t ON t.id = e.target_id
			 WHERE e.relation = 'translation_of'
			   AND e.source_id = $1::uuid
			   AND lower(t.body_locale) = lower($2)
			   AND t.status <> 'archived'
			   AND %s
		)
		SELECT id, slug, title, body_markdown, body_locale, updated_at
		  FROM candidates
		 ORDER BY updated_at DESC
		 LIMIT 1
	`, visibilityWhere, visibilityWhere), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out artifacts.CachedTranslation
	if rows.Next() {
		if err := rows.Scan(&out.ID, &out.Slug, &out.Title, &out.BodyMarkdown, &out.BodyLocale, &out.UpdatedAt); err != nil {
			return nil, err
		}
		return &out, rows.Err()
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, pgx.ErrNoRows
}

func normalizeBodyLocale(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func validBodyLocale(raw string) bool {
	switch normalizeBodyLocale(raw) {
	case "ko", "en", "ja", "ko-kr", "en-us", "en-gb", "ja-jp":
		return true
	default:
		return false
	}
}
