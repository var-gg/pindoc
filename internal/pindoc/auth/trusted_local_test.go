package auth

import (
	"context"
	"testing"
)

// TestTrustedLocalResolver_StampsAccountFields verifies the resolver
// faithfully copies the constructor inputs onto the Principal it
// returns. Account-level (Decision mcp-scope-account-level-industry-
// standard) means only UserID + AgentID + AuthMode travel on the
// Principal — project info comes from each tool input via
// ResolveProject.
func TestTrustedLocalResolver_StampsAccountFields(t *testing.T) {
	r := NewTrustedLocalResolver("user-uuid", "claude-code-A")
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
	r := NewTrustedLocalResolver("", "agent-x")
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
	if p.AuthMode != AuthModeTrustedLocal {
		t.Errorf("AuthMode = %q; want %q (empty UserID should not change mode)", p.AuthMode, AuthModeTrustedLocal)
	}
}

// TestTrustedLocalResolver_ReturnsCopy verifies handlers can safely
// mutate the returned Principal without bleeding state across
// requests. Important for any future debug code that augments the
// principal mid-call.
func TestTrustedLocalResolver_ReturnsCopy(t *testing.T) {
	r := NewTrustedLocalResolver("u", "a")
	first, _ := r.Resolve(context.Background(), nil)
	first.UserID = "mutated"

	second, _ := r.Resolve(context.Background(), nil)
	if second.UserID != "u" {
		t.Fatalf("second Resolve UserID = %q; mutation on first call leaked into resolver state", second.UserID)
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
