package tools

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/indexstate"
)

// refreshArtifactIndex is the MCP orchestration boundary around indexstate.
// Provider failures are durable, retryable index outcomes and do not reject an
// otherwise valid artifact write. Database/storage failures remain fatal so
// the caller rolls the whole transaction back.
func refreshArtifactIndex(
	ctx context.Context,
	tx pgx.Tx,
	deps Deps,
	artifactID string,
	revisionNumber int,
	title string,
	body string,
) (*indexstate.State, error) {
	in := indexstate.RefreshInput{
		ArtifactID:     artifactID,
		RevisionNumber: revisionNumber,
		Title:          title,
		Body:           body,
	}
	if deps.Embedder == nil {
		state, err := indexstate.MarkUnknown(ctx, tx, in)
		if err != nil {
			return nil, err
		}
		return &state, nil
	}

	state, err := indexstate.Refresh(ctx, tx, deps.Embedder, in)
	if indexstate.IsRetryable(err) {
		if deps.Logger != nil {
			deps.Logger.Warn("artifact index refresh failed; preserving previous chunks",
				"artifact_id", artifactID,
				"revision_number", revisionNumber,
				"err", err,
			)
		}
		return &state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("refresh artifact index: %w", err)
	}
	return &state, nil
}
