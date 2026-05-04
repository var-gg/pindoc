package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestArtifactListCursorRoundTrip(t *testing.T) {
	row := artifactRow{
		ID:        "11111111-1111-1111-1111-111111111111",
		Type:      "Task",
		UpdatedAt: time.Date(2026, 5, 4, 12, 0, 0, 123, time.UTC),
	}
	encoded, err := encodeArtifactListCursor(row)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := decodeArtifactListCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if decoded.TaskRank != 1 || decoded.ID != row.ID || !decoded.UpdatedAt.Equal(row.UpdatedAt) {
		t.Fatalf("decoded cursor = %+v, want row-derived cursor", decoded)
	}
	if _, err := decodeArtifactListCursor("not base64"); err == nil {
		t.Fatalf("decode invalid cursor error = nil")
	}
}

func TestParseArtifactListLimit(t *testing.T) {
	cases := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"", artifactListDefaultLimit, false},
		{"25", 25, false},
		{"500", artifactListMaxLimit, false},
		{"0", 0, true},
		{"bad", 0, true},
	}
	for _, tc := range cases {
		got, err := parseArtifactListLimit(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseArtifactListLimit(%q) error = nil", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseArtifactListLimit(%q) error = %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("parseArtifactListLimit(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestArtifactListPaginationIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact list pagination HTTP DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	projectSlug := "artifact-list-" + suffix
	ownerEmail := "artifact-list-owner-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Artifact List Owner "+suffix, ownerEmail)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create project tx: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            projectSlug,
		Name:            "Artifact List " + suffix,
		PrimaryLanguage: "en",
		OwnerUserID:     ownerID,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create project: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit create project tx: %v", err)
	}
	projectID := out.ID
	areaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, projectID, "misc")
	baseTime := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		slug := fmt.Sprintf("decision-%02d-%s", i, suffix)
		id := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, slug, projects.VisibilityPublic, ownerID)
		if _, err := pool.Exec(ctx, `UPDATE artifacts SET updated_at = $2 WHERE id = $1::uuid`, id, baseTime.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("update artifact time: %v", err)
		}
	}
	taskID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, "task-newest-"+suffix, projects.VisibilityPublic, ownerID)
	if _, err := pool.Exec(ctx, `
		UPDATE artifacts
		   SET type = 'Task', task_meta = '{"status":"open"}'::jsonb, updated_at = $2
		 WHERE id = $1::uuid
	`, taskID, baseTime.Add(time.Hour)); err != nil {
		t.Fatalf("update task artifact: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, projectSlug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(ownerEmail))
	})

	handler := New(&config.Config{BindAddr: "0.0.0.0:5830"}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: projectSlug,
		DefaultUserID:      ownerID,
		BindAddr:           "0.0.0.0:5830",
	})

	first := doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/p/"+projectSlug+"/artifacts?limit=3", "")
	if first.Code != http.StatusOK {
		t.Fatalf("first page status = %d, want 200; body=%s", first.Code, first.Body.String())
	}
	var firstBody artifactListPageTestBody
	if err := json.NewDecoder(first.Body).Decode(&firstBody); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(firstBody.Artifacts) != 3 || !firstBody.HasMore || firstBody.NextCursor == "" {
		t.Fatalf("first page = %+v, want 3 artifacts with next cursor", firstBody)
	}
	if firstBody.Artifacts[0].Type == "Task" {
		t.Fatalf("newest Task should not fill first wiki page: %+v", firstBody.Artifacts)
	}

	nextURL := "/api/p/" + projectSlug + "/artifacts?limit=3&cursor=" + url.QueryEscape(firstBody.NextCursor)
	second := doLoopbackVisibilityRequest(handler, http.MethodGet, nextURL, "")
	if second.Code != http.StatusOK {
		t.Fatalf("second page status = %d, want 200; body=%s", second.Code, second.Body.String())
	}
	var secondBody artifactListPageTestBody
	if err := json.NewDecoder(second.Body).Decode(&secondBody); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(secondBody.Artifacts) != 3 || !secondBody.HasMore || secondBody.NextCursor == "" {
		t.Fatalf("second page = %+v, want 3 artifacts with another cursor", secondBody)
	}
	if firstBody.Artifacts[0].Slug == secondBody.Artifacts[0].Slug {
		t.Fatalf("second page repeated first artifact %q", firstBody.Artifacts[0].Slug)
	}

	lastURL := "/api/p/" + projectSlug + "/artifacts?limit=3&cursor=" + url.QueryEscape(secondBody.NextCursor)
	last := doLoopbackVisibilityRequest(handler, http.MethodGet, lastURL, "")
	if last.Code != http.StatusOK {
		t.Fatalf("last page status = %d, want 200; body=%s", last.Code, last.Body.String())
	}
	var lastBody artifactListPageTestBody
	if err := json.NewDecoder(last.Body).Decode(&lastBody); err != nil {
		t.Fatalf("decode last page: %v", err)
	}
	if len(lastBody.Artifacts) != 1 || lastBody.HasMore || lastBody.NextCursor != "" || lastBody.Artifacts[0].Type != "Task" {
		t.Fatalf("last page = %+v, want final Task without cursor", lastBody)
	}

	invalid := doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/p/"+projectSlug+"/artifacts?cursor=bad", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status = %d, want 400; body=%s", invalid.Code, invalid.Body.String())
	}
}

type artifactListPageTestBody struct {
	Artifacts []struct {
		Slug string `json:"slug"`
		Type string `json:"type"`
	} `json:"artifacts"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}
