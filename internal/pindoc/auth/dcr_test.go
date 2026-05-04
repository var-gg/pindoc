package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestDCRSecretExpiresAtUnix(t *testing.T) {
	if got := dcrSecretExpiresAtUnix(time.Time{}); got != 0 {
		t.Fatalf("zero expiry = %d, want 0", got)
	}
	expiry := time.Unix(1893456000, 0).UTC()
	if got := dcrSecretExpiresAtUnix(expiry); got != 1893456000 {
		t.Fatalf("expiry = %d, want unix epoch", got)
	}
}

func TestDCRRateLimiter(t *testing.T) {
	limiter := newDCRRateLimiter()
	now := time.Unix(1000, 0)
	for i := 0; i < dcrRateLimitPerIP; i++ {
		if !limiter.Allow("203.0.113.10:49000", now.Add(time.Duration(i)*time.Minute), dcrRateLimitPerIP, dcrRateLimitWindow) {
			t.Fatalf("hit %d rejected before limit", i+1)
		}
	}
	if limiter.Allow("203.0.113.10:49000", now.Add(10*time.Minute), dcrRateLimitPerIP, dcrRateLimitWindow) {
		t.Fatal("limit hit was allowed")
	}
	if !limiter.Allow("203.0.113.10:49000", now.Add(dcrRateLimitWindow+time.Minute), dcrRateLimitPerIP, dcrRateLimitWindow) {
		t.Fatal("hit after window should be allowed")
	}
}

func TestDCRRateLimiterPrunesExpiredUniqueKeys(t *testing.T) {
	limiter := newDCRRateLimiter()
	now := time.Unix(1000, 0)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("203.0.%d.%d:49000", i/255, i%255)
		if !limiter.Allow(key, now, dcrRateLimitPerIP, dcrRateLimitWindow) {
			t.Fatalf("unique key %d rejected", i)
		}
	}
	if got := limiter.Len(); got == 0 {
		t.Fatal("limiter should hold keys before expiry")
	}
	pruned := limiter.PruneExpired(now.Add(dcrRateLimitWindow+time.Second), dcrRateLimitWindow)
	if pruned == 0 {
		t.Fatal("PruneExpired removed no expired keys")
	}
	if got := limiter.Len(); got != 0 {
		t.Fatalf("limiter map size after expiry = %d, want 0", got)
	}
}
