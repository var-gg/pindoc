package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// projectCreateInput is the MCP-tool projection of projects.CreateProjectInput.
// It carries the jsonschema tags the SDK uses to advertise the tool to
// agents — agent prompts see these descriptions verbatim. Plain Go fields
// would lose the docstrings, so the wrapper-level type stays separate from
// the package-level input.
type projectCreateInput struct {
	Slug            string `json:"slug" jsonschema:"lowercase kebab-case slug, 2-40 chars, unique per owner"`
	Name            string `json:"name" jsonschema:"human-readable display name"`
	PrimaryLanguage string `json:"primary_language" jsonschema:"required; one of en | ko | ja. Must be explicitly confirmed with the user; no default. Immutable after create — recreate the project if wrong"`
	Description     string `json:"description,omitempty" jsonschema:"one-line description shown on the project switcher; optional"`
	Color           string `json:"color,omitempty" jsonschema:"CSS color string (hex or oklch) used for the sidebar accent; optional"`
	// OwnerID is optional; defaults to 'default' for single-owner self-
	// host deployments. Larger deployments (multiple users sharing one
	// instance) set this to the logical owner identifier. Not a user
	// table reference today — just a string the server stores so future
	// permission scopes have something to hang off.
	OwnerID string `json:"owner_id,omitempty" jsonschema:"optional owner identifier; defaults to 'default'"`
}

type projectCreateOutput struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	URL         string `json:"url" jsonschema:"canonical UI path to the project's wiki — share this, not /wiki/..."`
	DefaultArea string `json:"default_area" jsonschema:"slug of the 'misc' area seeded so artifacts can be filed immediately"`
	Message     string `json:"message"`
	// ReconnectRequired + Activation + NextSteps advertise the Phase 14b
	// onboarding contract: project.create writes a row but does NOT
	// activate the new project in the current MCP session. Agents must
	// reconnect with PINDOC_PROJECT=<slug> to write into it.
	ReconnectRequired bool     `json:"reconnect_required"`
	Activation        string   `json:"activation" jsonschema:"one of: not_in_this_session"`
	NextSteps         []string `json:"next_steps"`
}

// RegisterProjectCreate wires pindoc.project.create. The handler is a
// thin wrapper around projects.CreateProject — the business logic
// (projects row + 9-area seed + 4-template seed) lives in
// internal/pindoc/projects so the REST endpoint, pindoc-admin CLI, and
// Reader UI can share the same source of truth (Decision
// project-bootstrap-canonical-flow-reader-ui-first-class).
//
// No UI button calls this — per architecture principle 1 (agent-only
// write surface), the user asks the agent and the agent calls this tool.
// The Reader UI's "+ New project" page goes through the REST endpoint
// instead.
func RegisterProjectCreate(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.project.create",
			Description: strings.TrimSpace(projectCreateDescription),
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in projectCreateInput) (*sdk.CallToolResult, projectCreateOutput, error) {
			tx, err := deps.DB.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				return nil, projectCreateOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
				Slug:            in.Slug,
				Name:            in.Name,
				Description:     in.Description,
				Color:           in.Color,
				PrimaryLanguage: in.PrimaryLanguage,
				OwnerID:         in.OwnerID,
			})
			if err != nil {
				return nil, projectCreateOutput{}, err
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, projectCreateOutput{}, fmt.Errorf("commit: %w", err)
			}

			deps.Logger.Info("project created",
				"slug", out.Slug, "name", out.Name, "lang", out.PrimaryLanguage)

			return nil, projectCreateOutput{
				ID:                out.ID,
				Slug:              out.Slug,
				Name:              out.Name,
				URL:               fmt.Sprintf("/p/%s/%s/wiki", out.Slug, out.PrimaryLanguage),
				DefaultArea:       out.DefaultArea,
				ReconnectRequired: true,
				Activation:        "not_in_this_session",
				NextSteps: []string{
					fmt.Sprintf("Restart pindoc-server with PINDOC_PROJECT=%s to make this MCP session write into the new project.", out.Slug),
					fmt.Sprintf("Open the Reader at /p/%s/%s/wiki once pindoc-api reloads.", out.Slug, out.PrimaryLanguage),
				},
				Message: strings.TrimSpace(fmt.Sprintf(`
Project %q (%s locale) created. Share this URL with the user: /p/%s/%s/wiki
Note: this MCP session is still scoped to the old project — to write
artifacts into %q, restart pindoc-server with PINDOC_PROJECT=%s.
`, out.Slug, out.PrimaryLanguage, out.Slug, out.PrimaryLanguage, out.Slug, out.Slug)),
			}, nil
		},
	)
}

const projectCreateDescription = `
Create a new Pindoc project. Returns the canonical
/p/{slug}/{primary_language}/wiki URL the user should bookmark.
Auto-creates 9 top-level/project-root areas
(Decision area-구조-top-level-고정-골격-depth-2-sub-area만-프로젝트별-자유):
the fixed 8 concern skeleton plus _unsorted, then starter sub-areas so
artifacts can be filed immediately.

primary_language is required and must be explicitly confirmed with the
user. No default: if the user did not specify the project language, ask
before calling. Supported languages are en, ko, ja. primary_language is
immutable after creation; if it is wrong, the only correction path is to
recreate the project (no automatic artifact/area migration).

When to call: user says "start a new project for X" or asks to split
docs for a repo that isn't covered by the current Pindoc instance.
Pick a kebab-case slug tied to the repo or product name.

This tool does not switch the MCP session's active project. The session
scope stays tied to PINDOC_PROJECT env; a future session starts under
the new project by launching pindoc-server with PINDOC_PROJECT=<new>.
`
