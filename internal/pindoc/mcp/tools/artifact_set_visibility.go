package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type artifactSetVisibilityInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call; omitted uses session/default resolver"`

	// Single-target mode: set visibility on exactly one artifact.
	SlugOrID string `json:"slug_or_id,omitempty" jsonschema:"artifact UUID, slug, or pindoc:// URL — single-target mode"`

	// Bulk mode: change every artifact under the resolved project (and
	// optionally a specific area). Requires confirm=true to actually
	// write — without confirm, the tool returns the would-affect count
	// for review. Use this for one-shot migrations like "flip all
	// existing dogfood artifacts to public when wiring up the OSS sample
	// page". Mutually exclusive with slug_or_id.
	BulkAllInProject bool   `json:"bulk_all_in_project,omitempty" jsonschema:"if true, target every artifact in the resolved project; mutually exclusive with slug_or_id"`
	AreaSlug         string `json:"area_slug,omitempty" jsonschema:"narrow bulk mode to a single area (slug)"`
	Confirm          bool   `json:"confirm,omitempty" jsonschema:"required to actually apply bulk writes; without it the tool reports the would-affect count and exits"`

	// Visibility tier to apply. Required.
	Visibility string `json:"visibility" jsonschema:"required; one of public|org|private"`
}

type artifactSetVisibilityOutput struct {
	Status         string   `json:"status"`
	Code           string   `json:"code,omitempty"`
	ErrorCode      string   `json:"error_code,omitempty"`
	Failed         []string `json:"failed,omitempty"`
	Mode           string   `json:"mode,omitempty"`           // "single" | "bulk"
	Visibility     string   `json:"visibility,omitempty"`     // resolved tier applied
	Affected       int      `json:"affected,omitempty"`       // rows actually written (0 in dry-run bulk)
	WouldAffect    int      `json:"would_affect,omitempty"`   // bulk dry-run count
	ConfirmHint    string   `json:"confirm_hint,omitempty"`   // shown on bulk dry-run
	ToolsetVersion string   `json:"toolset_version,omitempty"`
}

// RegisterArtifactSetVisibility wires pindoc.artifact.set_visibility. The
// tool changes visibility on one or many artifacts already in pindoc.
// Cascade rule from artifact.propose doesn't apply here — this is the
// strict-validation route that requires explicit user intent. Two modes:
//
//   - single (slug_or_id): exactly one artifact, returns affected=1
//   - bulk_all_in_project: every artifact in the project (optionally
//     filtered by area_slug). The first call without confirm=true
//     returns would_affect= for review; the operator re-runs with
//     confirm=true to actually apply.
//
// Bulk mode exists for the one-shot "flip my dogfood project to public"
// migration. Single mode is the everyday path for marking individual
// SaaS-strategy artifacts as private.
func RegisterArtifactSetVisibility(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.artifact.set_visibility",
			Description: strings.TrimSpace(`
Change artifact visibility tier (public|org|private). Single-target via slug_or_id, or bulk via bulk_all_in_project (with optional area_slug). Bulk mode requires confirm=true to write — without it the tool returns the would-affect count for review. Use when (a) marking a sensitive artifact (SaaS strategy, pricing memo, hiring notes) as private after the fact, or (b) one-shot flipping a dogfood project's existing artifacts to public for the OSS sample page. Validation is strict here unlike artifact.propose's cascade — invalid visibility values reject.
`),
		},
		func(ctx context.Context, p *auth.Principal, in artifactSetVisibilityInput) (*sdk.CallToolResult, artifactSetVisibilityOutput, error) {
			tier := normalizeVisibility(in.Visibility)
			if tier == "" {
				return nil, artifactSetVisibilityOutput{
					Status:    "not_ready",
					ErrorCode: "VISIBILITY_INVALID",
					Failed:    []string{"VISIBILITY_INVALID"},
				}, nil
			}

			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactSetVisibilityOutput{}, fmt.Errorf("artifact.set_visibility: %w", err)
			}
			if !scope.Can("write.project") {
				return nil, artifactSetVisibilityOutput{
					Status:    "not_ready",
					ErrorCode: "PROJECT_OWNER_REQUIRED",
					Failed:    []string{"PROJECT_OWNER_REQUIRED"},
				}, nil
			}

			singleTarget := strings.TrimSpace(in.SlugOrID)
			if singleTarget != "" && in.BulkAllInProject {
				return nil, artifactSetVisibilityOutput{
					Status:    "not_ready",
					ErrorCode: "MODE_AMBIGUOUS",
					Failed:    []string{"MODE_AMBIGUOUS"},
				}, nil
			}
			if singleTarget == "" && !in.BulkAllInProject {
				return nil, artifactSetVisibilityOutput{
					Status:    "not_ready",
					ErrorCode: "TARGET_REQUIRED",
					Failed:    []string{"TARGET_REQUIRED"},
				}, nil
			}

			if singleTarget != "" {
				return setVisibilitySingle(ctx, deps, scope.ProjectID, singleTarget, tier)
			}
			return setVisibilityBulk(ctx, deps, scope.ProjectID, in.AreaSlug, tier, in.Confirm)
		},
	)
}

