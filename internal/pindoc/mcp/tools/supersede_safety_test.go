package tools

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/receipts"
)

func TestReceiptSnapshotsContainAny(t *testing.T) {
	snapshots := []receipts.ArtifactRef{{ArtifactID: "a"}, {ArtifactID: "b"}}
	candidates := []semanticCandidate{{ArtifactID: "c"}, {ArtifactID: "b"}}
	if !receiptSnapshotsContainAny(snapshots, candidates) {
		t.Fatalf("expected snapshot/candidate overlap")
	}
	if receiptSnapshotsContainAny(snapshots, []semanticCandidate{{ArtifactID: "z"}}) {
		t.Fatalf("unexpected overlap")
	}
}
