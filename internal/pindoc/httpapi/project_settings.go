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
)

type projectSettingsPatchResp struct {
	Status       string `json:"status"`
	SensitiveOps string `json:"sensitive_ops"`
}

type projectSettingsError struct {
	status    int
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func handleProjectSettingsError(w http.ResponseWriter, err projectSettingsError) {
	writeJSON(w, err.status, err)
}

func (d Deps) handleProjectSettingsPatch(w http.ResponseWriter, r *http.Request) {
	mode, decodeErr := decodeProjectSettingsPatch(r.Body)
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

	_, err = d.DB.Exec(r.Context(), `
		UPDATE projects
		   SET sensitive_ops = $2
		 WHERE id = $1::uuid
	`, scope.ProjectID, mode)
	if err != nil {
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
	writeJSON(w, http.StatusOK, projectSettingsPatchResp{
		Status:       "ok",
		SensitiveOps: mode,
	})
}

func decodeProjectSettingsPatch(r io.Reader) (string, *projectSettingsError) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return "", &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_JSON",
			Message:   "could not parse request body as JSON",
		}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "BAD_JSON",
			Message:   "request body must contain a single JSON object",
		}
	}
	if len(raw) == 0 {
		return "", &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "PROJECT_SETTINGS_EMPTY",
			Message:   "sensitive_ops is required",
		}
	}
	for k := range raw {
		if k != "sensitive_ops" {
			return "", &projectSettingsError{
				status:    http.StatusBadRequest,
				ErrorCode: "PROJECT_SETTINGS_FIELD_UNSUPPORTED",
				Message:   fmt.Sprintf("unsupported project setting field %q", k),
			}
		}
	}
	var rawMode string
	if err := json.Unmarshal(raw["sensitive_ops"], &rawMode); err != nil {
		return "", &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "SENSITIVE_OPS_INVALID",
			Message:   "sensitive_ops must be auto or confirm",
		}
	}
	trimmed := strings.ToLower(strings.TrimSpace(rawMode))
	switch trimmed {
	case policy.SensitiveOpsAuto, policy.SensitiveOpsConfirm:
		return policy.NormalizeSensitiveOpsMode(trimmed), nil
	default:
		return "", &projectSettingsError{
			status:    http.StatusBadRequest,
			ErrorCode: "SENSITIVE_OPS_INVALID",
			Message:   "sensitive_ops must be auto or confirm",
		}
	}
}
