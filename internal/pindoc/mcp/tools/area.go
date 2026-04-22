package tools

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type areaListInput struct {
	// IncludeArchived is reserved; we only flip the flag once the archive
	// flow for areas lands. Shipping the field shape early so agents don't
	// need to relearn a new schema later.
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"reserved; has no effect yet"`
	// IncludeTemplates controls whether _template_* artifacts are counted
	// in artifact_count. Default false keeps counts aligned with the
	// artifact list (which also hides templates by default). Flip true
	// only when the caller already intends to fetch artifacts with
	// include_templates=true, so the two responses stay in sync.
	IncludeTemplates bool `json:"include_templates,omitempty" jsonschema:"count _template_* artifacts in artifact_count; default false matches artifact.search/list defaults"`
}

type AreaRef struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	ParentSlug       string   `json:"parent_slug,omitempty"`
	IsCrossCutting   bool     `json:"is_cross_cutting"`
	ArtifactCount    int      `json:"artifact_count"`
	ChildrenSlugs    []string `json:"children_slugs,omitempty"`
}

type areaListOutput struct {
	ProjectSlug string    `json:"project_slug"`
	Areas       []AreaRef `json:"areas"`
}

// RegisterAreaList wires pindoc.area.list. Returns every Area in the active
// project with counts an agent uses to route a propose call to the right
// scope without a second round-trip.
func RegisterAreaList(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.area.list",
			Description: "List every Area in the current project. Use this to pick the right area_slug before pindoc.artifact.propose. Every artifact must live in exactly one Area (use 'misc' if nothing else fits, 'cross-cutting' for concerns that span all areas).",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in areaListInput) (*sdk.CallToolResult, areaListOutput, error) {
			rows, err := deps.DB.Query(ctx, `
				WITH p AS (SELECT id FROM projects WHERE slug = $1)
				SELECT
					a.id::text,
					a.slug,
					a.name,
					a.description,
					parent.slug,
					a.is_cross_cutting,
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
			`, deps.ProjectSlug, in.IncludeTemplates)
			if err != nil {
				return nil, areaListOutput{}, fmt.Errorf("query areas: %w", err)
			}
			defer rows.Close()

			out := areaListOutput{ProjectSlug: deps.ProjectSlug, Areas: []AreaRef{}}
			for rows.Next() {
				var a AreaRef
				var desc, parentSlug *string
				if err := rows.Scan(
					&a.ID, &a.Slug, &a.Name,
					&desc, &parentSlug, &a.IsCrossCutting,
					&a.ArtifactCount, &a.ChildrenSlugs,
				); err != nil {
					return nil, areaListOutput{}, fmt.Errorf("scan: %w", err)
				}
				if desc != nil {
					a.Description = *desc
				}
				if parentSlug != nil {
					a.ParentSlug = *parentSlug
				}
				out.Areas = append(out.Areas, a)
			}
			return nil, out, rows.Err()
		},
	)
}
