package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/i18n"
)

// areaSlugRe enforces the same URL-safe shape used by project slugs
// (lowercase letter + kebab tail, 2-40 chars). Areas live under
// /p/{project}/wiki/{slug} alongside artifacts so the same cap keeps the
// URL bar readable.
var areaSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

type areaCreateInput struct {
	ProjectSlug    string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	ParentSlug     string `json:"parent_slug" jsonschema:"required top-level area slug that will own the new sub-area"`
	Slug           string `json:"slug" jsonschema:"lowercase kebab-case slug, 2-40 chars, unique within the project"`
	Name           string `json:"name" jsonschema:"display name, 2-60 chars"`
	Description    string `json:"description,omitempty" jsonschema:"optional sidebar tooltip text, max 240 chars"`
	IsCrossCutting bool   `json:"is_cross_cutting,omitempty" jsonschema:"whether this sub-area spans concerns; default false"`
}

type areaCreateOutput struct {
	Status          string               `json:"status"` // accepted | not_ready
	ErrorCode       string               `json:"error_code,omitempty"`
	Failed          []string             `json:"failed,omitempty"`
	Checklist       []string             `json:"checklist,omitempty"`
	ErrorCodes      []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	ChecklistItems  []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale   string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`
	PatchableFields []string             `json:"patchable_fields,omitempty"`

	ID             string    `json:"id,omitempty"`
	ProjectSlug    string    `json:"project_slug,omitempty"`
	ParentSlug     string    `json:"parent_slug,omitempty"`
	Slug           string    `json:"slug,omitempty"`
	Name           string    `json:"name,omitempty"`
	Description    string    `json:"description,omitempty"`
	IsCrossCutting bool      `json:"is_cross_cutting,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitzero"`
	ToolsetVersion string    `json:"toolset_version,omitempty"`
}

type normalizedAreaCreateInput struct {
	ParentSlug     string
	Slug           string
	Name           string
	Description    string
	IsCrossCutting bool
}

// RegisterAreaCreate wires pindoc.area.create. It creates depth-1 sub-areas
// only: top-level areas are governed by the fixed seed taxonomy, while
// project-specific sub-areas are intentionally free.
func RegisterAreaCreate(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.area.create",
			Description: strings.TrimSpace(`
Create a project-specific sub-area under an existing top-level area. Implements Decision area-구조-top-level-고정-골격-depth-2-sub-area만-프로젝트별-자유: parent_slug is required and must name a current-project top-level area (parent_id IS NULL). This tool never creates top-level areas; those are seeded by project.create/migrations.
`),
		},
		func(ctx context.Context, p *auth.Principal, in areaCreateInput) (*sdk.CallToolResult, areaCreateOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, areaCreateOutput{}, fmt.Errorf("area.create: %w", err)
			}

			lang := deps.UserLanguage
			norm, notReady := validateAreaCreateInput(in, lang)
			if notReady != nil {
				return nil, *notReady, nil
			}

			tx, err := deps.DB.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				return nil, areaCreateOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			var projectID string
			var parentID, parentParentID *string
			err = tx.QueryRow(ctx, `
				SELECT p.id::text, a.id::text, a.parent_id::text
				  FROM projects p
				  LEFT JOIN areas a ON a.project_id = p.id AND a.slug = $2
				 WHERE p.slug = $1
				 LIMIT 1
			`, scope.ProjectSlug, norm.ParentSlug).Scan(&projectID, &parentID, &parentParentID)
			if err != nil {
				return nil, areaCreateOutput{}, fmt.Errorf("resolve parent area: %w", err)
			}
			if code := classifyAreaCreateParent(parentID, parentParentID); code != "" {
				out := areaCreateNotReady(lang, code, norm.ParentSlug)
				return nil, out, nil
			}

			var descPtr *string
			if norm.Description != "" {
				descPtr = &norm.Description
			}

			var out areaCreateOutput
			err = tx.QueryRow(ctx, `
				INSERT INTO areas (project_id, parent_id, slug, name, description, is_cross_cutting)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
				RETURNING id::text, slug, name, COALESCE(description, ''), is_cross_cutting, created_at
			`, projectID, *parentID, norm.Slug, norm.Name, descPtr, norm.IsCrossCutting).Scan(
				&out.ID, &out.Slug, &out.Name, &out.Description, &out.IsCrossCutting, &out.CreatedAt,
			)
			if err != nil {
				if isAreaCreateSlugTaken(err) {
					out := areaCreateNotReady(lang, "AREA_SLUG_TAKEN", norm.Slug)
					return nil, out, nil
				}
				return nil, areaCreateOutput{}, fmt.Errorf("insert area: %w", err)
			}

			actorID := strings.TrimSpace(p.AgentID)
			if actorID == "" {
				actorID = "unassigned"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1::uuid, 'area.created', $2::uuid, jsonb_build_object(
					'slug', $3::text,
					'parent_slug', $4::text,
					'is_cross_cutting', $5::bool,
					'author_id', $6::text
				))
			`, projectID, out.ID, out.Slug, norm.ParentSlug, out.IsCrossCutting, actorID); err != nil {
				return nil, areaCreateOutput{}, fmt.Errorf("event insert: %w", err)
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, areaCreateOutput{}, fmt.Errorf("commit: %w", err)
			}

			out.Status = "accepted"
			out.ProjectSlug = scope.ProjectSlug
			out.ParentSlug = norm.ParentSlug
			return nil, out, nil
		},
	)
}

