package tools

import (
	"context"
	"errors"
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
	GitRemoteURL    string `json:"git_remote_url,omitempty" jsonschema:"optional git remote URL; normalized into project_repos for workspace detection"`
	// OwnerID is optional; defaults to 'default' for single-owner self-
	// host deployments. Larger deployments (multiple users sharing one
	// instance) set this to the logical owner identifier. Not a user
	// table reference today — just a string the server stores so future
	// permission scopes have something to hang off.
	OwnerID string `json:"owner_id,omitempty" jsonschema:"optional owner identifier; defaults to 'default'"`
}

type projectCreateOutput struct {
	Status         string               `json:"status,omitempty"`
	ErrorCode      string               `json:"error_code,omitempty"`
	Failed         []string             `json:"failed,omitempty"`
	ErrorCodes     []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist      []string             `json:"checklist,omitempty"`
	ChecklistItems []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale  string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`

	ID          string `json:"id,omitempty"`
	Slug        string `json:"slug,omitempty"`
	Name        string `json:"name,omitempty"`
	URL         string `json:"url,omitempty" jsonschema:"canonical UI path to the project's wiki — share this, not /wiki/..."`
	DefaultArea string `json:"default_area,omitempty" jsonschema:"slug of the 'misc' area seeded so artifacts can be filed immediately"`
	// BootstrapReceipt is a one-use search_receipt the agent can pass to
	// the first artifact.propose call in the newly-created project without
	// paying a separate artifact.search/context.for_task round-trip.
	BootstrapReceipt string `json:"bootstrap_receipt,omitempty"`
	// SearchReceipt is an alias for BootstrapReceipt for clients that
	// already expect the generic receipt field name.
	SearchReceipt string `json:"search_receipt,omitempty"`
	Message       string `json:"message,omitempty"`
	// ReconnectRequired + Activation + NextSteps describe how the new
	// project becomes addressable. Account-level scope (Decision
	// mcp-scope-account-level-industry-standard) means the new slug is
	// usable immediately — every subsequent tool call carries
	// project_slug in its input, so no MCP reconnect is needed. Kept
	// here for backward compat with agents that still branch on the
	// flag.
	ReconnectRequired bool           `json:"reconnect_required"`
	Activation        string         `json:"activation,omitempty" jsonschema:"one of: in_this_session"`
	NextSteps         []NextToolHint `json:"next_steps,omitempty"`
	ToolsetVersion    string         `json:"toolset_version,omitempty"`
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
		func(ctx context.Context, p *auth.Principal, in projectCreateInput) (*sdk.CallToolResult, projectCreateOutput, error) {
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
				GitRemoteURL:    in.GitRemoteURL,
				OwnerID:         in.OwnerID,
				OwnerUserID:     principalUserID(p),
			})
			if err != nil {
				if notReady, ok := projectCreateNotReady(deps.UserLanguage, err); ok {
					return nil, notReady, nil
				}
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
				Status:            "accepted",
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

func projectCreateNotReady(lang string, err error) (projectCreateOutput, bool) {
	code := projectCreateErrorCode(err)
	if code == "" {
		return projectCreateOutput{}, false
	}
	out := projectCreateOutput{
		Status:    "not_ready",
		ErrorCode: code,
		Failed:    []string{code},
		Checklist: []string{projectCreateChecklistMessage(lang, code, err)},
	}
	return applyMCPErrorContract(out, lang), true
}

func projectCreateErrorCode(err error) string {
	switch {
	case errors.Is(err, projects.ErrSlugInvalid):
		return "SLUG_INVALID"
	case errors.Is(err, projects.ErrSlugReserved):
		return "SLUG_RESERVED"
	case errors.Is(err, projects.ErrSlugTaken):
		return "SLUG_TAKEN"
	case errors.Is(err, projects.ErrNameRequired):
		return "NAME_REQUIRED"
	case errors.Is(err, projects.ErrLangRequired):
		return "LANG_REQUIRED"
	case errors.Is(err, projects.ErrLangInvalid):
		return "LANG_INVALID"
	case errors.Is(err, projects.ErrGitRemoteURLInvalid):
		return "GIT_REMOTE_URL_INVALID"
	default:
		return ""
	}
}

func projectCreateChecklistMessage(lang, code string, err error) string {
	if normalizeMessageLocale(lang) != "ko" {
		return err.Error()
	}
	switch code {
	case "SLUG_INVALID":
		return "slug는 소문자로 시작하는 2-40자 kebab-case여야 합니다."
	case "SLUG_RESERVED":
		return "이 slug는 라우팅/예약어와 충돌합니다. 프로젝트에 더 구체적인 slug를 선택하세요."
	case "SLUG_TAKEN":
		return "같은 owner 아래에 이미 같은 project slug가 있습니다. 다른 slug를 선택하세요."
	case "NAME_REQUIRED":
		return "name은 필수입니다."
	case "LANG_REQUIRED":
		return "primary_language는 필수입니다. 사용자에게 en, ko, ja 중 하나를 명시적으로 확인하세요."
	case "LANG_INVALID":
		return "primary_language는 en, ko, ja 중 하나여야 합니다."
	case "GIT_REMOTE_URL_INVALID":
		return "git_remote_url은 github.com/owner/repo 형태로 정규화 가능한 Git remote URL이어야 합니다."
	default:
		return err.Error()
	}
}

func principalUserID(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.UserID)
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

Optional git_remote_url stores the project's origin repository in
project_repos after normalizing https/ssh/SCP-style Git remote URLs to
host/owner/repo. This enables future pindoc.workspace.detect lookup by
git remote get-url origin.
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
