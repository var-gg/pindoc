package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNoResolverMatched is returned by Chain.Resolve when every resolver
// declined to handle the request (returned (nil, nil)). Servers should
// surface this as 401 / unauthenticated to the caller — there's no
// principal to attribute the call to. trusted_local chains never hit
// this in practice because the resolver always matches.
var ErrNoResolverMatched = errors.New("auth: no resolver matched the incoming request")

// Resolver maps an incoming MCP tool-call request to the Principal that
// issued it. The per-request shape (rather than per-process) lets
// resolvers (BearerToken, OAuthSession) inspect headers / cookies on
// the actual request — even though TrustedLocalResolver is process-
// scoped and could in principle ignore the request entirely.
//
// A resolver returns:
//   - (principal, nil) when it recognises the request and produced a
//     Principal — the chain stops here.
//   - (nil, nil) when the request isn't its concern (e.g.
//     BearerTokenResolver on a request with no Authorization header).
//     The chain advances to the next resolver.
//   - (nil, err) on any internal failure (DB unreachable, malformed
//     token). The chain stops and the error propagates so the operator
//     sees the underlying issue rather than silently falling through to
//     a less-privileged resolver.
type Resolver interface {
	Resolve(ctx context.Context, req *sdk.CallToolRequest) (*Principal, error)
}

// Chain runs a slice of Resolvers in order: first non-nil principal
// wins, first non-nil error short-circuits. Empty chain always returns
// ErrNoResolverMatched, which is what tests of unconfigured servers
// should see.
//
// The chain is intentionally not source-aware. It doesn't know that
// resolvers[0] is "bearer_token" and resolvers[1] is "trusted_local";
// it just iterates. Source selection lives entirely at the
// construction site (server.NewServer + cmd/pindoc-server's main
// wiring), where Config.AuthProviders + Config.BindAddr decide which
// resolvers to include.
type Chain struct {
	resolvers []Resolver
}

// NewChain returns a Chain that tries each resolver in order. Pass
// them from "most specific" (e.g. BearerTokenResolver, which only
// matches when an Authorization header is present) to "most general"
// (e.g. TrustedLocalResolver, which always matches). Nil resolvers
// are skipped so callers can conditionally wire them with `nil` when
// a source isn't configured rather than juggling slice appends.
func NewChain(resolvers ...Resolver) *Chain {
	clean := make([]Resolver, 0, len(resolvers))
	for _, r := range resolvers {
		if r != nil {
			clean = append(clean, r)
		}
	}
	return &Chain{resolvers: clean}
}

// Resolve walks the chain. See Resolver doc for the per-resolver
// contract; the chain itself converts "no resolver matched" into
// ErrNoResolverMatched so the call site has one error sentinel to test
// against.
func (c *Chain) Resolve(ctx context.Context, req *sdk.CallToolRequest) (*Principal, error) {
	if c == nil {
		return nil, ErrNoResolverMatched
	}
	for _, r := range c.resolvers {
		p, err := r.Resolve(ctx, req)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, nil
		}
	}
	return nil, ErrNoResolverMatched
}

// Len returns the number of resolvers configured. Useful for boot-
// time logging and capability advertisement ("chain has 2 resolvers:
// bearer + trusted_local") so operators can tell which sources are
// live.
func (c *Chain) Len() int {
	if c == nil {
		return 0
	}
	return len(c.resolvers)
}

// IsLoopbackAddr reports whether `addr` (an `r.RemoteAddr`-style
// host:port or a bare host) names a loopback interface. Decision § 2
// "Loopback Trust Policy" uses this as the request-side switch
// between auto-trusted owner principals and OAuth-mandatory paths.
//
// Empty addr is treated as loopback because the SDK's stdio path
// fills RemoteAddr with "" and stdio runs as a child process of the
// caller — process trust is the loopback-equivalent envelope. The
// helper deliberately uses `r.RemoteAddr` only; reverse-proxy headers
// (`X-Forwarded-For`) are out of scope until a future
// PINDOC_TRUSTED_PROXIES allowlist lands.
func IsLoopbackAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// IsLoopbackRequest reports whether `r.RemoteAddr` resolves to a
// loopback host. Wraps IsLoopbackAddr so callers can drop a single
// `if auth.IsLoopbackRequest(r) { ... }` instead of remembering to
// pass the field name.
func IsLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return IsLoopbackAddr(r.RemoteAddr)
}

// HTTPDeps captures the optional dependencies PrincipalFromRequest
// needs to resolve an OAuth Principal from a Reader-side HTTP request.
// Loopback-only deployments leave OAuth nil; OAuth-enabled deployments
// pass the OAuthService that owns BrowserSessionUserID. DefaultUserID
// is the bootstrap user the loopback principal binds to (mirrors the
// trusted_local resolver's UserID input).
//
// TrustedSameHostProxy=true tells PrincipalFromRequest to accept any
// source IP as loopback. Set when the daemon is behind a same-host
// proxy that operators arrange themselves — Docker port forwarding,
// NSSM-wrapped reverse proxy, systemd-socket activation. cfg.IsLoopback
// Bind() being true is the operator's intent assertion that the public
// surface is same-host only; trusting the proxy chain follows from
// that assertion. Mismatch (proxy publishes externally + intent says
// loopback) is the operator's footgun, not a daemon-side check —
// PINDOC_BIND_ADDR is the contract surface.
type HTTPDeps struct {
	OAuth                *OAuthService
	DefaultUserID        string
	DefaultAgentID       string
	TrustedSameHostProxy bool
}

// PrincipalFromRequest is the single principal resolver Reader-side
// HTTP handlers (`/api/...`) call. It folds the previous "switch on
// d.authMode()" branches in handlers.go / invite.go / members.go
// into one rule: loopback (or trusted same-host proxy) requests are
// auto-trusted, non-loopback addresses must present a valid OAuth
// browser session. Any other case returns nil so the handler can
// answer 401.
//
// Decision `decision-auth-model-loopback-and-providers` § 2 covers
// the loopback fastpath; `task-loopback-trust-policy` is the
// implementation of this single helper across all five HTTP handlers
// that previously branched on auth_mode.
func PrincipalFromRequest(r *http.Request, deps HTTPDeps) *Principal {
	if r == nil {
		return nil
	}
	if isTrustedLoopback(r, deps) {
		return &Principal{
			UserID:  deps.DefaultUserID,
			AgentID: deps.DefaultAgentID,
			Source:  SourceLoopback,
		}
	}
	if deps.OAuth == nil {
		return nil
	}
	userID := strings.TrimSpace(deps.OAuth.BrowserSessionUserID(r))
	if userID == "" {
		return nil
	}
	return &Principal{
		UserID:  userID,
		AgentID: deps.DefaultAgentID,
		Source:  SourceOAuth,
	}
}

// isTrustedLoopback combines the request-side loopback check with
// the operator's same-host proxy opt-in (TrustedSameHostProxy). The
// docker compose daemon listens on 0.0.0.0 inside the container so
// docker port forwarding sees source IPs from the bridge gateway,
// not loopback — without the opt-in, every Reader call from the
// host would 401. Daemons running outside containers leave the flag
// unset and behave the same as before.
func isTrustedLoopback(r *http.Request, deps HTTPDeps) bool {
	if IsLoopbackRequest(r) {
		return true
	}
	return deps.TrustedSameHostProxy
}
