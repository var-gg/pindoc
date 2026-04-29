package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/readstate"
)

type artifactReadStateInput struct {
	ProjectSlug  string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	ArtifactSlug string `json:"artifact_slug,omitempty" jsonschema:"artifact slug; mutually exclusive with artifact_id"`
	ArtifactID   string `json:"artifact_id,omitempty" jsonschema:"artifact UUID; mutually exclusive with artifact_slug"`
	UserKey      string `json:"user_key,omitempty" jsonschema:"reader user_key; defaults to 'local' in trusted_local mode"`
}

type artifactReadStateOutput struct {
	ArtifactID     string  `json:"artifact_id"`
	UserKey        string  `json:"user_key"`
	ReadState      string  `json:"read_state"`
	CompletionPct  float64 `json:"completion_pct"`
	LastSeenAt     string  `json:"last_seen_at,omitempty"`
	EventCount     int     `json:"event_count"`
	ToolsetVersion string  `json:"toolset_version,omitempty"`
}

// RegisterArtifactReadState wires pindoc.artifact.read_state — Layer 2 of
// the read tracking model (see docs/02-concepts.md). Agents call this to
// ask "has a human actually read this artifact?" before promoting an
// AI-authored revision into the verification candidate lane.
func RegisterArtifactReadState(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.read_state",
			Description: "Per-artifact read state for a given user_key. Returns one of unseen/glanced/read/deeply_read plus completion_pct and last_seen_at. Use before promoting an AI-authored revision into the verification candidate lane — 'deeply_read' is the human-engagement signal that bridges Layer 2 (read state) to Layer 4 (verification).",
		},
		func(ctx context.Context, p *auth.Principal, in artifactReadStateInput) (*sdk.CallToolResult, artifactReadStateOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactReadStateOutput{}, fmt.Errorf("resolve project: %w", err)
			}
			ref := strings.TrimSpace(in.ArtifactID)
			if ref == "" {
				ref = strings.TrimSpace(in.ArtifactSlug)
			}
			if ref == "" {
				return nil, artifactReadStateOutput{}, errors.New("artifact_id or artifact_slug is required")
			}
			userKey := strings.TrimSpace(in.UserKey)
			if userKey == "" {
				userKey = "local"
			}
			state, err := readstate.ArtifactState(ctx, deps.DB, scope.ProjectSlug, ref, userKey)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, artifactReadStateOutput{}, fmt.Errorf("artifact not found: %s", ref)
				}
				return nil, artifactReadStateOutput{}, fmt.Errorf("read state: %w", err)
			}
			out := artifactReadStateOutput{
				ArtifactID:    state.ArtifactID,
				UserKey:       state.UserKey,
				ReadState:     string(state.ReadState),
				CompletionPct: state.CompletionPct,
				EventCount:    state.EventCount,
			}
			if state.LastSeenAt != nil {
				out.LastSeenAt = state.LastSeenAt.Format(time.RFC3339)
			}
			return nil, out, nil
		},
	)
}
