package auth

import (
	"context"
	"testing"
)

// TestTrustedLocalResolver_StampsAllFields verifies the resolver
// faithfully copies the constructor inputs onto the Principal it
// returns. This is the contract the V1 server relies on — the
// env-derived UserID / project scope must reach handlers exactly as
// boot-time wiring set them.
func TestTrustedLocalResolver_StampsAllFields(t *testing.T) {
	r := NewTrustedLocalResolver(
		"user-uuid",
		"claude-code-A",
		"proj-uuid",
		"pindoc",
		"ko",
		"streamable_http",
	)
	p, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if p == nil {
		t.Fatal("Resolve returned nil principal — TrustedLocal must always match")
	}
	if p.UserID != "user-uuid" {
		t.Errorf("UserID = %q; want user-uuid", p.UserID)
	}
	if p.AgentID != "claude-code-A" {
		t.Errorf("AgentID = %q; want claude-code-A", p.AgentID)
	}
	if p.ProjectID != "proj-uuid" {
		t.Errorf("ProjectID = %q; want proj-uuid", p.ProjectID)
	}
	if p.ProjectSlug != "pindoc" {
		t.Errorf("ProjectSlug = %q; want pindoc", p.ProjectSlug)
	}
	if p.ProjectLocale != "ko" {
		t.Errorf("ProjectLocale = %q; want ko", p.ProjectLocale)
	}
	if p.Transport != "streamable_http" {
		t.Errorf("Transport = %q; want streamable_http", p.Transport)
	}
	if p.Role != RoleOwner {
		t.Errorf("Role = %q; want %q (V1 trusted_local always emits owner)", p.Role, RoleOwner)
	}
	if p.AuthMode != AuthModeTrustedLocal {
		t.Errorf("AuthMode = %q; want %q", p.AuthMode, AuthModeTrustedLocal)
	}
}

// TestTrustedLocalResolver_EmptyUserIDStillMatches covers the
// "operator skipped identity setup" boot path. The resolver must still
// produce a Principal so capability / project.current calls succeed —
// handlers that gate on identity check Principal.UserID themselves
// and surface USER_NOT_SET. Returning nil here would 401 the entire
// session, which is too aggressive for OSS first-boot UX.
func TestTrustedLocalResolver_EmptyUserIDStillMatches(t *testing.T) {
	r := NewTrustedLocalResolver("", "agent-x", "", "pindoc", "ko", "stdio")
	p, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil principal even with empty UserID")
	}
	if p.UserID != "" {
		t.Errorf("UserID = %q; want empty", p.UserID)
	}
	if p.Role != RoleOwner {
		t.Errorf("Role = %q; want owner — empty UserID should not demote role", p.Role)
	}
}

// TestTrustedLocalResolver_ReturnsCopy verifies handlers can safely
// mutate the returned Principal without bleeding state across
// requests. Important for any future debug code that augments the
// principal mid-call (e.g. rewriting Role for testing).
func TestTrustedLocalResolver_ReturnsCopy(t *testing.T) {
	r := NewTrustedLocalResolver("u", "a", "", "pindoc", "ko", "stdio")
	first, _ := r.Resolve(context.Background(), nil)
	first.Role = "viewer" // mutate the returned struct

	second, _ := r.Resolve(context.Background(), nil)
	if second.Role != RoleOwner {
		t.Fatalf("second Resolve Role = %q; mutation on first call leaked into resolver state", second.Role)
	}
}

// TestTrustedLocalResolver_NilReceiver guards against the rare
// boot-error path where the resolver pointer was never wired up.
// Returning (nil, nil) lets the chain advance cleanly to whatever
// follows (or surface ErrNoResolverMatched), rather than panicking
// inside the chain loop.
func TestTrustedLocalResolver_NilReceiver(t *testing.T) {
	var r *TrustedLocalResolver
	p, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if p != nil {
		t.Fatalf("p = %+v; want nil", p)
	}
}
