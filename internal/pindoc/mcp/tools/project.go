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
	// ScopeMode describes how an MCP session maps to projects.
	// "fixed_session" = one subprocess binds to one project for life.
	// Agents must not try switching project inside a session; new
	// project = new MCP connection.
	ScopeMode string `json:"scope_mode"`
	// NewProjectRequiresReconnect advertises the runtime fact that
	// pindoc.project.create makes a row but does NOT change the active
	// scope of the current MCP subprocess. Paired with project.create's
	// `reconnect_required` field in the response body.
	NewProjectRequiresReconnect bool `json:"new_project_requires_reconnect"`
	// RetrievalQuality: "stub" → hash-based (dev only), "http" → real
	// embedder backing pindoc.artifact.search / context.for_task.
	RetrievalQuality string `json:"retrieval_quality"`
	// AuthMode: "trusted_local" (M1 self-host local subprocess, no token),
	// "project_token" (V1.5+ per-project agent tokens),
	// "oauth" (V2+ hosted). Renamed from "none" — "none" implied "no
	// security at all" but the actual model is "trust the local
	// subprocess".
	AuthMode string `json:"auth_mode"`
	// UpdateVia: name of the propose field that triggers a revision append.
	// Agents can grep for this token so a future rename doesn't silently
	// reroute update flows to "create a new artifact".
	UpdateVia string `json:"update_via"`
	// RequiresExpectedVersion: when true, pindoc.artifact.propose with
	// update_of requires expected_version (optimistic lock). Surfaces
	// the Phase 14b decision so agents don't discover it via not_ready.
	RequiresExpectedVersion bool `json:"requires_expected_version"`
	// ReviewQueueSupported: sensitive-op confirm mode with pending_review
	// state routing. False in M1; comes with auth in V1.5.
	ReviewQueueSupported bool `json:"review_queue_supported"`
	// ReceiptTTLSec is the search_receipt TTL (seconds). Agents can use
	// this to decide whether to renew mid-loop.
	ReceiptTTLSec int `json:"receipt_ttl_sec"`
	// PublicBaseURL comes from server_settings.public_base_url. Empty
	// when the operator hasn't configured one — agents should fall back
	// to the relative human_url in that case. When present, tool
	// responses also include human_url_abs.
	PublicBaseURL string `json:"public_base_url,omitempty"`
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
	publicBase := ""
	if deps.Settings != nil {
		publicBase = deps.Settings.Get().PublicBaseURL
	}
	return Capabilities{
		MultiProject:                deps.MultiProject,
		ScopeMode:                   "fixed_session",
		NewProjectRequiresReconnect: true,
		RetrievalQuality:            quality,
		AuthMode:                    "trusted_local",
		UpdateVia:                   "update_of",
		RequiresExpectedVersion:     true,
		ReviewQueueSupported:        false,
		ReceiptTTLSec:               int(receiptTTLSeconds),
		PublicBaseURL:               publicBase,
	}
}

// receiptTTLSeconds mirrors receipts.DefaultTTL. Kept as a constant here
// so capability reporting doesn't need an import cycle via the receipts
// package at call sites.
const receiptTTLSeconds = 30 * 60
