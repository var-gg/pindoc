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
	"github.com/var-gg/pindoc/internal/pindoc/i18n"
	"github.com/var-gg/pindoc/internal/pindoc/policy"
)

// handleUpdateMetaPatch writes a meta-only revision: tags / completeness /
// task_meta / artifact_meta change without re-encoding body_markdown. The
// revision row carries body_markdown=NULL + body_hash=hash(currentBody)
// + revision_shape='meta_patch' + shape_payload={fields sent by caller}.
// Head artifacts.body_markdown is left untouched so reader paths keep
// returning the latest body without a join.
//
// Why a separate handler: re-embedding, canonical-rewrite detection,
// body-structure warnings, and no-op-against-title-and-body detection
// all key off body content. Running them against a meta-only update
// either produces false positives or wastes work. Copying the tx shell
// is a few dozen lines; bending handleUpdate around both paths would be
// more.
func handleUpdateMetaPatch(ctx context.Context, deps Deps, p *auth.Principal, scope *auth.ProjectScope, in artifactProposeInput, lang string) (*sdk.CallToolResult, artifactProposeOutput, error) {
	if strings.TrimSpace(in.CommitMsg) == "" {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "MISSING_COMMIT_MSG",
			Failed:    []string{"MISSING_COMMIT_MSG"},
			Checklist: []string{i18n.T(lang, "preflight.update_needs_commit")},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.commit_msg_hint"),
			},
			PatchableFields: patchFieldsFor("MISSING_COMMIT_MSG"),
		}, nil
	}

	// Meta-only revisions must not ship body content — if the agent sent
	// either body_markdown or body_patch they meant shape=body_patch.
	// Reject up-front so the error message is specific instead of a mid-
	// pipeline surprise.
	if strings.TrimSpace(in.BodyMarkdown) != "" || in.BodyPatch != nil {
		return nil, artifactProposeOutput{
			Status:          "not_ready",
			ErrorCode:       "META_PATCH_HAS_BODY",
			Failed:          []string{"META_PATCH_HAS_BODY"},
			Checklist:       []string{i18n.T(lang, "preflight.meta_patch_has_body")},
			PatchableFields: patchFieldsFor("META_PATCH_HAS_BODY"),
		}, nil
	}

	// Task status transitions are reserved for pindoc.task.transition /
	// task-specific lifecycle tools. meta_patch is the operational-metadata lane
	// (Decision agent-only-write-분할) — keeping status gates out of this
	// path prevents the UI-facing endpoint from bypassing the acceptance-
	// checklist check.
	if in.TaskMeta != nil && strings.TrimSpace(in.TaskMeta.Status) != "" {
		return nil, artifactProposeOutput{
			Status:          "not_ready",
			ErrorCode:       "TASK_STATUS_VIA_TRANSITION_TOOL",
			Failed:          []string{"TASK_STATUS_VIA_TRANSITION_TOOL"},
			Checklist:       []string{i18n.T(lang, "preflight.task_status_via_transition_tool")},
			PatchableFields: patchFieldsFor("TASK_STATUS_VIA_TRANSITION_TOOL"),
		}, nil
	}

	hasTags := in.Tags != nil
	hasCompleteness := strings.TrimSpace(in.Completeness) != ""
	hasTaskMeta := in.TaskMeta != nil
	hasArtifactMeta := in.ArtifactMeta != nil
	if !hasTags && !hasCompleteness && !hasTaskMeta && !hasArtifactMeta {
		return nil, artifactProposeOutput{
			Status:          "not_ready",
			ErrorCode:       "META_PATCH_EMPTY",
			Failed:          []string{"META_PATCH_EMPTY"},
			Checklist:       []string{i18n.T(lang, "preflight.meta_patch_empty")},
			PatchableFields: patchFieldsFor("META_PATCH_EMPTY"),
		}, nil
	}

	ref := normalizeRef(in.UpdateOf)

	var (
		artifactID, projectID, currentBody, currentTitle, currentType, currentSlug string
		sensitiveOps                                                               string
		currentTags                                                                []string
		currentCompleteness                                                        string
		lastRev                                                                    int
	)
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.body_markdown, a.title, a.type, a.slug,
		       a.tags, a.completeness,
		       COALESCE(NULLIF(p.sensitive_ops, ''), 'auto'),
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, scope.ProjectSlug, ref).Scan(
		&artifactID, &projectID, &currentBody, &currentTitle, &currentType, &currentSlug,
		&currentTags, &currentCompleteness, &sensitiveOps, &lastRev,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "UPDATE_TARGET_NOT_FOUND",
			Failed:    []string{"UPDATE_TARGET_NOT_FOUND"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.update_target_missing"), in.UpdateOf),
			},
			NextTools:       defaultNextTools("UPDATE_TARGET_NOT_FOUND"),
			PatchableFields: patchFieldsFor("UPDATE_TARGET_NOT_FOUND"),
		}, nil
	}
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("resolve meta_patch target: %w", err)
	}

	// Optimistic lock (same contract as body_patch).
	if in.ExpectedVersion == nil {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "NEED_VER",
			Failed:    []string{"NEED_VER"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.need_ver"), lastRev),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.reread_before_update"),
			},
			NextTools:       defaultNextTools("UPDATE_TARGET_NOT_FOUND"),
			PatchableFields: patchFieldsFor("NEED_VER"),
			Related: []RelatedRef{
				makeRelated(deps, scope, ref, artifactID, "", currentTitle, fmt.Sprintf("current revision = %d; pass expected_version = %d", lastRev, lastRev)),
			},
		}, nil
	}
	if *in.ExpectedVersion != lastRev {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "VER_CONFLICT",
			Failed:    []string{"VER_CONFLICT"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.ver_conflict"), *in.ExpectedVersion, lastRev),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.reread_before_update"),
			},
			NextTools:       defaultNextTools("VER_CONFLICT"),
			PatchableFields: patchFieldsFor("VER_CONFLICT"),
		}, nil
	}

	// Status transitions are blocked at the top of this handler, so there
	// is no claimed_done acceptance-checklist re-check to run here. If that
	// changes, gate via pindoc.task.transition instead of re-opening this
	// path.

	// Effective values. For tags / completeness, absent = preserve current;
	// present = replace. task_meta / artifact_meta follow the same
	// send-to-overwrite rule as the body_patch path (COALESCE in SQL).
	effectiveTags := currentTags
	if hasTags {
		effectiveTags = in.Tags
	}
	effectiveCompleteness := currentCompleteness
	if hasCompleteness {
		effectiveCompleteness = in.Completeness
	}
	reviewState := policy.ReviewStateFor(sensitiveOps, policy.OpCompletenessWrite, policy.SensitiveContext{
		FromCompleteness: currentCompleteness,
		ToCompleteness:   effectiveCompleteness,
	})

	var taskMetaPatch any
	if hasTaskMeta {
		taskMetaPatch = taskMetaToJSON(currentType, in.TaskMeta)
	}
	var artifactMetaPatch any
	var resolvedUpdateMeta ResolvedArtifactMeta
	if hasArtifactMeta {
		resolvedUpdateMeta = resolveArtifactMeta(in.ArtifactMeta, nil, currentBody, true)
		artifactMetaPatch = artifactMetaToJSON(resolvedUpdateMeta)
	}

	// shape_payload preserves the agent-supplied delta for audit / Reader
	// Trust Card. Only sent fields land here — this is the per-revision
	// "what did the agent claim to change" record.
	shapePayload := map[string]any{}
	if hasTags {
		shapePayload["tags"] = in.Tags
	}
	if hasCompleteness {
		shapePayload["completeness"] = in.Completeness
	}
	if hasTaskMeta {
		shapePayload["task_meta"] = in.TaskMeta
	}
	if hasArtifactMeta {
		shapePayload["artifact_meta"] = in.ArtifactMeta
	}
	shapePayloadJSON, err := json.Marshal(shapePayload)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("marshal shape_payload: %w", err)
	}
	fieldsChangedJSON, err := json.Marshal(metaFieldsChangedList(shapePayload))
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("marshal fields_changed: %w", err)
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	newRev := lastRev + 1
	prevBodyHash := bodyHash(currentBody)

	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, NULL, $4, $5, $6, 'agent', $7, $8, $9, $10, 'meta_patch', $11::jsonb)
	`, artifactID, newRev, currentTitle, prevBodyHash, effectiveTags, effectiveCompleteness,
		in.AuthorID, nullIfEmpty(in.AuthorVersion), in.CommitMsg,
		buildSourceSessionRef(p, in), string(shapePayloadJSON),
	); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("insert meta_patch revision: %w", err)
	}

	// task_meta update is a shallow merge (top-level key overwrite) so an
	// agent can PATCH one field without re-sending the rest. JSON null in
	// the patch clears a key after jsonb_strip_nulls; this preserves the
	// difference between omitted assignee and assignee="" clear.
	if _, err := tx.Exec(ctx, `
		UPDATE artifacts
		   SET tags           = $2,
		       completeness   = $3,
		       task_meta      = CASE
		           WHEN $4::jsonb IS NULL THEN task_meta
		           ELSE jsonb_strip_nulls(COALESCE(task_meta, '{}'::jsonb) || $4::jsonb)
		       END,
		       artifact_meta  = COALESCE($5::jsonb, artifact_meta),
		       review_state   = CASE
		           WHEN $8::text = 'pending_review' THEN 'pending_review'
		           ELSE review_state
		       END,
		       author_id      = $6,
		       author_version = $7,
		       updated_at     = now()
		 WHERE id = $1
	`, artifactID, effectiveTags, effectiveCompleteness,
		taskMetaPatch, artifactMetaPatch,
		in.AuthorID, nullIfEmpty(in.AuthorVersion),
		reviewState,
	); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("update artifact head meta: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.meta_patched', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'commit_msg',      $6::text,
			'fields_changed',  $7::jsonb
		))
	`, projectID, artifactID, newRev, currentSlug, in.AuthorID, in.CommitMsg,
		string(fieldsChangedJSON),
	); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("event: %w", err)
	}
	if reviewState == policy.ReviewStatePending {
		if err := recordReviewRequiredEvent(ctx, tx, projectID, artifactID, string(policy.OpCompletenessWrite), in.AuthorID); err != nil {
			return nil, artifactProposeOutput{}, fmt.Errorf("review required event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
	}

	var metaOut *ResolvedArtifactMeta
	if hasArtifactMeta {
		metaOut = &resolvedUpdateMeta
	}
	warnings := sortWarningsBySeverity(acceptanceUncheckedNudgeWarnings(currentType, currentBody, in.CommitMsg))
	severities := make([]string, len(warnings))
	for i, w := range warnings {
		severities[i] = warningSeverity(w)
	}

	return nil, artifactProposeOutput{
		Status:            "accepted",
		ArtifactID:        artifactID,
		Slug:              currentSlug,
		AgentRef:          "pindoc://" + currentSlug,
		HumanURL:          HumanURL(scope.ProjectSlug, scope.ProjectLocale, currentSlug),
		HumanURLAbs:       AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, currentSlug),
		Created:           false,
		RevisionNumber:    newRev,
		Warnings:          warnings,
		WarningSeverities: severities,
		ArtifactMeta:      metaOut,
	}, nil
}

// metaFieldsChangedList returns the sorted list of top-level meta fields
// the agent supplied on a meta_patch call. Feeds the
// events.artifact.meta_patched payload so downstream dashboards can query
// "which meta fields moved in the last N days" without re-parsing
// shape_payload.
func metaFieldsChangedList(shapePayload map[string]any) []string {
	if len(shapePayload) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(shapePayload))
	for k := range shapePayload {
		out = append(out, k)
	}
	// Stable order so diffs / snapshot tests don't churn.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
