package tools

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type projectCurrentInput struct{}

type projectCurrentOutput struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Color           string    `json:"color,omitempty"`
	PrimaryLanguage string    `json:"primary_language"`
	AreasCount      int       `json:"areas_count"`
	ArtifactsCount  int       `json:"artifacts_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// RegisterProjectCurrent wires pindoc.project.current. Returns the active
// project the MCP server is pointed at (by PINDOC_PROJECT env). Agents call
// this on session start to pin their subsequent write scope.
func RegisterProjectCurrent(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.project.current",
			Description: "Return the active Pindoc project (id, slug, name, primary language, area/artifact counts). Call this once per session before any write tool so the agent knows which project scope its propose calls will land in.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ projectCurrentInput) (*sdk.CallToolResult, projectCurrentOutput, error) {
			var out projectCurrentOutput
			var desc, color *string

			err := deps.DB.QueryRow(ctx, `
				SELECT
					p.id::text,
					p.slug,
					p.name,
					p.description,
					p.color,
					p.primary_language,
					p.created_at,
					(SELECT count(*) FROM areas     WHERE project_id = p.id),
					(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
				FROM projects p
				WHERE p.slug = $1
			`, deps.ProjectSlug).Scan(
				&out.ID, &out.Slug, &out.Name,
				&desc, &color,
				&out.PrimaryLanguage, &out.CreatedAt,
				&out.AreasCount, &out.ArtifactsCount,
			)
			if err != nil {
				return nil, projectCurrentOutput{}, fmt.Errorf("project %q not found: %w", deps.ProjectSlug, err)
			}
			if desc != nil {
				out.Description = *desc
			}
			if color != nil {
				out.Color = *color
			}
			return nil, out, nil
		},
	)
}
