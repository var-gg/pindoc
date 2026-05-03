package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/policy"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type projectSettingsPatchResp struct {
	Status                    string `json:"status"`
	SensitiveOps              string `json:"sensitive_ops,omitempty"`
	Visibility                string `json:"visibility,omitempty"`
	DefaultArtifactVisibility string `json:"default_artifact_visibility,omitempty"`
}

type projectSettingsError struct {
	status    int
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

// projectSettingsPatch is the parsed payload — every field is optional;
// at least one must be present (PROJECT_SETTINGS_EMPTY otherwise).
type projectSettingsPatch struct {
	SensitiveOps              *string
	Visibility                *string
	DefaultArtifactVisibility *string
}

func handleProjectSettingsError(w http.ResponseWriter, err projectSettingsError) {
	writeJSON(w, err.status, err)
}

func (d Deps) handleProjectSettingsPatch(w http.ResponseWriter, r *http.Request) {
	patch, decodeErr := decodeProjectSettingsPatch(r.Body)
	if decodeErr != nil {
		handleProjectSettingsError(w, *decodeErr)
		return
	}
	if d.DB == nil {
		handleProjectSettingsError(w, projectSettingsError{
			status:    http.StatusServiceUnavailable,
			ErrorCode: "DB_UNAVAILABLE",
			Message:   "database pool not configured",
		})
		return
	}

	projectSlug := projectSlugFrom(r)
	principal := d.principalForRequest(r)
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlug)
	if err != nil {
		status := http.StatusInternalServerError
		code := "PROJECT_SETTINGS_UPDATE_FAILED"
		switch {
		case errors.Is(err, pauth.ErrProjectSlugRequired):
			status, code = http.StatusBadRequest, "PROJECT_SLUG_REQUIRED"
		case errors.Is(err, pauth.ErrProjectNotFound):
			status, code = http.StatusNotFound, "PROJECT_NOT_FOUND"
		case errors.Is(err, pauth.ErrProjectAccessDenied):
			status, code = http.StatusForbidden, "PROJECT_OWNER_REQUIRED"
		}
		handleProjectSettingsError(w, projectSettingsError{
			status:    status,
			ErrorCode: code,
			Message:   err.Error(),
		})
		return
	}
	if !scope.Can("write.project") {
		handleProjectSettingsError(w, projectSettingsError{
			status:    http.StatusForbidden,
			ErrorCode: "PROJECT_OWNER_REQUIRED",
			Message:   "project owner role is required",
		})
		return
	}

	// Build a dynamic UPDATE that only touches the fields the caller
	// actually sent. Empty patch is rejected upstream (PROJECT_SETTINGS_
	// EMPTY), so we always have at least one assignment here.
	sets := make([]string, 0, 3)
	args := []any{scope.ProjectID}
	resp := projectSettingsPatchResp{Status: "ok"}
	if patch.SensitiveOps != nil {
		args = append(args, *patch.SensitiveOps)
		sets = append(sets, fmt.Sprintf("sensitive_ops = $%d", len(args)))
		resp.SensitiveOps = *patch.SensitiveOps
	}
	if patch.DefaultArtifactVisibility != nil {
		args = append(args, *patch.DefaultArtifactVisibility)
		sets = append(sets, fmt.Sprintf("default_artifact_visibility = $%d", len(args)))
		resp.DefaultArtifactVisibility = *patch.DefaultArtifactVisibility
	}
	if patch.Visibility != nil {
		args = append(args, *patch.Visibility)
		sets = append(sets, fmt.Sprintf("visibility = $%d", len(args)))
		resp.Visibility = *patch.Visibility
	}

	q := fmt.Sprintf(`UPDATE projects SET %s WHERE id = $1::uuid`,
		strings.Join(sets, ", "))
	if _, err := d.DB.Exec(r.Context(), q, args...); err != nil {
		if d.Logger != nil {
			d.Logger.Error("project settings update", "err", err)
		}
		handleProjectSettingsError(w, projectSettingsError{
			status:    http.StatusInternalServerError,
			ErrorCode: "PROJECT_SETTINGS_UPDATE_FAILED",
			Message:   "update failed",
		})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeProjectSettingsPatch(r io.Reader) (projectSettingsPatch, *projectSettingsError) {
	var patch projectSettingsPatch
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return patch, &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_JSON",
			Message:   "could not parse request body as JSON",
		}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return patch, &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_JSON",
			Message:   "request body must contain a single JSON object",
		}
	}
	if len(raw) == 0 {
		return patch, &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "PROJECT_SETTINGS_EMPTY",
			Message:   "at least one settable field is required",
		}
	}
	for k, v := range raw {
		switch k {
		case "sensitive_ops":
			var rawMode string
			if err := json.Unmarshal(v, &rawMode); err != nil {
				return patch, &projectSettingsError{
					status:    http.StatusBadRequest,
					ErrorCode: "SENSITIVE_OPS_INVALID",
					Message:   "sensitive_ops must be auto or confirm",
				}
			}
			trimmed := strings.ToLower(strings.TrimSpace(rawMode))
			switch trimmed {
			case policy.SensitiveOpsAuto, policy.SensitiveOpsConfirm:
				normalized := policy.NormalizeSensitiveOpsMode(trimmed)
				patch.SensitiveOps = &normalized
			default:
				return patch, &projectSettingsError{
					status:    http.StatusBadRequest,
					ErrorCode: "SENSITIVE_OPS_INVALID",
					Message:   "sensitive_ops must be auto or confirm",
				}
			}
		case "default_artifact_visibility":
			var rawTier string
			if err := json.Unmarshal(v, &rawTier); err != nil {
				return patch, &projectSettingsError{
					status:    http.StatusBadRequest,
					ErrorCode: "DEFAULT_VISIBILITY_INVALID",
					Message:   "default_artifact_visibility must be public|org|private",
				}
			}
			trimmed := strings.ToLower(strings.TrimSpace(rawTier))
			switch trimmed {
			case "public", "org", "private":
				patch.DefaultArtifactVisibility = &trimmed
			default:
				return patch, &projectSettingsError{
					status:    http.StatusBadRequest,
					ErrorCode: "DEFAULT_VISIBILITY_INVALID",
					Message:   "default_artifact_visibility must be public|org|private",
				}
			}
		case "visibility":
			var rawTier string
			if err := json.Unmarshal(v, &rawTier); err != nil {
				return patch, &projectSettingsError{
					status:    http.StatusBadRequest,
					ErrorCode: "VISIBILITY_INVALID",
					Message:   "visibility must be public|org|private",
				}
			}
			tier := projects.NormalizeVisibility(rawTier)
			if tier == "" {
				return patch, &projectSettingsError{
					status:    http.StatusBadRequest,
					ErrorCode: "VISIBILITY_INVALID",
					Message:   "visibility must be public|org|private",
				}
			}
			patch.Visibility = &tier
		default:
			return patch, &projectSettingsError{
				status:    http.StatusBadRequest,
				ErrorCode: "PROJECT_SETTINGS_FIELD_UNSUPPORTED",
				Message:   fmt.Sprintf("unsupported project setting field %q", k),
			}
		}
	}
	if patch.SensitiveOps == nil && patch.DefaultArtifactVisibility == nil && patch.Visibility == nil {
		return patch, &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "PROJECT_SETTINGS_EMPTY",
			Message:   "at least one settable field is required",
		}
	}
	return patch, nil
}
