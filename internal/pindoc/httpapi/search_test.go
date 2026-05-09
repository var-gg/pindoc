package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestSearchCrossProjectVisibilityIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run search HTTP DB integration")
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
	ownerEmail := "search-owner-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Search Owner "+suffix, ownerEmail)
	orgASlug := "cmdk-org-a-" + suffix
	orgBSlug := "cmdk-org-b-" + suffix
	currentSlug := "cmdk-current-" + suffix
	sisterSlug := "cmdk-sister-" + suffix
	orgAID := insertOrgRouteHTTPOrg(t, ctx, pool, orgASlug)
	orgBID := insertOrgRouteHTTPOrg(t, ctx, pool, orgBSlug)
	currentID := createHTTPSearchProject(t, ctx, pool, orgAID, currentSlug, ownerID, projects.VisibilityPublic)
	sisterID := createHTTPSearchProject(t, ctx, pool, orgBID, sisterSlug, ownerID, projects.VisibilityPublic)
	currentAreaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, currentID, "misc")
	sisterAreaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, sisterID, "misc")

	query := "cmdk cross project needle " + suffix
	embedder := embed.NewStub(32)
	currentPublic := "cmdk-current-public-" + suffix
	sisterPublic := "cmdk-sister-public-" + suffix
	sisterOrg := "cmdk-sister-org-" + suffix
	sisterPrivate := "cmdk-sister-private-" + suffix
	for _, row := range []struct {
		projectID string
		areaID    string
		slug      string
		tier      string
	}{
		{currentID, currentAreaID, currentPublic, projects.VisibilityPublic},
		{sisterID, sisterAreaID, sisterPublic, projects.VisibilityPublic},
		{sisterID, sisterAreaID, sisterOrg, projects.VisibilityOrg},
		{sisterID, sisterAreaID, sisterPrivate, projects.VisibilityPrivate},
	} {
		artifactID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, row.projectID, row.areaID, row.slug, row.tier, ownerID)
		insertHTTPSearchChunk(t, ctx, pool, embedder, artifactID, query)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = ANY($1::text[])`, []string{currentSlug, sisterSlug})
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE slug = ANY($1::text[])`, []string{orgASlug, orgBSlug})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(ownerEmail))
	})

	oauthSvc, err := pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
		Issuer:             "http://127.0.0.1:5830",
		PublicBaseURL:      "http://127.0.0.1:5830",
		RedirectBaseURL:    "http://127.0.0.1:5830",
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           "search-http-" + suffix,
		RedirectURIs:       []string{"http://127.0.0.1:3846/callback"},
		GitHubClientID:     "fake-gh-client",
		GitHubClientSecret: "fake-gh-secret",
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}
	handler := New(&config.Config{
		AuthProviders: []string{config.AuthProviderGitHub},
		BindAddr:      "0.0.0.0:5830",
	}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: currentSlug,
		Embedder:           embedder,
		OAuth:              oauthSvc,
		AuthProviders:      []string{config.AuthProviderGitHub},
		BindAddr:           "0.0.0.0:5830",
	})

	path := "/api/p/" + currentSlug + "/search?q=" + url.QueryEscape(query) + "&cross_project=1"
	anonymous := decodeHTTPSearchResponse(t, doInviteRequest(t, handler, nil, "", http.MethodGet, path, ""))
	anonymousSlugs := searchSlugSet(anonymous.Hits)
	if len(anonymousSlugs) != 2 || !anonymousSlugs[currentPublic] || !anonymousSlugs[sisterPublic] {
		t.Fatalf("anonymous cross-project search slugs = %s, want public current+sister only", strings.Join(searchSlugs(anonymous.Hits), ","))
	}
	assertHTTPSearchHitScope(t, anonymous.Hits, sisterPublic, sisterSlug, orgBSlug)

	owner := decodeHTTPSearchResponse(t, doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, path, ""))
	ownerSlugs := searchSlugSet(owner.Hits)
	for _, want := range []string{currentPublic, sisterPublic, sisterOrg, sisterPrivate} {
		if !ownerSlugs[want] {
			t.Fatalf("owner cross-project search slugs = %s, missing %s", strings.Join(searchSlugs(owner.Hits), ","), want)
		}
	}
	assertHTTPSearchHitScope(t, owner.Hits, sisterPrivate, sisterSlug, orgBSlug)

	taskSlug := "cmdk-filter-task-" + suffix
	taskID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, sisterID, sisterAreaID, taskSlug, projects.VisibilityPublic, ownerID)
	if _, err := pool.Exec(ctx, `UPDATE artifacts SET type = 'Task' WHERE id = $1::uuid`, taskID); err != nil {
		t.Fatalf("mark search artifact task: %v", err)
	}
	insertHTTPSearchChunk(t, ctx, pool, embedder, taskID, query)

	taskFiltered := decodeHTTPSearchResponse(t, doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, path+"&type=task", ""))
	taskFilteredSlugs := searchSlugSet(taskFiltered.Hits)
	if len(taskFilteredSlugs) != 1 || !taskFilteredSlugs[taskSlug] {
		t.Fatalf("type-filtered search slugs = %s, want only %s", strings.Join(searchSlugs(taskFiltered.Hits), ","), taskSlug)
	}
	if taskFiltered.Hits[0].Type != "Task" {
		t.Fatalf("type-filtered hit type = %q, want Task", taskFiltered.Hits[0].Type)
	}
}

