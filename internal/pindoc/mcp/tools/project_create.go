package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
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
	// BootstrapReceipt is a one-use search_receipt the agent can pass to
	// the first artifact.propose call in the newly-created project without
	// paying a separate artifact.search/context.for_task round-trip.
	BootstrapReceipt string `json:"bootstrap_receipt,omitempty"`
	// SearchReceipt is an alias for BootstrapReceipt for clients that
	// already expect the generic receipt field name.
	SearchReceipt string `json:"search_receipt,omitempty"`
	Message       string `json:"message"`
	// ReconnectRequired + Activation + NextSteps describe how the new
	// project becomes addressable. Account-level scope (Decision
	// mcp-scope-account-level-industry-standard) means the new slug is
	// usable immediately — every subsequent tool call carries
	// project_slug in its input, so no MCP reconnect is needed. Kept
	// here for backward compat with agents that still branch on the
	// flag.
	ReconnectRequired bool           `json:"reconnect_required"`
	Activation        string         `json:"activation" jsonschema:"one of: in_this_session"`
	NextSteps         []NextToolHint `json:"next_steps"`
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
		func(ctx context.Context, _ *auth.Principal, in projectCreateInput) (*sdk.CallToolResult, projectCreateOutput, error) {
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

			bootstrapReceipt := ""
			if deps.Receipts != nil {
				bootstrapReceipt = deps.Receipts.IssueOneUse(out.Slug, "project.create bootstrap", nil)
			}

			return nil, projectCreateOutput{
				ID:                out.ID,
				Slug:              out.Slug,
				Name:              out.Name,
				URL:               fmt.Sprintf("/p/%s/wiki", out.Slug),
				DefaultArea:       out.DefaultArea,
				BootstrapReceipt:  bootstrapReceipt,
				SearchReceipt:     bootstrapReceipt,
				ReconnectRequired: false,
				Activation:        "in_this_session",
				NextSteps:         projectCreateNextSteps(deps.UserLanguage, out.Slug),
				Message: strings.TrimSpace(fmt.Sprintf(`
Project %q (%s canonical language) created. Share this URL with the user: /p/%s/wiki
The new slug is usable immediately — pass project_slug=%q in subsequent
tool inputs to write into it; no MCP reconnect needed (account-level
scope, Decision mcp-scope-account-level-industry-standard).
`, out.Slug, out.PrimaryLanguage, out.Slug, out.Slug)),
			}, nil
		},
	)
}

const projectCreateDescription = `
Create a new Pindoc project. Returns the canonical
/p/{slug}/wiki URL the user should bookmark.
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

The new slug is addressable immediately on this MCP connection —
account-level scope (Decision mcp-scope-account-level-industry-
standard) means every project-scoped tool takes a project_slug input
and the new slug works on the very next call without reconnect.
The response includes a one-use bootstrap_receipt/search_receipt for the
first artifact.propose call in the new project, so agents do not need an
extra search round-trip before writing the initial artifact.
`

func projectCreateNextSteps(lang, projectSlug string) []NextToolHint {
	return []NextToolHint{
		{
			Tool: "pindoc.harness.install",
			Args: map[string]any{
				"project_slug": projectSlug,
			},
			Reason: projectCreateHarnessReason(lang),
		},
		{
			Tool: "pindoc.ping",
			Args: map[string]any{
				"project_slug": projectSlug,
			},
			Reason: fmt.Sprintf("Verify project_slug=%q is addressable in this MCP session.", projectSlug),
		},
	}
}

func projectCreateHarnessReason(lang string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "ko") {
		return "먼저 PINDOC.md를 설치하세요. 설치하지 않으면 이후 artifact.propose가 harness/context 부족으로 거부될 수 있습니다."
	}
	return "Install PINDOC.md first; later artifact.propose calls can be rejected when the harness context is missing."
}
