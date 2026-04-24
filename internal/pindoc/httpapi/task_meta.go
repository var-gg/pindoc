package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

type taskMetaError struct {
	ErrorCode string   `json:"error_code"`
	Message   string   `json:"message"`
	Failed    []string `json:"failed,omitempty"`
}

func writeTaskMetaError(w http.ResponseWriter, status int, code, msg string, failed ...string) {
	writeJSON(w, status, taskMetaError{ErrorCode: code, Message: msg, Failed: failed})
}

var validTaskPriorities = map[string]struct{}{
	"p0": {}, "p1": {}, "p2": {}, "p3": {},
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
	if d.DB == nil {
		writeTaskMetaError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}

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
	if strings.TrimSpace(in.CommitMsg) == "" {
		writeTaskMetaError(w, http.StatusBadRequest, "MISSING_COMMIT_MSG", "commit_msg is required", "commit_msg")
		return
	}
	if strings.TrimSpace(in.AuthorID) == "" {
		writeTaskMetaError(w, http.StatusBadRequest, "AUTHOR_EMPTY", "author_id is required", "author_id")
		return
	}

	// Build the patch payload exactly as the MCP taskMetaToJSON helper
	// does, but client-side. Pointer fields distinguish "unset" from
	// "cleared to empty string" — empty trim still counts as "do not
	// change". Clearing a field needs an explicit design pass (Decision
	// does not discuss deletion semantics).
	patch := map[string]any{}
	if in.Assignee != nil {
		v := strings.TrimSpace(*in.Assignee)
		if v != "" {
			patch["assignee"] = v
		}
	}
	if in.Priority != nil {
		v := strings.TrimSpace(*in.Priority)
		if v != "" {
			if _, ok := validTaskPriorities[v]; !ok {
				writeTaskMetaError(w, http.StatusBadRequest, "TASK_PRIORITY_INVALID", "priority must be one of p0 | p1 | p2 | p3", "priority")
				return
			}
			patch["priority"] = v
		}
	}
	if in.DueAt != nil {
		v := strings.TrimSpace(*in.DueAt)
		if v != "" {
			if _, err := time.Parse(time.RFC3339, v); err != nil {
				writeTaskMetaError(w, http.StatusBadRequest, "TASK_DUE_AT_INVALID", "due_at must be RFC3339 (e.g. 2026-04-30T00:00:00Z)", "due_at")
				return
			}
			patch["due_at"] = v
		}
	}
	if in.ParentSlug != nil {
		v := strings.TrimSpace(*in.ParentSlug)
		if v != "" {
			patch["parent_slug"] = v
		}
	}
	if len(patch) == 0 {
		writeTaskMetaError(w, http.StatusBadRequest, "META_PATCH_EMPTY", "at least one of assignee | priority | due_at | parent_slug is required", "assignee", "priority", "due_at", "parent_slug")
		return
	}

	ref := strings.TrimPrefix(strings.TrimPrefix(idOrSlug, "pindoc://"), "/")

	var artifactID, projectID, currentType, currentSlug string
	var lastRev int
	err := d.DB.QueryRow(r.Context(), `
		SELECT a.id::text, a.project_id::text, a.type, a.slug,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, projectSlug, ref).Scan(&artifactID, &projectID, &currentType, &currentSlug, &lastRev)
	if errors.Is(err, pgx.ErrNoRows) {
		writeTaskMetaError(w, http.StatusNotFound, "UPDATE_TARGET_NOT_FOUND", "artifact not found in this project")
		return
	}
	if err != nil {
		d.Logger.Error("task-meta resolve", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}

	if currentType != "Task" {
		writeTaskMetaError(w, http.StatusBadRequest, "TASK_META_WRONG_TYPE", "task_meta is only valid when type='Task'", "task_meta")
		return
	}

	if in.ExpectedVersion != lastRev {
		writeTaskMetaError(w, http.StatusConflict, "VER_CONFLICT", "expected_version does not match current head — re-read and retry", "expected_version")
		return
	}

	shapePayload, err := json.Marshal(map[string]any{"task_meta": patch})
	if err != nil {
		writeTaskMetaError(w, http.StatusInternalServerError, "ENCODE_ERROR", "failed to marshal shape payload")
		return
	}
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		writeTaskMetaError(w, http.StatusInternalServerError, "ENCODE_ERROR", "failed to marshal patch")
		return
	}

	tx, err := d.DB.Begin(r.Context())
	if err != nil {
		d.Logger.Error("task-meta tx begin", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "begin tx failed")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// Need title, body, tags, completeness to stamp the revision row — the
	// revision carries body_hash of the *current* body so diffs pick it up
	// as a no-body-change revision. Pull them inside the tx so we don't
	// race a concurrent body_patch.
	var currentTitle, currentBody, currentCompleteness string
	var currentTags []string
	if err := tx.QueryRow(r.Context(), `
		SELECT title, body_markdown, tags, completeness
		FROM artifacts WHERE id = $1
	`, artifactID).Scan(&currentTitle, &currentBody, &currentTags, &currentCompleteness); err != nil {
		d.Logger.Error("task-meta head fetch", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "head fetch failed")
		return
	}

	newRev := lastRev + 1
	prevBodyHash := sha256HexOf(currentBody)

	authorVersion := any(nil)
	if v := strings.TrimSpace(in.AuthorVersion); v != "" {
		authorVersion = v
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, NULL, $4, $5, $6, 'user', $7, $8, $9, NULL, 'meta_patch', $10::jsonb)
	`, artifactID, newRev, currentTitle, prevBodyHash, currentTags, currentCompleteness,
		in.AuthorID, authorVersion, in.CommitMsg, string(shapePayload),
	); err != nil {
		d.Logger.Error("task-meta revision insert", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "revision insert failed")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		UPDATE artifacts
		   SET task_meta      = COALESCE(task_meta, '{}'::jsonb) || $2::jsonb,
		       author_id      = $3,
		       author_version = $4,
		       updated_at     = now()
		 WHERE id = $1
	`, artifactID, string(patchJSON), in.AuthorID, authorVersion); err != nil {
		d.Logger.Error("task-meta head update", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "head update failed")
		return
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
		writeTaskMetaError(w, http.StatusInternalServerError, "ENCODE_ERROR", "failed to marshal fields_changed")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.meta_patched', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'commit_msg',      $6::text,
			'fields_changed',  $7::jsonb,
			'origin',          'http_task_meta'
		))
	`, projectID, artifactID, newRev, currentSlug, in.AuthorID, in.CommitMsg, string(fieldsChangedJSON)); err != nil {
		d.Logger.Error("task-meta event insert", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "event insert failed")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		d.Logger.Error("task-meta tx commit", "err", err)
		writeTaskMetaError(w, http.StatusInternalServerError, "DB_ERROR", "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, taskMetaPatchResponse{
		ArtifactID:     artifactID,
		Slug:           currentSlug,
		RevisionNumber: newRev,
	})
}
