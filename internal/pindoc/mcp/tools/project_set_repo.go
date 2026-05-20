package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type projectSetRepoInput struct {
	ProjectSlug   string   `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call; omitted uses session/default resolver"`
	GitRemoteURL  string   `json:"git_remote_url" jsonschema:"required; canonical git remote URL (https or scp-style ssh accepted; server normalizes to host/owner/repo)"`
	LocalPaths    []string `json:"local_paths,omitempty" jsonschema:"optional absolute workspace paths to associate; merged with any existing rows"`
	Name          string   `json:"name,omitempty" jsonschema:"optional remote name (defaults to 'origin')"`
	DefaultBranch string   `json:"default_branch,omitempty" jsonschema:"optional default branch (defaults to 'main')"`
}

type projectSetRepoOutput struct {
	Status               string   `json:"status"`
	Code                 string   `json:"code,omitempty"`
	ErrorCode            string   `json:"error_code,omitempty"`
	Failed               []string `json:"failed,omitempty"`
	ProjectID            string   `json:"project_id,omitempty"`
	ProjectSlug          string   `json:"project_slug,omitempty"`
	RepoID               string   `json:"repo_id,omitempty"`
	Created              bool     `json:"created"`
	GitRemoteURL         string   `json:"git_remote_url,omitempty"`
	GitRemoteURLOriginal string   `json:"git_remote_url_original,omitempty"`
	Name                 string   `json:"name,omitempty"`
	DefaultBranch        string   `json:"default_branch,omitempty"`
	LocalPaths           []string `json:"local_paths,omitempty"`
	URLs                 []string `json:"urls,omitempty"`
	ToolsetVersion       string   `json:"toolset_version,omitempty"`
}

func RegisterProjectSetRepo(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.project.set_repo",
			Description: strings.TrimSpace(`
Register or refresh a git_remote_url + local_paths mapping on an existing
project (project_repos upsert). Owner-only. Closes the workspace.detect
no-row / pin.add_pin PIN_REPO_NOT_REGISTERED gap without raw SQL: call once
per workspace checkout to seed the row, then add_pin's repo_id auto-mapping
resolves cleanly. Idempotent on (project_id, git_remote_url); local_paths
and urls merge with the existing row.
`),
		},
		func(ctx context.Context, p *auth.Principal, in projectSetRepoInput) (*sdk.CallToolResult, projectSetRepoOutput, error) {
			rawRemote := strings.TrimSpace(in.GitRemoteURL)
			if rawRemote == "" {
				return nil, projectSetRepoOutput{
					Status:    "not_ready",
					ErrorCode: "GIT_REMOTE_URL_REQUIRED",
					Failed:    []string{"GIT_REMOTE_URL_REQUIRED"},
				}, nil
			}
			normalized, err := projects.NormalizeGitRemoteURL(rawRemote)
			if err != nil {
				return nil, projectSetRepoOutput{
					Status:    "not_ready",
					ErrorCode: "GIT_REMOTE_URL_INVALID",
					Failed:    []string{"GIT_REMOTE_URL_INVALID"},
				}, nil
			}

			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, projectSetRepoOutput{}, fmt.Errorf("project.set_repo: %w", err)
			}
			if !scope.Can("write.project") {
				return nil, projectSetRepoOutput{
					Status:    "not_ready",
					ErrorCode: "PROJECT_OWNER_REQUIRED",
					Failed:    []string{"PROJECT_OWNER_REQUIRED"},
				}, nil
			}

			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, projectSetRepoOutput{}, fmt.Errorf("project.set_repo begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			repoID, created, err := projects.UpsertProjectRepo(ctx, tx, projects.ProjectRepoInput{
				ProjectID:     scope.ProjectID,
				GitRemoteURL:  rawRemote,
				Name:          in.Name,
				DefaultBranch: in.DefaultBranch,
				LocalPaths:    in.LocalPaths,
			})
			if err != nil {
				if errors.Is(err, projects.ErrGitRemoteURLInvalid) {
					return nil, projectSetRepoOutput{
						Status:    "not_ready",
						ErrorCode: "GIT_REMOTE_URL_INVALID",
						Failed:    []string{"GIT_REMOTE_URL_INVALID"},
					}, nil
				}
				return nil, projectSetRepoOutput{}, fmt.Errorf("project.set_repo upsert: %w", err)
			}

			var (
				gotName, gotBranch, gotOriginal string
				localPaths, urls                []string
			)
			// The row was just upserted in this same transaction, so a
			// no-rows result is a data-integrity violation, not a normal
			// path — surface it instead of emitting an event + response
			// built from zero values.
			if err := tx.QueryRow(ctx, `
				SELECT name, default_branch, git_remote_url_original, local_paths, urls
				  FROM project_repos
				 WHERE id = $1::uuid
			`, repoID).Scan(&gotName, &gotBranch, &gotOriginal, &localPaths, &urls); err != nil {
				return nil, projectSetRepoOutput{}, fmt.Errorf("project.set_repo read-back: %w", err)
			}

			actorID := "pindoc.project.set_repo"
			if p != nil && strings.TrimSpace(p.AgentID) != "" {
				actorID = strings.TrimSpace(p.AgentID)
			}
			actorUserID := principalUserID(p)
			eventKind := "project.repo_registered"
			if !created {
				eventKind = "project.repo_refreshed"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1, $2, $3::uuid, jsonb_build_object(
					'git_remote_url',          $4::text,
					'git_remote_url_original', $5::text,
					'local_paths',             $6::text[],
					'actor_user_id',           NULLIF($7, '')::uuid,
					'actor_id',                $8::text,
					'origin',                  'mcp_project_set_repo'
				))
			`, scope.ProjectID, eventKind, repoID, normalized, gotOriginal, localPaths, actorUserID, actorID); err != nil {
				return nil, projectSetRepoOutput{}, fmt.Errorf("project.set_repo event: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, projectSetRepoOutput{}, fmt.Errorf("project.set_repo commit: %w", err)
			}

			code := "PROJECT_REPO_REGISTERED"
			if !created {
				code = "PROJECT_REPO_REFRESHED"
			}
			return nil, projectSetRepoOutput{
				Status:               "ok",
				Code:                 code,
				ProjectID:            scope.ProjectID,
				ProjectSlug:          scope.ProjectSlug,
				RepoID:               repoID,
				Created:              created,
				GitRemoteURL:         normalized,
				GitRemoteURLOriginal: gotOriginal,
				Name:                 gotName,
				DefaultBranch:        gotBranch,
				LocalPaths:           localPaths,
				URLs:                 urls,
			}, nil
		},
	)
}
