package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/i18n"
)

// ValidArtifactTypes are the types Phase 2 accepts. Tier A (7) + Tier B
// Web-SaaS pack (4). When a Tier B pack is activated for a project in
// Phase 4+ this set becomes project-scoped, but for M1 a single flat
// whitelist is enough.
var validArtifactTypes = map[string]struct{}{
	// Tier A core
	"Decision": {}, "Analysis": {}, "Debug": {}, "Flow": {},
	"Task": {}, "TC": {}, "Glossary": {},
	// Tier B Web SaaS
	"Feature": {}, "APIEndpoint": {}, "Screen": {}, "DataModel": {},
}

type artifactProposeInput struct {
	Type         string   `json:"type" jsonschema:"one of Decision|Analysis|Debug|Flow|Task|TC|Glossary|Feature|APIEndpoint|Screen|DataModel"`
	AreaSlug     string   `json:"area_slug" jsonschema:"slug from pindoc.area.list; use 'misc' if unsure"`
	Title        string   `json:"title"`
	BodyMarkdown string   `json:"body_markdown" jsonschema:"main content in markdown"`
	Slug         string   `json:"slug,omitempty" jsonschema:"optional; auto-generated from title if absent"`
	Tags         []string `json:"tags,omitempty"`
	Completeness string   `json:"completeness,omitempty" jsonschema:"draft|partial|settled; default partial"`
	AuthorID     string   `json:"author_id" jsonschema:"'claude-code', 'cursor', 'codex', etc."`
	AuthorVersion string  `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`
}

