package tools

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type projectCurrentInput struct{}

type projectCurrentOutput struct {
	ID              string        `json:"id"`
	Slug            string        `json:"slug"`
	Name            string        `json:"name"`
	OwnerID         string        `json:"owner_id"`
	Description     string        `json:"description,omitempty"`
	Color           string        `json:"color,omitempty"`
	PrimaryLanguage string        `json:"primary_language"`
	// Locale is the authoritative canonical-key field (Task task-phase-
	// 18-project-locale-implementation, migration 0015). primary_language
	// is retained as a soft back-compat column; new code should branch
	// on locale. Same slug may exist across locales —
	// `(owner_id, slug, locale)` is the unique key.
	Locale          string        `json:"locale"`
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
	// MultiProject is derived per call from the projects table — true
	// when more than one project is visible to the caller (V1: row
	// count; V1.5+: ACL-filtered count). Reader uses it to decide
	// whether to render the project switcher; advisory for chat UX
	// only since MCP scope is pinned per-connection by the URL.
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
	// Transport identifies how the agent reached this server. "stdio" =
	// classic subprocess-per-session model where Claude Code launches
	// pindoc-server as a child process. "streamable_http" = daemon mode
	// where many MCP sessions connect to one long-running pindoc-server
	// over HTTP, each pinned to its project via /mcp/p/{project} URL.
	// Drives ScopeMode and NewProjectRequiresReconnect; agents can branch
	// their UX (e.g. "switch project means open a new url" vs "restart
	// subprocess"). Added with the streamable-HTTP transport rollout.
	Transport string `json:"transport"`
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
	AddInstrumentedTool(server, deps,
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
					p.owner_id,
					p.description,
					p.color,
					p.primary_language,
					p.locale,
					p.created_at,
					(SELECT count(*) FROM areas     WHERE project_id = p.id),
					(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
				FROM projects p
				WHERE p.slug = $1
			`, deps.ProjectSlug).Scan(
				&out.ID, &out.Slug, &out.Name, &out.OwnerID,
				&desc, &color,
				&out.PrimaryLanguage, &out.Locale, &out.CreatedAt,
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
			out.Capabilities = buildCapabilities(deps, deriveMultiProject(ctx, deps))
			return nil, out, nil
		},
	)
}

// deriveMultiProject queries the projects table on every capability
// advertisement. The cost is a single COUNT, run once per
// pindoc.project.current call (which agents call rarely — bootstrap +
// the occasional explicit re-check), so we don't bother caching at V1.
// Errors and a missing DB pool both fall back to false so a transient
// DB hiccup hides the switcher rather than crashing the tool call —
// the UI falling back to single-project chrome is preferable to a
// bootstrap failure.
func deriveMultiProject(ctx context.Context, deps Deps) bool {
	if deps.DB == nil {
		return false
	}
	n, err := projects.CountVisible(ctx, deps.DB, deps.UserID)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("multi_project derivation failed; defaulting to false",
				"err", err,
			)
		}
		return false
	}
	return projects.IsMultiProject(n)
}

func buildCapabilities(deps Deps, multiProject bool) Capabilities {
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
	transport := deps.Transport
	if transport == "" {
		transport = "stdio"
	}
	// Transport drives the scope-mode pair. Streamable-HTTP daemons accept
	// many connections, each scoped to one project via the /mcp/p/{project}
	// URL, so a "new project" is just a new url — no reconnect of the
	// daemon itself. Stdio binds the project at process start, so
	// switching projects means tearing down the subprocess and launching
	// a new one with a different PINDOC_PROJECT env.
	scopeMode := "fixed_session"
	requiresReconnect := true
	if transport == "streamable_http" {
		scopeMode = "per_connection"
		requiresReconnect = false
	}
	return Capabilities{
		MultiProject:                multiProject,
		ScopeMode:                   scopeMode,
		NewProjectRequiresReconnect: requiresReconnect,
		RetrievalQuality:            quality,
		AuthMode:                    "trusted_local",
		Transport:                   transport,
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
