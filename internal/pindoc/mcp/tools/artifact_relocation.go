package tools

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// artifactRelocationMove is one group of artifacts moving to a single
// target area. Decision taxonomy-change-operation T12: the artifact set
// is materialized at propose time, so apply moves exactly that fixed list
// — no dynamic query at apply time. ArtifactIDs entries are artifact
// UUIDs, slugs, or old slugs (lockAreaArtifact resolves all three).
type artifactRelocationMove struct {
	ToAreaSlug  string
	ArtifactIDs []string
}

// executeArtifactRelocation moves artifacts between areas inside a
// taxonomy change-set apply. It reuses the artifact.set_area per-artifact
// machinery (recordAreaChange), so each move emits a revision and an
// artifact.area_changed event — tagged here with taxonomy_change_id.
//
// It runs inside the caller's transaction: a failed move returns an error
// and the caller's rollback unwinds every prior move, so a partial
// relocation never commits (Decision T12). artifact slug/ref/edges are
// untouched — only area_id changes. The target must be a fileable, active
// area (resolveSetAreaTarget enforces it); a retiring source is allowed
// because relocation is precisely how a retiring area is drained.
func executeArtifactRelocation(ctx context.Context, tx pgx.Tx, p *auth.Principal, projectID, changeID, reason string, moves []artifactRelocationMove) (moved int, err error) {
	for _, m := range moves {
		targetArea, code := resolveSetAreaTarget(ctx, tx, projectID, m.ToAreaSlug)
		if code != "" {
			return moved, fmt.Errorf("relocation target %q rejected: %s", m.ToAreaSlug, code)
		}
		for _, ref := range m.ArtifactIDs {
			artifact, lockErr := lockAreaArtifact(ctx, tx, projectID, ref)
			if lockErr != nil {
				return moved, fmt.Errorf("lock relocation artifact %q: %w", ref, lockErr)
			}
			if artifact.AreaID == targetArea.ID {
				// Already in the target area — nothing to move.
				continue
			}
			if _, recErr := recordAreaChange(ctx, tx, p, artifact, targetArea, reason, "", "mcp_taxonomy_relocation", changeID); recErr != nil {
				return moved, recErr
			}
			moved++
		}
	}
	return moved, nil
}
