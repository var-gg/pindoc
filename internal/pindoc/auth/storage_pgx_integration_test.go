package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ory/fosite"
	foauth2 "github.com/ory/fosite/handler/oauth2"
	fjwt "github.com/ory/fosite/token/jwt"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestFositeStoreIntegration(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	store := NewFositeStore(pool)
	suffix := uniqueOAuthSuffix()
	clientID := "store-client-" + suffix

	if err := store.UpsertClient(ctx, OAuthClient{
		ID:            clientID,
		RedirectURIs:  []string{"http://127.0.0.1:3846/callback"},
		GrantTypes:    []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes: []string{"code"},
		Scopes:        SupportedOAuthScopes(),
		Public:        true,
	}); err != nil {
		t.Fatalf("UpsertClient: %v", err)
	}
	client, err := store.GetClient(ctx, clientID)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	req := &fosite.Request{
		ID:                "req-" + suffix,
		RequestedAt:       time.Now().UTC(),
		Client:            client,
		RequestedScope:    fosite.Arguments{ScopePindoc},
		GrantedScope:      fosite.Arguments{ScopePindoc},
		RequestedAudience: fosite.Arguments{"http://127.0.0.1:5830/mcp"},
		GrantedAudience:   fosite.Arguments{"http://127.0.0.1:5830/mcp"},
		Form: url.Values{
			"code_challenge":        {"challenge"},
			"code_challenge_method": {"S256"},
		},
		Session: testOAuthSession("user-" + suffix),
	}

	if err := store.CreateAuthorizeCodeSession(ctx, "code-sig-"+suffix, req); err != nil {
		t.Fatalf("CreateAuthorizeCodeSession: %v", err)
	}
	gotAuth, err := store.GetAuthorizeCodeSession(ctx, "code-sig-"+suffix, nil)
	if err != nil {
		t.Fatalf("GetAuthorizeCodeSession: %v", err)
	}
	if gotAuth.GetClient().GetID() != clientID || !gotAuth.GetGrantedScopes().Has(ScopePindoc) {
		t.Fatalf("authorize request = %+v", gotAuth)
	}
	if err := store.InvalidateAuthorizeCodeSession(ctx, "code-sig-"+suffix); err != nil {
		t.Fatalf("InvalidateAuthorizeCodeSession: %v", err)
	}
	if _, err := store.GetAuthorizeCodeSession(ctx, "code-sig-"+suffix, nil); !errors.Is(err, fosite.ErrInvalidatedAuthorizeCode) {
		t.Fatalf("GetAuthorizeCodeSession after invalidate err = %v, want ErrInvalidatedAuthorizeCode", err)
	}

	if err := store.CreatePKCERequestSession(ctx, "pkce-sig-"+suffix, req); err != nil {
		t.Fatalf("CreatePKCERequestSession: %v", err)
	}
	gotPKCE, err := store.GetPKCERequestSession(ctx, "pkce-sig-"+suffix, nil)
	if err != nil {
		t.Fatalf("GetPKCERequestSession: %v", err)
	}
	if gotPKCE.GetRequestForm().Get("code_challenge_method") != "S256" {
		t.Fatalf("pkce form = %#v", gotPKCE.GetRequestForm())
	}
	if err := store.DeletePKCERequestSession(ctx, "pkce-sig-"+suffix); err != nil {
		t.Fatalf("DeletePKCERequestSession: %v", err)
	}

	if err := store.CreateAccessTokenSession(ctx, "access-sig-"+suffix, req); err != nil {
		t.Fatalf("CreateAccessTokenSession: %v", err)
	}
	gotAccess, err := store.GetAccessTokenSession(ctx, "access-sig-"+suffix, nil)
	if err != nil {
		t.Fatalf("GetAccessTokenSession: %v", err)
	}
	if gotAccess.GetSession().GetSubject() != "user-"+suffix {
		t.Fatalf("access subject = %q", gotAccess.GetSession().GetSubject())
	}
	if err := store.DeleteAccessTokenSession(ctx, "access-sig-"+suffix); err != nil {
		t.Fatalf("DeleteAccessTokenSession: %v", err)
	}

	if err := store.CreateRefreshTokenSession(ctx, "refresh-sig-"+suffix, "access-sig-"+suffix, req); err != nil {
		t.Fatalf("CreateRefreshTokenSession: %v", err)
	}
	gotRefresh, err := store.GetRefreshTokenSession(ctx, "refresh-sig-"+suffix, nil)
	if err != nil {
		t.Fatalf("GetRefreshTokenSession: %v", err)
	}
	if !gotRefresh.GetGrantedScopes().Has(ScopePindoc) {
		t.Fatalf("refresh scopes = %#v", gotRefresh.GetGrantedScopes())
	}
	if err := store.RotateRefreshToken(ctx, req.ID, "refresh-sig-"+suffix); err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}
	if _, err := store.GetRefreshTokenSession(ctx, "refresh-sig-"+suffix, nil); !errors.Is(err, fosite.ErrInactiveToken) {
		t.Fatalf("GetRefreshTokenSession after rotate err = %v, want ErrInactiveToken", err)
	}
}

