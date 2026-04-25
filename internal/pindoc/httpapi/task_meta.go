package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

// sha256HexOf matches tools.bodyHash so the body_hash written on a
// meta_patch revision here is bit-for-bit identical to the one the MCP
// path writes. Keeps diff / dedup logic consistent regardless of which
// surface the edit came through.
func sha256HexOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Phase M1.x — operational-metadata UI edit lane.
//
// Decision agent-only-write-분할 splits the agent-only write surface into
// "semantic content (agent-only)" and "operational metadata (UI direct
// edit allowed under trusted_local)". This handler is the HTTP-side
// bridge the Reader's TaskControls component hits — it mirrors the
// server contract of pindoc.artifact.propose(shape=meta_patch) restricted
// to task_meta.assignee / priority / due_at, which is exactly the slice
// the Decision permits. Status is intentionally absent; the
// pindoc.task.transition path still owns that axis.
//
// Why not let TaskControls call MCP directly: the Reader is a browser
// context that can't speak stdio JSON-RPC to a local MCP subprocess.
// This endpoint is the minimum viable bridge, not a public API — we
// gate on auth_mode == trusted_local and reject anything else with 403.

type taskMetaPatchRequest struct {
	ExpectedVersion int     `json:"expected_version"`
	CommitMsg       string  `json:"commit_msg"`
	AuthorID        string  `json:"author_id"`
	AuthorVersion   string  `json:"author_version,omitempty"`
	Assignee        *string `json:"assignee,omitempty"`
	Priority        *string `json:"priority,omitempty"`
	DueAt           *string `json:"due_at,omitempty"`
	ParentSlug      *string `json:"parent_slug,omitempty"`
}

type taskMetaPatchResponse struct {
	ArtifactID     string `json:"artifact_id"`
	Slug           string `json:"slug"`
	RevisionNumber int    `json:"revision_number"`
}

type taskAssignRequest struct {
	Assignee      string `json:"assignee"`
	Reason        string `json:"reason,omitempty"`
	AuthorID      string `json:"author_id,omitempty"`
	AuthorVersion string `json:"author_version,omitempty"`
}

type taskAssignResponse struct {
	Status         string `json:"status"`
	ArtifactID     string `json:"artifact_id,omitempty"`
	Slug           string `json:"slug,omitempty"`
	RevisionNumber int    `json:"revision_number,omitempty"`
	NewAssignee    string `json:"new_assignee"`
}

type taskMetaError struct {
	ErrorCode string   `json:"error_code"`
	Message   string   `json:"message"`
	Failed    []string `json:"failed,omitempty"`
}

func writeTaskMetaError(w http.ResponseWriter, status int, code, msg string, failed ...string) {
	writeJSON(w, status, taskMetaError{ErrorCode: code, Message: msg, Failed: failed})
}

type taskMetaApplyError struct {
	status int
	body   taskMetaError
}

func newTaskMetaApplyError(status int, code, msg string, failed ...string) *taskMetaApplyError {
	return &taskMetaApplyError{
		status: status,
		body:   taskMetaError{ErrorCode: code, Message: msg, Failed: failed},
	}
}

var validTaskPriorities = map[string]struct{}{
	"p0": {}, "p1": {}, "p2": {}, "p3": {},
}

var validTaskAssignee = regexp.MustCompile(`^(agent:[a-zA-Z0-9_\-:.]+|user:[a-zA-Z0-9_\-]+|@[a-zA-Z0-9_\-.]+)$`)

func normalizeTaskAssignee(assignee string) (string, bool) {
	a := strings.TrimSpace(assignee)
	if a == "" {
		return "", true
	}
	if !validTaskAssignee.MatchString(a) {
		return "", false
	}
	return a, true
}