type artifactProposeOutput struct {
	Status           string   `json:"status"` // "accepted" | "not_ready"
	ErrorCode        string   `json:"error_code,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	// Only set on Status == "accepted".
	ArtifactID   string    `json:"artifact_id,omitempty"`
	Slug         string    `json:"slug,omitempty"`
	URL          string    `json:"url,omitempty"`
	PublishedAt  time.Time `json:"published_at,omitzero"`
}

// RegisterArtifactPropose wires pindoc.artifact.propose — the only write
// tool in Phase 2. Implements the Pre-flight Check mechanism (M0.5 in
// docs/05): on failing checks the tool returns Status=not_ready with a
// checklist telling the agent what to fix, not a hard error. The agent
// re-submits after addressing the checklist.
//
// Accepted propose calls auto-publish (review_state=auto_published,
// status=published). Review Queue (sensitive ops) lands in Phase 2.x+.
func RegisterArtifactPropose(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.artifact.propose",
			Description: "Propose a new artifact (the only write path humans use — always via an agent). Returns Status=accepted + artifact_id on success, or Status=not_ready + checklist + suggested_actions if Pre-flight fails. Always read the checklist; never surface the raw error to the user without trying the suggested actions first.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactProposeInput) (*sdk.CallToolResult, artifactProposeOutput, error) {
			// --- Pre-flight ----------------------------------------------
			lang := deps.UserLanguage
			checklist, code := preflight(&in, lang)
			if len(checklist) > 0 {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: code,
					Checklist: checklist,
					SuggestedActions: []string{
						i18n.T(lang, "suggested.fix_all"),
						i18n.T(lang, "suggested.confirm_types"),
						i18n.T(lang, "suggested.use_misc"),
					},
				}, nil
			}

			// --- Resolve area + project ----------------------------------
			var projectID, areaID string
			err := deps.DB.QueryRow(ctx, `
				SELECT proj.id::text, area.id::text
				FROM projects proj
				JOIN areas area ON area.project_id = proj.id
				WHERE proj.slug = $1 AND area.slug = $2
			`, deps.ProjectSlug, in.AreaSlug).Scan(&projectID, &areaID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: "AREA_NOT_FOUND",
					Checklist: []string{
						fmt.Sprintf(i18n.T(lang, "preflight.area_not_found"), in.AreaSlug, deps.ProjectSlug),
					},
					SuggestedActions: []string{
						i18n.T(lang, "suggested.list_areas"),
						i18n.T(lang, "suggested.area_or_misc"),
					},
				}, nil
			}
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("resolve scope: %w", err)
			}

			// --- Exact-title conflict check (embedding-based lands Phase 3) ---
			var existingID, existingSlug string
			err = deps.DB.QueryRow(ctx, `
				SELECT id::text, slug FROM artifacts
				WHERE project_id = $1
				  AND lower(title) = lower($2)
				  AND status <> 'archived'
				LIMIT 1
			`, projectID, in.Title).Scan(&existingID, &existingSlug)
			if err == nil {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: "CONFLICT_EXACT_TITLE",
					Checklist: []string{
						fmt.Sprintf(i18n.T(lang, "preflight.conflict_exact"), existingID, existingSlug),
					},
					SuggestedActions: []string{
						fmt.Sprintf(i18n.T(lang, "suggested.read_existing"), existingSlug),
						i18n.T(lang, "suggested.supersede"),
						i18n.T(lang, "suggested.pick_title"),
					},
				}, nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactProposeOutput{}, fmt.Errorf("conflict check: %w", err)
			}

			// --- Slug: either the explicit one or a generated one. Retry on
			//     unique-constraint violation with a -N suffix until we settle.
			baseSlug := in.Slug
			if baseSlug == "" {
				baseSlug = slugify(in.Title)
			}
			if baseSlug == "" {
				baseSlug = strings.ToLower(in.Type) + "-" + time.Now().UTC().Format("20060102150405")
			}

			completeness := in.Completeness
			if completeness == "" {
				completeness = "partial"
			}
			if in.Tags == nil {
				in.Tags = []string{}
			}

			// --- INSERT + event in one tx --------------------------------
			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			finalSlug := baseSlug
			var newID string
			var publishedAt time.Time
			for attempt := 0; attempt < 10; attempt++ {
				err = tx.QueryRow(ctx, `
					INSERT INTO artifacts (
						project_id, area_id, slug, type, title, body_markdown, tags,
						completeness, status, review_state,
						author_kind, author_id, author_version,
						published_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'published', 'auto_published', 'agent', $9, $10, now())
					RETURNING id::text, published_at
				`, projectID, areaID, finalSlug, in.Type, in.Title, in.BodyMarkdown, in.Tags,
					completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion)).Scan(&newID, &publishedAt)
				if err == nil {
					break
				}
				if isUniqueViolation(err, "artifacts_project_id_slug_key") {
					finalSlug = fmt.Sprintf("%s-%d", baseSlug, attempt+2)
					continue
				}
				return nil, artifactProposeOutput{}, fmt.Errorf("insert: %w", err)
			}
			if newID == "" {
				return nil, artifactProposeOutput{}, errors.New("could not allocate a unique slug after 10 attempts")
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1, 'artifact.published', $2, jsonb_build_object(
					'area_slug', $3::text,
					'type', $4::text,
					'slug', $5::text,
					'author_id', $6::text
				))
			`, projectID, newID, in.AreaSlug, in.Type, finalSlug, in.AuthorID); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("event insert: %w", err)
			}

			// Embed title + body chunks in the same transaction so search
			// never observes a half-indexed artifact. If the embedder fails
			// we still keep the artifact — search becomes keyword-only for
			// that row until re-embedding lands in Phase 3.x.
			if deps.Embedder != nil {
				if err := embedAndStoreChunks(ctx, tx, deps.Embedder, newID, in.Title, in.BodyMarkdown); err != nil {
					deps.Logger.Warn("chunk/embed failed — artifact saved without vectors",
						"artifact_id", newID, "err", err)
				}
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
			}

			return nil, artifactProposeOutput{
				Status:      "accepted",
				ArtifactID:  newID,
				Slug:        finalSlug,
				URL:         fmt.Sprintf("pindoc://%s", finalSlug),
				PublishedAt: publishedAt,
			}, nil
		},
	)
}

