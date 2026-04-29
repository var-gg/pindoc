package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// taskAssignInput is the agent-facing shape for pindoc.task.assign.
// Semantic shortcut over artifact.propose(shape="meta_patch",
// task_meta={assignee}) — Decision task-operation-tools-task-assign-단건-
// task-bulk-assign-배치-reas. Single-task variant; reason is optional.
type taskAssignInput struct {
	// ProjectSlug picks which project owns the Task (account-level scope,
	// Decision mcp-scope-account-level-industry-standard). Required.
	ProjectSlug string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`

	// SlugOrID identifies the Task. Accepts UUID, project-scoped slug, or
	// pindoc://slug URL.
	SlugOrID string `json:"slug_or_id" jsonschema:"Task artifact UUID, slug, or pindoc:// URL"`

	// Assignee is the new assignee value. Must match agent:<id> | user:<id>
	// | @<handle>, or empty string to explicitly clear.
	Assignee string `json:"assignee" jsonschema:"e.g. 'agent:codex', 'agent:claude', '@alice'; empty string explicitly clears"`

	// Reason is optional free-form rationale; stored as the revision
	// commit_msg. Empty string falls back to an auto-generated message.
	Reason string `json:"reason,omitempty" jsonschema:"optional one-line rationale (stored as commit_msg)"`

	// AuthorID overrides the server-issued agent_id for the revision's
	// display label. Empty = use Principal.AgentID.
	AuthorID string `json:"author_id,omitempty" jsonschema:"override author display label; defaults to server agent_id"`

	// AuthorVersion is the model/client version tag stored alongside the
	// revision (e.g. "opus-4.7").
	AuthorVersion string `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`
}

type taskAssignOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`

	Failed           []string             `json:"failed,omitempty"`
	ErrorCodes       []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist        []string             `json:"checklist,omitempty"`
	ChecklistItems   []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale    string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`
	SuggestedActions []string             `json:"suggested_actions,omitempty"`

	// Populated on accepted paths.
	ArtifactID     string `json:"artifact_id,omitempty"`
	Slug           string `json:"slug,omitempty"`
	AgentRef       string `json:"agent_ref,omitempty"`
	RevisionNumber int    `json:"revision_number,omitempty"`
	HumanURL       string `json:"human_url,omitempty"`
	HumanURLAbs    string `json:"human_url_abs,omitempty"`
	NewAssignee    string `json:"new_assignee,omitempty"`
}

// assigneePattern accepts the three principal shapes defined by Decision
// task-operation-tools-task-assign-단건-task-bulk-assign-배치-reas:
//   - agent:<id>   — MCP-subprocess identities (agent:codex, agent:claude-code)
//   - user:<id>    — user-table references, typically UUIDs
//   - @<handle>    — free-form human handles
//
// Empty string is permitted separately (explicit clear) and is not run
// against this regex.
var assigneePattern = regexp.MustCompile(`^(agent:[a-zA-Z0-9_\-:.]+|user:[a-zA-Z0-9_\-]+|@[a-zA-Z0-9_\-.]+)$`)

const (
	// reasonMinLen / reasonMaxLen gate the bulk_assign reason field. Decision
	// Open questions permit contentless strings like "ok" — length is the
	// only check. Kept as package-level constants so both task.assign (reason
	// optional) and task.bulk_assign (reason required) share them if future
	// changes introduce reason validation on the single-task path.
	reasonMinLen = 2
	reasonMaxLen = 200
)

// validateAssignee normalises and validates the assignee string. Empty
// string short-circuits to (empty, true) meaning "explicit clear" — the
// caller still gets a non-nil TaskMetaInput with assigneeSet=true so
// handleUpdateMetaPatch writes JSON null and jsonb_strip_nulls removes
// the field after the shallow merge.
func validateAssignee(assignee string) (string, bool) {
	a := strings.TrimSpace(assignee)
	if a == "" {
		return "", true
	}
	if !assigneePattern.MatchString(a) {
		return "", false
	}
	return a, true
}

// RegisterTaskAssign wires pindoc.task.assign. The handler validates the
// assignee shape, resolves the target, and calls the shared assignOneTask
// helper which in turn delegates to handleUpdateMetaPatch. Single-task
// calls pass bulk_op_id="" so revisions are not tagged with a batch
// correlation key.
func RegisterTaskAssign(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.task.assign",
			Description: "Change the assignee of a single Task. Semantic shortcut over artifact.propose(shape='meta_patch', task_meta={assignee}) — bypasses search_receipt gating (operational metadata lane). Assignee format: agent:<id> | user:<id> | @<handle>, or empty to clear. Reason is optional (stored as commit_msg). For batch rebalance use pindoc.task.bulk_assign instead.",
		},
		func(ctx context.Context, p *auth.Principal, in taskAssignInput) (*sdk.CallToolResult, taskAssignOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskAssignOutput{}, fmt.Errorf("task.assign: %w", err)
			}
			assignee, ok := validateAssignee(in.Assignee)
			if !ok {
				return nil, taskAssignOutput{
					Status:    "not_ready",
					ErrorCode: "ASSIGNEE_FORMAT_INVALID",
					Failed:    []string{"ASSIGNEE_FORMAT_INVALID"},
					Checklist: []string{"assignee must match agent:<id> | user:<id> | @<handle>, or be empty string to clear"},
				}, nil
			}
			res, err := assignOneTask(ctx, deps, p, scope, in.SlugOrID, assignee, in.Reason, in.AuthorID, in.AuthorVersion, "")
			if err != nil {
				return nil, taskAssignOutput{}, err
			}
			return nil, res, nil
		},
	)
}

// assignOneTask resolves slug/ID, reads current head revision, and calls
// handleUpdateMetaPatch with task_meta.assignee set. Shared by task.assign
// (bulkOpID="") and task.bulk_assign (bulkOpID="<hex>"). Returns a
// populated taskAssignOutput with either accepted fields or a stable
// error code; err is reserved for actual server faults (DB down, etc.)
// that callers should surface as 5xx-equivalent.
func assignOneTask(
	ctx context.Context,
	deps Deps,
	p *auth.Principal,
	scope *auth.ProjectScope,
	slugOrID, assignee, reason, authorID, authorVersion, bulkOpID string,
) (taskAssignOutput, error) {
	ref := normalizeRef(slugOrID)
	if ref == "" {
		return taskAssignOutput{
			Status:    "not_ready",
			ErrorCode: "ASSIGN_MISSING_REF",
			Failed:    []string{"ASSIGN_MISSING_REF"},
			Checklist: []string{"slug_or_id is required"},
		}, nil
	}

	// Resolve target: must exist, must be Task, capture revision number
	// for the optimistic-lock check handleUpdateMetaPatch runs.
	var (
		artifactID, resolvedSlug, artifactType string
		lastRev                                int
	)
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.slug, a.type,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, scope.ProjectSlug, ref).Scan(&artifactID, &resolvedSlug, &artifactType, &lastRev)
	if errors.Is(err, pgx.ErrNoRows) {
		return taskAssignOutput{
			Status:    "not_ready",
			ErrorCode: "ASSIGN_TARGET_NOT_FOUND",
			Failed:    []string{"ASSIGN_TARGET_NOT_FOUND"},
			Checklist: []string{fmt.Sprintf("Task %q not found in project %q", slugOrID, scope.ProjectSlug)},
		}, nil
	}
	if err != nil {
		return taskAssignOutput{}, fmt.Errorf("resolve task.assign target: %w", err)
	}
	if artifactType != "Task" {
		return taskAssignOutput{
			Status:    "not_ready",
			ErrorCode: "ASSIGN_NOT_A_TASK",
			Failed:    []string{"ASSIGN_NOT_A_TASK"},
			Checklist: []string{fmt.Sprintf("target %q has type=%s; task.assign accepts Task artifacts only", resolvedSlug, artifactType)},
		}, nil
	}

	// Compose commit_msg. Empty reason auto-fills. bulk_op_id (when set)
	// prefixes the message so a raw revisions scan can group batch-derived
	// changes without joining against events.
	commitMsg := strings.TrimSpace(reason)
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("assignee → %s", displayAssignee(assignee))
	}
	if bulkOpID != "" && len(bulkOpID) >= 8 {
		commitMsg = fmt.Sprintf("[bulk:%s] %s", bulkOpID[:8], commitMsg)
	}

	effAuthorID := strings.TrimSpace(authorID)
	if effAuthorID == "" {
		effAuthorID = p.AgentID
	}
	if effAuthorID == "" {
		effAuthorID = "unknown"
	}

	// Assemble artifactProposeInput and delegate. area_slug / title /
	// body_markdown are untouched by handleUpdateMetaPatch (verified by
	// META_PATCH_HAS_BODY rejecting any body delivery). Basis carries
	// bulk_op_id into source_session_ref so bulk audit queries can key on
	// the JSONB field.
	expected := lastRev
	propIn := artifactProposeInput{
		Type:            "Task",
		AuthorID:        effAuthorID,
		AuthorVersion:   authorVersion,
		UpdateOf:        artifactID,
		ExpectedVersion: &expected,
		Shape:           "meta_patch",
		CommitMsg:       commitMsg,
		TaskMeta:        &TaskMetaInput{Assignee: assignee},
	}
	propIn.TaskMeta.markAssigneeSet()
	if bulkOpID != "" {
		propIn.Basis = &artifactProposeBasis{
			SourceSession: "pindoc.task.bulk_assign",
			BulkOpID:      bulkOpID,
		}
	}

	_, out, err := handleUpdateMetaPatch(ctx, deps, p, scope, propIn, deps.UserLanguage)
	if err != nil {
		return taskAssignOutput{}, err
	}
	if out.Status != "accepted" {
		return taskAssignOutput{
			Status:           out.Status,
			ErrorCode:        out.ErrorCode,
			Failed:           out.Failed,
			Checklist:        out.Checklist,
			SuggestedActions: out.SuggestedActions,
			ArtifactID:       artifactID,
			Slug:             resolvedSlug,
		}, nil
	}
	return taskAssignOutput{
		Status:         "accepted",
		ArtifactID:     out.ArtifactID,
		Slug:           out.Slug,
		AgentRef:       out.AgentRef,
		RevisionNumber: out.RevisionNumber,
		HumanURL:       out.HumanURL,
		HumanURLAbs:    out.HumanURLAbs,
		NewAssignee:    assignee,
	}, nil
}

// displayAssignee returns a human-readable form for commit messages. Used
// only when the caller didn't supply a reason — turns an empty assignee
// (explicit clear) into "(cleared)" rather than a dangling arrow.
func displayAssignee(a string) string {
	if a == "" {
		return "(cleared)"
	}
	return a
}
