// Package auth holds Pindoc's caller-identity abstraction. A Principal is
// the per-call answer to "who is calling, into which project, with which
// role". Resolvers are the per-mode flow that produces a Principal from
// the incoming request. The Resolver chain is the only place auth_mode
// branches exist — every tool handler downstream is mode-blind.
//
// Decision: principal-resolver-architecture (V1). Adding a new auth_mode
// (e.g. oauth_github via fosite IntrospectToken) means writing one new
// Resolver and prepending it to the chain — handler code does not change.
package auth

import "time"

// Principal is the caller-identity + scope + role bound to one MCP tool
// call. Every handler receives a non-nil Principal — when no Resolver
// matches the incoming request, the chain short-circuits with an error
// before the handler is ever entered.
//
// The struct intentionally carries every field handlers reach for today
// (UserID, AgentID, ProjectSlug, ProjectLocale, Transport) so the
// migration is a mechanical rename. Fields specific to V1.5+ modes
// (TokenID, ExpiresAt, ProjectID) are present but left at zero values
// for trusted_local — adding BearerTokenResolver later will populate them
// without touching the struct.
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

	// ProjectID is the projects.id (uuid). Empty in V1 — the current
	// resolver chain pins by slug only and lets handlers dereference via
	// SQL JOIN. Reserved for V1.5+ when ACL checks need the row id
	// up-front to short-circuit DB roundtrips.
	ProjectID string

	// ProjectSlug is the projects.slug bound to this call's MCP server
	// scope. Stdio transport binds at process start (PINDOC_PROJECT);
	// streamable-HTTP binds per-connection from the /mcp/p/{project} URL.
	// Always populated for tool calls — empty would indicate a server
	// boot bug, not a runtime condition.
	ProjectSlug string

	// ProjectLocale is the projects.locale column for the active scope
	// (Task task-phase-18-project-locale-implementation, migration
	// 0015). HumanURL / AbsHumanURL embed it in /p/{slug}/{locale}/wiki/
	// share paths. Empty falls back to "en" inside HumanURL.
	ProjectLocale string

	// Role is the caller's permission tier within the active project.
	// V1 always emits "owner" (single-user trusted_local). V1.5 adds
	// "editor" / "viewer" once auth modes ship — handlers should
	// already use Can(action) rather than role-string equality so the
	// later expansion is a map edit, not a handler edit.
	Role string

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

	// Transport identifies which MCP transport delivered this call.
	// "stdio" = subprocess-per-session (legacy), "streamable_http" =
	// long-running daemon serving multiple connections. Drives
	// capability advertisement in pindoc.project.current —
	// fixed_session vs per_connection scope mode.
	Transport string
}

// roleActions enumerates which Principal.Role values are permitted to
// invoke each named action. V1 ships with one role ("owner") that
// satisfies every action — adding "editor" / "viewer" later is a map
// edit rather than a handler audit. New actions added here MUST also be
// referenced by the call site that needs them — Can() returns false for
// unknown actions on purpose so a typo at the call site fails closed.
var roleActions = map[string]map[string]bool{
	// Read actions: any authenticated principal can pull artifact /
	// project metadata. V1.5+ "viewer" role still satisfies these.
	"read.project":  {"owner": true, "editor": true, "viewer": true},
	"read.artifact": {"owner": true, "editor": true, "viewer": true},
	"read.area":     {"owner": true, "editor": true, "viewer": true},

	// Write actions: artifact.propose, area.create, project.create,
	// task.assign — owner + editor only. Viewer is read-only.
	"write.artifact": {"owner": true, "editor": true},
	"write.area":     {"owner": true, "editor": true},
	"write.task":     {"owner": true, "editor": true},
	"write.project":  {"owner": true},

	// User identity: only the principal's own row. owner can mutate
	// their own user; editor inherits self-edit; viewer cannot mutate.
	// V1.5 adds an explicit `user.update_other` action behind admin role.
	"write.user_self": {"owner": true, "editor": true},

	// Telemetry / capability surfaces — open to anyone authenticated.
	"read.capabilities": {"owner": true, "editor": true, "viewer": true},
}

// Can reports whether this Principal's role permits the named action.
// Returns false on unknown action names so a typo fails closed instead
// of silently allowing the call.
func (p *Principal) Can(action string) bool {
	if p == nil {
		return false
	}
	roles, ok := roleActions[action]
	if !ok {
		return false
	}
	return roles[p.Role]
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
