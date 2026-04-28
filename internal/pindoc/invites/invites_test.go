package invites

import (
	"testing"
	"time"
)

func TestInactiveTreatsNilExpiryAsPermanent(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	if inactive(&Record{ExpiresAt: nil}, now) {
		t.Fatal("permanent invite should remain active")
	}
}

func TestInactiveRejectsExpiredTimestamp(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	if !inactive(&Record{ExpiresAt: &expiresAt}, now) {
		t.Fatal("expired invite should be inactive")
	}
}

func TestExtendToValidation(t *testing.T) {
	for _, input := range []string{"+7d", "+30d", "permanent"} {
		if !ValidExtendTo(input) {
			t.Fatalf("%q should be a valid extend target", input)
		}
	}
	if ValidExtendTo("+1d") {
		t.Fatal("+1d should not be accepted as an extend target")
	}
}