// setVisibilitySingle resolves the slug_or_id to one artifact in the
// project and updates its visibility. Returns affected=1 on success
// or NOT_FOUND when the lookup misses.
func setVisibilitySingle(ctx context.Context, deps Deps, projectID, target, tier string) (*sdk.CallToolResult, artifactSetVisibilityOutput, error) {
	target = stripPindocURL(target)
	tag, err := deps.DB.Exec(ctx, `
		UPDATE artifacts
		   SET visibility = $1, updated_at = now()
		 WHERE project_id = $2::uuid
		   AND (id::text = $3 OR slug = $3)
	`, tier, projectID, target)
	if err != nil {
		return nil, artifactSetVisibilityOutput{}, fmt.Errorf("set_visibility update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, artifactSetVisibilityOutput{
			Status:    "not_ready",
			ErrorCode: "ARTIFACT_NOT_FOUND",
			Failed:    []string{"ARTIFACT_NOT_FOUND"},
			Mode:      "single",
		}, nil
	}
	return nil, artifactSetVisibilityOutput{
		Status:     "ok",
		Code:       "VISIBILITY_UPDATED",
		Mode:       "single",
		Visibility: tier,
		Affected:   int(tag.RowsAffected()),
	}, nil
}

// setVisibilityBulk counts the would-affect rows on a dry-run (confirm
// false) and applies the UPDATE on confirm=true. Area-scoped if
// areaSlug is set; otherwise the whole project.
func setVisibilityBulk(ctx context.Context, deps Deps, projectID, areaSlug, tier string, confirm bool) (*sdk.CallToolResult, artifactSetVisibilityOutput, error) {
	areaSlug = strings.TrimSpace(areaSlug)
	var areaID string
	if areaSlug != "" {
		err := deps.DB.QueryRow(ctx, `
			SELECT id::text FROM areas
			 WHERE project_id = $1::uuid AND slug = $2
			 LIMIT 1
		`, projectID, areaSlug).Scan(&areaID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, artifactSetVisibilityOutput{
				Status:    "not_ready",
				ErrorCode: "AREA_UNKNOWN",
				Failed:    []string{"AREA_UNKNOWN"},
				Mode:      "bulk",
			}, nil
		}
		if err != nil {
			return nil, artifactSetVisibilityOutput{}, fmt.Errorf("resolve bulk area: %w", err)
		}
	}

	// Skip rows that already match the target tier so the affected count
	// reflects actual changes, not no-op writes.
	countQuery := `
		SELECT count(*) FROM artifacts
		 WHERE project_id = $1::uuid
		   AND visibility <> $2
	`
	args := []any{projectID, tier}
	if areaID != "" {
		countQuery += " AND area_id = $3::uuid"
		args = append(args, areaID)
	}
	var would int
	if err := deps.DB.QueryRow(ctx, countQuery, args...).Scan(&would); err != nil {
		return nil, artifactSetVisibilityOutput{}, fmt.Errorf("count bulk targets: %w", err)
	}

	if !confirm {
		hint := fmt.Sprintf(
			"would set visibility=%q on %d artifact(s). Re-call with confirm=true to apply.",
			tier, would)
		return nil, artifactSetVisibilityOutput{
			Status:      "informational",
			Code:        "BULK_DRY_RUN",
			Mode:        "bulk",
			Visibility:  tier,
			WouldAffect: would,
			ConfirmHint: hint,
		}, nil
	}

	updateQuery := `
		UPDATE artifacts
		   SET visibility = $2, updated_at = now()
		 WHERE project_id = $1::uuid
		   AND visibility <> $2
	`
	updateArgs := []any{projectID, tier}
	if areaID != "" {
		updateQuery += " AND area_id = $3::uuid"
		updateArgs = append(updateArgs, areaID)
	}
	tag, err := deps.DB.Exec(ctx, updateQuery, updateArgs...)
	if err != nil {
		return nil, artifactSetVisibilityOutput{}, fmt.Errorf("bulk set_visibility update: %w", err)
	}
	return nil, artifactSetVisibilityOutput{
		Status:     "ok",
		Code:       "VISIBILITY_UPDATED",
		Mode:       "bulk",
		Visibility: tier,
		Affected:   int(tag.RowsAffected()),
	}, nil
}

// stripPindocURL trims the pindoc:// scheme + leading project slug from
// inputs that came in as a fully-qualified URL. The bare slug or UUID
// stays untouched.
func stripPindocURL(in string) string {
	in = strings.TrimSpace(in)
	const prefix = "pindoc://"
	if !strings.HasPrefix(in, prefix) {
		return in
	}
	rest := strings.TrimPrefix(in, prefix)
	// pindoc://{project}/{slug-or-id} or pindoc://{slug-or-id}; take the
	// last path segment so both forms produce the same lookup key.
	if idx := strings.LastIndex(rest, "/"); idx >= 0 {
		return rest[idx+1:]
	}
	return rest
}
