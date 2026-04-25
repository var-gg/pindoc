package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// projectCurrentInput accepts an optional ProjectSlug. Account-level
// scope (Decision mcp-scope-account-level-industry-standard) means the
// MCP connection is no longer pinned to a project — agents pass the
// slug they want metadata for. Empty falls back to deps.DefaultProjectSlug
// (PINDOC_PROJECT env) so existing single-project setups keep working
// without harness updates.
type projectCurrentInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"projects.slug to inspect; omit to use the server's default project (PINDOC_PROJECT env)"`
}

type projectCurrentOutput struct {
	// not_ready surface — populated when the server can't pick a project
	// (no input slug, no PINDOC_PROJECT env, no projects table row).
	Status           string   `json:"status,omitempty"`
	ErrorCode        string   `json:"error_code,omitempty"`
	Failed           []string `json:"failed,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	ID          string `json:"id,omitempty"`
	Slug        string `json:"slug,omitempty"`
	Name        string `json:"name,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	PrimaryLanguage string `json:"primary_language,omitempty"`
	// Locale is the authoritative canonical-key field (Task task-phase-
	// 18-project-locale-implementation, migration 0015). primary_language
	// is retained as a soft back-compat column; new code should branch
	// on locale. Same slug may exist across locales —
	// `(owner_id, slug, locale)` is the unique key.
	Locale          string        `json:"locale,omitempty"`
	AreasCount      int           `json:"areas_count,omitempty"`
	ArtifactsCount  int           `json:"artifacts_count,omitempty"`
	CreatedAt       time.Time     `json:"created_at,omitzero"`
	Rendering       RenderingCaps `json:"rendering,omitzero"`
	Capabilities    Capabilities  `json:"capabilities,omitzero"`
}

// Capabilities tells the agent which optional features the server
// currently honours. Lets a prompt branch without probing each tool. Fields
// are intentionally flat — every string value is a stable enum, not prose.
type Capabilities struct {
	// MultiProject is derived per call from the projects table — true
	// when more than one project is visible to the caller (V1: row
	// count; V1.5+: ACL-filtered count). Reader uses it to decide
	// whether to render the project switcher; advisory for chat UX.
	MultiProject bool `json:"multi_project"`
	// ScopeMode describes how an MCP session maps to projects.
	// "per_call" = the connection is account-level and each tool input
	// carries project_slug (Decision mcp-scope-account-level-industry-
	// standard, supersedes per_connection). Always per_call now;
	// the field stays for forward compatibility with future scope
	// modes (e.g. tenant-pinned SaaS).
	ScopeMode string `json:"scope_mode"`
	// NewProjectRequiresReconnect was meaningful when scope was
	// per-connection — pindoc.project.create wrote a row but didn't
	// activate the slug for the current MCP subprocess. With account-
	// level scope the new slug is usable on the very next tool call,
	// so this is always false. Kept in the schema so older agents that
	// branch on it don't crash; remove in V2 once consumers migrate.
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
	// over HTTP, all sharing the single account-level /mcp endpoint.
	// Carried for telemetry / debugging only — scope_mode is per_call
	// regardless of transport.
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

// RegisterProjectCurrent wires pindoc.project.current. Returns metadata
// for the project named by input.project_slug (or the server's default
// project when omitted). Agents call this once per session for the
// capability advertisement; with account-level scope the call no longer
// pins the connection to one project.
func RegisterProjectCurrent(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.project.current",
			Description: "Return Pindoc project metadata (id, slug, name, primary language, area/artifact counts, capabilities). Pass project_slug to inspect a specific project; omit to fall back to the server's default (PINDOC_PROJECT env). Returns not_ready with PROJECT_SLUG_REQUIRED when neither is set.",
		},
		func(ctx context.Context, princ *auth.Principal, in projectCurrentInput) (*sdk.CallToolResult, projectCurrentOutput, error) {
			slug := strings.TrimSpace(in.ProjectSlug)
			if slug == "" {
				slug = strings.TrimSpace(deps.DefaultProjectSlug)
			}
			if slug == "" {
				return nil, projectCurrentOutput{
					Status:    "not_ready",
					ErrorCode: "PROJECT_SLUG_REQUIRED",
					Failed:    []string{"PROJECT_SLUG_REQUIRED"},
					Checklist: []string{
						"project_slug input was empty and the server has no PINDOC_PROJECT default — pass project_slug explicitly.",
					},
					SuggestedActions: []string{
						"Retry with project_slug set to one of your project slugs (call pindoc.project.create first if no project exists).",
					},
				}, nil
			}

			scope, err := auth.ResolveProject(ctx, deps.DB, princ, slug)
			if err != nil {
				if errors.Is(err, auth.ErrProjectNotFound) {
					return nil, projectCurrentOutput{
						Status:    "not_ready",
						ErrorCode: "PROJECT_NOT_FOUND",
						Failed:    []string{"PROJECT_NOT_FOUND"},
						Checklist: []string{fmt.Sprintf("project %q does not exist on this server", slug)},
						SuggestedActions: []string{
							"List available projects with the Reader's project switcher, or call pindoc.project.create to create it.",
						},
					}, nil
				}
				return nil, projectCurrentOutput{}, fmt.Errorf("project.current: %w", err)
			}

			var out projectCurrentOutput
			var desc, color *string
			err = deps.DB.QueryRow(ctx, `
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
			`, scope.ProjectSlug).Scan(
				&out.ID, &out.Slug, &out.Name, &out.OwnerID,
				&desc, &color,
				&out.PrimaryLanguage, &out.Locale, &out.CreatedAt,
				&out.AreasCount, &out.ArtifactsCount,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				// Race: ResolveProject saw the row but it was deleted before
				// we read the rest. Surface as PROJECT_NOT_FOUND for symmetry.
				return nil, projectCurrentOutput{
					Status:    "not_ready",
					ErrorCode: "PROJECT_NOT_FOUND",
					Failed:    []string{"PROJECT_NOT_FOUND"},
					Checklist: []string{fmt.Sprintf("project %q vanished between ACL check and read; retry", scope.ProjectSlug)},
				}, nil
			}
			if err != nil {
				return nil, projectCurrentOutput{}, fmt.Errorf("project %q lookup: %w", scope.ProjectSlug, err)
			}
			if desc != nil {
				out.Description = *desc
			}
			if color != nil {
				out.Color = *color
			}
			out.Status = "accepted"
			out.Rendering = pindocRenderingCaps
			out.Capabilities = buildCapabilities(deps, princ, deriveMultiProject(ctx, deps, princ))
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
func deriveMultiProject(ctx context.Context, deps Deps, p *auth.Principal) bool {
	if deps.DB == nil || p == nil {
		return false
	}
	n, err := projects.CountVisible(ctx, deps.DB, p.UserID)
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

// buildCapabilities composes the Capabilities block returned by
// pindoc.project.current. ScopeMode is always "per_call" and
// NewProjectRequiresReconnect always false now — Decision mcp-scope-
// account-level-industry-standard. Transport is reported for
// telemetry / debugging but no longer drives scope branching.
func buildCapabilities(deps Deps, p *auth.Principal, multiProject bool) Capabilities {
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
	authMode := auth.AuthModeTrustedLocal
	if p != nil && p.AuthMode != "" {
		authMode = p.AuthMode
	}
	transport := strings.TrimSpace(deps.Transport)
	if transport == "" {
		transport = "stdio"
	}
	return Capabilities{
		MultiProject:                multiProject,
		ScopeMode:                   "per_call",
		NewProjectRequiresReconnect: false,
		RetrievalQuality:            quality,
		AuthMode:                    authMode,
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
