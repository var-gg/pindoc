package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	pgit "github.com/var-gg/pindoc/internal/pindoc/git"
	pinmodel "github.com/var-gg/pindoc/internal/pindoc/pins"
)

const claimDoneAutopinDefaultLimit = 20

const (
	claimDonePinStrategyAuto      = "auto"
	claimDonePinStrategyAllowlist = "allowlist"
	claimDonePinStrategyExplicit  = "explicit"
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

	// CommitSHA is an optional commit hash that proves this Task's
	// implementation. Recorded in shape_payload (claim_done lane), used
	// to auto-pin changed files from the commit diff when possible, and
	// prefixed onto commit_msg as "[<short>] ..." so revision history
	// shows the implementation source at a glance. Length 7-64,
	// hex-only; empty / whitespace is treated as "no commit attached".
	CommitSHA string `json:"commit_sha,omitempty" jsonschema:"optional 7-64 char hex commit hash that proves implementation; auto-pins changed files from the commit diff when possible"`

	// PinStrategy selects how commit_sha turns into artifact_pins.
	// "auto" (default) preserves the historical behavior and pins up to
	// 20 changed files from the commit diff. "allowlist" pins only
	// paths listed in changed_paths_allowlist. "explicit" disables
	// commit-diff auto pins entirely and stores only pins[].
	PinStrategy string `json:"pin_strategy,omitempty" jsonschema:"optional auto|allowlist|explicit; default auto preserves commit-diff auto pins"`

	// ChangedPathsAllowlist limits commit-diff auto pins when
	// pin_strategy="allowlist". Paths are matched against normalized Git
	// changed-file paths (trimmed, backslashes converted to slashes).
	ChangedPathsAllowlist []string `json:"changed_paths_allowlist,omitempty" jsonschema:"for pin_strategy=allowlist, only these changed paths are eligible for commit-diff auto pins"`

	// Pins attaches structured implementation evidence to the artifact.
	// Same shape as artifact.propose pins[]. Each pin lands in
	// artifact_pins so the Reader Sidecar references panel renders it
	// without any extra hop. Duplicates against existing pins are
	// silently skipped (warnings carry PIN_DUPLICATE_SKIPPED:<path>) so
	// claim_done stays idempotent on retry.
	Pins []ArtifactPinInput `json:"pins,omitempty" jsonschema:"optional implementation evidence; same shape as artifact.propose pins[] — duplicates are skipped silently"`

	// EvidenceArtifacts attaches non-code deliverables such as Decision
	// or Analysis artifacts as relates_to=evidence edges on the Task.
	// Code/config/doc changes should still use commit_sha or pins[].
	EvidenceArtifacts []string `json:"evidence_artifacts,omitempty" jsonschema:"optional non-code deliverable artifacts; slug, UUID, or pindoc:// refs stored as relates_to=evidence edges"`
}

type taskClaimDoneOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`

	Failed           []string `json:"failed,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	// Populated on accepted paths.
	ArtifactID             string   `json:"artifact_id,omitempty"`
	Slug                   string   `json:"slug,omitempty"`
	AgentRef               string   `json:"agent_ref,omitempty"`
	RevisionNumber         int      `json:"revision_number,omitempty"`
	HumanURL               string   `json:"human_url,omitempty"`
	HumanURLAbs            string   `json:"human_url_abs,omitempty"`
	ChangedAcceptanceCount int      `json:"changed_acceptance_count"`
	PrevStatus             string   `json:"prev_status,omitempty"`
	NewStatus              string   `json:"new_status,omitempty"`
	CommitSHA              string   `json:"commit_sha,omitempty"`
	PinStrategy            string   `json:"pin_strategy,omitempty"`
	ChangedPathsAllowlist  []string `json:"changed_paths_allowlist,omitempty"`
	PinsStored             int      `json:"pins_stored"`
	PinsAutopinCount       int      `json:"pins_autopin_count"`
	PinsExplicitCount      int      `json:"pins_explicit_count"`
	EvidenceEdgesStored    int      `json:"evidence_edges_stored,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
	ToolsetVersion         string   `json:"toolset_version,omitempty"`
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
			Description: "Mark a Task implementation complete. Toggles every unchecked acceptance item ('- [ ]') to '[x]' and sets task_meta.status='claimed_done' in one atomic revision. Already-resolved markers ([x]/[~]/[-]) are preserved — partial / deferred judgment calls are not overwritten. Bypasses search_receipt gating (operational metadata lane). When the Task involved code/doc/config changes, pass commit_sha (7-64 hex chars, prefixed onto commit_msg as '[<short>] ...') and choose pin_strategy: auto (default) auto-pins changed files from the commit diff up to 20, allowlist auto-pins only changed_paths_allowlist entries, explicit disables commit-diff auto pins and stores only pins[]. pins[] uses the same shape as artifact.propose pins[]; duplicate pins are silently skipped. For non-code deliverables such as Decision or Analysis artifacts, pass evidence_artifacts (slug/id/pindoc://) to store relates_to=evidence edges; commit pins and evidence_artifacts can be used together. Reason is optional (stored as commit_msg).",
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
//   - claimed_done → ALREADY_DONE
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

	// Evidence args (commit_sha, pins[]) are validated up-front so a
	// malformed pin path or a paste-error commit hash doesn't slip into
	// the transaction. Both are optional — empty input means "claim_done
	// without structured evidence", which is still a valid call (e.g.
	// doc-only or decision-only Tasks).
	commitSHA, sCode, sMsg := validateClaimDoneCommitSHA(in.CommitSHA)
	if sCode != "" {
		return taskClaimDoneOutput{
			Status:    "not_ready",
			ErrorCode: sCode,
			Failed:    []string{sCode},
			Checklist: []string{sMsg},
		}, nil
	}
	pinStrategy, psCode, psMsg := normalizeClaimDonePinStrategy(in.PinStrategy)
	if psCode != "" {
		return taskClaimDoneOutput{
			Status:    "not_ready",
			ErrorCode: psCode,
			Failed:    []string{psCode},
			Checklist: []string{psMsg},
		}, nil
	}
	changedPathsAllowlist := normalizeClaimDoneChangedPathsAllowlist(in.ChangedPathsAllowlist)
	if pinStrategy == claimDonePinStrategyAllowlist && len(changedPathsAllowlist) == 0 {
		return taskClaimDoneOutput{
			Status:    "not_ready",
			ErrorCode: "CLAIM_DONE_PIN_ALLOWLIST_EMPTY",
			Failed:    []string{"CLAIM_DONE_PIN_ALLOWLIST_EMPTY"},
			Checklist: []string{"pin_strategy=allowlist requires at least one non-empty changed_paths_allowlist entry"},
		}, nil
	}
	pinsValidated, pCode, pMsg := validateClaimDonePins(in.Pins)
	if pCode != "" {
		return taskClaimDoneOutput{
			Status:    "not_ready",
			ErrorCode: pCode,
			Failed:    []string{pCode},
			Checklist: []string{pMsg},
		}, nil
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
			Checklist:  []string{fmt.Sprintf("Task %q is already claimed_done; create a follow-up Task if more work remains", currentSlug)},
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
	commitMsg = prefixClaimDoneCommitMsg(commitMsg, commitSHA)

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
	//
	// commit_sha and pins_stored land here so revision history alone
	// (no extra join on artifact_pins) tells the Reader which revision
	// attached which evidence — pins_stored is filled in after the
	// transaction-time dedup, not from len(pinsValidated).
	shapePayload := map[string]any{
		"kind":                     "claim_done",
		"changed_acceptance_count": changedCount,
		"prev_status":              prevStatus,
		"new_status":               "claimed_done",
		"pin_strategy":             pinStrategy,
	}
	if commitSHA != "" {
		shapePayload["commit_sha"] = commitSHA
	}
	if len(changedPathsAllowlist) > 0 {
		shapePayload["changed_paths_allowlist"] = changedPathsAllowlist
	}
	evidenceRefs := normalizeClaimDoneEvidenceArtifacts(in.EvidenceArtifacts)
	if len(evidenceRefs) > 0 {
		shapePayload["evidence_artifacts"] = evidenceRefs
	}

	autoPins, autoWarnings := buildClaimDoneAutoPins(ctx, deps, projectID, commitSHA, claimDoneAutopinDefaultLimit, pinStrategy, changedPathsAllowlist)
	pinWarnings := append([]string{}, autoWarnings...)
	pinSources := make([]claimDonePinWithSource, 0, len(pinsValidated)+len(autoPins))
	for _, pin := range pinsValidated {
		pinSources = append(pinSources, claimDonePinWithSource{Pin: pin, Source: claimDonePinSourceExplicit})
	}
	for _, pin := range autoPins {
		pinSources = append(pinSources, claimDonePinWithSource{Pin: pin, Source: claimDonePinSourceAutopin})
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	evidenceEdgesStored := 0
	if len(evidenceRefs) > 0 {
		relations := make([]ArtifactRelationInput, 0, len(evidenceRefs))
		for _, ref := range evidenceRefs {
			relations = append(relations, ArtifactRelationInput{TargetID: ref, Relation: "evidence"})
		}
		targetIDs, relErr := resolveRelatesTo(ctx, tx, scope.ProjectSlug, relations, deps.UserLanguage)
		if relErr != nil {
			return taskClaimDoneOutput{
				Status:           "not_ready",
				ErrorCode:        "EVIDENCE_TARGET_NOT_FOUND",
				Failed:           []string{"EVIDENCE_TARGET_NOT_FOUND"},
				Checklist:        []string{fmt.Sprintf("evidence_artifacts contains an unknown artifact ref in project %q", scope.ProjectSlug)},
				SuggestedActions: relErr.SuggestedActions,
				ArtifactID:       artifactID,
				Slug:             currentSlug,
			}, nil
		}
		stored, err := insertEdges(ctx, tx, artifactID, targetIDs, relations)
		if err != nil {
			return taskClaimDoneOutput{}, err
		}
		evidenceEdgesStored = stored
	}

	// Pin attachment runs *inside* the transaction so artifact_pins,
	// artifact_revisions, and the events row commit or roll back
	// together. Duplicates against existing pins are skipped silently
	// (PIN_DUPLICATE_SKIPPED:<path> warning) — claim_done must be
	// idempotent on retry, unlike artifact.add_pin which rejects dups
	// to surface accidental re-adds.
	pinsStored, pinsExplicitCount, pinsAutopinCount := 0, 0, 0
	if len(pinSources) > 0 {
		nonDup := make([]ArtifactPinInput, 0, len(pinSources))
		seen := map[string]struct{}{}
		for _, sourced := range pinSources {
			pin := sourced.Pin
			resolvedRepoID, _, rerr := pgit.ResolvePinRepoID(ctx, tx, projectID, pin.RepoID, pin.Repo, pin.Path, deps.RepoRoot)
			if rerr != nil {
				return taskClaimDoneOutput{}, fmt.Errorf("resolve claim_done pin repo: %w", rerr)
			}
			if resolvedRepoID != "" {
				pin.RepoID = resolvedRepoID
			}
			dup, derr := pinDuplicateExists(ctx, tx, artifactID, resolvedRepoID, pin)
			if derr != nil {
				return taskClaimDoneOutput{}, fmt.Errorf("check claim_done pin duplicate: %w", derr)
			}
			if dup {
				pinWarnings = append(pinWarnings, "PIN_DUPLICATE_SKIPPED:"+pin.Path)
				continue
			}
			key := claimDonePinDedupeKey(resolvedRepoID, pin)
			if _, ok := seen[key]; ok {
				pinWarnings = append(pinWarnings, "PIN_DUPLICATE_SKIPPED:"+pin.Path)
				continue
			}
			seen[key] = struct{}{}
			if sourced.Source == claimDonePinSourceAutopin {
				pinsAutopinCount++
			} else {
				pinsExplicitCount++
			}
			nonDup = append(nonDup, pin)
		}
		stored, repoWarnings, ierr := insertPins(ctx, tx, projectID, artifactID, nonDup, deps.RepoRoot)
		if ierr != nil {
			return taskClaimDoneOutput{}, ierr
		}
		pinsStored = stored
		pinWarnings = append(pinWarnings, repoWarnings...)
	}
	if pinsStored > 0 {
		shapePayload["pins_stored"] = pinsStored
	}
	shapePayload["pins_explicit_count"] = pinsExplicitCount
	shapePayload["pins_autopin_count"] = pinsAutopinCount
	shapePayload["evidence_edges_stored"] = evidenceEdgesStored
	shapePayloadJSON, err := json.Marshal(shapePayload)
	if err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("marshal shape_payload: %w", err)
	}

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
			'prev_status',              $8::text,
			'commit_sha',               NULLIF($9::text, ''),
			'pins_stored',              $10::int,
			'pins_explicit_count',      $11::int,
			'pins_autopin_count',       $12::int,
			'pin_strategy',             $13::text,
			'evidence_edges_stored',    $14::int
		))
	`, projectID, artifactID, newRev, currentSlug, effAuthorID, commitMsg, changedCount, prevStatus, commitSHA, pinsStored, pinsExplicitCount, pinsAutopinCount, pinStrategy, evidenceEdgesStored); err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("emit task_claimed_done event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return taskClaimDoneOutput{}, fmt.Errorf("commit task.claim_done: %w", err)
	}

	var outWarnings []string
	if len(pinWarnings) > 0 {
		outWarnings = pinWarnings
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
		CommitSHA:              commitSHA,
		PinStrategy:            pinStrategy,
		ChangedPathsAllowlist:  changedPathsAllowlist,
		PinsStored:             pinsStored,
		PinsExplicitCount:      pinsExplicitCount,
		PinsAutopinCount:       pinsAutopinCount,
		EvidenceEdgesStored:    evidenceEdgesStored,
		Warnings:               outWarnings,
	}, nil
}

