package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// taskBulkAssignInput is the agent-facing shape for pindoc.task.bulk_assign.
// Batch variant of task.assign — Decision task-operation-tools-task-assign-
// 단건-task-bulk-assign-배치-reas. `reason` is required (audit rationale
// shared by every generated revision). Partial success is allowed: per-
// slug failures land in Results[].ErrorCode and don't abort the batch.
type taskBulkAssignInput struct {
	// ProjectSlug picks which project owns the Tasks (account-level
	// scope, Decision mcp-scope-account-level-industry-standard).
	// Required.
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`

	// Slugs is the list of Task references to update. Order is preserved
	// in the response. Accepts UUID, slug, or pindoc:// URL.
	Slugs []string `json:"slugs" jsonschema:"list of Task slugs or IDs (min 1, recommended ≤ 50 per call)"`

	// Assignee is the target assignee, same format as task.assign.
	Assignee string `json:"assignee" jsonschema:"e.g. 'agent:codex', '@alice'; empty string explicitly clears"`

	// AllowReassign explicitly opts into overwriting a non-empty current
	// assignee. Default false keeps broad bulk "claim all open work" calls
	// from stealing work already owned by a user or another agent.
	AllowReassign bool `json:"allow_reassign,omitempty" jsonschema:"optional - default false; true permits overwriting non-empty current assignees in this batch"`

	// Reason is the shared rationale (required, 2-200 runes). Surfaced in
	// every generated revision's commit_msg so the diff view can explain
	// why 12 Tasks moved together.
	Reason string `json:"reason" jsonschema:"required; 2-200 runes; why these Tasks move together"`

	// AuthorID overrides Principal.AgentID for the display label. Empty =
	// use the server's agent_id.
	AuthorID string `json:"author_id,omitempty"`

	// AuthorVersion pairs with AuthorID for audit.
	AuthorVersion string `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`
}

// taskBulkAssignResult is the per-slug outcome. Slug is always populated
// (falls back to the input slug when resolution failed). ArtifactID and
// RevisionNumber only on accepted. ErrorCode only on not_ready.
type taskBulkAssignResult struct {
	Slug            string `json:"slug"`
	ArtifactID      string `json:"artifact_id,omitempty"`
	Status          string `json:"status"` // "accepted" | "not_ready"
	ErrorCode       string `json:"error_code,omitempty"`
	CurrentAssignee string `json:"current_assignee,omitempty"`
	RevisionNumber  int    `json:"revision_number,omitempty"`
	HumanURL        string `json:"human_url,omitempty"`
}

type taskBulkAssignOutput struct {
	// Status is "accepted" when every slug succeeded, "partial" when at
	// least one succeeded and at least one failed, "not_ready" when the
	// whole call was rejected (validation failure) or every slug failed.
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`

	Failed         []string             `json:"failed,omitempty"`
	ErrorCodes     []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist      []string             `json:"checklist,omitempty"`
	ChecklistItems []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale  string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`

	// BulkOpID correlates every revision emitted by this call. Present
	// only when the batch reached the fan-out stage — validation failures
	// that short-circuit return an empty value.
	BulkOpID string `json:"bulk_op_id,omitempty"`

	Results        []taskBulkAssignResult `json:"results,omitempty"`
	SuccessCount   int                    `json:"success_count"`
	FailCount      int                    `json:"fail_count"`
	NewAssignee    string                 `json:"new_assignee,omitempty"`
	ToolsetVersion string                 `json:"toolset_version,omitempty"`
}

// RegisterTaskBulkAssign wires pindoc.task.bulk_assign. Validates the
// batch-level inputs (reason length, assignee format, non-empty slugs),
// generates a bulk_op_id, then loops over slugs calling assignOneTask.
// Per-slug errors are captured and reported; the batch never aborts mid-
// flight on a single failure. By default it refuses to overwrite a
// non-empty assignee; callers must set allow_reassign=true when they are
// intentionally rebalancing already-owned work.
func RegisterTaskBulkAssign(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.task.bulk_assign",
			Description: "Change the assignee of multiple Tasks in one batch. Reason is required (one-line rationale shared by every revision's commit_msg). Default safety: Tasks with a non-empty current assignee are rejected with ASSIGNEE_ALREADY_SET unless allow_reassign=true is passed. Each accepted slug produces a revision prefixed with a shared bulk_op_id for audit. Partial success allowed: per-slug failures land in results[].error_code and do not abort the batch. For a single Task use pindoc.task.assign.",
		},
		func(ctx context.Context, p *auth.Principal, in taskBulkAssignInput) (*sdk.CallToolResult, taskBulkAssignOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskBulkAssignOutput{}, fmt.Errorf("task.bulk_assign: %w", err)
			}
			// --- Batch-level validation (fail-fast, no bulk_op_id issued). ---
			reason := strings.TrimSpace(in.Reason)
			if reason == "" {
				return nil, taskBulkAssignOutput{
					Status:    "not_ready",
					ErrorCode: "BULK_REASON_EMPTY",
					Failed:    []string{"BULK_REASON_EMPTY"},
					Checklist: []string{"reason is required on bulk_assign — one line explaining why these Tasks move together"},
				}, nil
			}
			runeCount := utf8.RuneCountInString(reason)
			if runeCount < reasonMinLen || runeCount > reasonMaxLen {
				return nil, taskBulkAssignOutput{
					Status:    "not_ready",
					ErrorCode: "REASON_LENGTH_INVALID",
					Failed:    []string{"REASON_LENGTH_INVALID"},
					Checklist: []string{fmt.Sprintf("reason must be %d-%d runes (got %d)", reasonMinLen, reasonMaxLen, runeCount)},
				}, nil
			}
			if len(in.Slugs) == 0 {
				return nil, taskBulkAssignOutput{
					Status:    "not_ready",
					ErrorCode: "BULK_NO_SLUGS",
					Failed:    []string{"BULK_NO_SLUGS"},
					Checklist: []string{"slugs[] must contain at least one entry"},
				}, nil
			}
			assignee, ok := validateAssignee(in.Assignee)
			if !ok {
				return nil, taskBulkAssignOutput{
					Status:    "not_ready",
					ErrorCode: "ASSIGNEE_FORMAT_INVALID",
					Failed:    []string{"ASSIGNEE_FORMAT_INVALID"},
					Checklist: []string{"assignee must match agent:<id> | user:<id> | @<handle>, or be empty to clear"},
				}, nil
			}

			// --- Issue batch correlation token. ---
			bulkOpID, err := newBulkOpID()
			if err != nil {
				return nil, taskBulkAssignOutput{}, err
			}

			// --- Fan out. Per-slug failures recorded, batch continues. ---
			results := make([]taskBulkAssignResult, 0, len(in.Slugs))
			successCount, failCount := 0, 0
			for _, slug := range in.Slugs {
				single, sErr := assignOneTask(ctx, deps, p, scope, slug, assignee, reason, in.AuthorID, in.AuthorVersion, bulkOpID, in.AllowReassign)
				if sErr != nil {
					// Server-side fault on this slug — record and move on.
					// Not aborting the batch because other slugs may still be
					// writable (e.g. one bad row shouldn't strand 11 others).
					results = append(results, taskBulkAssignResult{
						Slug:      slug,
						Status:    "not_ready",
						ErrorCode: "INTERNAL_ERROR",
					})
					failCount++
					continue
				}
				r := taskBulkAssignResult{
					Slug:            single.Slug,
					ArtifactID:      single.ArtifactID,
					Status:          single.Status,
					ErrorCode:       single.ErrorCode,
					CurrentAssignee: single.CurrentAssignee,
					RevisionNumber:  single.RevisionNumber,
					HumanURL:        single.HumanURL,
				}
				if r.Slug == "" {
					// Resolution failed before the target was known — surface
					// the caller-supplied reference so the response still
					// lines up with the input order.
					r.Slug = slug
				}
				results = append(results, r)
				if single.Status == "accepted" {
					successCount++
				} else {
					failCount++
				}
			}

			// --- Roll up batch-level status. ---
			status := "accepted"
			switch {
			case failCount > 0 && successCount > 0:
				status = "partial"
			case failCount > 0 && successCount == 0:
				status = "not_ready"
			}

			return nil, taskBulkAssignOutput{
				Status:       status,
				BulkOpID:     bulkOpID,
				Results:      results,
				SuccessCount: successCount,
				FailCount:    failCount,
				NewAssignee:  assignee,
			}, nil
		},
	)
}

// newBulkOpID returns a 32-hex-char random token (16 random bytes). Used
// as the correlation key for every revision emitted by a single
// bulk_assign call. Collision probability over the expected call volume
// is negligible; a ULID-style k-sortable id would be nicer but adds a
// dependency for marginal benefit on a field that is already indexed via
// the source_session_ref JSONB path.
func newBulkOpID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("newBulkOpID: rand.Read: %w", err)
	}
	return hex.EncodeToString(b), nil
}
