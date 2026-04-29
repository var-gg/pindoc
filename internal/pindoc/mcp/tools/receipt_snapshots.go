package tools

import (
	"context"

	"github.com/var-gg/pindoc/internal/pindoc/receipts"
)

// headSnapshotsForArtifacts looks up the current revision number for each
// artifact in the list. Used at search-time to record the snapshot the
// receipt is bound to; Phase E moved the primary staleness signal off
// the 30-min clock and onto "did any of these artifacts move past this
// revision". Returns snapshots in the same order as input ids; artifacts
// with no revision row (shouldn't happen post-0017) are omitted.
func headSnapshotsForArtifacts(ctx context.Context, deps Deps, artifactIDs []string) []receipts.ArtifactRef {
	if len(artifactIDs) == 0 {
		return nil
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT artifact_id::text,
		       COALESCE(max(revision_number), 0) AS head_rev
		FROM artifact_revisions
		WHERE artifact_id = ANY($1::uuid[])
		GROUP BY artifact_id
	`, artifactIDs)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("snapshot head lookup failed — receipt issued without snapshots", "err", err)
		}
		return nil
	}
	defer rows.Close()
	found := make(map[string]int, len(artifactIDs))
	for rows.Next() {
		var id string
		var rev int
		if err := rows.Scan(&id, &rev); err != nil {
			if deps.Logger != nil {
				deps.Logger.Warn("snapshot scan failed", "err", err)
			}
			continue
		}
		found[id] = rev
	}
	if err := rows.Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("snapshot rows err", "err", err)
		}
	}
	out := make([]receipts.ArtifactRef, 0, len(artifactIDs))
	for _, id := range artifactIDs {
		if rev, ok := found[id]; ok {
			out = append(out, receipts.ArtifactRef{ArtifactID: id, RevisionNumber: rev})
		}
	}
	return out
}

// checkReceiptSupersedes returns the subset of snapshot refs whose
// artifact has advanced past the snapshotted revision_number since the
// receipt was issued. The caller decides what to do: all superseded =>
// RECEIPT_SUPERSEDED (reject), partial drift => advisory warning, none
// => carry on.
func checkReceiptSupersedes(ctx context.Context, deps Deps, snapshots []receipts.ArtifactRef) ([]receipts.ArtifactRef, error) {
	if len(snapshots) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(snapshots))
	want := make(map[string]int, len(snapshots))
	for _, s := range snapshots {
		ids = append(ids, s.ArtifactID)
		want[s.ArtifactID] = s.RevisionNumber
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT artifact_id::text,
		       COALESCE(max(revision_number), 0) AS head_rev
		FROM artifact_revisions
		WHERE artifact_id = ANY($1::uuid[])
		GROUP BY artifact_id
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	current := make(map[string]int, len(ids))
	for rows.Next() {
		var id string
		var rev int
		if err := rows.Scan(&id, &rev); err != nil {
			return nil, err
		}
		current[id] = rev
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []receipts.ArtifactRef
	for _, s := range snapshots {
		cur, ok := current[s.ArtifactID]
		if !ok {
			// Artifact vanished (archived?) — treat as superseded.
			out = append(out, s)
			continue
		}
		if cur > s.RevisionNumber {
			out = append(out, s)
		}
	}
	return out, nil
}
