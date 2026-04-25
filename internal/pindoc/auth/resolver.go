package auth

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNoResolverMatched is returned by Chain.Resolve when every resolver
// declined to handle the request (returned (nil, nil)). Servers should
// surface this as 401 / unauthenticated to the caller — there's no
// principal to attribute the call to. trusted_local chains never hit
// this in practice because the resolver always matches.
var ErrNoResolverMatched = errors.New("auth: no resolver matched the incoming request")

// Resolver maps an incoming MCP tool-call request to the Principal that
// issued it. One Resolver represents exactly one auth_mode. The
// per-request shape (rather than per-process) lets future resolvers
// (BearerToken, OAuthSession) inspect headers / cookies on the actual
// request — even though V1's TrustedLocalResolver is process-scoped and
// could in principle ignore the request entirely.
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
// The chain is intentionally not auth_mode-aware. It doesn't know that
// resolvers[0] is "trusted_local" and resolvers[1] is "bearer_token";
// it just iterates. Mode selection lives entirely in the construction
// site (server.NewServer + cmd/pindoc-server's main wiring), where
// PINDOC_AUTH_MODE / future config decides which resolvers to include.
type Chain struct {
	resolvers []Resolver
}

// NewChain returns a Chain that tries each resolver in order. Pass them
// from "most specific" (e.g. BearerTokenResolver, which only matches
// when an Authorization header is present) to "most general" (e.g.
// TrustedLocalResolver, which always matches). Nil resolvers are
// skipped so callers can conditionally wire them with `nil` when a
// mode isn't configured rather than juggling slice appends.
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

// Len returns the number of resolvers configured. Useful for boot-time
// logging and capability advertisement ("chain has 2 resolvers: bearer
// + trusted_local") so operators can tell which auth modes are live.
func (c *Chain) Len() int {
	if c == nil {
		return 0
	}
	return len(c.resolvers)
}
