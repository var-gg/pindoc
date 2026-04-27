// Package auth holds Pindoc's caller-identity abstraction. A Principal is
// who is calling — UserID + AgentID + Source metadata, account-level.
// ProjectScope is per-call: the project slug travels in each tool input
// and ResolveProject turns it into id/canonical-language/role after a
// project_members lookup. Resolvers are the per-source flow that
// produces a Principal from the incoming request.
//
// Decision `decision-auth-model-loopback-and-providers` retired the
// 4-mode `auth_mode` enum that previously straddled "is the daemon
// public", "which IdP is active", and "how many users are there" axes
// in one variable. Principal.Source is the single bit handlers branch
// on now — "loopback" (process / 127.0.0.1 trust boundary) vs "oauth"
// (Pindoc AS validated bearer JWT). New IdPs are still SourceOAuth
// once exchanged for a Pindoc AS token; they don't add new Source
// values because the framing is "Pindoc-issued JWT vs not".
package auth

import "time"

// Source values stamped on Principal by the resolver chain. New IdPs
// (Google / passkey / local-password) all flow through the Pindoc AS
// and surface as SourceOAuth — Source reports the trust path Pindoc
// took to identify this caller, not the upstream IdP.
const (
	// SourceLoopback marks Principals produced by the trusted-local
	// resolver: the request came in over stdio (process trust) or from
	// a loopback HTTP address. Decision § 2 "Loopback Trust Policy".
	SourceLoopback = "loopback"

	// SourceOAuth marks Principals produced from a Bearer JWT issued
	// by the Pindoc Authorization Server. Decision § 1 "AS / IdP / RS
	// 명시적 분리".
	SourceOAuth = "oauth"
)

// Principal is the per-call answer to "who is calling". Scope (which
// project the call is about) lives on auth.ProjectScope which the
// handler resolves from the input's project_slug field. Every handler
// receives a non-nil Principal — when no Resolver matches the incoming
// request, the chain short-circuits with an error before the handler
// is ever entered.
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

	// Source is the trust path that produced this Principal:
	// SourceLoopback (process / 127.0.0.1) or SourceOAuth (Pindoc AS
	// JWT). project_scope.go branches off this to decide whether to
	// short-circuit role lookup as owner (loopback) or consult the
	// project_members table (oauth). Carried for telemetry too.
	Source string

	// TokenID is the auth_tokens.token_hash matched by a BearerToken /
	// OAuth resolver. Empty for loopback. Surfaced so audit log can
	// attribute writes to a specific issued token without storing the
	// bearer secret itself.
	TokenID string

	// ExpiresAt is the absolute moment the underlying credential stops
	// being valid. Zero = no expiry (loopback, long-lived tokens).
	// Resolvers that wrap an OAuth token populate this so handlers
	// needing long-running work can decide whether to refresh or fail
	// fast.
	ExpiresAt time.Time
}

// IsExpired reports whether the underlying credential's TTL has passed.
// Zero ExpiresAt is treated as "no expiry" so loopback Principals
// always return false. Handlers issuing long-running operations may
// gate on this before kicking off work that would outlive the token.
func (p *Principal) IsExpired(now time.Time) bool {
	if p == nil || p.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(p.ExpiresAt)
}

// IsLoopback reports whether this Principal came from the loopback
// trust path. Sugar over `p.Source == SourceLoopback` so handlers
// reading their own logic don't have to import the constant.
func (p *Principal) IsLoopback() bool {
	return p != nil && p.Source == SourceLoopback
}

// IsOAuth reports whether this Principal came from a Pindoc AS bearer
// JWT. Used by project_scope.go to decide between "loopback owner
// short-circuit" and "consult project_members".
func (p *Principal) IsOAuth() bool {
	return p != nil && p.Source == SourceOAuth
}
