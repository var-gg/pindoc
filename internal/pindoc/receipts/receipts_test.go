package receipts

import (
	"testing"
	"time"
)

// TestIssueVerifyRoundtrip is the baseline contract: a freshly-issued
// receipt verifies clean for the same project.
func TestIssueVerifyRoundtrip(t *testing.T) {
	s := New(time.Hour)
	id := s.Issue("pindoc", "why", nil)
	if id == "" {
		t.Fatalf("Issue returned empty id")
	}
	res := s.Verify(id, "pindoc")
	if !res.Valid {
		t.Fatalf("expected Valid=true, got %+v", res)
	}
	if res.IssuedQuery != "why" {
		t.Fatalf("query echo got=%q want=%q", res.IssuedQuery, "why")
	}
	if len(res.Snapshots) != 0 {
		t.Fatalf("nil-snapshots issue should verify with empty snapshots, got %v", res.Snapshots)
	}
}

func TestIssueOneUseConsumesAfterSuccessfulVerify(t *testing.T) {
	s := New(time.Hour)
	id := s.IssueOneUse("pindoc", "project.create bootstrap", nil)
	if id == "" {
		t.Fatalf("IssueOneUse returned empty id")
	}
	if res := s.Verify(id, "pindoc"); !res.Valid {
		t.Fatalf("first Verify should succeed, got %+v", res)
	}
	if res := s.Verify(id, "pindoc"); !res.Unknown {
		t.Fatalf("second Verify should be consumed/unknown, got %+v", res)
	}
}

func TestIssueOneUseWrongProjectDoesNotConsume(t *testing.T) {
	s := New(time.Hour)
	id := s.IssueOneUse("proj-a", "project.create bootstrap", nil)
	if res := s.Verify(id, "proj-b"); !res.WrongProject {
		t.Fatalf("wrong-project Verify should report WrongProject, got %+v", res)
	}
	if res := s.Verify(id, "proj-a"); !res.Valid {
		t.Fatalf("correct project should still consume after wrong-project attempt, got %+v", res)
	}
}

// TestVerifyWrongProject proves project-scope isolation — a receipt
// issued for project A cannot be used to write into project B.
func TestVerifyWrongProject(t *testing.T) {
	s := New(time.Hour)
	id := s.Issue("proj-a", "q", nil)
	res := s.Verify(id, "proj-b")
	if !res.WrongProject {
		t.Fatalf("expected WrongProject=true, got %+v", res)
	}
}

// TestVerifyUnknown covers the "never issued (or already swept)" branch.
func TestVerifyUnknown(t *testing.T) {
	s := New(time.Hour)
	res := s.Verify("sr_nosuch", "pindoc")
	if !res.Unknown {
		t.Fatalf("expected Unknown=true, got %+v", res)
	}
}

// TestSnapshotsRoundtrip asserts Phase E — snapshots survive Issue/
// Verify so the DB-aware call site can run its corpus-drift check.
func TestSnapshotsRoundtrip(t *testing.T) {
	s := New(time.Hour)
	want := []ArtifactRef{
		{ArtifactID: "00000000-0000-0000-0000-000000000001", RevisionNumber: 3},
		{ArtifactID: "00000000-0000-0000-0000-000000000002", RevisionNumber: 7},
	}
	id := s.Issue("pindoc", "snap query", want)
	res := s.Verify(id, "pindoc")
	if !res.Valid {
		t.Fatalf("Verify failed: %+v", res)
	}
	if len(res.Snapshots) != len(want) {
		t.Fatalf("snapshot count got=%d want=%d", len(res.Snapshots), len(want))
	}
	for i, got := range res.Snapshots {
		if got.ArtifactID != want[i].ArtifactID || got.RevisionNumber != want[i].RevisionNumber {
			t.Fatalf("snapshot[%d] got=%+v want=%+v", i, got, want[i])
		}
	}
}

// TestSnapshotsIsolated asserts the Issue caller can mutate their
// original slice after handoff without affecting the stored snapshot —
// matters because artifact.search reuses a hits buffer across rows.
func TestSnapshotsIsolated(t *testing.T) {
	s := New(time.Hour)
	snaps := []ArtifactRef{{ArtifactID: "x", RevisionNumber: 1}}
	id := s.Issue("pindoc", "q", snaps)
	snaps[0].RevisionNumber = 99 // caller mutation after handoff
	res := s.Verify(id, "pindoc")
	if res.Snapshots[0].RevisionNumber != 1 {
		t.Fatalf("mutation leaked into store: got rev=%d want=1", res.Snapshots[0].RevisionNumber)
	}
}

// TestExpiryFallback confirms the long fallback TTL still removes old
// receipts. Phase E demoted the clock to memory-pressure relief, but
// the sweep goroutine still has to evict stale entries. Either outcome
// is acceptable — Expired=true (clock tripped before sweep) or
// Unknown=true (sweep already dropped it); Valid=true after expiry is
// the only way this can fail.
func TestExpiryFallback(t *testing.T) {
	s := New(10 * time.Millisecond)
	id := s.Issue("pindoc", "q", nil)
	time.Sleep(30 * time.Millisecond)
	res := s.Verify(id, "pindoc")
	if res.Valid {
		t.Fatalf("receipt should not validate after fallback TTL, got %+v", res)
	}
	if !(res.Expired || res.Unknown) {
		t.Fatalf("expected Expired or Unknown after fallback TTL, got %+v", res)
	}
}
