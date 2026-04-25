package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// TestMapProjectCreateError locks the contract between projects sentinel
// errors and the REST envelope's (status, error_code) pair. UI / CLI /
// curl callers all switch on error_code, so a typo or missing case here
// silently breaks every entrypoint at once. SLUG_TAKEN gets 409
// (resource conflict) — everything else is a 400 except the catchall
// 500 INTERNAL_ERROR for unwrapped errors.
func TestMapProjectCreateError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "slug invalid",
			err:        fmt.Errorf("%w: bad shape", projects.ErrSlugInvalid),
			wantStatus: http.StatusBadRequest,
			wantCode:   "SLUG_INVALID",
		},
		{
			name:       "slug reserved",
			err:        fmt.Errorf("%w: collides", projects.ErrSlugReserved),
			wantStatus: http.StatusBadRequest,
			wantCode:   "SLUG_RESERVED",
		},
		{
			name:       "slug already taken (409)",
			err:        fmt.Errorf("%w: dup", projects.ErrSlugTaken),
			wantStatus: http.StatusConflict,
			wantCode:   "SLUG_TAKEN",
		},
		{
			name:       "name required",
			err:        fmt.Errorf("%w: empty", projects.ErrNameRequired),
			wantStatus: http.StatusBadRequest,
			wantCode:   "NAME_REQUIRED",
		},
		{
			name:       "language required",
			err:        fmt.Errorf("%w: empty", projects.ErrLangRequired),
			wantStatus: http.StatusBadRequest,
			wantCode:   "LANG_REQUIRED",
		},
		{
			name:       "language invalid",
			err:        fmt.Errorf("%w: fr", projects.ErrLangInvalid),
			wantStatus: http.StatusBadRequest,
			wantCode:   "LANG_INVALID",
		},
		{
			name:       "unwrapped DB error → INTERNAL_ERROR 500",
			err:        errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStatus, gotCode := mapProjectCreateError(c.err)
			if gotStatus != c.wantStatus {
				t.Errorf("status = %d, want %d", gotStatus, c.wantStatus)
			}
			if gotCode != c.wantCode {
				t.Errorf("code = %q, want %q", gotCode, c.wantCode)
			}
		})
	}
}
