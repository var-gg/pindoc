package auth

import (
	"testing"
)

// TestSetGitHubCredentials_Lifecycle locks the hot-reload contract
// task-providers-admin-ui depends on: SetGitHubCredentials swaps the
// GitHub OAuth client at runtime, empty inputs unwire the IdP, and
// inconsistent inputs (one half missing) surface as an error rather
// than corrupting the stored config silently. Doesn't touch the DB or
// build a full OAuthService — exercises the swap path in isolation.
func TestSetGitHubCredentials_Lifecycle(t *testing.T) {
	svc := &OAuthService{
		issuer:          "http://127.0.0.1:5830",
		publicBaseURL:   "http://127.0.0.1:5830",
		redirectBaseURL: "http://127.0.0.1:5830",
		cookieSecret:    []byte("test-cookie-secret-padding-bytes"),
	}
	if svc.HasGitHub() {
		t.Fatal("HasGitHub = true on fresh service")
	}

	if err := svc.SetGitHubCredentials("client-a", "secret-a"); err != nil {
		t.Fatalf("SetGitHubCredentials initial: %v", err)
	}
	if !svc.HasGitHub() {
		t.Fatal("HasGitHub = false after credential set")
	}
	first := svc.currentGitHub()
	if first == nil || first.config.ClientID != "client-a" {
		t.Fatalf("first credentials wired wrong: %+v", first)
	}

	if err := svc.SetGitHubCredentials("client-b", "secret-b"); err != nil {
		t.Fatalf("rotate credentials: %v", err)
	}
	rotated := svc.currentGitHub()
	if rotated == nil || rotated.config.ClientID != "client-b" {
		t.Fatalf("rotated credentials wired wrong: %+v", rotated)
	}

	if err := svc.SetGitHubCredentials("", ""); err != nil {
		t.Fatalf("unwire credentials: %v", err)
	}
	if svc.HasGitHub() {
		t.Fatal("HasGitHub = true after unwire")
	}

	if err := svc.SetGitHubCredentials("client-c", ""); err == nil {
		t.Fatal("inconsistent (id only) error = nil; want non-nil")
	}
	if err := svc.SetGitHubCredentials("", "secret-c"); err == nil {
		t.Fatal("inconsistent (secret only) error = nil; want non-nil")
	}
}
