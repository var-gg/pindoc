package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// artifactVerifyInput is the agent-facing shape for pindoc.artifact.verify
// (migration 0013 / Task status v2). Moves a Task from claimed_done to
// verified by linking a VerificationReport artifact filed by a *different*
// agent than the one(s) who wrote the Task revisions.
type artifactVerifyInput struct {
	// TaskIDOrSlug identifies the Task being verified. Accepts UUID,
	// project-scoped slug, or pindoc://slug URL.
	TaskIDOrSlug string `json:"task_id_or_slug" jsonschema:"Task artifact UUID, slug, or pindoc:// URL"`

	// ReportIDOrSlug points at the VerificationReport artifact (already
	// created via pindoc.artifact.propose type=VerificationReport) that
	// records the evidence and verdict behind this verification step.
	// Keeping the report as a separate artifact means re-verification is
	// a supersede chain of reports and Reader surfaces the evidence
	// independently of the Task body.
	ReportIDOrSlug string `json:"report_id_or_slug" jsonschema:"VerificationReport artifact UUID, slug, or pindoc:// URL"`

	// VerifierAgentID is the display-label the verifier agent wants shown
	// in the event payload. The server still uses its own trusted
	// agent_id (deps.AgentID) for the Implementer ≠ Verifier check; the
	// client-reported label is informational.
	VerifierAgentID string `json:"verifier_agent_id,omitempty" jsonschema:"e.g. 'claude-code:verifier-session', 'codex-reviewer'"`

	// CommitMsg is a short one-liner stored on the verify event for
	// audit / changelog. Required so the Reader's verification history
	// has a human-readable pointer.
	CommitMsg string `json:"commit_msg" jsonschema:"one-line rationale for this verification step"`
}

