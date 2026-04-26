package auth

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type BearerTokenResolver struct {
	agentID string
}

func NewBearerTokenResolver(agentID string) *BearerTokenResolver {
	return &BearerTokenResolver{agentID: agentID}
}

func (r *BearerTokenResolver) Resolve(_ context.Context, req *sdk.CallToolRequest) (*Principal, error) {
	if r == nil || req == nil || req.GetExtra() == nil || req.GetExtra().TokenInfo == nil {
		return nil, nil
	}
	info := req.GetExtra().TokenInfo
	userID := strings.TrimSpace(info.UserID)
	if userID == "" {
		return nil, fmt.Errorf("auth: bearer token has no user id")
	}
	p := &Principal{
		UserID:    userID,
		AgentID:   r.agentID,
		AuthMode:  AuthModeOAuthGitHub,
		ExpiresAt: info.Expiration,
	}
	if info.Extra != nil {
		if tokenID, ok := info.Extra["token_id"].(string); ok {
			p.TokenID = tokenID
		}
	}
	return p, nil
}
