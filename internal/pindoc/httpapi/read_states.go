package httpapi

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/readstate"
)

type readStatesResponse struct {
	ProjectSlug string            `json:"project_slug"`
	UserKey     string            `json:"user_key"`
	States      []readstate.State `json:"states"`
}

// handleReadStates returns the classified read state for every non-archived
// artifact in the project, scoped to the calling user_key. Reader UI fetches
// this once per project page load and looks up artifact_id → state in the
// returned slice; there is no per-artifact endpoint because the dataset is
// small and the round-trip cost dominates.
func (d Deps) handleReadStates(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	userKey := readerUserKey(r)
	states, err := readstate.ProjectStates(r.Context(), d.DB, projectSlug, userKey)
	if err != nil {
		d.Logger.Error("project read states", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, readStatesResponse{
		ProjectSlug: projectSlug,
		UserKey:     userKey,
		States:      states,
	})
}

// handleArtifactReadState returns the read state for a single artifact.
// Trust Card on the Reader detail surface uses this to decorate the
// 'human read' axis without pulling the full project-wide map.
func (d Deps) handleArtifactReadState(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	userKey := readerUserKey(r)
	idOrSlug := r.PathValue("idOrSlug")
	state, err := readstate.ArtifactState(r.Context(), d.DB, projectSlug, idOrSlug, userKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		d.Logger.Error("artifact read state", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, state)
}
