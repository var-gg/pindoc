package auth

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// AuthModeTrustedLocal is the AuthMode string TrustedLocalResolver
// stamps on every Principal it produces. Other resolvers use their own
// constants ("oauth_github", "project_token") so telemetry can group by
// mode without parsing the resolver type at runtime.
const AuthModeTrustedLocal = "trusted_local"

// RoleOwner is the V1 default role assigned to every trusted_local
// caller by ResolveProject. Single-user self-host deployments don't
// yet have a meaningful distinction between owner / editor / viewer —
// all writes come from "the operator". V1.5 ACL Task introduces
// actual role tiers via the project_members table.
const RoleOwner = "owner"

// TrustedLocalResolver produces a Principal from process-level state
// (env-derived user). Always matches every request — it never returns
// (nil, nil), so chains that include it terminate at this resolver if
// no upstream resolver claimed the request first.
//
// This resolver is the OSS default and the one that lets dev / homelab
// deployments run without any token wiring. It is also why the
// auth_mode spectrum keeps "trusted_local" as a first-class mode
// rather than retiring it once OAuth ships.
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
			UserID:   userID,
			AgentID:  agentID,
			AuthMode: AuthModeTrustedLocal,
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
// signature is stable across all modes.
func (r *TrustedLocalResolver) Resolve(_ context.Context, _ *sdk.CallToolRequest) (*Principal, error) {
	if r == nil {
		return nil, nil
	}
	p := r.template
	return &p, nil
}
