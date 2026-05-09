package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type artifactSetAreaInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call; omitted uses session/default resolver"`

	// Single-target mode: set area on exactly one artifact.
	SlugOrID        string `json:"slug_or_id,omitempty" jsonschema:"artifact UUID, slug, or pindoc:// URL — single-target mode"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"optional optimistic lock; current artifact revision number"`

	// Bulk mode: move every active artifact under the resolved project,
	// optionally from one source area. Requires confirm=true to actually
	// write; without confirm the tool returns a would-affect count.
	BulkAllInProject bool   `json:"bulk_all_in_project,omitempty" jsonschema:"if true, target active artifacts in the resolved project; mutually exclusive with slug_or_id"`
	FromAreaSlug     string `json:"from_area_slug,omitempty" jsonschema:"optional source area slug filter for bulk mode"`
	Confirm          bool   `json:"confirm,omitempty" jsonschema:"required to actually apply bulk writes; without it the tool reports the would-affect count and exits"`

	// Target area to apply. Required.
	AreaSlug string `json:"area_slug" jsonschema:"required target area slug from pindoc.area.list"`

	// Reason is stored in each emitted revision as "set_area: {reason}".
	Reason   string `json:"reason" jsonschema:"required; stored as commit_msg with set_area prefix"`
	AuthorID string `json:"author_id,omitempty" jsonschema:"optional author display label; defaults to server agent_id"`
}

type artifactSetAreaOutput struct {
	Status         string   `json:"status"`
	Code           string   `json:"code,omitempty"`
	ErrorCode      string   `json:"error_code,omitempty"`
	Failed         []string `json:"failed,omitempty"`
	Mode           string   `json:"mode,omitempty"`            // "single" | "bulk"
	AreaSlug       string   `json:"area_slug,omitempty"`       // target area slug
	FromAreaSlug   string   `json:"from_area_slug,omitempty"`  // old/source area slug
	Affected       int      `json:"affected"`                  // rows actually written (0 for dry-run/no-op)
	WouldAffect    int      `json:"would_affect,omitempty"`    // bulk dry-run count
	RevisionNumber int      `json:"revision_number,omitempty"` // single mode revision emitted on actual change
	ConfirmHint    string   `json:"confirm_hint,omitempty"`    // shown on bulk dry-run
	ToolsetVersion string   `json:"toolset_version,omitempty"`
}

type setAreaQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type setAreaInfo struct {
	ID            string
	Slug          string
	ParentID      string
	ParentSlug    string
	GrandparentID string
}

type areaArtifact struct {
	ID           string
	ProjectID    string
	Slug         string
	AreaID       string
	AreaSlug     string
	Status       string
	Title        string
	Body         string
	Tags         []string
	Completeness string
	LastRev      int
}

// RegisterArtifactSetArea wires pindoc.artifact.set_area. It mirrors
// set_visibility's operational lane for taxonomy moves: one artifact via
// slug_or_id, or bulk migration under a project with confirm=true.
func RegisterArtifactSetArea(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.artifact.set_area",
			Description: strings.TrimSpace(`
Move artifacts between Areas / artifact area 재분류 도구. Use single-target mode with slug_or_id + area_slug, or bulk mode with bulk_all_in_project=true + optional from_area_slug + area_slug. Bulk mode requires confirm=true to write; confirm=false returns would_affect for review. This is the only supported area-move lane: artifact.propose update paths preserve area and warn when a different area_slug is supplied. Target area must already exist in the project, top-level taxonomy areas are protected except misc and _unsorted, and only active artifacts (published/stale) are moved.
`),
		},
		func(ctx context.Context, p *auth.Principal, in artifactSetAreaInput) (*sdk.CallToolResult, artifactSetAreaOutput, error) {
			targetAreaSlug := strings.TrimSpace(in.AreaSlug)
			if targetAreaSlug == "" {
				return nil, artifactSetAreaNotReady("AREA_REQUIRED", "", ""), nil
			}
			reason := strings.TrimSpace(in.Reason)
			if reason == "" {
				return nil, artifactSetAreaNotReady("REASON_REQUIRED", "", targetAreaSlug), nil
			}

			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactSetAreaOutput{}, fmt.Errorf("artifact.set_area: %w", err)
			}
			if !scope.Can("write.project") {
				return nil, artifactSetAreaOutput{
					Status:    "not_ready",
					ErrorCode: "PROJECT_OWNER_REQUIRED",
					Failed:    []string{"PROJECT_OWNER_REQUIRED"},
					AreaSlug:  targetAreaSlug,
				}, nil
			}

			singleTarget := strings.TrimSpace(in.SlugOrID)
			if singleTarget != "" && in.BulkAllInProject {
				return nil, artifactSetAreaNotReady("MODE_AMBIGUOUS", "", targetAreaSlug), nil
			}
			if singleTarget == "" && !in.BulkAllInProject {
				return nil, artifactSetAreaNotReady("TARGET_REQUIRED", "", targetAreaSlug), nil
			}

			if singleTarget != "" {
				return setAreaSingle(ctx, deps, p, scope.ProjectID, singleTarget, targetAreaSlug, reason, strings.TrimSpace(in.AuthorID), in.ExpectedVersion)
			}
			return setAreaBulk(ctx, deps, p, scope.ProjectID, strings.TrimSpace(in.FromAreaSlug), targetAreaSlug, reason, strings.TrimSpace(in.AuthorID), in.Confirm)
		},
	)
}

