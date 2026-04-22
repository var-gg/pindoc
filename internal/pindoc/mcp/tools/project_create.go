package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type projectCreateInput struct {
	Slug            string `json:"slug" jsonschema:"lowercase kebab-case slug, 2-40 chars, unique within this Pindoc instance"`
	Name            string `json:"name" jsonschema:"human-readable display name"`
	PrimaryLanguage string `json:"primary_language" jsonschema:"en | ko (M1 support); other languages land in V1.5"`
	Description     string `json:"description,omitempty" jsonschema:"one-line description shown on the project switcher; optional"`
	Color           string `json:"color,omitempty" jsonschema:"CSS color string (hex or oklch) used for the sidebar accent; optional"`
}

type projectCreateOutput struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	URL         string `json:"url" jsonschema:"canonical UI path to the project's wiki — share this, not /wiki/..."`
	DefaultArea string `json:"default_area" jsonschema:"slug of the 'misc' area auto-created so artifacts can be filed immediately"`
	Message     string `json:"message"`
}

// projectSlugRe enforces the URL-safe shape we promise downstream routes.
// Leading letter keeps the slug from parsing as a number; length caps at 40
// so the /p/{project}/... URL stays readable in shares.
var projectSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

// RegisterProjectCreate wires pindoc.project.create. Creates a new project
// and seeds it with a single 'misc' area so the first artifact has somewhere
// to land. No UI button calls this — per architecture principle 1 (agent-only
// write surface), the user asks the agent and the agent calls this tool.
func RegisterProjectCreate(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name: "pindoc.project.create",
			Description: strings.TrimSpace(`
Create a new Pindoc project. Returns the canonical /p/{slug}/wiki URL
the user should bookmark. Auto-creates a 'misc' area so artifacts can
be filed immediately.

When to call: user says "start a new project for X" or asks to split
docs for a repo that isn't covered by the current Pindoc instance.
Pick a kebab-case slug tied to the repo or product name.

This tool does not switch the MCP session's active project. The session
scope stays tied to PINDOC_PROJECT env; a future session starts under
the new project by launching pindoc-server with PINDOC_PROJECT=<new>.
`),
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in projectCreateInput) (*sdk.CallToolResult, projectCreateOutput, error) {
			slug := strings.ToLower(strings.TrimSpace(in.Slug))
			name := strings.TrimSpace(in.Name)
			lang := strings.ToLower(strings.TrimSpace(in.PrimaryLanguage))

			if !projectSlugRe.MatchString(slug) {
				return nil, projectCreateOutput{}, fmt.Errorf("slug must be lowercase kebab-case (2-40 chars, starts with a letter): got %q", in.Slug)
			}
			if name == "" {
				return nil, projectCreateOutput{}, fmt.Errorf("name is required")
			}
			if lang != "en" && lang != "ko" {
				return nil, projectCreateOutput{}, fmt.Errorf("primary_language must be 'en' or 'ko' in M1 (others land in V1.5); got %q", in.PrimaryLanguage)
			}

			var descPtr, colorPtr *string
			if desc := strings.TrimSpace(in.Description); desc != "" {
				descPtr = &desc
			}
			if color := strings.TrimSpace(in.Color); color != "" {
				colorPtr = &color
			}

			tx, err := deps.DB.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				return nil, projectCreateOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			var projectID string
			err = tx.QueryRow(ctx, `
				INSERT INTO projects (slug, name, description, color, primary_language)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id::text
			`, slug, name, descPtr, colorPtr, lang).Scan(&projectID)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return nil, projectCreateOutput{}, fmt.Errorf("project slug %q already exists — pick a different slug", slug)
				}
				return nil, projectCreateOutput{}, fmt.Errorf("project insert: %w", err)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO areas (project_id, slug, name, description, is_cross_cutting)
				VALUES
				  ($1::uuid, 'misc', 'Miscellaneous', 'Catch-all area for artifacts without a better home. Promote to a real area via pindoc.area.propose once a pattern emerges.', false),
				  ($1::uuid, '_unsorted', '_Unsorted', 'Quarantine queue — artifacts the agent couldn''t classify. Reader UI surfaces them for reclassification.', false)
			`, projectID)
			if err != nil {
				return nil, projectCreateOutput{}, fmt.Errorf("seed default areas: %w", err)
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, projectCreateOutput{}, fmt.Errorf("commit: %w", err)
			}

			deps.Logger.Info("project created", "slug", slug, "name", name, "lang", lang)

			return nil, projectCreateOutput{
				ID:          projectID,
				Slug:        slug,
				Name:        name,
				URL:         fmt.Sprintf("/p/%s/wiki", slug),
				DefaultArea: "misc",
				Message: strings.TrimSpace(fmt.Sprintf(`
Project %q created. Share this URL with the user: /p/%s/wiki
Note: this MCP session is still scoped to the old project — to write
artifacts into %q, restart pindoc-server with PINDOC_PROJECT=%s.
`, slug, slug, slug, slug)),
			}, nil
		},
	)
}

