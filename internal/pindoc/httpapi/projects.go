package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// projectCreateRequest mirrors projects.CreateProjectInput at the wire
// boundary. snake_case JSON keys match the rest of httpapi (task_meta /
// task_assign already use that convention). Only Slug, Name, and
// PrimaryLanguage are required — the rest are optional.
type projectCreateRequest struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Color           string `json:"color,omitempty"`
	PrimaryLanguage string `json:"primary_language"`
	GitRemoteURL    string `json:"git_remote_url,omitempty"`
}

// projectCreateResponse is the 201 success envelope. Fields are flat
// because callers (UI form submit, curl, CLI on top of REST) are easier
// to write against a flat shape than the MCP tool's nested
// reconnect/activation block — those are MCP-specific framing the agent
// needs but the REST audience does not.
type projectCreateResponse struct {
	ProjectID        string `json:"project_id"`
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	PrimaryLanguage  string `json:"primary_language"`
	URL              string `json:"url"`
	DefaultArea      string `json:"default_area"`
	AreasCreated     int    `json:"areas_created"`
	TemplatesCreated int    `json:"templates_created"`
}

// projectCreateError mirrors the task_meta error envelope so UI can use
// one error mapper for both surfaces. error_code values match the
// projects package sentinels (SLUG_INVALID, SLUG_RESERVED, SLUG_TAKEN,
// NAME_REQUIRED, LANG_REQUIRED, LANG_INVALID, GIT_REMOTE_URL_INVALID)
// plus generic BAD_JSON / INTERNAL_ERROR.
type projectCreateError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

// handleProjectCreate is the Reader/CLI bridge for projects.CreateProject.
// Behind the wire it does the exact same work pindoc.project.create
// does — Decision project-bootstrap-canonical-flow-reader-ui-first-class
// promises a single source of truth across MCP, REST, CLI, and UI; this
// handler is the REST entrypoint of that quartet. auth_mode is still
// trusted_local: the daemon binds to 127.0.0.1, so anyone hitting this
// endpoint already controls the host.
func (d Deps) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	var in projectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, projectCreateError{
			ErrorCode: "BAD_JSON",
			Message:   "could not parse request body as JSON",
		})
		return
	}

	tx, err := d.DB.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		d.Logger.Error("project create: begin tx", "err", err)
		writeJSON(w, http.StatusInternalServerError, projectCreateError{
			ErrorCode: "INTERNAL_ERROR",
			Message:   "could not begin transaction",
		})
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	out, err := projects.CreateProject(r.Context(), tx, projects.CreateProjectInput{
		Slug:            in.Slug,
		Name:            in.Name,
		Description:     in.Description,
		Color:           in.Color,
		PrimaryLanguage: in.PrimaryLanguage,
		GitRemoteURL:    in.GitRemoteURL,
	})
	if err != nil {
		status, code := mapProjectCreateError(err)
		d.Logger.Info("project create rejected", "code", code, "err", err)
		writeJSON(w, status, projectCreateError{
			ErrorCode: code,
			Message:   err.Error(),
		})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		d.Logger.Error("project create: commit", "err", err)
		writeJSON(w, http.StatusInternalServerError, projectCreateError{
			ErrorCode: "INTERNAL_ERROR",
			Message:   "could not commit transaction",
		})
		return
	}

	d.Logger.Info("project created via REST",
		"slug", out.Slug, "name", out.Name, "lang", out.PrimaryLanguage)

	writeJSON(w, http.StatusCreated, projectCreateResponse{
		ProjectID:        out.ID,
		Slug:             out.Slug,
		Name:             out.Name,
		PrimaryLanguage:  out.PrimaryLanguage,
		URL:              "/p/" + out.Slug + "/wiki",
		DefaultArea:      out.DefaultArea,
		AreasCreated:     out.AreasCreated,
		TemplatesCreated: out.TemplatesCreated,
	})
}

// mapProjectCreateError translates a wrapped sentinel from the projects
// package into (HTTP status, stable error_code). Validation errors are
// 400 except SLUG_TAKEN which is 409 (conflict semantics — the slug
// already exists). Unwrapped errors fall through to 500 INTERNAL_ERROR
// so the caller doesn't see a leaked stack and the operator can grep
// for the original stack in the server log.
func mapProjectCreateError(err error) (int, string) {
	switch {
	case errors.Is(err, projects.ErrSlugInvalid):
		return http.StatusBadRequest, "SLUG_INVALID"
	case errors.Is(err, projects.ErrSlugReserved):
		return http.StatusBadRequest, "SLUG_RESERVED"
	case errors.Is(err, projects.ErrSlugTaken):
		return http.StatusConflict, "SLUG_TAKEN"
	case errors.Is(err, projects.ErrNameRequired):
		return http.StatusBadRequest, "NAME_REQUIRED"
	case errors.Is(err, projects.ErrLangRequired):
		return http.StatusBadRequest, "LANG_REQUIRED"
	case errors.Is(err, projects.ErrLangInvalid):
		return http.StatusBadRequest, "LANG_INVALID"
	case errors.Is(err, projects.ErrGitRemoteURLInvalid):
		return http.StatusBadRequest, "GIT_REMOTE_URL_INVALID"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}
