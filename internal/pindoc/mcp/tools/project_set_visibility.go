package tools

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type projectSetVisibilityInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call; omitted uses session/default resolver"`
	Visibility  string `json:"visibility" jsonschema:"required; one of public|org|private"`
}

type projectSetVisibilityOutput struct {
	Status         string   `json:"status"`
	Code           string   `json:"code,omitempty"`
	ErrorCode      string   `json:"error_code,omitempty"`
	Failed         []string `json:"failed,omitempty"`
	ProjectID      string   `json:"project_id,omitempty"`
	ProjectSlug    string   `json:"project_slug,omitempty"`
	Visibility     string   `json:"visibility,omitempty"`
	Affected       int      `json:"affected"`
	ToolsetVersion string   `json:"toolset_version,omitempty"`
}

func RegisterProjectSetVisibility(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.project.set_visibility",
			Description: strings.TrimSpace(`
Change a project's visibility tier (public|org|private). Owner-only.
Use this for OSS public-demo setup so projects.visibility can be changed
without raw SQL. No-op calls return status=informational and affected=0.
`),
		},
		func(ctx context.Context, p *auth.Principal, in projectSetVisibilityInput) (*sdk.CallToolResult, projectSetVisibilityOutput, error) {
			tier := projects.NormalizeVisibility(in.Visibility)
			if tier == "" {
				return nil, projectSetVisibilityOutput{
					Status:    "not_ready",
					ErrorCode: "VISIBILITY_INVALID",
					Failed:    []string{"VISIBILITY_INVALID"},
				}, nil
			}

			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, projectSetVisibilityOutput{}, fmt.Errorf("project.set_visibility: %w", err)
			}
			if !scope.Can("write.project") {
				return nil, projectSetVisibilityOutput{
					Status:    "not_ready",
					ErrorCode: "PROJECT_OWNER_REQUIRED",
					Failed:    []string{"PROJECT_OWNER_REQUIRED"},
				}, nil
			}

			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, projectSetVisibilityOutput{}, fmt.Errorf("project.set_visibility begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			var currentVisibility string
			if err := tx.QueryRow(ctx, `
				SELECT visibility
				  FROM projects
				 WHERE id = $1::uuid
				 FOR UPDATE
			`, scope.ProjectID).Scan(&currentVisibility); err != nil {
				return nil, projectSetVisibilityOutput{}, fmt.Errorf("project.set_visibility lookup: %w", err)
			}
			if currentVisibility == tier {
				return nil, projectSetVisibilityOutput{
					Status:      "informational",
					Code:        "PROJECT_VISIBILITY_NO_OP",
					ProjectID:   scope.ProjectID,
					ProjectSlug: scope.ProjectSlug,
					Visibility:  tier,
					Affected:    0,
				}, nil
			}

			tag, err := tx.Exec(ctx, `
				UPDATE projects
				   SET visibility = $2,
				       updated_at = now()
				 WHERE id = $1::uuid
				   AND visibility <> $2
			`, scope.ProjectID, tier)
			if err != nil {
				return nil, projectSetVisibilityOutput{}, fmt.Errorf("project.set_visibility update: %w", err)
			}
			actorID := "pindoc.project.set_visibility"
			if p != nil && strings.TrimSpace(p.AgentID) != "" {
				actorID = strings.TrimSpace(p.AgentID)
			}
			actorUserID := principalUserID(p)
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1, 'project.visibility_changed', $1, jsonb_build_object(
					'from',          $2::text,
					'to',            $3::text,
					'actor_user_id', NULLIF($4, '')::uuid,
					'actor_id',      $5::text,
					'origin',        'mcp_project_set_visibility'
				))
			`, scope.ProjectID, currentVisibility, tier, actorUserID, actorID); err != nil {
				return nil, projectSetVisibilityOutput{}, fmt.Errorf("project.set_visibility event: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, projectSetVisibilityOutput{}, fmt.Errorf("project.set_visibility commit: %w", err)
			}
			return nil, projectSetVisibilityOutput{
				Status:      "ok",
				Code:        "PROJECT_VISIBILITY_UPDATED",
				ProjectID:   scope.ProjectID,
				ProjectSlug: scope.ProjectSlug,
				Visibility:  tier,
				Affected:    int(tag.RowsAffected()),
			}, nil
		},
	)
}
