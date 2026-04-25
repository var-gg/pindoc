// Package auth holds Pindoc's caller-identity abstraction. A Principal is
// who is calling — UserID + AgentID + auth-mode metadata, account-level.
// ProjectScope is per-call: the project slug travels in each tool input
// and ResolveProject turns it into id/canonical-language/role after a project_members
// lookup. Resolvers are the per-mode flow that produces a Principal from
// the incoming request. The Resolver chain is the only place auth_mode
// branches exist — every tool handler downstream is mode-blind.
//
// Decision: mcp-scope-account-level-industry-standard supersedes the
// per-connection ProjectID/ProjectSlug fields once carried on Principal —
// industry MCPs (Notion / Figma / Linear / GitHub) all bind the
// connection at account level and take resource ids per call. Adding a
// new auth_mode (e.g. oauth_github via fosite IntrospectToken) means
// writing one new Resolver and prepending it to the chain — handler code
// does not change.
package auth

import "time"

// Principal is the per-call answer to "who is calling". Scope (which
// project the call is about) lives on auth.ProjectScope which the
// handler resolves from the input's project_slug field. Every handler
// receives a non-nil Principal — when no Resolver matches the incoming
// request, the chain short-circuits with an error before the handler is
// ever entered.
//
// Fields specific to V1.5+ modes (TokenID, ExpiresAt) are present but
// left at zero values for trusted_local — adding BearerTokenResolver
// later will populate them without touching the struct.
type Principal struct {
	// UserID is the users.id (uuid) row this caller is bound to. Empty
	// when the operator hasn't configured PINDOC_USER_NAME — handlers
	// that gate on identity (artifact.propose author tracking,
	// user.current/update) must check this.
	UserID string

	// AgentID is the server-issued display label for the calling agent
	// process (claude-code / codex / pindoc-admin). Server-trusted; the
	// `author_id` field in tool inputs remains a client-reported label
	// and is recorded separately. Empty falls back to "unassigned" in
	// audit rows.
	AgentID string

	// AuthMode is the resolver that produced this Principal
	// ("trusted_local" today; "oauth_github" / "project_token" /
	// "public_readonly" later). Carried for telemetry + debugging only;
	// handler logic must not branch on it.
	AuthMode string

	// TokenID is the auth_tokens.token_hash matched by a BearerToken /
	// OAuth resolver. Empty for trusted_local. Surfaced so audit log
	// can attribute writes to a specific issued token without storing
	// the bearer secret itself.
	TokenID string

	// ExpiresAt is the absolute moment the underlying credential stops
	// being valid. Zero = no expiry (trusted_local, long-lived
	// project_token without TTL). Resolvers that wrap an OAuth token
	// populate this so handlers needing long-running work can decide
	// whether to refresh or fail fast.
	ExpiresAt time.Time
}

// IsExpired reports whether the underlying credential's TTL has passed.
// Zero ExpiresAt is treated as "no expiry" so trusted_local Principals
// always return false. Handlers issuing long-running operations may
// gate on this before kicking off work that would outlive the token.
func (p *Principal) IsExpired(now time.Time) bool {
	if p == nil || p.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(p.ExpiresAt)
}