// preflight runs the cheap synchronous checks. Returns a list of ✗-prefixed
// lines the agent should address, plus a short error code. Empty list +
// empty code means clean. Strings pulled from i18n bundle.
func preflight(in *artifactProposeInput, lang string) ([]string, string) {
	var checklist []string
	code := ""

	if _, ok := validArtifactTypes[in.Type]; !ok {
		checklist = append(checklist,
			fmt.Sprintf(i18n.T(lang, "preflight.type_invalid"), in.Type))
		code = "INVALID_TYPE"
	}
	if strings.TrimSpace(in.Title) == "" {
		checklist = append(checklist, i18n.T(lang, "preflight.title_empty"))
		if code == "" {
			code = "MISSING_FIELD"
		}
	}
	if strings.TrimSpace(in.BodyMarkdown) == "" {
		checklist = append(checklist, i18n.T(lang, "preflight.body_empty"))
		if code == "" {
			code = "MISSING_FIELD"
		}
	}
	if strings.TrimSpace(in.AreaSlug) == "" {
		checklist = append(checklist, i18n.T(lang, "preflight.area_empty"))
		if code == "" {
			code = "MISSING_FIELD"
		}
	}
	if strings.TrimSpace(in.AuthorID) == "" {
		checklist = append(checklist, i18n.T(lang, "preflight.author_empty"))
		if code == "" {
			code = "MISSING_FIELD"
		}
	}
	if in.Completeness != "" {
		switch in.Completeness {
		case "draft", "partial", "settled":
		default:
			checklist = append(checklist,
				fmt.Sprintf(i18n.T(lang, "preflight.completeness_invalid"), in.Completeness))
			if code == "" {
				code = "INVALID_FIELD"
			}
		}
	}

	// Type-specific guardrails.
	switch in.Type {
	case "Task":
		if !strings.Contains(strings.ToLower(in.BodyMarkdown), "acceptance") {
			checklist = append(checklist, i18n.T(lang, "preflight.task_acceptance"))
			if code == "" {
				code = "TYPE_GUARDRAIL"
			}
		}
	case "Decision":
		lower := strings.ToLower(in.BodyMarkdown)
		if !strings.Contains(lower, "decision") || !strings.Contains(lower, "context") {
			checklist = append(checklist, i18n.T(lang, "preflight.adr_sections"))
			if code == "" {
				code = "TYPE_GUARDRAIL"
			}
		}
	}

	return checklist, code
}

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases, replaces runs of non-alnum with '-', trims dashes,
// and caps at 60 chars. Keeps ASCII only — Korean characters drop out,
// which is fine because slug is a URL/path component and the real human
// label lives in title. If the title has no ASCII letters the caller
// falls back to a type+timestamp slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// embedAndStoreChunks computes vectors for the title and each body chunk
// and inserts them into artifact_chunks inside the caller's transaction.
// All vectors pad to the DB column width (768) — see embed/vector.go for
// the rationale.
func embedAndStoreChunks(ctx context.Context, tx pgx.Tx, provider embed.Provider, artifactID, title, body string) error {
	info := provider.Info()

	// Title vector (always one, kind='title').
	titleRes, err := provider.Embed(ctx, embed.Request{Texts: []string{title}, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed title: %w", err)
	}
	if len(titleRes.Vectors) != 1 {
		return fmt.Errorf("embed title: got %d vectors", len(titleRes.Vectors))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, span_start, span_end,
			text, embedding, model_name, model_dim
		) VALUES ($1, 'title', 0, NULL, 0, 0, $2, $3::vector, $4, $5)
	`,
		artifactID,
		title,
		embed.VectorString(embed.PadTo768(titleRes.Vectors[0])),
		info.Name+":"+info.ModelID,
		info.Dimension,
	); err != nil {
		return fmt.Errorf("store title chunk: %w", err)
	}

	// Body chunks (kind='body').
	chunks := embed.ChunkBody(title, body, 600)
	if len(chunks) == 0 {
		return nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	bodyRes, err := provider.Embed(ctx, embed.Request{Texts: texts, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed body: %w", err)
	}
	if len(bodyRes.Vectors) != len(chunks) {
		return fmt.Errorf("embed body: got %d vectors want %d", len(bodyRes.Vectors), len(chunks))
	}
	for i, c := range chunks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_chunks (
				artifact_id, kind, chunk_index, heading, span_start, span_end,
				text, embedding, model_name, model_dim
			) VALUES ($1, 'body', $2, $3, $4, $5, $6, $7::vector, $8, $9)
		`,
			artifactID,
			c.Index,
			nullIfEmpty(c.Heading),
			c.SpanStart, c.SpanEnd,
			c.Text,
			embed.VectorString(embed.PadTo768(bodyRes.Vectors[i])),
			info.Name+":"+info.ModelID,
			info.Dimension,
		); err != nil {
			return fmt.Errorf("store body chunk %d: %w", c.Index, err)
		}
	}
	return nil
}

// isUniqueViolation is a best-effort check against pgx's error message.
// The typed error route requires importing pgconn; until we add that we
// string-match on the known constraint name. Good enough for one retry
// loop.
func isUniqueViolation(err error, constraint string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") &&
		(constraint == "" || strings.Contains(err.Error(), constraint))
}
