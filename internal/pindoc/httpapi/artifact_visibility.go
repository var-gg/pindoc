package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type artifactPatchRequest struct {
	Visibility *string
}

type artifactPatchResp struct {
	Status         string `json:"status"`
	Code           string `json:"code,omitempty"`
	ArtifactID     string `json:"artifact_id"`
	Slug           string `json:"slug"`
	Visibility     string `json:"visibility"`
	Affected       int    `json:"affected"`
	RevisionNumber int    `json:"revision_number,omitempty"`
}

type artifactPatchError struct {
	status    int
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func writeArtifactPatchError(w http.ResponseWriter, err artifactPatchError) {
	writeJSON(w, err.status, err)
}

func (d Deps) handleArtifactPatch(w http.ResponseWriter, r *http.Request) {
	patch, decodeErr := decodeArtifactPatch(r.Body)
	if decodeErr != nil {
		writeArtifactPatchError(w, *decodeErr)
		return
	}
	if d.DB == nil {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusServiceUnavailable,
			ErrorCode: "DB_UNAVAILABLE",
			Message:   "database pool not configured",
		})
		return
	}

	projectSlug := projectSlugFrom(r)
	idOrSlug := strings.TrimSpace(r.PathValue("idOrSlug"))
	if projectSlug == "" || idOrSlug == "" {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_PATH",
			Message:   "project and idOrSlug are required path segments",
		})
		return
	}

	principal := d.principalForRequest(r)
	if principal == nil {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusUnauthorized,
			ErrorCode: "AUTH_REQUIRED",
			Message:   "login is required",
		})
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlug)
	if err != nil {
		writeArtifactPatchError(w, artifactProjectAuthPatchError(err))
		return
	}
	if !scope.Can("write.project") {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusForbidden,
			ErrorCode: "PROJECT_OWNER_REQUIRED",
			Message:   "project owner role is required",
		})
		return
	}

	tx, err := d.DB.Begin(r.Context())
	if err != nil {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "begin transaction failed",
		})
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var out artifactPatchResp
	var currentVisibility, title, body, completeness, projectID string
	var tags []string
	var lastRev int
	err = tx.QueryRow(r.Context(), `
		SELECT a.id::text, a.project_id::text, a.slug, a.visibility,
		       a.title, a.body_markdown, a.tags, a.completeness,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
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
	`, scope.ProjectID, idOrSlug).Scan(
		&out.ArtifactID, &projectID, &out.Slug, &currentVisibility,
		&title, &body, &tags, &completeness, &lastRev,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusNotFound,
			ErrorCode: "ARTIFACT_NOT_FOUND",
			Message:   "artifact not found",
		})
		return
	}
	if err != nil {
		if d.Logger != nil {
			d.Logger.Error("artifact visibility patch", "err", err, "project", projectSlug, "ref", idOrSlug)
		}
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "artifact update failed",
		})
		return
	}
	out.Visibility = *patch.Visibility
	if currentVisibility == *patch.Visibility {
		out.Status = "informational"
		out.Code = "VISIBILITY_NO_OP"
		out.Affected = 0
		if err := tx.Commit(r.Context()); err != nil {
			writeArtifactPatchError(w, artifactPatchError{
				status:    http.StatusInternalServerError,
				ErrorCode: "ARTIFACT_PATCH_FAILED",
				Message:   "commit failed",
			})
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	newRev := lastRev + 1
	shapePayload, err := json.Marshal(map[string]any{
		"kind": "visibility_change",
		"visibility": map[string]string{
			"from": currentVisibility,
			"to":   *patch.Visibility,
		},
	})
	if err != nil {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "encode visibility audit payload failed",
		})
		return
	}
	authorID := "user:web-reader"
	commitMsg := fmt.Sprintf("visibility: %s -> %s", currentVisibility, *patch.Visibility)
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			author_user_id, commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, NULL, $4, $5, $6, 'user', $7, NULL, NULLIF($8, '')::uuid, $9, NULL, 'meta_patch', $10::jsonb)
	`, out.ArtifactID, newRev, title, sha256HexOf(body), tags, completeness,
		authorID, strings.TrimSpace(principal.UserID), commitMsg, string(shapePayload),
	); err != nil {
		if d.Logger != nil {
			d.Logger.Error("artifact visibility revision insert", "err", err, "project", projectSlug, "ref", idOrSlug)
		}
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "artifact revision insert failed",
		})
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE artifacts
		   SET visibility = $2,
		       updated_at = now()
		 WHERE id = $1::uuid
	`, out.ArtifactID, *patch.Visibility); err != nil {
		if d.Logger != nil {
			d.Logger.Error("artifact visibility head update", "err", err, "project", projectSlug, "ref", idOrSlug)
		}
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "artifact update failed",
		})
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.visibility_changed', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'author_user_id',  NULLIF($6, '')::uuid,
			'from',            $7::text,
			'to',              $8::text,
			'origin',          'http_artifact_visibility'
		))
	`, projectID, out.ArtifactID, newRev, out.Slug, authorID, strings.TrimSpace(principal.UserID), currentVisibility, *patch.Visibility); err != nil {
		if d.Logger != nil {
			d.Logger.Error("artifact visibility event insert", "err", err, "project", projectSlug, "ref", idOrSlug)
		}
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "artifact event insert failed",
		})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeArtifactPatchError(w, artifactPatchError{
			status:    http.StatusInternalServerError,
			ErrorCode: "ARTIFACT_PATCH_FAILED",
			Message:   "commit failed",
		})
		return
	}
	out.Status = "ok"
	out.Code = "VISIBILITY_UPDATED"
	out.Affected = 1
	out.RevisionNumber = newRev
	writeJSON(w, http.StatusOK, out)
}

func decodeArtifactPatch(r io.Reader) (artifactPatchRequest, *artifactPatchError) {
	var patch artifactPatchRequest
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return patch, &artifactPatchError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_JSON",
			Message:   "could not parse request body as JSON",
		}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return patch, &artifactPatchError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_JSON",
			Message:   "request body must contain a single JSON object",
		}
	}
	if len(raw) == 0 {
		return patch, &artifactPatchError{
			status:    http.StatusBadRequest,
			ErrorCode: "ARTIFACT_PATCH_EMPTY",
			Message:   "at least one settable field is required",
		}
	}
	for k, v := range raw {
		switch k {
		case "visibility":
			var rawVisibility string
			if err := json.Unmarshal(v, &rawVisibility); err != nil {
				return patch, &artifactPatchError{
					status:    http.StatusBadRequest,
					ErrorCode: "VISIBILITY_INVALID",
					Message:   "visibility must be public|org|private",
				}
			}
			tier := normalizeArtifactPatchVisibility(rawVisibility)
			if tier == "" {
				return patch, &artifactPatchError{
					status:    http.StatusBadRequest,
					ErrorCode: "VISIBILITY_INVALID",
					Message:   "visibility must be public|org|private",
				}
			}
			patch.Visibility = &tier
		default:
			return patch, &artifactPatchError{
				status:    http.StatusBadRequest,
				ErrorCode: "ARTIFACT_PATCH_FIELD_UNSUPPORTED",
				Message:   fmt.Sprintf("unsupported artifact patch field %q", k),
			}
		}
	}
	if patch.Visibility == nil {
		return patch, &artifactPatchError{
			status:    http.StatusBadRequest,
			ErrorCode: "ARTIFACT_PATCH_EMPTY",
			Message:   "at least one settable field is required",
		}
	}
	return patch, nil
}

func normalizeArtifactPatchVisibility(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case projects.VisibilityPublic:
		return projects.VisibilityPublic
	case projects.VisibilityOrg:
		return projects.VisibilityOrg
	case projects.VisibilityPrivate:
		return projects.VisibilityPrivate
	default:
		return ""
	}
}

func artifactProjectAuthPatchError(err error) artifactPatchError {
	switch {
	case errors.Is(err, pauth.ErrProjectSlugRequired):
		return artifactPatchError{status: http.StatusBadRequest, ErrorCode: "PROJECT_SLUG_REQUIRED", Message: "project slug is required"}
	case errors.Is(err, pauth.ErrProjectNotFound):
		return artifactPatchError{status: http.StatusNotFound, ErrorCode: "PROJECT_NOT_FOUND", Message: "project not found"}
	case errors.Is(err, pauth.ErrProjectAccessDenied):
		return artifactPatchError{status: http.StatusForbidden, ErrorCode: "PROJECT_ACCESS_DENIED", Message: "project access denied"}
	default:
		return artifactPatchError{status: http.StatusInternalServerError, ErrorCode: "PROJECT_LOOKUP_FAILED", Message: "project lookup failed"}
	}
}
