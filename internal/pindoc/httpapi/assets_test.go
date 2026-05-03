package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/assets"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestAssetBlobSecurityHeadersRangeAndCacheIntegration(t *testing.T) {
	fixture := newAssetHTTPFixture(t)
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	assetID, sha := insertHTTPAsset(t, fixture, "note.md", "text/markdown", content)
	attachHTTPAsset(t, fixture, fixture.publicArtifactID, assetID, assets.RoleAttachment)

	rangeResp := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(assetID), "10.0.0.5:54321", map[string]string{
		"Range": "bytes=0-9",
	})
	if rangeResp.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206; body=%s", rangeResp.Code, rangeResp.Body.String())
	}
	if got := rangeResp.Body.String(); got != "0123456789" {
		t.Fatalf("range body = %q", got)
	}
	if got := rangeResp.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", got)
	}
	if got := rangeResp.Header().Get("ETag"); got != `W/"`+sha+`"` {
		t.Fatalf("ETag = %q, want weak sha", got)
	}
	if got := rangeResp.Header().Get("Last-Modified"); got == "" {
		t.Fatal("Last-Modified missing")
	}
	if got := rangeResp.Header().Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
		t.Fatalf("public Cache-Control = %q", got)
	}
	if got := rangeResp.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}

	notModified := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(assetID), "10.0.0.5:54321", map[string]string{
		"If-None-Match": `W/"` + sha + `"`,
	})
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match status = %d, want 304; body=%s", notModified.Code, notModified.Body.String())
	}

	pngID, _ := insertHTTPAsset(t, fixture, "shot.png", "image/png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	attachHTTPAsset(t, fixture, fixture.publicArtifactID, pngID, assets.RoleAttachment)
	pngResp := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(pngID), "10.0.0.5:54321", nil)
	if got := pngResp.Header().Get("Content-Type"); strings.Contains(got, "charset") {
		t.Fatalf("binary Content-Type = %q, should not include charset", got)
	}
}