type artifactVerifyOutput struct {
	Status           string   `json:"status"` // "accepted" | "not_ready"
	ErrorCode        string   `json:"error_code,omitempty"`
	Failed           []string `json:"failed,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	// Populated on accepted paths.
	TaskID      string `json:"task_id,omitempty"`
	TaskSlug    string `json:"task_slug,omitempty"`
	ReportID    string `json:"report_id,omitempty"`
	ReportSlug  string `json:"report_slug,omitempty"`
	NewStatus   string `json:"new_status,omitempty"` // "verified"
	HumanURL    string `json:"human_url,omitempty"`  // Task Reader link
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

// RegisterArtifactVerify wires pindoc.artifact.verify. Handler checks:
//
//   - Task exists + type='Task' + current status is claimed_done
//   - VerificationReport exists + type='VerificationReport'
//   - verifier agent (server-issued deps.AgentID) differs from every
//     agent_id that authored the Task's revisions (self-verification block)
//   - Links Task ↔ Report via artifact_edges relation='verified_by'
//   - Flips Task's task_meta.status to 'verified' + emits
//     artifact.verified event
//
// Anything unmet → Status=not_ready with stable error codes so agents can
// branch without parsing prose.
func RegisterArtifactVerify(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.artifact.verify",
			Description: "Move a Task from claimed_done to verified by linking a VerificationReport filed by a different agent than the Task's implementers. The only way to reach task_meta.status='verified' — artifact.propose rejects that transition directly. Call this once the VerificationReport artifact is already created (via artifact.propose type=VerificationReport).",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactVerifyInput) (*sdk.CallToolResult, artifactVerifyOutput, error) {
			taskRef := normalizeRef(in.TaskIDOrSlug)
			reportRef := normalizeRef(in.ReportIDOrSlug)
			if taskRef == "" || reportRef == "" {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_MISSING_REF",
					Failed:    []string{"VERIFY_MISSING_REF"},
					Checklist: []string{"task_id_or_slug and report_id_or_slug are both required"},
					SuggestedActions: []string{
						"Supply the Task slug you just implemented and the VerificationReport slug the verifier just filed.",
					},
				}, nil
			}
			if strings.TrimSpace(in.CommitMsg) == "" {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_NO_COMMIT_MSG",
					Failed:    []string{"VERIFY_NO_COMMIT_MSG"},
					Checklist: []string{"commit_msg is required — one line explaining why this verification step matters"},
					SuggestedActions: []string{
						"Try commit_msg='verified via repro on commit <sha>' or similar.",
					},
				}, nil
			}

			// --- Resolve Task ---
			var taskID, taskSlug, taskType string
			var taskMetaRaw []byte
			err := deps.DB.QueryRow(ctx, `
				SELECT a.id::text, a.slug, a.type, a.task_meta
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
				LIMIT 1
			`, deps.ProjectSlug, taskRef).Scan(&taskID, &taskSlug, &taskType, &taskMetaRaw)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_TASK_NOT_FOUND",
					Failed:    []string{"VERIFY_TASK_NOT_FOUND"},
					Checklist: []string{fmt.Sprintf("Task %q not found in project %q", in.TaskIDOrSlug, deps.ProjectSlug)},
				}, nil
			}
			if err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("resolve task: %w", err)
			}
			if taskType != "Task" {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_NOT_A_TASK",
					Failed:    []string{"VERIFY_NOT_A_TASK"},
					Checklist: []string{fmt.Sprintf("target %q has type=%s; artifact.verify only accepts Task artifacts", taskSlug, taskType)},
				}, nil
			}

			// --- Task must currently be claimed_done ---
			currentStatus := ""
			if len(taskMetaRaw) > 0 {
				var m map[string]any
				if err := json.Unmarshal(taskMetaRaw, &m); err == nil {
					if s, ok := m["status"].(string); ok {
						currentStatus = s
					}
				}
			}
			if currentStatus != "claimed_done" {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_WRONG_STATE",
					Failed:    []string{"VERIFY_WRONG_STATE"},
					Checklist: []string{fmt.Sprintf("Task %q current status=%q — verify only accepts claimed_done. Implementer agent should update_of the Task with task_meta.status='claimed_done' first (all acceptance checkboxes must be checked).", taskSlug, currentStatus)},
					SuggestedActions: []string{
						"Ask the implementer agent to flip status to claimed_done (acceptance checkboxes 100%) first.",
					},
				}, nil
			}

			// --- Resolve VerificationReport ---
			var reportID, reportSlug, reportType string
			err = deps.DB.QueryRow(ctx, `
				SELECT a.id::text, a.slug, a.type
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
				LIMIT 1
			`, deps.ProjectSlug, reportRef).Scan(&reportID, &reportSlug, &reportType)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_REPORT_NOT_FOUND",
					Failed:    []string{"VERIFY_REPORT_NOT_FOUND"},
					Checklist: []string{fmt.Sprintf("VerificationReport %q not found. Create it first via artifact.propose(type='VerificationReport').", in.ReportIDOrSlug)},
				}, nil
			}
			if err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("resolve report: %w", err)
			}
			if reportType != "VerificationReport" {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_WRONG_REPORT_TYPE",
					Failed:    []string{"VERIFY_WRONG_REPORT_TYPE"},
					Checklist: []string{fmt.Sprintf("Linked artifact %q has type=%s; expected VerificationReport", reportSlug, reportType)},
				}, nil
			}

			// --- Implementer ≠ Verifier check ---
			// Server-trusted agent_id (deps.AgentID) must not be among the
			// Task's revision authors. If deps.AgentID is empty the server
			// has no trusted identity to compare — fall back to the
			// client-reported VerifierAgentID but flag it with a warning
			// so operators know the invariant was weakened.
			verifierID := strings.TrimSpace(deps.AgentID)
			fallbackUsed := false
			if verifierID == "" {
				verifierID = strings.TrimSpace(in.VerifierAgentID)
				fallbackUsed = true
			}
			if verifierID == "" {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_NO_IDENTITY",
					Failed:    []string{"VERIFY_NO_IDENTITY"},
					Checklist: []string{"server has no agent_id to trust and no verifier_agent_id was supplied — cannot enforce Implementer ≠ Verifier"},
					SuggestedActions: []string{
						"Set PINDOC_AGENT_ID on the verifier's MCP process or pass verifier_agent_id explicitly.",
					},
				}, nil
			}
			selfVerify, err := agentAuthoredTaskRevision(ctx, deps, taskID, verifierID)
			if err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("implementer check: %w", err)
			}
			if selfVerify {
				return nil, artifactVerifyOutput{
					Status:    "not_ready",
					ErrorCode: "VERIFY_SELF",
					Failed:    []string{"VERIFY_SELF"},
					Checklist: []string{fmt.Sprintf("agent %q authored at least one Task revision and therefore cannot verify it. Spawn a different agent session (different model or different agent_id) to file this verification.", verifierID)},
					SuggestedActions: []string{
						"Run the verify call from a separate MCP session with a different PINDOC_AGENT_ID.",
					},
				}, nil
			}

			// --- Commit: edge + status flip + event ---
			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			// Idempotent edge — repeated verify calls attach the same
			// (task, report, relation) triple at most once.
			if _, err := tx.Exec(ctx, `
				INSERT INTO artifact_edges (source_id, target_id, relation)
				VALUES ($1::uuid, $2::uuid, 'verified_by')
				ON CONFLICT DO NOTHING
			`, taskID, reportID); err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("edge insert: %w", err)
			}

			// Flip task_meta.status to verified. jsonb_set preserves other
			// fields (priority, assignee, etc.).
			if _, err := tx.Exec(ctx, `
				UPDATE artifacts
				   SET task_meta = jsonb_set(COALESCE(task_meta, '{}'::jsonb), '{status}', '"verified"'),
				       updated_at = now()
				 WHERE id = $1::uuid
			`, taskID); err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("status flip: %w", err)
			}

			// Event payload records which agent performed the verify step
			// and which report was attached. fallback_used flag surfaces
			// the weakened-invariant case in audit logs.
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				SELECT a.project_id, 'artifact.verified', $1::uuid, jsonb_build_object(
					'verifier_agent_id', $2::text,
					'report_id', $3::uuid,
					'report_slug', $4::text,
					'commit_msg', $5::text,
					'fallback_identity_used', $6::bool
				)
				FROM artifacts a WHERE a.id = $1::uuid
			`, taskID, verifierID, reportID, reportSlug, in.CommitMsg, fallbackUsed); err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("event insert: %w", err)
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, artifactVerifyOutput{}, fmt.Errorf("commit: %w", err)
			}

			return nil, artifactVerifyOutput{
				Status:      "accepted",
				TaskID:      taskID,
				TaskSlug:    taskSlug,
				ReportID:    reportID,
				ReportSlug:  reportSlug,
				NewStatus:   "verified",
				HumanURL:    HumanURL(deps.ProjectSlug, taskSlug),
				HumanURLAbs: AbsHumanURL(deps.Settings, deps.ProjectSlug, taskSlug),
			}, nil
		},
	)
}

// agentAuthoredTaskRevision returns true when the given agent_id matches
// any author_id on the Task's revision history. Used to enforce the
// Implementer ≠ Verifier invariant in pindoc.artifact.verify.
func agentAuthoredTaskRevision(ctx context.Context, deps Deps, taskID, agentID string) (bool, error) {
	var exists bool
	err := deps.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM artifact_revisions
			WHERE artifact_id = $1::uuid
			  AND author_id = $2
		)
	`, taskID, agentID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
