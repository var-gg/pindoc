package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// taskClaimDoneInput is the agent-facing shape for pindoc.task.claim_done.
// Atomic shortcut over the two halves of "I finished implementing this
// Task": (a) flipping every "[ ]" acceptance marker in the body to "[x]",
// and (b) setting task_meta.status="claimed_done". Without this tool agents
// must propose(shape="body_patch", task_meta={status="claimed_done"})
// which round-trips the whole body. Decision mcp-dx-외부-리뷰-codex-1차-
// 피드백-6항목 발견 1.
//
// Reason is optional — when supplied it lands as the revision commit_msg.
// AuthorID / AuthorVersion mirror task.assign for audit consistency.
type taskClaimDoneInput struct {
	// ProjectSlug picks which project owns the Task (account-level scope,
	// Decision mcp-scope-account-level-industry-standard).
	ProjectSlug string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`

	// SlugOrID identifies the Task. Accepts UUID, project-scoped slug, or
	// pindoc://slug URL.
	SlugOrID string `json:"slug_or_id" jsonschema:"Task artifact UUID, slug, or pindoc:// URL"`

	// Reason is optional one-line rationale stored as the revision
	// commit_msg. Empty falls back to an auto-generated message.
	Reason string `json:"reason,omitempty" jsonschema:"optional one-line rationale (stored as commit_msg)"`

	// AuthorID overrides Principal.AgentID for the revision's display
	// label. Empty falls back to Principal.AgentID.
	AuthorID string `json:"author_id,omitempty" jsonschema:"override author display label; defaults to server agent_id"`

	// AuthorVersion is the model/client version tag stored alongside the
	// revision (e.g. "opus-4.7").
	AuthorVersion string `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`
}

type taskClaimDoneOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`

	Failed           []string `json:"failed,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	// Populated on accepted paths.
	ArtifactID             string `json:"artifact_id,omitempty"`
	Slug                   string `json:"slug,omitempty"`
	AgentRef               string `json:"agent_ref,omitempty"`
	RevisionNumber         int    `json:"revision_number,omitempty"`
	HumanURL               string `json:"human_url,omitempty"`
	HumanURLAbs            string `json:"human_url_abs,omitempty"`
	ChangedAcceptanceCount int    `json:"changed_acceptance_count"`
	PrevStatus             string `json:"prev_status,omitempty"`
	NewStatus              string `json:"new_status,omitempty"`
}

// RegisterTaskClaimDone wires pindoc.task.claim_done. The handler resolves
// the target Task, rewrites every unchecked acceptance marker in the body
// to "[x]", and updates task_meta.status to "claimed_done" — all in one
// revision so audit / Reader Trust Card see a single atomic transition.
//
// Like task.assign, this is the operational-metadata lane: search_receipt
// gating is bypassed because targeting an existing Task is itself proof of
// context.
func RegisterTaskClaimDone(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.task.claim_done",
			Description: "Mark a Task implementation complete. Toggles every unchecked acceptance item ('- [ ]') to '[x]' and sets task_meta.status='claimed_done' in one atomic revision. Already-resolved markers ([x]/[~]/[-]) are preserved — partial / deferred judgment calls are not overwritten. Bypasses search_receipt gating (operational metadata lane). Reason is optional (stored as commit_msg). Use pindoc.artifact.verify to move from claimed_done → verified.",
		},
		func(ctx context.Context, p *auth.Principal, in taskClaimDoneInput) (*sdk.CallToolResult, taskClaimDoneOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskClaimDoneOutput{}, fmt.Errorf("task.claim_done: %w", err)
			}
			res, err := claimOneTaskDone(ctx, deps, p, scope, in)
			if err != nil {
				return nil, taskClaimDoneOutput{}, err
			}
			return nil, res, nil
		},
	)
}

// claimOneTaskDone is the unwrapped handler. Returns a populated
// taskClaimDoneOutput with either accepted fields or a stable error code;
// err is reserved for actual server faults (DB down, etc.) that callers
// should surface as 5xx-equivalent.
//
// The status guard rejects transitions from already-terminal states:
//   - claimed_done → ALREADY_DONE (use artifact.verify next)
//   - verified    → ALREADY_VERIFIED (reopen needs a fresh revision)
//   - cancelled   → TASK_CANCELLED (caller must reopen first)
//
// open / blocked / nil status all proceed to the body+meta write.
func claimOneTaskDone(
	ctx context.Context,
	deps Deps,
	p *auth.Principal,
	scope *auth.ProjectScope,
	in taskClaimDoneInput,
) (taskClaimDoneOutput, error) {
	ref := normalizeRef(in.SlugOrID)
	if ref == "" {
		return taskClaimDoneOutput{
			Status:    "not_ready",
			ErrorCode: "CLAIM_DONE_MISSING_REF",
			Failed:    []string{"CLAIM_DONE_MISSING_REF"},
			Checklist: []string{"slug_or_id is required"},
		}, nil
	}

	// Optional reason length check mirrors bulk_assign so commit_msg shape
	// is uniform across operational tools. Empty is accepted; only
	// supplied-but-malformed strings fail here.
	reason := strings.TrimSpace(in.Reason)
	if reason != "" {
		runeCount := utf8.RuneCountInString(reason)
		if runeCount < reasonMinLen || runeCount > reasonMaxLen {
			return taskClaimDoneOutput{
				Status:    "not_ready",
				ErrorCode: "REASON_LENGTH_INVALID",
				Failed:    []string{"REASON_LENGTH_INVALID"},
				Checklist: []string{fmt.Sprintf("reason must be %d-%d runes (got %d)", reasonMinLen, reasonMaxLen, runeCount)},
			}, nil
		}
	}

	var (
		artifactID, projectID, currentBody, currentTitle, currentType, currentSlug string
		currentTags                                                                []string
		currentCompleteness                                                        string
		currentTaskMetaJSON                                                        []byte
		lastRev                                                                    int
	)
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.body_markdown, a.title, a.type, a.slug,
		       a.tags, a.completeness,
		       COALESCE(a.task_meta, '{}'::jsonb)::text,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, scope.ProjectSlug, ref).Scan(
		&artifactID, &projectID, &currentBody, &currentTitle, &currentType, &currentSlug,
		&currentTags, &currentCompleteness, &currentTaskMetaJSON, &lastRev,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return taskClaimDoneOutput{
			Status:    "not_ready",
			ErrorCode: "CLAIM_DONE_TARGET_NOT_FOUND",
			Failed:    []string{"CLAIM_DONE_TARGET_NOT_FOUND"},
			Checklist: []string{fmt.Sprintf("Task %q not found in project %q", in.SlugOrID, scope.ProjectSlug)},
		}, nil
	}
	if err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("resolve task.claim_done target: %w", err)
	}

	if currentType != "Task" {
		return taskClaimDoneOutput{
			Status:     "not_ready",
			ErrorCode:  "CLAIM_DONE_NOT_A_TASK",
			Failed:     []string{"CLAIM_DONE_NOT_A_TASK"},
			Checklist:  []string{fmt.Sprintf("target %q has type=%s; task.claim_done accepts Task artifacts only", currentSlug, currentType)},
			ArtifactID: artifactID,
			Slug:       currentSlug,
		}, nil
	}

	// Decode current task_meta to read prev_status. Terminal states block
	// the transition; the response includes ArtifactID/Slug so callers can
	// jump straight to the next tool (verify / propose) without a re-read.
	currentTaskMeta := map[string]any{}
	if err := json.Unmarshal(currentTaskMetaJSON, &currentTaskMeta); err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("decode task_meta: %w", err)
	}
	prevStatus, _ := currentTaskMeta["status"].(string)
	switch prevStatus {
	case "claimed_done":
		return taskClaimDoneOutput{
			Status:     "not_ready",
			ErrorCode:  "CLAIM_DONE_ALREADY_DONE",
			Failed:     []string{"CLAIM_DONE_ALREADY_DONE"},
			Checklist:  []string{fmt.Sprintf("Task %q is already claimed_done; use pindoc.artifact.verify to move to verified", currentSlug)},
			ArtifactID: artifactID,
			Slug:       currentSlug,
			PrevStatus: prevStatus,
		}, nil
	case "verified":
		return taskClaimDoneOutput{
			Status:     "not_ready",
			ErrorCode:  "CLAIM_DONE_ALREADY_VERIFIED",
			Failed:     []string{"CLAIM_DONE_ALREADY_VERIFIED"},
			Checklist:  []string{fmt.Sprintf("Task %q is already verified; reopening requires a fresh revision via artifact.propose", currentSlug)},
			ArtifactID: artifactID,
			Slug:       currentSlug,
			PrevStatus: prevStatus,
		}, nil
	case "cancelled":
		return taskClaimDoneOutput{
			Status:     "not_ready",
			ErrorCode:  "CLAIM_DONE_TASK_CANCELLED",
			Failed:     []string{"CLAIM_DONE_TASK_CANCELLED"},
			Checklist:  []string{fmt.Sprintf("Task %q is cancelled; reopen via pindoc.artifact.propose first", currentSlug)},
			ArtifactID: artifactID,
			Slug:       currentSlug,
			PrevStatus: prevStatus,
		}, nil
	}

	newBody, changedCount := markUncheckedAsDone(currentBody)

	commitMsg := reason
	if commitMsg == "" {
		if changedCount > 0 {
			commitMsg = fmt.Sprintf("claim_done: %d acceptance toggled → [x]", changedCount)
		} else {
			commitMsg = "claim_done: status → claimed_done"
		}
	}

	effAuthorID := strings.TrimSpace(in.AuthorID)
	if effAuthorID == "" && p != nil {
		effAuthorID = p.AgentID
	}
	if effAuthorID == "" {
		effAuthorID = "unknown"
	}

	// shape_payload preserves the per-revision delta. revision_shape stays
	// at "body_patch" because the DB CHECK constraint (migration 0017)
	// only knows the four legacy values. The kind=claim_done marker lets
	// future analytics / Reader Trust Card pick claim_done revisions out
	// of the body_patch bucket without a schema change.
	shapePayload := map[string]any{
		"kind":                     "claim_done",
		"changed_acceptance_count": changedCount,
		"prev_status":              prevStatus,
		"new_status":               "claimed_done",
	}
	shapePayloadJSON, err := json.Marshal(shapePayload)
	if err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("marshal shape_payload: %w", err)
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	newRev := lastRev + 1

	// When changedCount > 0 the body genuinely changed and the revision
	// stores the new content. When changedCount == 0 (no [ ] markers
	// remained) we still write a revision so the status transition is
	// auditable, but body_markdown reuses the previous body verbatim so
	// diff views show "metadata only" rather than a phantom edit.
	revBody := newBody
	if changedCount == 0 {
		revBody = currentBody
	}
	revBodyHash := bodyHash(revBody)

	sourceSessionRef := buildSourceSessionRef(p, artifactProposeInput{
		AuthorID: effAuthorID,
		Basis: &artifactProposeBasis{
			SourceSession: "pindoc.task.claim_done",
		},
	})

	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'agent', $8, $9, $10, $11, 'body_patch', $12::jsonb)
	`, artifactID, newRev, currentTitle, revBody, revBodyHash,
		currentTags, currentCompleteness,
		effAuthorID, nullIfEmpty(in.AuthorVersion), commitMsg,
		sourceSessionRef, string(shapePayloadJSON),
	); err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("insert claim_done revision: %w", err)
	}

	// Head update: body only when the toggle actually changed it (avoids
	// a redundant write when changedCount=0), task_meta.status is always
	// merged forward via the same shallow-merge pattern task.assign uses.
	if changedCount > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE artifacts
			   SET body_markdown  = $2,
			       task_meta      = COALESCE(task_meta, '{}'::jsonb)
			                        || jsonb_build_object('status', 'claimed_done'),
			       author_id      = $3,
			       author_version = $4,
			       updated_at     = now()
			 WHERE id = $1
		`, artifactID, newBody, effAuthorID, nullIfEmpty(in.AuthorVersion)); err != nil {
			return taskClaimDoneOutput{}, fmt.Errorf("update artifact head body+meta: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE artifacts
			   SET task_meta      = COALESCE(task_meta, '{}'::jsonb)
			                        || jsonb_build_object('status', 'claimed_done'),
			       author_id      = $2,
			       author_version = $3,
			       updated_at     = now()
			 WHERE id = $1
		`, artifactID, effAuthorID, nullIfEmpty(in.AuthorVersion)); err != nil {
			return taskClaimDoneOutput{}, fmt.Errorf("update artifact head meta: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.task_claimed_done', $2, jsonb_build_object(
			'revision_number',          $3::int,
			'slug',                     $4::text,
			'author_id',                $5::text,
			'commit_msg',               $6::text,
			'changed_acceptance_count', $7::int,
			'prev_status',              $8::text
		))
	`, projectID, artifactID, newRev, currentSlug, effAuthorID, commitMsg, changedCount, prevStatus); err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("emit task_claimed_done event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("commit task.claim_done: %w", err)
	}

	return taskClaimDoneOutput{
		Status:                 "accepted",
		ArtifactID:             artifactID,
		Slug:                   currentSlug,
		AgentRef:               "pindoc://" + currentSlug,
		RevisionNumber:         newRev,
		HumanURL:               HumanURL(scope.ProjectSlug, scope.ProjectLocale, currentSlug),
		HumanURLAbs:            AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, currentSlug),
		ChangedAcceptanceCount: changedCount,
		PrevStatus:             prevStatus,
		NewStatus:              "claimed_done",
	}, nil
}

// markUncheckedAsDone returns (newBody, changedCount). Walks every 4-state
// checkbox in the body and rewrites only " " → "x". [x]/[X]/[~]/[-] are
// preserved — the latter two represent prior judgment calls (partial /
// deferred) that an automatic mass-toggle should not overwrite.
//
// When no unchecked markers exist, returns (prev, 0) without allocating a
// new string.
func markUncheckedAsDone(prev string) (string, int) {
	hits := iterateCheckboxes(prev)
	if len(hits) == 0 {
		return prev, 0
	}
	changed := 0
	var lines []string
	for _, cb := range hits {
		if cb.marker != ' ' {
			continue
		}
		if lines == nil {
			lines = strings.Split(prev, "\n")
		}
		line := lines[cb.lineIndex]
		mo := cb.markerByteOffset + 1
		lines[cb.lineIndex] = line[:mo] + "x" + line[mo+1:]
		changed++
	}
	if changed == 0 {
		return prev, 0
	}
	return strings.Join(lines, "\n"), changed
}
