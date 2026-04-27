package auth

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Role values mirror project_members.role. Defined in the auth package
// so handlers don't need a separate `roles` import for the three
// strings the table accepts.
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// TrustedLocalResolver produces a loopback Principal from process-level
// state (env-derived user). Always matches every request — it never
// returns (nil, nil), so chains that include it terminate at this
// resolver if no upstream resolver claimed the request first.
//
// "Loopback Trust" (Decision § 2): stdio MCP transport runs as a child
// process of the caller, so OS process boundaries are the trust
// envelope. The streamable_http transport relies on the daemon binding
// to a loopback address — non-loopback HTTP requests must reach a
// resolver that demands a Bearer JWT (the OAuth middleware refuses
// them before the chain runs).
type TrustedLocalResolver struct {
	template Principal
}

// NewTrustedLocalResolver captures the per-process identity the chain
// should stamp on every Principal it produces. Account-level scope
// (Decision mcp-scope-account-level-industry-standard) means project
// info no longer travels on the Principal — handlers read project_slug
// from each tool input and call auth.ResolveProject for the per-call
// ProjectScope, so the constructor takes only the env-derived user
// fields.
//
// Pass an empty UserID when PINDOC_USER_NAME isn't set — handlers that
// gate on identity (artifact.propose author tracking, user.current /
// user.update) check Principal.UserID and surface a stable USER_NOT_SET
// not_ready code so the agent knows to prompt the operator. The
// resolver still returns a non-nil Principal so capability /
// project.current calls don't 401 just because identity isn't wired.
func NewTrustedLocalResolver(userID, agentID string) *TrustedLocalResolver {
	return &TrustedLocalResolver{
		template: Principal{
			UserID:  userID,
			AgentID: agentID,
			Source:  SourceLoopback,
		},
	}
}

// Resolve returns a copy of the captured Principal. The copy is
// intentional — handlers that mutate the returned struct (rare, but
// possible during debugging) must not bleed those mutations back into
// future calls. Returning a fresh struct each call keeps the model
// "Principal is per-call data" even though TrustedLocal's payload
// happens to be constant for a given process.
//
// The req argument is unused today — trusted_local doesn't inspect
// headers — but is part of the Resolver contract so the chain
// signature is stable across all sources.
func (r *TrustedLocalResolver) Resolve(_ context.Context, _ *sdk.CallToolRequest) (*Principal, error) {
	if r == nil {
		return nil, nil
	}
	p := r.template
	return &p, nil
}