func TestAssetBlobSVGServedAsAttachmentWithHardenedCSPIntegration(t *testing.T) {
	fixture := newAssetHTTPFixture(t)
	svgID, _ := insertHTTPAsset(t, fixture, "attack.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>fetch("https://example.invalid")</script></svg>`))
	attachHTTPAsset(t, fixture, fixture.publicArtifactID, svgID, assets.RoleAttachment)

	resp := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(svgID), "10.0.0.5:54321", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	csp := resp.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "sandbox", "img-src 'self'", "script-src 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("asset CSP = %q, missing %q", csp, want)
		}
	}
	if got := resp.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestAssetBlobPrivateCacheAndVisibilityTransitionIntegration(t *testing.T) {
	fixture := newAssetHTTPFixture(t)
	privateID, _ := insertHTTPAsset(t, fixture, "private.txt", "text/plain", []byte("private"))
	attachHTTPAsset(t, fixture, fixture.privateArtifactID, privateID, assets.RoleAttachment)

	loopback := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(privateID), "127.0.0.1:5830", nil)
	if loopback.Code != http.StatusOK {
		t.Fatalf("loopback status = %d, want 200; body=%s", loopback.Code, loopback.Body.String())
	}
	if got := loopback.Header().Get("Cache-Control"); got != "private, no-cache" {
		t.Fatalf("private Cache-Control = %q", got)
	}

	anonymous := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(privateID), "10.0.0.5:54321", nil)
	if anonymous.Code != http.StatusNotFound {
		t.Fatalf("anonymous private status = %d, want 404; body=%s", anonymous.Code, anonymous.Body.String())
	}

	publicID, _ := insertHTTPAsset(t, fixture, "public.txt", "text/plain", []byte("public"))
	attachHTTPAsset(t, fixture, fixture.publicArtifactID, publicID, assets.RoleAttachment)
	publicResp := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(publicID), "10.0.0.5:54321", nil)
	if publicResp.Code != http.StatusOK {
		t.Fatalf("public status = %d, want 200; body=%s", publicResp.Code, publicResp.Body.String())
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE artifacts SET visibility = $1 WHERE id = $2::uuid`, projects.VisibilityPrivate, fixture.publicArtifactID); err != nil {
		t.Fatalf("make public artifact private: %v", err)
	}
	afterTransition := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(publicID), "10.0.0.5:54321", nil)
	if afterTransition.Code != http.StatusNotFound {
		t.Fatalf("after transition status = %d, want 404; body=%s", afterTransition.Code, afterTransition.Body.String())
	}
}

func TestAssetBlobNotFoundResponsesDoNotEnumerateIntegration(t *testing.T) {
	fixture := newAssetHTTPFixture(t)
	missingBlobID := insertHTTPAssetRowOnly(t, fixture, "missing.txt", "text/plain", int64(len("missing")), strings.Repeat("a", 64), "ff/missing")
	attachHTTPAsset(t, fixture, fixture.publicArtifactID, missingBlobID, assets.RoleAttachment)

	random := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath("00000000-0000-0000-0000-000000000000"), "10.0.0.5:54321", nil)
	missing := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(missingBlobID), "10.0.0.5:54321", nil)
	for name, resp := range map[string]*httptest.ResponseRecorder{"random": random, "missing": missing} {
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404; body=%s", name, resp.Code, resp.Body.String())
		}
		var body assetAPIError
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s decode body: %v", name, err)
		}
		if body.ErrorCode != "ASSET_NOT_FOUND" || body.Message != "asset not found" {
			t.Fatalf("%s body = %+v, want unified ASSET_NOT_FOUND", name, body)
		}
	}
	if random.Body.String() != missing.Body.String() {
		t.Fatalf("not-found bodies differ: random=%q missing=%q", random.Body.String(), missing.Body.String())
	}
}

func TestAssetBlobMostPermissiveVisibilityPolicyIntegration(t *testing.T) {
	fixture := newAssetHTTPFixture(t)
	assetID, _ := insertHTTPAsset(t, fixture, "shared.txt", "text/plain", []byte("shared"))
	attachHTTPAsset(t, fixture, fixture.privateArtifactID, assetID, assets.RoleAttachment)
	attachHTTPAsset(t, fixture, fixture.publicArtifactID, assetID, assets.RoleAttachment)

	sharedPublic := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(assetID), "10.0.0.5:54321", nil)
	if sharedPublic.Code != http.StatusOK {
		t.Fatalf("shared public status = %d, want 200; body=%s", sharedPublic.Code, sharedPublic.Body.String())
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `DELETE FROM artifact_assets WHERE artifact_id = $1::uuid`, fixture.publicArtifactID); err != nil {
		t.Fatalf("delete public attachment: %v", err)
	}
	privateOnly := doAssetBlobRequest(fixture.handler, http.MethodGet, fixture.assetPath(assetID), "10.0.0.5:54321", nil)
	if privateOnly.Code != http.StatusNotFound {
		t.Fatalf("private-only shared status = %d, want 404; body=%s", privateOnly.Code, privateOnly.Body.String())
	}
}

type assetHTTPFixture struct {
	ctx               context.Context
	pool              *db.Pool
	handler           http.Handler
	projectSlug       string
	projectID         string
	assetRoot         string
	publicArtifactID  string
	privateArtifactID string
}

func newAssetHTTPFixture(t *testing.T) assetHTTPFixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run asset blob HTTP DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	slug := "asset-http-" + suffix
	ownerEmail := "asset-http-owner-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Asset HTTP Owner "+suffix, ownerEmail)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create project tx: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            "Asset HTTP " + suffix,
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
	publicArtifactID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, "public-"+suffix, projects.VisibilityPublic, ownerID)
	privateArtifactID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, "private-"+suffix, projects.VisibilityPrivate, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(ownerEmail))
	})

	root := t.TempDir()
	handler := New(&config.Config{}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: slug,
		AssetRoot:          root,
	})
	return assetHTTPFixture{
		ctx:               ctx,
		pool:              pool,
		handler:           handler,
		projectSlug:       slug,
		projectID:         projectID,
		assetRoot:         root,
		publicArtifactID:  publicArtifactID,
		privateArtifactID: privateArtifactID,
	}
}

func (f assetHTTPFixture) assetPath(assetID string) string {
	return "/api/p/" + f.projectSlug + "/assets/" + assetID + "/blob"
}

func insertHTTPAsset(t *testing.T, f assetHTTPFixture, filename, mimeType string, content []byte) (string, string) {
	t.Helper()
	sha := assets.Hash(content)
	store, err := assets.NewLocalFS(f.assetRoot)
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	key, err := store.Put(f.ctx, sha, content)
	if err != nil {
		t.Fatalf("store asset: %v", err)
	}
	return insertHTTPAssetRowOnly(t, f, filename, mimeType, int64(len(content)), sha, key), sha
}

func insertHTTPAssetRowOnly(t *testing.T, f assetHTTPFixture, filename, mimeType string, size int64, sha, storageKey string) string {
	t.Helper()
	var assetID string
	if err := f.pool.QueryRow(f.ctx, `
		INSERT INTO assets (
			project_id, sha256, mime_type, size_bytes, original_filename,
			storage_driver, storage_key, created_by
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, 'test'
		)
		RETURNING id::text
	`, f.projectID, sha, mimeType, size, filename, assets.DriverLocalFS, storageKey).Scan(&assetID); err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	return assetID
}

func attachHTTPAsset(t *testing.T, f assetHTTPFixture, artifactID, assetID, role string) {
	t.Helper()
	var revisionID string
	if err := f.pool.QueryRow(f.ctx, `
		SELECT id::text
		  FROM artifact_revisions
		 WHERE artifact_id = $1::uuid
		 ORDER BY revision_number DESC
		 LIMIT 1
	`, artifactID).Scan(&revisionID); err != nil {
		t.Fatalf("select revision: %v", err)
	}
	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO artifact_assets (artifact_id, artifact_revision_id, asset_id, role, display_order, created_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 0, 'test')
		ON CONFLICT DO NOTHING
	`, artifactID, revisionID, assetID, role); err != nil {
		t.Fatalf("attach asset: %v", err)
	}
}

func doAssetBlobRequest(handler http.Handler, method, path, remoteAddr string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	req.Host = strings.Split(remoteAddr, ":")[0]
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
