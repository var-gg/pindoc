package auth

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// BearerTokenResolver claims requests where the SDK's bearer-token
// middleware has already validated a Pindoc AS-issued JWT and stashed
// the resulting TokenInfo on the request. The resolver itself does not
// re-verify the token — that work belongs to the OAuth middleware that
// runs before the chain. Source is stamped as SourceOAuth (Decision
// `decision-auth-model-loopback-and-providers` § 1).
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
		Source:    SourceOAuth,
		ExpiresAt: info.Expiration,
	}
	if info.Extra != nil {
		if tokenID, ok := info.Extra["token_id"].(string); ok {
			p.TokenID = tokenID
		}
	}
	return p, nil
}
