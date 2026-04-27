package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestRequest builds an httptest request with the given remote
// address. RemoteAddr is normally set by the HTTP server; httptest
// leaves it empty by default, so the helper sets it explicitly to
// exercise the loopback judgement.
func newTestRequest(method, path, remoteAddr string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = remoteAddr
	return r
}

// stubResolver is a configurable Resolver used by chain tests. Set
// `principal` to make it claim the request, `err` to make it fail, or
// leave both nil to make it pass through to the next resolver.
type stubResolver struct {
	principal *Principal
	err       error
	called    bool
}

func (s *stubResolver) Resolve(_ context.Context, _ *sdk.CallToolRequest) (*Principal, error) {
	s.called = true
	return s.principal, s.err
}

// TestChainResolve_FirstMatchWins covers the happy path: the first
// resolver to return a non-nil Principal terminates the chain, even if
// later resolvers would also have matched. This is the property that
// lets chains be ordered "most specific first" — BearerToken before
// TrustedLocal — without TrustedLocal silently overriding bearer
// auth.
func TestChainResolve_FirstMatchWins(t *testing.T) {
	first := &stubResolver{principal: &Principal{UserID: "first"}}
	second := &stubResolver{principal: &Principal{UserID: "second"}}

	c := NewChain(first, second)
	got, err := c.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got == nil || got.UserID != "first" {
		t.Fatalf("got principal %+v; want first", got)
	}
	if !first.called {
		t.Fatal("first resolver was not invoked")
	}
	if second.called {
		t.Fatal("second resolver was invoked despite first match")
	}
}

// TestChainResolve_PassThrough verifies the (nil, nil) contract: a
// resolver that doesn't claim the request advances to the next one.
// This is how BearerTokenResolver will gracefully decline requests
// with no Authorization header.
func TestChainResolve_PassThrough(t *testing.T) {
	skip := &stubResolver{} // nil principal, nil err
	match := &stubResolver{principal: &Principal{UserID: "match"}}

	c := NewChain(skip, match)
	got, err := c.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got == nil || got.UserID != "match" {
		t.Fatalf("got principal %+v; want match", got)
	}
	if !skip.called || !match.called {
		t.Fatalf("expected both resolvers called; skip=%v match=%v", skip.called, match.called)
	}
}

// TestChainResolve_ErrorShortCircuits verifies that any resolver
// returning a non-nil error stops the chain. Critical: a malformed
// bearer token must NOT silently fall through to TrustedLocal — the
// agent supplied credentials that failed to validate and should see a
// 401, not a free pass.
func TestChainResolve_ErrorShortCircuits(t *testing.T) {
	wantErr := errors.New("bad token")
	failing := &stubResolver{err: wantErr}
	fallback := &stubResolver{principal: &Principal{UserID: "fallback"}}

	c := NewChain(failing, fallback)
	got, err := c.Resolve(context.Background(), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Resolve err = %v; want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("expected nil principal on error; got %+v", got)
	}
	if fallback.called {
		t.Fatal("fallback resolver was invoked despite earlier error")
	}
}

// TestChainResolve_NoneMatch returns ErrNoResolverMatched when every
// resolver passes through. The error sentinel lets callers
// distinguish "no auth configured" (which the operator should fix)
// from "auth failed" (which the caller should retry or surface).
func TestChainResolve_NoneMatch(t *testing.T) {
	skip1 := &stubResolver{}
	skip2 := &stubResolver{}

	c := NewChain(skip1, skip2)
	_, err := c.Resolve(context.Background(), nil)
	if !errors.Is(err, ErrNoResolverMatched) {
		t.Fatalf("err = %v; want ErrNoResolverMatched", err)
	}
}

// TestChainResolve_EmptyChain mirrors NoneMatch for the degenerate
// empty case — useful for tests that wire up servers without auth and
// want the failure to be loud.
func TestChainResolve_EmptyChain(t *testing.T) {
	c := NewChain()
	_, err := c.Resolve(context.Background(), nil)
	if !errors.Is(err, ErrNoResolverMatched) {
		t.Fatalf("err = %v; want ErrNoResolverMatched", err)
	}
}