func validateAreaCreateInput(in areaCreateInput, lang string) (normalizedAreaCreateInput, *areaCreateOutput) {
	out := normalizedAreaCreateInput{
		ParentSlug:     strings.ToLower(strings.TrimSpace(in.ParentSlug)),
		Slug:           strings.ToLower(strings.TrimSpace(in.Slug)),
		Name:           strings.TrimSpace(in.Name),
		Description:    strings.TrimSpace(in.Description),
		IsCrossCutting: in.IsCrossCutting,
	}
	switch {
	case out.ParentSlug == "":
		return out, ptrAreaCreateNotReady(lang, "PARENT_REQUIRED")
	case !areaSlugRe.MatchString(out.Slug):
		return out, ptrAreaCreateNotReady(lang, "SLUG_INVALID", in.Slug)
	case len([]rune(out.Name)) < 2 || len([]rune(out.Name)) > 60:
		return out, ptrAreaCreateNotReady(lang, "AREA_NAME_INVALID")
	case len([]rune(out.Description)) > 240:
		return out, ptrAreaCreateNotReady(lang, "AREA_DESCRIPTION_TOO_LONG")
	default:
		return out, nil
	}
}

func classifyAreaCreateParent(parentID, parentParentID *string) string {
	if parentID == nil || strings.TrimSpace(*parentID) == "" {
		return "PARENT_NOT_FOUND"
	}
	if parentParentID != nil && strings.TrimSpace(*parentParentID) != "" {
		return "PARENT_NOT_TOP_LEVEL"
	}
	return ""
}

func isAreaCreateSlugTaken(err error) bool {
	return isUniqueViolation(err, "areas_project_id_slug_key")
}

func ptrAreaCreateNotReady(lang, code string, args ...any) *areaCreateOutput {
	out := areaCreateNotReady(lang, code, args...)
	return &out
}

func areaCreateNotReady(lang, code string, args ...any) areaCreateOutput {
	msg := i18n.T(lang, areaCreateI18NKey(code))
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	return areaCreateOutput{
		Status:          "not_ready",
		ErrorCode:       code,
		Failed:          []string{code},
		Checklist:       []string{msg},
		PatchableFields: areaCreatePatchFieldsFor(code),
	}
}

func areaCreateI18NKey(code string) string {
	switch code {
	case "PARENT_REQUIRED":
		return "preflight.area_parent_required"
	case "PARENT_NOT_FOUND":
		return "preflight.area_parent_not_found"
	case "PARENT_NOT_TOP_LEVEL":
		return "preflight.area_parent_not_top_level"
	case "SLUG_INVALID":
		return "preflight.area_slug_invalid"
	case "AREA_SLUG_TAKEN":
		return "preflight.area_slug_taken"
	case "AREA_NAME_INVALID":
		return "preflight.area_name_invalid"
	case "AREA_DESCRIPTION_TOO_LONG":
		return "preflight.area_description_too_long"
	default:
		return "preflight.area_create_invalid"
	}
}

func areaCreatePatchFieldsFor(code string) []string {
	switch code {
	case "PARENT_REQUIRED", "PARENT_NOT_FOUND", "PARENT_NOT_TOP_LEVEL":
		return []string{"parent_slug"}
	case "SLUG_INVALID", "AREA_SLUG_TAKEN":
		return []string{"slug"}
	case "AREA_NAME_INVALID":
		return []string{"name"}
	case "AREA_DESCRIPTION_TOO_LONG":
		return []string{"description"}
	default:
		return nil
	}
}