func setAreaSingle(ctx context.Context, deps Deps, p *auth.Principal, projectID, target, targetAreaSlug, reason, explicitAuthorID string, expectedVersion *int) (*sdk.CallToolResult, artifactSetAreaOutput, error) {
	target = stripPindocURL(target)
	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("set_area begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	targetArea, targetErr := resolveSetAreaTarget(ctx, tx, projectID, targetAreaSlug)
	if targetErr != "" {
		return nil, artifactSetAreaNotReady(targetErr, "single", targetAreaSlug), nil
	}

	artifact, err := lockAreaArtifact(ctx, tx, projectID, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, artifactSetAreaNotReady("ARTIFACT_NOT_FOUND", "single", targetAreaSlug), nil
	}
	if err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("set_area lookup: %w", err)
	}
	if !artifactAreaStatusIsActive(artifact.Status) {
		return nil, artifactSetAreaNotReady("ARTIFACT_STATUS_NOT_ACTIVE", "single", targetAreaSlug), nil
	}
	if expectedVersion != nil && *expectedVersion != artifact.LastRev {
		out := artifactSetAreaNotReady("VER_CONFLICT", "single", targetAreaSlug)
		out.FromAreaSlug = artifact.AreaSlug
		out.RevisionNumber = artifact.LastRev
		return nil, out, nil
	}
	if artifact.AreaID == targetArea.ID {
		if err := tx.Commit(ctx); err != nil {
			return nil, artifactSetAreaOutput{}, fmt.Errorf("set_area unchanged commit: %w", err)
		}
		return nil, artifactSetAreaOutput{
			Status:       "not_ready",
			ErrorCode:    "AREA_UNCHANGED",
			Failed:       []string{"AREA_UNCHANGED"},
			Mode:         "single",
			AreaSlug:     targetArea.Slug,
			FromAreaSlug: artifact.AreaSlug,
			Affected:     0,
		}, nil
	}

	newRev, err := recordAreaChange(ctx, tx, p, artifact, targetArea, reason, explicitAuthorID, "mcp_artifact_set_area")
	if err != nil {
		return nil, artifactSetAreaOutput{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("set_area commit: %w", err)
	}
	return nil, artifactSetAreaOutput{
		Status:         "ok",
		Code:           "AREA_UPDATED",
		Mode:           "single",
		AreaSlug:       targetArea.Slug,
		FromAreaSlug:   artifact.AreaSlug,
		Affected:       1,
		RevisionNumber: newRev,
	}, nil
}

func setAreaBulk(ctx context.Context, deps Deps, p *auth.Principal, projectID, fromAreaSlug, targetAreaSlug, reason, explicitAuthorID string, confirm bool) (*sdk.CallToolResult, artifactSetAreaOutput, error) {
	targetArea, targetErr := resolveSetAreaTarget(ctx, deps.DB, projectID, targetAreaSlug)
	if targetErr != "" {
		return nil, artifactSetAreaNotReady(targetErr, "bulk", targetAreaSlug), nil
	}

	var fromArea setAreaInfo
	var fromAreaID string
	if fromAreaSlug != "" {
		var fromErr string
		fromArea, fromErr = resolveSetAreaExisting(ctx, deps.DB, projectID, fromAreaSlug)
		if fromErr != "" {
			return nil, artifactSetAreaNotReady(fromErr, "bulk", targetAreaSlug), nil
		}
		fromAreaID = fromArea.ID
		if fromAreaID == targetArea.ID {
			return nil, artifactSetAreaOutput{
				Status:       "not_ready",
				ErrorCode:    "AREA_UNCHANGED",
				Failed:       []string{"AREA_UNCHANGED"},
				Mode:         "bulk",
				AreaSlug:     targetArea.Slug,
				FromAreaSlug: fromArea.Slug,
				Affected:     0,
			}, nil
		}
	}

	countQuery := `
		SELECT count(*)
		  FROM artifacts
		 WHERE project_id = $1::uuid
		   AND area_id <> $2::uuid
		   AND status IN ('published', 'stale')
	`
	args := []any{projectID, targetArea.ID}
	if fromAreaID != "" {
		countQuery += " AND area_id = $3::uuid"
		args = append(args, fromAreaID)
	}
	var would int
	if err := deps.DB.QueryRow(ctx, countQuery, args...).Scan(&would); err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("count bulk set_area targets: %w", err)
	}

	if !confirm {
		hint := fmt.Sprintf(
			"would move %d active artifact(s) to area_slug=%q. Re-call with confirm=true to apply.",
			would, targetArea.Slug)
		if would >= 50 {
			hint += " Large move (50+ artifacts): verify from_area_slug and target area before confirming."
		}
		return nil, artifactSetAreaOutput{
			Status:       "informational",
			Code:         "BULK_CONFIRM_REQUIRED",
			Mode:         "bulk",
			AreaSlug:     targetArea.Slug,
			FromAreaSlug: fromArea.Slug,
			WouldAffect:  would,
			ConfirmHint:  hint,
		}, nil
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("bulk set_area begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	selectQuery := `
		SELECT a.id::text, a.project_id::text, a.slug,
		       a.area_id::text, ar.slug, a.status,
		       a.title, a.body_markdown, a.tags, a.completeness,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
		  JOIN areas ar ON ar.id = a.area_id
		 WHERE a.project_id = $1::uuid
		   AND a.area_id <> $2::uuid
		   AND a.status IN ('published', 'stale')
	`
	selectArgs := []any{projectID, targetArea.ID}
	if fromAreaID != "" {
		selectQuery += " AND a.area_id = $3::uuid"
		selectArgs = append(selectArgs, fromAreaID)
	}
	selectQuery += " ORDER BY a.slug FOR UPDATE"
	rows, err := tx.Query(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("bulk set_area select: %w", err)
	}
	defer rows.Close()

	affected := 0
	for rows.Next() {
		var artifact areaArtifact
		if err := rows.Scan(
			&artifact.ID, &artifact.ProjectID, &artifact.Slug,
			&artifact.AreaID, &artifact.AreaSlug, &artifact.Status,
			&artifact.Title, &artifact.Body, &artifact.Tags, &artifact.Completeness, &artifact.LastRev,
		); err != nil {
			return nil, artifactSetAreaOutput{}, fmt.Errorf("bulk set_area scan: %w", err)
		}
		if _, err := recordAreaChange(ctx, tx, p, artifact, targetArea, reason, explicitAuthorID, "mcp_artifact_set_area_bulk"); err != nil {
			return nil, artifactSetAreaOutput{}, err
		}
		affected++
	}
	if err := rows.Err(); err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("bulk set_area rows: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, artifactSetAreaOutput{}, fmt.Errorf("bulk set_area commit: %w", err)
	}
	return nil, artifactSetAreaOutput{
		Status:       "ok",
		Code:         "AREA_UPDATED",
		Mode:         "bulk",
		AreaSlug:     targetArea.Slug,
		FromAreaSlug: fromArea.Slug,
		Affected:     affected,
	}, nil
}

func resolveSetAreaTarget(ctx context.Context, q setAreaQuerier, projectID, slug string) (setAreaInfo, string) {
	area, code := resolveSetAreaExisting(ctx, q, projectID, slug)
	if code != "" {
		return area, code
	}
	if code := validateSetAreaTarget(area); code != "" {
		return area, code
	}
	return area, ""
}

func resolveSetAreaExisting(ctx context.Context, q setAreaQuerier, projectID, slug string) (setAreaInfo, string) {
	slug = strings.TrimSpace(slug)
	var area setAreaInfo
	var parentID, parentSlug, grandparentID *string
	err := q.QueryRow(ctx, `
		SELECT a.id::text,
		       a.slug,
		       a.parent_id::text,
		       parent.slug,
		       parent.parent_id::text
		  FROM areas a
		  LEFT JOIN areas parent ON parent.id = a.parent_id
		 WHERE a.project_id = $1::uuid
		   AND a.slug = $2
		 LIMIT 1
	`, projectID, slug).Scan(&area.ID, &area.Slug, &parentID, &parentSlug, &grandparentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return setAreaInfo{}, "AREA_NOT_FOUND"
	}
	if err != nil {
		return setAreaInfo{}, "AREA_LOOKUP_FAILED:" + err.Error()
	}
	if parentID != nil {
		area.ParentID = *parentID
	}
	if parentSlug != nil {
		area.ParentSlug = *parentSlug
	}
	if grandparentID != nil {
		area.GrandparentID = *grandparentID
	}
	return area, ""
}

func validateSetAreaTarget(area setAreaInfo) string {
	if area.ID == "" {
		return "AREA_NOT_FOUND"
	}
	if area.ParentID == "" {
		switch area.Slug {
		case "_unsorted", "misc":
			return ""
		default:
			return "AREA_TOP_LEVEL_PROTECTED"
		}
	}
	if area.GrandparentID != "" {
		return "AREA_DEPTH_VIOLATION"
	}
	return ""
}

func lockAreaArtifact(ctx context.Context, tx pgx.Tx, projectID, target string) (areaArtifact, error) {
	var artifact areaArtifact
	err := tx.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.slug,
		       a.area_id::text, ar.slug, a.status,
		       a.title, a.body_markdown, a.tags, a.completeness,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
		  JOIN areas ar ON ar.id = a.area_id
		 WHERE a.project_id = $1::uuid
		   AND (
		        a.id::text = $2 OR a.slug = $2 OR
		        a.id = (
		          SELECT asa.artifact_id
		            FROM artifact_slug_aliases asa
		           WHERE asa.project_id = $1::uuid AND asa.old_slug = $2
		           LIMIT 1
		        )
		   )
		 FOR UPDATE
	`, projectID, target).Scan(
		&artifact.ID, &artifact.ProjectID, &artifact.Slug,
		&artifact.AreaID, &artifact.AreaSlug, &artifact.Status,
		&artifact.Title, &artifact.Body, &artifact.Tags, &artifact.Completeness, &artifact.LastRev,
	)
	return artifact, err
}

func recordAreaChange(ctx context.Context, tx pgx.Tx, p *auth.Principal, artifact areaArtifact, targetArea setAreaInfo, reason, explicitAuthorID, origin string) (int, error) {
	newRev := artifact.LastRev + 1
	shapePayload, err := json.Marshal(map[string]any{
		"kind": "area_change",
		"area_slug": map[string]string{
			"from": artifact.AreaSlug,
			"to":   targetArea.Slug,
		},
		"reason": reason,
	})
	if err != nil {
		return 0, fmt.Errorf("marshal area shape payload: %w", err)
	}

	authorID := strings.TrimSpace(explicitAuthorID)
	if authorID == "" {
		authorID = "pindoc.artifact.set_area"
		if p != nil && strings.TrimSpace(p.AgentID) != "" {
			authorID = strings.TrimSpace(p.AgentID)
		}
	}
	authorUserID := principalUserID(p)
	commitMsg := "set_area: " + strings.TrimSpace(reason)
	sourceSession := buildSourceSessionRef(p, artifactProposeInput{AuthorID: authorID, CommitMsg: commitMsg})
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			author_user_id, commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, NULL, $4, $5, $6, 'agent', $7, NULL, NULLIF($8, '')::uuid, $9, $10, 'meta_patch', $11::jsonb)
	`, artifact.ID, newRev, artifact.Title, bodyHash(artifact.Body), artifact.Tags, artifact.Completeness,
		authorID, authorUserID, commitMsg, sourceSession, string(shapePayload),
	); err != nil {
		return 0, fmt.Errorf("insert area revision: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE artifacts
		   SET area_id = $2::uuid,
		       updated_at = now()
		 WHERE id = $1::uuid
	`, artifact.ID, targetArea.ID); err != nil {
		return 0, fmt.Errorf("update artifact area: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.area_changed', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'author_user_id',  NULLIF($6, '')::uuid,
			'from_area_slug',  $7::text,
			'to_area_slug',    $8::text,
			'origin',          $9::text,
			'reason',          $10::text
		))
	`, artifact.ProjectID, artifact.ID, newRev, artifact.Slug, authorID, authorUserID,
		artifact.AreaSlug, targetArea.Slug, origin, reason); err != nil {
		return 0, fmt.Errorf("insert area event: %w", err)
	}
	return newRev, nil
}

func artifactSetAreaNotReady(code, mode, areaSlug string) artifactSetAreaOutput {
	out := artifactSetAreaOutput{
		Status:    "not_ready",
		ErrorCode: code,
		Failed:    []string{code},
		Mode:      mode,
		AreaSlug:  areaSlug,
	}
	if strings.HasPrefix(code, "AREA_LOOKUP_FAILED:") {
		out.ErrorCode = "AREA_LOOKUP_FAILED"
		out.Failed = []string{"AREA_LOOKUP_FAILED"}
	}
	return out
}

func artifactAreaStatusIsActive(status string) bool {
	switch strings.TrimSpace(status) {
	case "published", "stale":
		return true
	default:
		return false
	}
}
