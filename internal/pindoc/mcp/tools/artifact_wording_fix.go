package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type artifactWordingFixInput struct {
	ProjectSlug     string          `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	SlugOrID        string          `json:"slug_or_id" jsonschema:"target artifact UUID, slug, or pindoc:// URL"`
	BodyPatch       *BodyPatchInput `json:"body_patch" jsonschema:"required; mode must be section_replace or append"`
	Reason          string          `json:"reason" jsonschema:"required, 2-200 runes; stored as commit_msg with wording_fix prefix"`
	ExpectedVersion *int            `json:"expected_version" jsonschema:"required optimistic lock; current artifact revision number"`
}

// RegisterArtifactWordingFix wires pindoc.artifact.wording_fix. It is a
// narrow shortcut over artifact.propose(update_of + body_patch) for wording
// changes that should not be treated as evidence-free canonical rewrites.
func RegisterArtifactWordingFix(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.artifact.wording_fix",
			Description: strings.TrimSpace(`
Apply a narrow wording fix to an existing artifact without resending the full body. Accepts body_patch.mode=section_replace or append only, requires expected_version, and bypasses search_receipt like other operational shortcut tools. commit_msg is stored as "wording_fix: {reason}". Use ordinary pindoc.artifact.propose(update_of=...) when the body meaning or a canonical Decision/Debug/Analysis claim changes.
`),
		},
		func(ctx context.Context, p *auth.Principal, in artifactWordingFixInput) (*sdk.CallToolResult, artifactProposeOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("artifact.wording_fix: %w", err)
			}
			if strings.TrimSpace(in.SlugOrID) == "" {
				return nil, wordingFixNotReady("WORDING_FIX_TARGET_REQUIRED", "slug_or_id is required"), nil
			}
			if in.ExpectedVersion == nil {
				return nil, wordingFixNotReady("WORDING_FIX_EXPECTED_VERSION_REQUIRED", "expected_version is required"), nil
			}
			reason := strings.TrimSpace(in.Reason)
			if utf8.RuneCountInString(reason) < reasonMinLen || utf8.RuneCountInString(reason) > reasonMaxLen {
				return nil, wordingFixNotReady("WORDING_FIX_REASON_INVALID", "reason must be 2-200 characters"), nil
			}
			if in.BodyPatch == nil {
				return nil, wordingFixNotReady("WORDING_FIX_PATCH_REQUIRED", "body_patch is required"), nil
			}
			mode := strings.TrimSpace(in.BodyPatch.Mode)
			if mode != "section_replace" && mode != "append" {
				return nil, wordingFixNotReady("WORDING_FIX_MODE_REJECTED", "body_patch.mode must be section_replace or append for wording_fix"), nil
			}

			title, artifactType, areaSlug, err := wordingFixTarget(ctx, deps, scope.ProjectSlug, in.SlugOrID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, wordingFixNotReady("WORDING_FIX_TARGET_NOT_FOUND", fmt.Sprintf("artifact %q not found", in.SlugOrID)), nil
				}
				return nil, artifactProposeOutput{}, err
			}
			authorID := stripAgentPrefix(taskAttentionCallerID(p))
			if authorID == "" {
				authorID = "codex"
			}
			_, out, err := handleUpdate(ctx, deps, p, scope, artifactProposeInput{
				ProjectSlug:     scope.ProjectSlug,
				Type:            artifactType,
				AreaSlug:        areaSlug,
				Title:           title,
				AuthorID:        authorID,
				UpdateOf:        in.SlugOrID,
				CommitMsg:       "wording_fix: " + reason,
				ExpectedVersion: in.ExpectedVersion,
				Shape:           string(ShapeBodyPatch),
				BodyPatch:       in.BodyPatch,
				WordingFix:      true,
			}, deps.UserLanguage)
			if err != nil || out.Status != "accepted" {
				return nil, out, err
			}
			out.Warnings = append(out.Warnings, "WORDING_FIX_APPLIED")
			out.WarningSeverities = append(out.WarningSeverities, SeverityInfo)
			return nil, out, nil
		},
	)
}

func wordingFixNotReady(code, msg string) artifactProposeOutput {
	return artifactProposeOutput{
		Status:          "not_ready",
		ErrorCode:       code,
		Failed:          []string{code},
		ErrorCodes:      []string{code},
		Checklist:       []string{msg},
		PatchableFields: []string{"slug_or_id", "body_patch", "reason", "expected_version"},
	}
}

func wordingFixTarget(ctx context.Context, deps Deps, projectSlug, slugOrID string) (title, artifactType, areaSlug string, err error) {
	ref := normalizeRef(slugOrID)
	err = deps.DB.QueryRow(ctx, `
		SELECT a.title, a.type, ar.slug
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		  JOIN areas ar ON ar.id = a.area_id
		 WHERE p.slug = $1
		   AND (a.id::text = $2 OR a.slug = $2)
		   AND a.status <> 'archived'
		 LIMIT 1
	`, projectSlug, ref).Scan(&title, &artifactType, &areaSlug)
	return title, artifactType, areaSlug, err
}
