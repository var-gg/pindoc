package auth

import (
	"testing"
	"time"
)

// TestPrincipalIsExpired covers the three states: zero time (no
// expiry), future time (still valid), past time (expired). The boundary
// case is "now == ExpiresAt": defined as expired so token leaks at the
// exact deadline don't get one extra free request.
func TestPrincipalIsExpired(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "zero never expires", expiresAt: time.Time{}, want: false},
		{name: "past is expired", expiresAt: now.Add(-time.Hour), want: true},
		{name: "future is valid", expiresAt: now.Add(time.Hour), want: false},
		{name: "exactly now is expired", expiresAt: now, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Principal{ExpiresAt: c.expiresAt}
			if got := p.IsExpired(now); got != c.want {
				t.Fatalf("IsExpired(now) with ExpiresAt=%v = %v; want %v", c.expiresAt, got, c.want)
			}
		})
	}

	// Nil receiver also reports not expired so the failure mode for a
	// missing Principal is "auth blocks at handler check" rather than
	// "expiry check panics".
	var nilP *Principal
	if nilP.IsExpired(now) {
		t.Fatal("IsExpired on nil receiver returned true; want false")
	}
}
