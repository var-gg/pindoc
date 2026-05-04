package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestParseInboxLimit(t *testing.T) {
	cases := []struct {
		raw  string
		want int
		ok   bool
	}{
		{"", defaultInboxLimit, true},
		{"5", 5, true},
		{"999", maxInboxLimit, true},
		{"0", 0, false},
		{"nope", 0, false},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/api/p/pindoc/inbox?limit="+c.raw, nil)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			got, ok := parseInboxLimit(req)
			if got != c.want || ok != c.ok {
				t.Fatalf("parseInboxLimit(%q) = (%d,%v), want (%d,%v)", c.raw, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestInboxMembershipVisibilityAndAuditIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run inbox HTTP DB integration")
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
	slug := "inbox-sec-" + suffix
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Inbox Owner "+suffix, "inbox-owner-"+suffix+"@example.invalid")
	viewerID := insertInviteHTTPUser(t, ctx, pool, "Inbox Viewer "+suffix, "inbox-viewer-"+suffix+"@example.invalid")
	outsiderID := insertInviteHTTPUser(t, ctx, pool, "Inbox Outsider "+suffix, "inbox-outsider-"+suffix+"@example.invalid")
	victimID := insertInviteHTTPUser(t, ctx, pool, "Inbox Victim "+suffix, "inbox-victim-"+suffix+"@example.invalid")
	projectID := insertInviteHTTPProject(t, ctx, pool, slug, ownerID)
	insertInviteHTTPMember(t, ctx, pool, projectID, viewerID, pauth.RoleViewer)
	areaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, projectID, "misc")
	publicID := insertInboxPendingArtifact(t, ctx, pool, projectID, areaID, "inbox-public-"+suffix, projects.VisibilityPublic, ownerID)
	orgSlug := "inbox-org-" + suffix
	insertInboxPendingArtifact(t, ctx, pool, projectID, areaID, orgSlug, projects.VisibilityOrg, ownerID)
	privateSlug := "inbox-private-" + suffix
	privateID := insertInboxPendingArtifact(t, ctx, pool, projectID, areaID, privateSlug, projects.VisibilityPrivate, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2, $3, $4)
		`, "inbox-owner-"+suffix+"@example.invalid", "inbox-viewer-"+suffix+"@example.invalid", "inbox-outsider-"+suffix+"@example.invalid", "inbox-victim-"+suffix+"@example.invalid")
	})

	oauthSvc := newInboxOAuthService(t, ctx, pool, suffix)
	handler := newInboxHTTPHandler(pool, oauthSvc, slug, ownerID)

	outsiderGet := doInviteRequest(t, handler, oauthSvc, outsiderID, http.MethodGet, "/api/p/"+slug+"/inbox", "")
	if outsiderGet.Code != http.StatusNotFound {
		t.Fatalf("outsider legacy inbox status = %d, want 404; body=%s", outsiderGet.Code, outsiderGet.Body.String())
	}
	outsiderReview := doInviteRequest(t, handler, oauthSvc, outsiderID, http.MethodPost, "/api/p/"+slug+"/inbox/"+orgSlug+"/review", `{"decision":"approve","commit_msg":"ok"}`)
	if outsiderReview.Code != http.StatusNotFound {
		t.Fatalf("outsider legacy review status = %d, want 404; body=%s", outsiderReview.Code, outsiderReview.Body.String())
	}

	viewerGet := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/inbox", "")
	if viewerGet.Code != http.StatusOK {
		t.Fatalf("viewer inbox status = %d, want 200; body=%s", viewerGet.Code, viewerGet.Body.String())
	}
	var viewerBody inboxResp
	if err := json.NewDecoder(viewerGet.Body).Decode(&viewerBody); err != nil {
		t.Fatalf("decode viewer inbox: %v", err)
	}
	if viewerBody.TotalCount != 2 || viewerBody.Count != 2 {
		t.Fatalf("viewer count = %d total=%d, want 2/2", viewerBody.Count, viewerBody.TotalCount)
	}
	if inboxHasArtifact(viewerBody.Items, privateSlug) {
		t.Fatalf("viewer inbox leaked private artifact %q", privateSlug)
	}

	viewerPrivateReview := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodPost, "/api/p/"+slug+"/inbox/"+privateSlug+"/review", `{"decision":"approve","commit_msg":"ok"}`)
	if viewerPrivateReview.Code != http.StatusNotFound {
		t.Fatalf("viewer private review status = %d, want 404; body=%s", viewerPrivateReview.Code, viewerPrivateReview.Body.String())
	}

	loopbackGet := doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/p/"+slug+"/inbox", "")
	if loopbackGet.Code != http.StatusOK {
		t.Fatalf("loopback inbox status = %d, want 200; body=%s", loopbackGet.Code, loopbackGet.Body.String())
	}
	var loopbackBody inboxResp
	if err := json.NewDecoder(loopbackGet.Body).Decode(&loopbackBody); err != nil {
		t.Fatalf("decode loopback inbox: %v", err)
	}
	if loopbackBody.TotalCount != 3 || !inboxHasArtifact(loopbackBody.Items, privateSlug) {
		t.Fatalf("loopback inbox = total %d items %+v, want all pending review rows", loopbackBody.TotalCount, loopbackBody.Items)
	}

	ownerReview := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPost, "/api/p/"+slug+"/inbox/"+privateSlug+"/review", `{"decision":"approve","reviewer_id":"`+victimID+`","commit_msg":""}`)
	if ownerReview.Code != http.StatusOK {
		t.Fatalf("owner approve status = %d, want 200; body=%s", ownerReview.Code, ownerReview.Body.String())
	}
	assertInboxReviewEvent(t, ctx, pool, privateID, ownerID, "")

	rejectBlank := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPost, "/api/p/"+slug+"/inbox/"+publicID+"/review", `{"decision":"reject","commit_msg":"   "}`)
	if rejectBlank.Code != http.StatusBadRequest {
		t.Fatalf("blank reject status = %d, want 400; body=%s", rejectBlank.Code, rejectBlank.Body.String())
	}
	assertInboxErrorCode(t, rejectBlank.Body.String(), inboxRejectReasonErrorCode)

	oversized := strings.Repeat("x", maxInboxReviewCommitBytes+1)
	tooLarge := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPost, "/api/p/"+slug+"/inbox/"+publicID+"/review", `{"decision":"approve","commit_msg":"`+oversized+`"}`)
	if tooLarge.Code != http.StatusBadRequest {
		t.Fatalf("oversized approve status = %d, want 400; body=%s", tooLarge.Code, tooLarge.Body.String())
	}
	assertInboxErrorCode(t, tooLarge.Body.String(), "REVIEW_COMMIT_MSG_TOO_LONG")
}

func TestInboxPaginationIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run inbox pagination HTTP DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
	slug := "inbox-page-" + suffix
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Inbox Page Owner "+suffix, "inbox-page-owner-"+suffix+"@example.invalid")
	projectID := insertInviteHTTPProject(t, ctx, pool, slug, ownerID)
	areaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, projectID, "misc")
	for i := 0; i < 250; i++ {
		insertInboxPendingArtifact(t, ctx, pool, projectID, areaID, fmt.Sprintf("inbox-page-%03d-%s", i, suffix), projects.VisibilityPublic, ownerID)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, "inbox-page-owner-"+suffix+"@example.invalid")
	})

	handler := newInboxHTTPHandler(pool, nil, slug, ownerID)
	defaultPage := doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/p/"+slug+"/inbox", "")
	if defaultPage.Code != http.StatusOK {
		t.Fatalf("default page status = %d, want 200; body=%s", defaultPage.Code, defaultPage.Body.String())
	}
	var defaultBody inboxResp
	if err := json.NewDecoder(defaultPage.Body).Decode(&defaultBody); err != nil {
		t.Fatalf("decode default page: %v", err)
	}
	if len(defaultBody.Items) != defaultInboxLimit || defaultBody.TotalCount != 250 || !defaultBody.Truncated {
		t.Fatalf("default page len=%d total=%d truncated=%v, want 200/250/true", len(defaultBody.Items), defaultBody.TotalCount, defaultBody.Truncated)
	}

	smallPage := doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/p/"+slug+"/inbox?limit=5", "")
	if smallPage.Code != http.StatusOK {
		t.Fatalf("small page status = %d, want 200; body=%s", smallPage.Code, smallPage.Body.String())
	}
	var smallBody inboxResp
	if err := json.NewDecoder(smallPage.Body).Decode(&smallBody); err != nil {
		t.Fatalf("decode small page: %v", err)
	}
	if len(smallBody.Items) != 5 || smallBody.Limit != 5 || smallBody.TotalCount != 250 || !smallBody.Truncated {
		t.Fatalf("small page len=%d limit=%d total=%d truncated=%v, want 5/5/250/true", len(smallBody.Items), smallBody.Limit, smallBody.TotalCount, smallBody.Truncated)
	}
}

func insertInboxPendingArtifact(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug, visibility, authorUserID string) string {
	t.Helper()
	artifactID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, slug, visibility, authorUserID)
	if _, err := pool.Exec(ctx, `
		UPDATE artifacts
		   SET review_state = 'pending_review',
		       status = 'draft',
		       updated_at = now()
		 WHERE id = $1::uuid
	`, artifactID); err != nil {
		t.Fatalf("mark pending review: %v", err)
	}
	return artifactID
}

func newInboxOAuthService(t *testing.T, ctx context.Context, pool *db.Pool, suffix string) *pauth.OAuthService {
	t.Helper()
	oauthSvc, err := pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
		Issuer:             "http://127.0.0.1:5830",
		PublicBaseURL:      "http://127.0.0.1:5830",
		RedirectBaseURL:    "http://127.0.0.1:5830",
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           "inbox-http-" + suffix,
		RedirectURIs:       []string{"http://127.0.0.1:3846/callback"},
		GitHubClientID:     "fake-gh-client",
		GitHubClientSecret: "fake-gh-secret",
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}
	return oauthSvc
}

func newInboxHTTPHandler(pool *db.Pool, oauthSvc *pauth.OAuthService, slug, defaultUserID string) http.Handler {
	authProviders := []string{}
	if oauthSvc != nil {
		authProviders = []string{config.AuthProviderGitHub}
	}
	return New(&config.Config{
		AuthProviders: authProviders,
		BindAddr:      "0.0.0.0:5830",
	}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: slug,
		DefaultUserID:      defaultUserID,
		OAuth:              oauthSvc,
		AuthProviders:      authProviders,
		BindAddr:           "0.0.0.0:5830",
	})
}

func inboxHasArtifact(items []artifactRow, slug string) bool {
	for _, item := range items {
		if item.Slug == slug {
			return true
		}
	}
	return false
}

func assertInboxErrorCode(t *testing.T, body, want string) {
	t.Helper()
	var got inboxAPIError
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode inbox error %q: %v", body, err)
	}
	if got.ErrorCode != want {
		t.Fatalf("error_code = %q, want %q; body=%s", got.ErrorCode, want, body)
	}
}

func assertInboxReviewEvent(t *testing.T, ctx context.Context, pool *db.Pool, artifactID, wantReviewerID, wantCommitMsg string) {
	t.Helper()
	var reviewerID, commitMsg *string
	if err := pool.QueryRow(ctx, `
		SELECT payload->>'reviewer_id', payload->>'commit_msg'
		  FROM events
		 WHERE subject_id = $1::uuid
		   AND kind = 'review.approved'
		 ORDER BY created_at DESC
		 LIMIT 1
	`, artifactID).Scan(&reviewerID, &commitMsg); err != nil {
		t.Fatalf("select review event: %v", err)
	}
	if reviewerID == nil || *reviewerID != wantReviewerID {
		t.Fatalf("reviewer_id = %v, want %q", reviewerID, wantReviewerID)
	}
	if commitMsg == nil || *commitMsg != wantCommitMsg {
		t.Fatalf("commit_msg = %v, want %q", commitMsg, wantCommitMsg)
	}
}