const (
	claimDonePinSourceExplicit = "explicit"
	claimDonePinSourceAutopin  = "autopin"
)

type claimDonePinWithSource struct {
	Pin    ArtifactPinInput
	Source string
}

// validateClaimDoneCommitSHA normalises and length/charset-checks the
// optional commit_sha argument. Empty / whitespace-only input returns
// ("", "", "") so callers can treat "no commit attached" the same as the
// pre-extension contract. Bad input returns the trimmed value plus a
// stable error code so the handler can short-circuit to not_ready.
//
// We accept 7-64 hex chars: 7 covers `git rev-parse --short`, 40 covers
// SHA-1, 64 covers SHA-256 (git's future objformat). Anything outside
// that band is almost certainly a paste error.
func validateClaimDoneCommitSHA(raw string) (string, string, string) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", "", ""
	}
	runeLen := utf8.RuneCountInString(v)
	if runeLen < 7 || runeLen > 64 {
		return v, "CLAIM_DONE_COMMIT_SHA_LENGTH_INVALID",
			fmt.Sprintf("commit_sha must be 7-64 chars (got %d)", runeLen)
	}
	for _, r := range v {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return v, "CLAIM_DONE_COMMIT_SHA_FORMAT_INVALID",
				"commit_sha must be hex (0-9 / a-f / A-F)"
		}
	}
	return v, "", ""
}