type httpSearchHit struct {
	Slug        string `json:"slug"`
	ProjectSlug string `json:"project_slug"`
	OrgSlug     string `json:"org_slug"`
	Type        string `json:"type"`
}

type httpSearchResponse struct {
	CrossProject bool            `json:"cross_project"`
	Hits         []httpSearchHit `json:"hits"`
}

func decodeHTTPSearchResponse(t *testing.T, rec *httptest.ResponseRecorder) httpSearchResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var out httpSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if !out.CrossProject {
		t.Fatalf("cross_project = false, want true")
	}
	return out
}

func createHTTPSearchProject(t *testing.T, ctx context.Context, pool *db.Pool, orgID, slug, ownerID, visibility string) string {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create search project: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            "Search " + slug,
		PrimaryLanguage: "en",
		OrganizationID:  orgID,
		OwnerUserID:     ownerID,
		Visibility:      visibility,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create search project %s: %v", slug, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit search project: %v", err)
	}
	return out.ID
}

func insertHTTPSearchChunk(t *testing.T, ctx context.Context, pool *db.Pool, provider embed.Provider, artifactID, text string) {
	t.Helper()
	res, err := provider.Embed(ctx, embed.Request{Texts: []string{text}, Kind: embed.KindDocument})
	if err != nil {
		t.Fatalf("embed chunk: %v", err)
	}
	info := provider.Info()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, span_start, span_end,
			text, embedding, model_name, model_dim
		) VALUES (
			$1::uuid, 'body', 0, 'Body', 0, $2,
			$3, $4::vector, $5, $6
		)
	`, artifactID, len(text), text, embed.VectorString(embed.PadTo768(res.Vectors[0])), info.Name, info.Dimension); err != nil {
		t.Fatalf("insert search chunk: %v", err)
	}
}

func searchSlugs(hits []httpSearchHit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.Slug)
	}
	return out
}

func searchSlugSet(hits []httpSearchHit) map[string]bool {
	out := map[string]bool{}
	for _, hit := range hits {
		out[hit.Slug] = true
	}
	return out
}

func assertHTTPSearchHitScope(t *testing.T, hits []httpSearchHit, slug, projectSlug, orgSlug string) {
	t.Helper()
	for _, hit := range hits {
		if hit.Slug != slug {
			continue
		}
		if hit.ProjectSlug != projectSlug || hit.OrgSlug != orgSlug {
			t.Fatalf("hit %s scope = project=%q org=%q, want project=%q org=%q", slug, hit.ProjectSlug, hit.OrgSlug, projectSlug, orgSlug)
		}
		return
	}
	t.Fatalf("hit %s not found in %+v", slug, hits)
}