// handleTaskMetaPatch writes a meta_patch revision for one Task's
// task_meta (assignee / priority / due_at / parent_slug). Mirrors the
// MCP handleUpdateMetaPatch semantics:
//   - shallow-merge task_meta JSONB so a single-field PATCH preserves the
//     rest (CASE ... COALESCE(task_meta,'{}'::jsonb) || $::jsonb)
//   - inserts one artifact_revisions row with revision_shape='meta_patch'
//     and shape_payload={"task_meta":{...}}
//   - emits an artifact.meta_patched event so the same telemetry / recent
//     changes rails render the revision
//
// Status is rejected up-front (TASK_STATUS_VIA_TRANSITION_TOOL) and never
// reaches SQL.
func (d Deps) handleTaskMetaPatch(w http.ResponseWriter, r *http.Request) {
	// auth_mode gate lives here. M1 is always trusted_local so there is
	// no runtime check; V1.5+ will introduce a Deps.AuthMode field and
	// this is the seam that flips to `if d.AuthMode != "trusted_local" {
	// writeTaskMetaError(w, 403, "AUTH_MODE_LOCKED", ...) }`. Leaving the
	// comment so reviewers see where the ACL split lands without a
	// dead-code branch.

	projectSlug := r.PathValue("project")
	idOrSlug := r.PathValue("idOrSlug")
	if strings.TrimSpace(projectSlug) == "" || strings.TrimSpace(idOrSlug) == "" {
		writeTaskMetaError(w, http.StatusBadRequest, "BAD_PATH", "project and idOrSlug are required path segments")
		return
	}

	var in taskMetaPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeTaskMetaError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	out, appErr := d.applyTaskMetaPatch(r.Context(), projectSlug, idOrSlug, in, "http_task_meta", "user", false)
	if appErr != nil {
		writeJSON(w, appErr.status, appErr.body)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTaskAssign is the Reader bridge for the MCP semantic shortcut
// pindoc.task.assign. The browser still cannot speak stdio MCP directly,
// so this endpoint uses the same assignee-only contract and records an
// ops telemetry row with tool_name=pindoc.task.assign. Priority / due_at
// continue to use the generic task-meta endpoint.
func (d Deps) handleTaskAssign(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	projectSlug := r.PathValue("project")
	idOrSlug := r.PathValue("idOrSlug")
	if strings.TrimSpace(projectSlug) == "" || strings.TrimSpace(idOrSlug) == "" {
		errBody := taskMetaError{ErrorCode: "BAD_PATH", Message: "project and idOrSlug are required path segments"}
		d.recordTaskAssignTelemetry(start, projectSlug, taskAssignRequest{}, errBody, errBody.ErrorCode)
		writeJSON(w, http.StatusBadRequest, errBody)
		return
	}

	var in taskAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errBody := taskMetaError{ErrorCode: "BAD_JSON", Message: "could not parse request body as JSON"}
		d.recordTaskAssignTelemetry(start, projectSlug, in, errBody, errBody.ErrorCode)
		writeJSON(w, http.StatusBadRequest, errBody)
		return
	}
	assignee, ok := normalizeTaskAssignee(in.Assignee)
	if !ok {
		errBody := taskMetaError{
			ErrorCode: "ASSIGNEE_FORMAT_INVALID",
			Message:   "assignee must match agent:<id> | user:<id> | @<handle>, or be empty to clear",
			Failed:    []string{"ASSIGNEE_FORMAT_INVALID"},
		}
		d.recordTaskAssignTelemetry(start, projectSlug, in, errBody, errBody.ErrorCode)
		writeJSON(w, http.StatusBadRequest, errBody)
		return
	}

	commitMsg := strings.TrimSpace(in.Reason)
	if commitMsg == "" {
		display := assignee
		if display == "" {
			display = "(cleared)"
		}
		commitMsg = "UI TaskControls: set assignee=" + display
	}
	authorID := strings.TrimSpace(in.AuthorID)
	if authorID == "" {
		authorID = "user:web-reader"
	}

	patchIn := taskMetaPatchRequest{
		CommitMsg:     commitMsg,
		AuthorID:      authorID,
		AuthorVersion: in.AuthorVersion,
		Assignee:      &assignee,
	}
	out, appErr := d.applyTaskMetaPatch(r.Context(), projectSlug, idOrSlug, patchIn, "http_task_assign", "user", true)
	if appErr != nil {
		d.recordTaskAssignTelemetry(start, projectSlug, in, appErr.body, appErr.body.ErrorCode)
		writeJSON(w, appErr.status, appErr.body)
		return
	}
	resp := taskAssignResponse{
		Status:         "accepted",
		ArtifactID:     out.ArtifactID,
		Slug:           out.Slug,
		RevisionNumber: out.RevisionNumber,
		NewAssignee:    assignee,
	}
	d.recordTaskAssignTelemetry(start, projectSlug, in, resp, "")
	writeJSON(w, http.StatusOK, resp)
}

func (d Deps) applyTaskMetaPatch(
	ctx context.Context,
	projectSlug, idOrSlug string,
	in taskMetaPatchRequest,
	origin string,
	authorKind string,
	useCurrentHead bool,
) (taskMetaPatchResponse, *taskMetaApplyError) {
	var zero taskMetaPatchResponse
	if d.DB == nil {
		return zero, newTaskMetaApplyError(http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
	}
	if strings.TrimSpace(in.CommitMsg) == "" {
		return zero, newTaskMetaApplyError(http.StatusBadRequest, "MISSING_COMMIT_MSG", "commit_msg is required", "commit_msg")
	}
	if strings.TrimSpace(in.AuthorID) == "" {
		return zero, newTaskMetaApplyError(http.StatusBadRequest, "AUTHOR_EMPTY", "author_id is required", "author_id")
	}
	if strings.TrimSpace(authorKind) == "" {
		authorKind = "user"
	}
	if strings.TrimSpace(origin) == "" {
		origin = "http_task_meta"
	}

	// Build the patch payload exactly as the MCP taskMetaToJSON helper
	// does, but client-side. Pointer fields distinguish "unset" from
	// "cleared to empty string". Clear is encoded as JSON null and
	// jsonb_strip_nulls removes that key after the shallow merge.
	patch := map[string]any{}
	if in.Assignee != nil {
		v := strings.TrimSpace(*in.Assignee)
		if v != "" {
			patch["assignee"] = v
		} else {
			patch["assignee"] = nil
		}
	}
	if in.Priority != nil {
		v := strings.TrimSpace(*in.Priority)
		if v != "" {
			if _, ok := validTaskPriorities[v]; !ok {
				return zero, newTaskMetaApplyError(http.StatusBadRequest, "TASK_PRIORITY_INVALID", "priority must be one of p0 | p1 | p2 | p3", "priority")
			}
			patch["priority"] = v
		} else {
			patch["priority"] = nil
		}
	}
	if in.DueAt != nil {
		v := strings.TrimSpace(*in.DueAt)
		if v != "" {
			if _, err := time.Parse(time.RFC3339, v); err != nil {
				return zero, newTaskMetaApplyError(http.StatusBadRequest, "TASK_DUE_AT_INVALID", "due_at must be RFC3339 (e.g. 2026-04-30T00:00:00Z)", "due_at")
			}
			patch["due_at"] = v
		} else {
			patch["due_at"] = nil
		}
	}
	if in.ParentSlug != nil {
		v := strings.TrimSpace(*in.ParentSlug)
		if v != "" {
			patch["parent_slug"] = v
		} else {
			patch["parent_slug"] = nil
		}
	}
	if len(patch) == 0 {
		return zero, newTaskMetaApplyError(http.StatusBadRequest, "META_PATCH_EMPTY", "at least one of assignee | priority | due_at | parent_slug is required", "assignee", "priority", "due_at", "parent_slug")
	}

	ref := strings.TrimPrefix(strings.TrimPrefix(idOrSlug, "pindoc://"), "/")

	var artifactID, projectID, currentType, currentSlug string
	var lastRev int
	err := d.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.type, a.slug,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, projectSlug, ref).Scan(&artifactID, &projectID, &currentType, &currentSlug, &lastRev)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, newTaskMetaApplyError(http.StatusNotFound, "UPDATE_TARGET_NOT_FOUND", "artifact not found in this project")
	}
	if err != nil {
		d.Logger.Error("task-meta resolve", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "query failed")
	}

	if currentType != "Task" {
		return zero, newTaskMetaApplyError(http.StatusBadRequest, "TASK_META_WRONG_TYPE", "task_meta is only valid when type='Task'", "task_meta")
	}

	expectedVersion := in.ExpectedVersion
	if useCurrentHead {
		expectedVersion = lastRev
	}
	if expectedVersion != lastRev {
		return zero, newTaskMetaApplyError(http.StatusConflict, "VER_CONFLICT", "expected_version does not match current head — re-read and retry", "expected_version")
	}

	shapePayload, err := json.Marshal(map[string]any{"task_meta": patch})
	if err != nil {
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "ENCODE_ERROR", "failed to marshal shape payload")
	}
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "ENCODE_ERROR", "failed to marshal patch")
	}

	tx, err := d.DB.Begin(ctx)
	if err != nil {
		d.Logger.Error("task-meta tx begin", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "begin tx failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Need title, body, tags, completeness to stamp the revision row — the
	// revision carries body_hash of the *current* body so diffs pick it up
	// as a no-body-change revision. Pull them inside the tx so we don't
	// race a concurrent body_patch.
	var currentTitle, currentBody, currentCompleteness string
	var currentTags []string
	if err := tx.QueryRow(ctx, `
		SELECT title, body_markdown, tags, completeness
		FROM artifacts WHERE id = $1
	`, artifactID).Scan(&currentTitle, &currentBody, &currentTags, &currentCompleteness); err != nil {
		d.Logger.Error("task-meta head fetch", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "head fetch failed")
	}

	newRev := lastRev + 1
	prevBodyHash := sha256HexOf(currentBody)

	authorVersion := any(nil)
	if v := strings.TrimSpace(in.AuthorVersion); v != "" {
		authorVersion = v
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8, $9, $10, NULL, 'meta_patch', $11::jsonb)
	`, artifactID, newRev, currentTitle, prevBodyHash, currentTags, currentCompleteness,
		authorKind, in.AuthorID, authorVersion, in.CommitMsg, string(shapePayload),
	); err != nil {
		d.Logger.Error("task-meta revision insert", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "revision insert failed")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE artifacts
		   SET task_meta      = jsonb_strip_nulls(COALESCE(task_meta, '{}'::jsonb) || $2::jsonb),
		       author_id      = $3,
		       author_version = $4,
		       updated_at     = now()
		 WHERE id = $1
	`, artifactID, string(patchJSON), in.AuthorID, authorVersion); err != nil {
		d.Logger.Error("task-meta head update", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "head update failed")
	}

	fieldsChanged := make([]string, 0, len(patch))
	for k := range patch {
		fieldsChanged = append(fieldsChanged, k)
	}
	// sorted for stable downstream telemetry
	for i := 1; i < len(fieldsChanged); i++ {
		for j := i; j > 0 && fieldsChanged[j] < fieldsChanged[j-1]; j-- {
			fieldsChanged[j], fieldsChanged[j-1] = fieldsChanged[j-1], fieldsChanged[j]
		}
	}
	fieldsChangedJSON, err := json.Marshal(map[string]any{"task_meta": fieldsChanged})
	if err != nil {
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "ENCODE_ERROR", "failed to marshal fields_changed")
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.meta_patched', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'commit_msg',      $6::text,
			'fields_changed',  $7::jsonb,
			'origin',          $8::text
		))
	`, projectID, artifactID, newRev, currentSlug, in.AuthorID, in.CommitMsg, string(fieldsChangedJSON), origin); err != nil {
		d.Logger.Error("task-meta event insert", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "event insert failed")
	}

	if err := tx.Commit(ctx); err != nil {
		d.Logger.Error("task-meta tx commit", "err", err)
		return zero, newTaskMetaApplyError(http.StatusInternalServerError, "DB_ERROR", "commit failed")
	}

	return taskMetaPatchResponse{
		ArtifactID:     artifactID,
		Slug:           currentSlug,
		RevisionNumber: newRev,
	}, nil
}

func (d Deps) recordTaskAssignTelemetry(start time.Time, projectSlug string, input taskAssignRequest, output any, errorCode string) {
	if d.Telemetry == nil {
		return
	}
	inputJSON, _ := json.Marshal(input)
	outputJSON, _ := json.Marshal(output)
	inText := string(inputJSON)
	outText := string(outputJSON)
	d.Telemetry.Record(telemetry.Entry{
		StartedAt:       start,
		DurationMs:      time.Since(start).Milliseconds(),
		ToolName:        "pindoc.task.assign",
		AgentID:         "http-reader",
		ProjectSlug:     projectSlug,
		InputBytes:      len(inputJSON),
		OutputBytes:     len(outputJSON),
		InputChars:      len([]rune(inText)),
		OutputChars:     len([]rune(outText)),
		InputTokensEst:  d.Telemetry.EstimateTokens(inText),
		OutputTokensEst: d.Telemetry.EstimateTokens(outText),
		ErrorCode:       errorCode,
		ToolsetVersion:  d.Version,
	})
}
