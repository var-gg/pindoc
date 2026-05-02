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
	Status     string `json:"status"`
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Visibility string `json:"visibility"`
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

	var out artifactPatchResp
	err = d.DB.QueryRow(r.Context(), `
		UPDATE artifacts
		   SET visibility = $1,
		       updated_at = now()
		 WHERE project_id = $2::uuid
		   AND (
		        id::text = $3 OR slug = $3 OR
		        id = (
		          SELECT asa.artifact_id
		            FROM artifact_slug_aliases asa
		           WHERE asa.project_id = $2::uuid AND asa.old_slug = $3
		           LIMIT 1
		        )
		   )
		 RETURNING id::text, slug, visibility
	`, *patch.Visibility, scope.ProjectID, idOrSlug).Scan(&out.ArtifactID, &out.Slug, &out.Visibility)
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
	out.Status = "ok"
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
