package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestReaderHiddenProjectSlug(t *testing.T) {
	cases := []struct {
		slug string
		want bool
	}{
		{"oauth-it-abc123", true},
		{"invite-http-abc123", true},
		{"workspace-detect-abc123", true},
		{"vis-http-18abc7f4129be000", true},
		{"vis-mcp-1777735890813002700", true},
		{"artifact-audit-1777735957821357800", true},
		{"task-flow-a-1777735961285390100", true},
		{"task-flow-b-1777735961285390100", true},
		{"task-queue-across-a-1777735962378049400", true},
		{"task-queue-across-b-1777735962378049400", true},
		{"pindoc-18abd57be67af9f8", true},
		{"PINDOC-18ABD57BE67AF9F8", true},
		{"OAuth-IT-ABC123", true},
		{"pindoc", false},
		{"pindoc-tour", false},
		{"pindoc-18abd57be67af9f", false},
		{"pindoc-18abd57be67af9fg", false},
		{"pindoc-18abd57be67af9f8-extra", false},
		{"customer-docs", false},
	}
	for _, c := range cases {
		t.Run(c.slug, func(t *testing.T) {
			if got := readerHiddenProjectSlug(c.slug); got != c.want {
				t.Fatalf("readerHiddenProjectSlug(%q) = %v, want %v", c.slug, got, c.want)
			}
		})
	}
}

func TestIncludeReaderHiddenProjects(t *testing.T) {
	cases := []struct {
		query string
		want  bool
	}{
		{"", false},
		{"include_hidden=true", true},
		{"include_internal=true", true},
		{"ops=1", true},
		{"debug=ops", true},
		{"include_hidden=false", false},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/projects?"+c.query, nil)
			if got := includeReaderHiddenProjects(req); got != c.want {
				t.Fatalf("includeReaderHiddenProjects(%q) = %v, want %v", c.query, got, c.want)
			}
		})
	}
}

func TestProjectCreateDefaultURLUsesToday(t *testing.T) {
	if got := projectCreateDefaultURL("shop-fe"); got != "/p/shop-fe/today" {
		t.Fatalf("projectCreateDefaultURL() = %q, want /p/shop-fe/today", got)
	}
}
