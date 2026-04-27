package auth

import (
	"context"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBearerTokenResolver_StampsTokenPrincipal(t *testing.T) {
	expiresAt := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	req := &sdk.CallToolRequest{
		Extra: &sdk.RequestExtra{
			TokenInfo: &mcpauth.TokenInfo{
				UserID:     "user-uuid",
				Expiration: expiresAt,
				Extra:      map[string]any{"token_id": "sig-123"},
			},
		},
	}

	p, err := NewBearerTokenResolver("agent-x").Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if p == nil {
		t.Fatal("Resolve() principal = nil")
	}
	if p.UserID != "user-uuid" || p.AgentID != "agent-x" || p.Source != SourceOAuth || p.TokenID != "sig-123" {
		t.Fatalf("principal = %+v", p)
	}
	if !p.IsOAuth() {
		t.Fatalf("IsOAuth() = false; want true on bearer-token principal")
	}
	if !p.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", p.ExpiresAt, expiresAt)
	}
}

func TestBearerTokenResolver_PassThroughWithoutTokenInfo(t *testing.T) {
	p, err := NewBearerTokenResolver("agent-x").Resolve(context.Background(), &sdk.CallToolRequest{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if p != nil {
		t.Fatalf("Resolve() principal = %+v, want nil", p)
	}
}