// prefixClaimDoneCommitMsg prepends "[<short>] " to commitMsg when
// commitSHA is non-empty. Short SHA is the first 8 hex chars (or the
// full SHA when shorter). When commitSHA is empty the message is
// returned unchanged so callers can pass it through unconditionally.
func prefixClaimDoneCommitMsg(commitMsg, commitSHA string) string {
	if commitSHA == "" {
		return commitMsg
	}
	short := commitSHA
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("[%s] %s", short, commitMsg)
}

func normalizeClaimDonePinStrategy(raw string) (string, string, string) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return claimDonePinStrategyAuto, "", ""
	}
	switch v {
	case claimDonePinStrategyAuto, claimDonePinStrategyAllowlist, claimDonePinStrategyExplicit:
		return v, "", ""
	default:
		return v, "CLAIM_DONE_PIN_STRATEGY_INVALID",
			"pin_strategy must be one of auto, allowlist, explicit"
	}
}

func normalizeClaimDoneChangedPathsAllowlist(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		path := normalizeClaimDoneChangedPath(value)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizeClaimDoneChangedPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	for strings.HasPrefix(path, "./") {
		path = strings.TrimPrefix(path, "./")
	}
	return path
}

func normalizeClaimDoneEvidenceArtifacts(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		ref := normalizeRef(value)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func buildClaimDoneAutoPins(ctx context.Context, deps Deps, projectID, commitSHA string, limit int, pinStrategy string, changedPathsAllowlist []string) ([]ArtifactPinInput, []string) {
	if strings.TrimSpace(commitSHA) == "" {
		return nil, nil
	}
	if pinStrategy == claimDonePinStrategyExplicit {
		return nil, nil
	}
	if limit <= 0 {
		limit = claimDoneAutopinDefaultLimit
	}
	repos, err := pgit.LoadProjectRepos(ctx, deps.DB, projectID)
	if err != nil {
		return nil, []string{"PINS_AUTOPIN_UNAVAILABLE:repo_lookup_failed"}
	}
	if len(repos) == 0 && strings.TrimSpace(deps.RepoRoot) != "" {
		repos = []pgit.Repo{{Name: "origin", LocalPaths: []string{deps.RepoRoot}}}
	}
	if len(repos) == 0 {
		return nil, []string{"PINS_AUTOPIN_UNAVAILABLE:no_repo_registered"}
	}

	provider := pgit.LocalGitProvider{}
	var lastWarning string
	for _, repo := range repos {
		if len(repo.LocalPaths) == 0 && strings.TrimSpace(deps.RepoRoot) != "" {
			repo.LocalPaths = []string{deps.RepoRoot}
		}
		files, err := provider.ChangedFiles(ctx, repo, commitSHA)
		if err == nil {
			return claimDoneAutoPinsFromChangedFiles(files, commitSHA, repo, limit, changedPathsAllowlist)
		}
		switch {
		case errors.Is(err, pgit.ErrCommitNotFound):
			lastWarning = "PINS_AUTOPIN_UNAVAILABLE:commit_not_found"
		case errors.Is(err, pgit.ErrNoProviderForRepo):
			lastWarning = "PINS_AUTOPIN_UNAVAILABLE:no_local_repo"
		default:
			lastWarning = "PINS_AUTOPIN_UNAVAILABLE:git_diff_failed"
		}
	}
	if lastWarning == "" {
		lastWarning = "PINS_AUTOPIN_UNAVAILABLE:git_diff_failed"
	}
	return nil, []string{lastWarning}
}

func claimDoneAutoPinsFromChangedFiles(files []pgit.ChangedFile, commitSHA string, repo pgit.Repo, limit int, changedPathsAllowlist []string) ([]ArtifactPinInput, []string) {
	if limit <= 0 {
		limit = claimDoneAutopinDefaultLimit
	}
	allowlisted := map[string]struct{}{}
	for _, path := range changedPathsAllowlist {
		path = normalizeClaimDoneChangedPath(path)
		if path != "" {
			allowlisted[path] = struct{}{}
		}
	}
	out := make([]ArtifactPinInput, 0, min(len(files), limit))
	warnings := []string{}
	validFiles := 0
	for _, file := range files {
		path := normalizeClaimDoneChangedPath(file.Path)
		if path == "" {
			continue
		}
		if len(allowlisted) > 0 {
			if _, ok := allowlisted[path]; !ok {
				continue
			}
		}
		validFiles++
		if len(out) >= limit {
			continue
		}
		out = append(out, ArtifactPinInput{
			Kind:      pinmodel.NormalizeKind("", path),
			RepoID:    strings.TrimSpace(repo.ID),
			Repo:      strings.TrimSpace(repo.Name),
			CommitSHA: commitSHA,
			Path:      path,
		})
	}
	if validFiles > len(out) {
		warnings = append(warnings, fmt.Sprintf("PINS_AUTOPIN_TRUNCATED:%d", validFiles-len(out)))
	}
	return out, warnings
}

func claimDonePinDedupeKey(repoID string, pin ArtifactPinInput) string {
	kind := pinmodel.NormalizeKind(pin.Kind, pin.Path)
	var commit string
	var linesStart, linesEnd int
	if addPinUsesGitCoordinate(kind) {
		commit = strings.TrimSpace(pin.CommitSHA)
		linesStart = pin.LinesStart
		linesEnd = pin.LinesEnd
	}
	return strings.Join([]string{
		strings.TrimSpace(repoID),
		commit,
		strings.TrimSpace(pin.Path),
		fmt.Sprint(linesStart),
		fmt.Sprint(linesEnd),
	}, "\x00")
}

// validateClaimDonePins runs the same path/url/line-range checks as
// artifact.add_pin's normalizeAddPinInput across the whole pins[]
// array, normalising kinds in place. Returns the first failing index
// + a stable error code so the handler can surface it without a
// half-applied state. nil / empty input returns (nil, "", "").
func validateClaimDonePins(pins []ArtifactPinInput) ([]ArtifactPinInput, string, string) {
	if len(pins) == 0 {
		return nil, "", ""
	}
	out := make([]ArtifactPinInput, len(pins))
	copy(out, pins)
	for i := range out {
		normalised, code, msg := normalizeAddPinInput(out[i])
		if code != "" {
			return nil, "CLAIM_DONE_PIN_INVALID:" + code, fmt.Sprintf("pins[%d]: %s", i, msg)
		}
		out[i] = normalised
	}
	return out, "", ""
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
