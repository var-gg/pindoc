package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type taskAcceptanceTransitionInput struct {
	ProjectSlug  string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	TaskIDOrSlug string `json:"task_id_or_slug" jsonschema:"Task artifact UUID, slug, or pindoc:// URL"`

	CheckboxIndex   *int   `json:"checkbox_index,omitempty" jsonschema:"single checkbox index; optional shorthand for checkbox_indices=[N]"`
	CheckboxIndices []int  `json:"checkbox_indices,omitempty" jsonschema:"0-based checkbox indices across all 4-state acceptance checkboxes"`
	NewState        string `json:"new_state" jsonschema:"one of '[ ]' | '[x]' | '[~]' | '[-]'"`
	Reason          string `json:"reason,omitempty" jsonschema:"required for [~] and [-]; stored in revision shape_payload"`

	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"required optimistic lock; current artifact revision number"`
	CommitMsg       string `json:"commit_msg,omitempty" jsonschema:"optional one-line rationale; auto-filled when omitted"`
	AuthorID        string `json:"author_id,omitempty" jsonschema:"override author display label; defaults to server agent_id"`
	AuthorVersion   string `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`
}

type taskAcceptanceTransitionOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`

	Failed          []string             `json:"failed,omitempty"`
	ErrorCodes      []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist       []string             `json:"checklist,omitempty"`
	ChecklistItems  []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale   string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`
	PatchableFields []string             `json:"patchable_fields,omitempty"`

	ArtifactID      string `json:"artifact_id,omitempty"`
	Slug            string `json:"slug,omitempty"`
	AgentRef        string `json:"agent_ref,omitempty"`
	RevisionNumber  int    `json:"revision_number,omitempty"`
	HumanURL        string `json:"human_url,omitempty"`
	HumanURLAbs     string `json:"human_url_abs,omitempty"`
	NewStatus       string `json:"new_status,omitempty"`
	ResolvedCount   int    `json:"resolved_count,omitempty"`
	TotalCount      int    `json:"total_count,omitempty"`
	TransitionCount int    `json:"transition_count,omitempty"`
	ToolsetVersion  string `json:"toolset_version,omitempty"`
}

type appliedAcceptanceTransition struct {
	CheckboxIndex int    `json:"checkbox_index"`
	FromState     string `json:"from_state"`
	NewState      string `json:"new_state"`
	Reason        string `json:"reason,omitempty"`
}

// RegisterTaskAcceptanceTransition wires pindoc.task.acceptance.transition.
// It is the dedicated acceptance lane for Task artifacts: one call can
// flip N checkbox markers, emits exactly one revision, and promotes open
// Tasks to claimed_done in the same transaction once every acceptance item
// is resolved.
func RegisterTaskAcceptanceTransition(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.task.acceptance.transition",
			Description: "Transition one or more Task acceptance checkboxes in a single revision. Pass checkbox_indices plus new_state; expected_version is required. When the final unchecked item is resolved, task_meta.status automatically becomes claimed_done.",
		},
		func(ctx context.Context, p *auth.Principal, in taskAcceptanceTransitionInput) (*sdk.CallToolResult, taskAcceptanceTransitionOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("task.acceptance.transition: %w", err)
			}
			ref := normalizeRef(in.TaskIDOrSlug)
			if ref == "" {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       "ACCEPT_TRANSITION_MISSING_TASK",
					Failed:          []string{"ACCEPT_TRANSITION_MISSING_TASK"},
					Checklist:       []string{"task_id_or_slug is required"},
					PatchableFields: []string{"task_id_or_slug"},
				}, nil
			}

			indices, code := normalizeTransitionIndices(in.CheckboxIndex, in.CheckboxIndices)
			if code != "" {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       code,
					Failed:          []string{code},
					Checklist:       []string{transitionIndexChecklist(code)},
					PatchableFields: []string{"checkbox_index", "checkbox_indices"},
				}, nil
			}

			var (
				artifactID, projectID, currentBody, currentTitle, currentType, currentSlug string
				currentTags                                                                []string
				currentCompleteness                                                        string
				currentTaskMetaRaw                                                         []byte
				lastRev                                                                    int
			)
			err = deps.DB.QueryRow(ctx, `
				SELECT a.id::text, a.project_id::text, a.body_markdown, a.title, a.type, a.slug,
				       a.tags, a.completeness, a.task_meta,
				       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
				LIMIT 1
			`, scope.ProjectSlug, ref).Scan(
				&artifactID, &projectID, &currentBody, &currentTitle, &currentType, &currentSlug,
				&currentTags, &currentCompleteness, &currentTaskMetaRaw, &lastRev,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       "ACCEPT_TRANSITION_TASK_NOT_FOUND",
					Failed:          []string{"ACCEPT_TRANSITION_TASK_NOT_FOUND"},
					Checklist:       []string{fmt.Sprintf("Task %q not found in project %q", in.TaskIDOrSlug, scope.ProjectSlug)},
					PatchableFields: []string{"task_id_or_slug"},
				}, nil
			}
			if err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("resolve task: %w", err)
			}
			if currentType != "Task" {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       "ACCEPT_TRANSITION_NOT_A_TASK",
					Failed:          []string{"ACCEPT_TRANSITION_NOT_A_TASK"},
					Checklist:       []string{fmt.Sprintf("target %q has type=%s; acceptance transition accepts Task artifacts only", currentSlug, currentType)},
					PatchableFields: []string{"task_id_or_slug"},
				}, nil
			}
			if in.ExpectedVersion == nil {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       "NEED_VER",
					Failed:          []string{"NEED_VER"},
					Checklist:       []string{fmt.Sprintf("expected_version is required on the acceptance transition path (current head = %d).", lastRev)},
					PatchableFields: []string{"expected_version"},
					ArtifactID:      artifactID,
					Slug:            currentSlug,
				}, nil
			}
			if *in.ExpectedVersion != lastRev {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       "VER_CONFLICT",
					Failed:          []string{"VER_CONFLICT"},
					Checklist:       []string{fmt.Sprintf("expected_version=%d is stale; current head=%d.", *in.ExpectedVersion, lastRev)},
					PatchableFields: []string{"expected_version"},
					ArtifactID:      artifactID,
					Slug:            currentSlug,
				}, nil
			}

			newBody, applied, applyCode := applyAcceptanceTransitions(currentBody, indices, in.NewState, in.Reason)
			if applyCode != "" {
				return nil, taskAcceptanceTransitionOutput{
					Status:          "not_ready",
					ErrorCode:       applyCode,
					Failed:          []string{applyCode},
					Checklist:       []string{acceptanceTransitionChecklist(deps.UserLanguage, applyCode)},
					PatchableFields: []string{"checkbox_indices", "new_state", "reason"},
					ArtifactID:      artifactID,
					Slug:            currentSlug,
				}, nil
			}

			autoClaimedDone := shouldAutoClaimDone(currentType, currentTaskMetaRaw, newBody)
			resolved, total := countAcceptanceResolution(newBody)
			newRev := lastRev + 1
			effAuthorID := strings.TrimSpace(in.AuthorID)
			if effAuthorID == "" {
				effAuthorID = p.AgentID
			}
			if effAuthorID == "" {
				effAuthorID = "unknown"
			}
			commitMsg := strings.TrimSpace(in.CommitMsg)
			if commitMsg == "" {
				commitMsg = fmt.Sprintf("acceptance %s on %d item(s)", strings.TrimSpace(in.NewState), len(applied))
			}
			shapePayload, err := json.Marshal(map[string]any{
				"transitions":       applied,
				"auto_claimed_done": autoClaimedDone,
				"resolved_count":    resolved,
				"total_count":       total,
			})
			if err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("marshal shape payload: %w", err)
			}

			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			if _, err := tx.Exec(ctx, `
				INSERT INTO artifact_revisions (
					artifact_id, revision_number, title, body_markdown, body_hash, tags,
					completeness, author_kind, author_id, author_version, commit_msg,
					source_session_ref, revision_shape, shape_payload
				) VALUES ($1, $2, $3, $4, $5, $6, $7, 'agent', $8, $9, $10, $11, 'acceptance_transition', $12::jsonb)
			`, artifactID, newRev, currentTitle, newBody, bodyHash(newBody), currentTags,
				currentCompleteness, effAuthorID, nullIfEmpty(in.AuthorVersion), commitMsg,
				buildSourceSessionRef(p, artifactProposeInput{AuthorID: effAuthorID}), string(shapePayload),
			); err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("insert revision: %w", err)
			}

			var publishedAt time.Time
			if err := tx.QueryRow(ctx, `
				UPDATE artifacts
				   SET body_markdown = $2,
				       author_id = $3,
				       author_version = $4,
				       task_meta = CASE
				           WHEN $5::bool THEN jsonb_set(COALESCE(task_meta, '{}'::jsonb), '{status}', '"claimed_done"')
				           ELSE task_meta
				       END,
				       updated_at = now()
				 WHERE id = $1
				RETURNING COALESCE(published_at, now())
			`, artifactID, newBody, effAuthorID, nullIfEmpty(in.AuthorVersion), autoClaimedDone).Scan(&publishedAt); err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("update task: %w", err)
			}

			if _, err := tx.Exec(ctx, `DELETE FROM artifact_chunks WHERE artifact_id = $1`, artifactID); err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("purge chunks: %w", err)
			}
			if deps.Embedder != nil {
				if err := embedAndStoreChunks(ctx, tx, deps.Embedder, artifactID, currentTitle, newBody); err != nil {
					deps.Logger.Warn("re-embed failed after acceptance transition",
						"artifact_id", artifactID, "err", err)
				}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1, 'task.acceptance_transitioned', $2, jsonb_build_object(
					'revision_number', $3::int,
					'slug', $4::text,
					'author_id', $5::text,
					'transition_count', $6::int,
					'auto_claimed_done', $7::bool
				))
			`, projectID, artifactID, newRev, currentSlug, effAuthorID, len(applied), autoClaimedDone); err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("event insert: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, taskAcceptanceTransitionOutput{}, fmt.Errorf("commit: %w", err)
			}

			newStatus := taskStatusFromJSON(currentTaskMetaRaw)
			if autoClaimedDone {
				newStatus = "claimed_done"
			}
			return nil, taskAcceptanceTransitionOutput{
				Status:          "accepted",
				ArtifactID:      artifactID,
				Slug:            currentSlug,
				AgentRef:        "pindoc://" + currentSlug,
				RevisionNumber:  newRev,
				HumanURL:        HumanURL(scope.ProjectSlug, scope.ProjectLocale, currentSlug),
				HumanURLAbs:     AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, currentSlug),
				NewStatus:       newStatus,
				ResolvedCount:   resolved,
				TotalCount:      total,
				TransitionCount: len(applied),
			}, nil
		},
	)
}

