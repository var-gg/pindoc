package tools

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type projectCurrentInput struct{}

type projectCurrentOutput struct {
	ID              string        `json:"id"`
	Slug            string        `json:"slug"`
	Name            string        `json:"name"`
	Description     string        `json:"description,omitempty"`
	Color           string        `json:"color,omitempty"`
	PrimaryLanguage string        `json:"primary_language"`
	AreasCount      int           `json:"areas_count"`
	ArtifactsCount  int           `json:"artifacts_count"`
	CreatedAt       time.Time     `json:"created_at"`
	Rendering       RenderingCaps `json:"rendering"`
	Capabilities    Capabilities  `json:"capabilities"`
}

// Capabilities tells the agent which optional features the server
// currently honours. Lets a prompt branch without probing each tool. Fields
// are intentionally flat — every string value is a stable enum, not prose.
type Capabilities struct {
	// MultiProject: does this instance expect >1 project in the UI
	// switcher? MCP tool calls are still scoped per-subprocess to the
	// PINDOC_PROJECT env; this flag is advisory for chat UX only.
	MultiProject bool `json:"multi_project"`
	// RetrievalQuality: "stub" → hash-based (dev only), "http" → real
	// embedder backing pindoc.artifact.search / context.for_task.
	RetrievalQuality string `json:"retrieval_quality"`
	// AuthMode: "none" in M1 self-host local. "github_oauth" lands in V1.5.
	AuthMode string `json:"auth_mode"`
	// UpdateVia: name of the propose field that triggers a revision append.
	// Agents can grep for this token so a future rename doesn't silently
	// reroute update flows to "create a new artifact".
	UpdateVia string `json:"update_via"`
	// ReviewQueueSupported: sensitive-op confirm mode with pending_review
	// state routing. False in M1; comes with auth in V1.5.
	ReviewQueueSupported bool `json:"review_queue_supported"`
}

// RenderingCaps mirrors the HTTP API shape so MCP callers get the same
// guidance. Kept in lockstep with internal/pindoc/httpapi/handlers.go.
type RenderingCaps struct {
	MarkdownFlavor string   `json:"markdown_flavor"`
	Extensions     []string `json:"extensions"`
	CodeLanguages  []string `json:"code_languages"`
	Notes          string   `json:"notes,omitempty"`
}

var pindocRenderingCaps = RenderingCaps{
	MarkdownFlavor: "gfm",
	Extensions: []string{
		"tables",
		"task_lists",
		"strikethrough",
		"autolink",
		"mermaid",
	},
	CodeLanguages: []string{"any"},
	Notes:         "Headings H1-H6, ordered/unordered lists, blockquotes, inline code, fenced code, links. Mermaid via ```mermaid fence. Math/KaTeX not supported (M1.x).",
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
			out.Rendering = pindocRenderingCaps
			out.Capabilities = buildCapabilities(deps)
			return nil, out, nil
		},
	)
}

func buildCapabilities(deps Deps) Capabilities {
	quality := "stub"
	if deps.Embedder != nil {
		if name := deps.Embedder.Info().Name; name != "" && name != "stub" {
			quality = name
		}
	}
	return Capabilities{
		MultiProject:         deps.MultiProject,
		RetrievalQuality:     quality,
		AuthMode:             "none",
		UpdateVia:            "update_of",
		ReviewQueueSupported: false,
	}
}