func TestDCRPruneAndCapRetryIntegration(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	store := NewFositeStore(pool)
	suffix := uniqueOAuthSuffix()
	userID := insertOAuthTestUser(t, ctx, pool, suffix)
	projectSlug := insertOAuthTestProject(t, ctx, pool, suffix, userID)
	now := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < dcrMaxClients; i++ {
		createdAt := now
		expiresAt := now.Add(dcrClientLifetime)
		if i == 0 {
			createdAt = now.Add(-100 * 24 * time.Hour)
			expiresAt = now.Add(-time.Hour)
		}
		if err := store.UpsertClient(ctx, OAuthClient{
			ID:            fmt.Sprintf("dcr-cap-%s-%03d", suffix, i),
			DisplayName:   "DCR Cap",
			RedirectURIs:  []string{"http://127.0.0.1:3846/callback"},
			GrantTypes:    []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
			ResponseTypes: []string{"code"},
			Scopes:        SupportedOAuthScopes(),
			Public:        true,
			CreatedVia:    OAuthClientCreatedViaDCR,
			CreatedAt:     &createdAt,
			ExpiresAt:     &expiresAt,
		}); err != nil {
			t.Fatalf("UpsertClient(%d): %v", i, err)
		}
	}
	total, err := store.CountDCRClients(ctx)
	if err != nil {
		t.Fatalf("CountDCRClients before: %v", err)
	}
	if total != dcrMaxClients {
		t.Fatalf("DCR client count before = %d, want %d", total, dcrMaxClients)
	}

	svc := &OAuthService{store: store, defaultProjectSlug: projectSlug}
	if err := svc.ensureDCRCapacity(ctx, now); err != nil {
		t.Fatalf("ensureDCRCapacity: %v", err)
	}
	total, err = store.CountDCRClients(ctx)
	if err != nil {
		t.Fatalf("CountDCRClients after: %v", err)
	}
	if total != dcrMaxClients-1 {
		t.Fatalf("DCR client count after = %d, want %d", total, dcrMaxClients-1)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::int
		  FROM events e
		  JOIN projects p ON p.id = e.project_id
		 WHERE p.slug = $1 AND e.kind = 'oauth.dcr.clients_pruned'
	`, projectSlug).Scan(&eventCount); err != nil {
		t.Fatalf("select prune event: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("prune event count = %d, want 1", eventCount)
	}
}

func openOAuthIntegrationDB(t *testing.T) (context.Context, *db.Pool) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run OAuth DB integration")
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
	return ctx, pool
}

func uniqueOAuthSuffix() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func testOAuthSession(subject string) *foauth2.JWTSession {
	now := time.Now().UTC()
	headers := fjwt.NewHeaders()
	headers.Add("kid", "test")
	return &foauth2.JWTSession{
		JWTClaims: &fjwt.JWTClaims{
			Subject:   subject,
			Issuer:    "http://127.0.0.1:5830",
			Audience:  []string{"http://127.0.0.1:5830/mcp"},
			IssuedAt:  now,
			NotBefore: now,
		},
		JWTHeader: headers,
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AuthorizeCode: now.Add(defaultAuthorizeCodeTTL),
			fosite.AccessToken:   now.Add(defaultAccessTokenTTL),
			fosite.RefreshToken:  now.Add(defaultRefreshTokenTTL),
		},
		Subject:  subject,
		Username: subject,
	}
}