func normalizeTransitionIndices(single *int, bulk []int) ([]int, string) {
	var out []int
	if single != nil {
		out = append(out, *single)
	}
	out = append(out, bulk...)
	if len(out) == 0 {
		return nil, "ACCEPT_TRANSITION_INDEX_REQUIRED"
	}
	seen := map[int]bool{}
	for _, idx := range out {
		if idx < 0 {
			return nil, "ACCEPT_TRANSITION_INDEX_NEGATIVE"
		}
		if seen[idx] {
			return nil, "ACCEPT_TRANSITION_DUPLICATE_INDEX"
		}
		seen[idx] = true
	}
	return out, ""
}

func applyAcceptanceTransitions(body string, indices []int, newState, reason string) (string, []appliedAcceptanceTransition, string) {
	next := body
	applied := make([]appliedAcceptanceTransition, 0, len(indices))
	for _, idx := range indices {
		i := idx
		in := &AcceptanceTransitionInput{
			CheckboxIndex: &i,
			NewState:      newState,
			Reason:        reason,
		}
		rewritten, from, code := applyAcceptanceTransition(next, in)
		if code != "" {
			return "", nil, code
		}
		next = rewritten
		applied = append(applied, appliedAcceptanceTransition{
			CheckboxIndex: idx,
			FromState:     markerState(from),
			NewState:      strings.TrimSpace(newState),
			Reason:        strings.TrimSpace(reason),
		})
	}
	return next, applied, ""
}

func markerState(m byte) string {
	switch m {
	case 'x', 'X':
		return "[x]"
	case '~':
		return "[~]"
	case '-':
		return "[-]"
	default:
		return "[ ]"
	}
}

func transitionIndexChecklist(code string) string {
	switch code {
	case "ACCEPT_TRANSITION_DUPLICATE_INDEX":
		return "checkbox_indices must not contain duplicate indexes."
	default:
		return acceptanceTransitionChecklist("en", code)
	}
}