// TestChainResolve_NilChain guards against panics when the AuthChain
// dependency on tools.Deps is unset (legacy tests, partial wiring).
// The chain should still produce ErrNoResolverMatched rather than
// panic on a nil receiver.
func TestChainResolve_NilChain(t *testing.T) {
	var c *Chain
	_, err := c.Resolve(context.Background(), nil)
	if !errors.Is(err, ErrNoResolverMatched) {
		t.Fatalf("err = %v; want ErrNoResolverMatched", err)
	}
}

// TestNewChain_SkipsNilResolvers lets callers conditionally pass nil
// when a source isn't configured ("var bearer Resolver; if oauth then
// bearer = ...; chain := NewChain(bearer, trustedLocal)") without
// having to juggle slice appends.
func TestNewChain_SkipsNilResolvers(t *testing.T) {
	match := &stubResolver{principal: &Principal{UserID: "match"}}
	c := NewChain(nil, match, nil)
	if c.Len() != 1 {
		t.Fatalf("Len = %d; want 1 (nils filtered)", c.Len())
	}
	got, err := c.Resolve(context.Background(), nil)
	if err != nil || got == nil || got.UserID != "match" {
		t.Fatalf("Resolve = (%+v, %v); want match", got, err)
	}
}

// TestIsLoopbackAddr locks the loopback judgement table the request-
// side trust boundary depends on. Stdio (RemoteAddr="") gets the
// loopback verdict because process trust is the same envelope. IPv4
// loopback, IPv6 loopback, and the literal "localhost" host all
// register; non-loopback IPs do not.
func TestIsLoopbackAddr(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want bool
	}{
		{name: "empty (stdio)", addr: "", want: true},
		{name: "ipv4 loopback host:port", addr: "127.0.0.1:54321", want: true},
		{name: "ipv4 loopback bare", addr: "127.0.0.1", want: true},
		{name: "ipv6 loopback bracketed", addr: "[::1]:54321", want: true},
		{name: "ipv6 loopback bare", addr: "::1", want: true},
		{name: "localhost host:port", addr: "localhost:54321", want: true},
		{name: "localhost case-insensitive", addr: "Localhost:80", want: true},
		{name: "private LAN", addr: "192.168.1.4:54321", want: false},
		{name: "ipv4 mapped public", addr: "203.0.113.5:443", want: false},
		{name: "garbage", addr: "definitely-not-an-addr", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsLoopbackAddr(c.addr); got != c.want {
				t.Fatalf("IsLoopbackAddr(%q) = %v; want %v", c.addr, got, c.want)
			}
		})
	}
}

// TestPrincipalFromRequest_Loopback verifies the loopback fastpath
// returns a Source=loopback principal with the configured default
// user. This is the request-side mirror of TrustedLocalResolver for
// HTTP handlers.
func TestPrincipalFromRequest_Loopback(t *testing.T) {
	r := newTestRequest("GET", "/api/p/foo", "127.0.0.1:54321")
	p := PrincipalFromRequest(r, HTTPDeps{DefaultUserID: "user-1", DefaultAgentID: "agent-x"})
	if p == nil {
		t.Fatal("PrincipalFromRequest returned nil for loopback")
	}
	if !p.IsLoopback() {
		t.Fatalf("Source = %q; want loopback", p.Source)
	}
	if p.UserID != "user-1" || p.AgentID != "agent-x" {
		t.Fatalf("principal = %+v", p)
	}
}

// TestPrincipalFromRequest_NonLoopbackNoOAuth covers the deny path:
// non-loopback request with no OAuth dependency wired returns nil so
// the handler can emit 401. This is the security guarantee the
// Public-Without-Auth Refusal at boot-time relies on at runtime.
func TestPrincipalFromRequest_NonLoopbackNoOAuth(t *testing.T) {
	r := newTestRequest("GET", "/api/p/foo", "10.0.0.5:54321")
	if got := PrincipalFromRequest(r, HTTPDeps{}); got != nil {
		t.Fatalf("PrincipalFromRequest = %+v; want nil for non-loopback without OAuth", got)
	}
}

// TestPrincipalFromRequest_NilRequest is a defensive guard: handlers
// invoking the helper before the request is wired (test fixtures, odd
// server middleware) get a nil principal rather than a panic.
func TestPrincipalFromRequest_NilRequest(t *testing.T) {
	if got := PrincipalFromRequest(nil, HTTPDeps{DefaultUserID: "x"}); got != nil {
		t.Fatalf("PrincipalFromRequest(nil) = %+v; want nil", got)
	}
}
