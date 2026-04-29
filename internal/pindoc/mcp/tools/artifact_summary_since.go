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
	"github.com/var-gg/pindoc/internal/pindoc/diff"
)

type summarySinceInput struct {
	ProjectSlug string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	IDOrSlug    string `json:"id_or_slug"`
	SinceRev    int    `json:"since_rev,omitempty" jsonschema:"revision number to compare from"`
	SinceTime   string `json:"since_time,omitempty" jsonschema:"RFC3339 timestamp; revisions after this"`
}

type summarySinceOutput struct {
	ArtifactID     string        `json:"artifact_id"`
	Slug           string        `json:"slug"`
	Steps          []summaryStep `json:"steps"`
	TotalStats     diff.Stats    `json:"total_stats"`
	ToolsetVersion string        `json:"toolset_version,omitempty"`
}

// Split into its own file so the AddTool registration uses the real input
// type (artifact_history.go's SummarySince registration was a placeholder
// in an iteration; this replaces it).
func RegisterArtifactSummary(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.summary_since",
			Description: "List every revision since a reference point (since_rev OR since_time) with per-step section_deltas and aggregate stats. Use this when a user asks 'what changed recently on X?' — the steps array is designed to be read aloud directly.",
		},
		func(ctx context.Context, p *auth.Principal, in summarySinceInput) (*sdk.CallToolResult, summarySinceOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, summarySinceOutput{}, fmt.Errorf("artifact.summary_since: %w", err)
			}
			ref := normalizeRef(in.IDOrSlug)
			if ref == "" {
				return nil, summarySinceOutput{}, errors.New("id_or_slug is required")
			}

			var artifactID, slug string
			err = deps.DB.QueryRow(ctx, `
				SELECT a.id::text, a.slug
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
			`, scope.ProjectSlug, ref).Scan(&artifactID, &slug)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, summarySinceOutput{}, fmt.Errorf("artifact %q not found", ref)
			}
			if err != nil {
				return nil, summarySinceOutput{}, err
			}

			// Which revs qualify?
			var qualifyingRevs []int
			switch {
			case in.SinceRev > 0:
				rows, err := deps.DB.Query(ctx, `
					SELECT revision_number FROM artifact_revisions
					WHERE artifact_id = $1 AND revision_number > $2
					ORDER BY revision_number
				`, artifactID, in.SinceRev)
				if err != nil {
					return nil, summarySinceOutput{}, err
				}
				for rows.Next() {
					var r int
					if err := rows.Scan(&r); err != nil {
						rows.Close()
						return nil, summarySinceOutput{}, err
					}
					qualifyingRevs = append(qualifyingRevs, r)
				}
				rows.Close()
			case strings.TrimSpace(in.SinceTime) != "":
				t, err := time.Parse(time.RFC3339, in.SinceTime)
				if err != nil {
					return nil, summarySinceOutput{}, fmt.Errorf("since_time %q: %w", in.SinceTime, err)
				}
				rows, err := deps.DB.Query(ctx, `
					SELECT revision_number FROM artifact_revisions
					WHERE artifact_id = $1 AND created_at > $2
					ORDER BY revision_number
				`, artifactID, t)
				if err != nil {
					return nil, summarySinceOutput{}, err
				}
				for rows.Next() {
					var r int
					if err := rows.Scan(&r); err != nil {
						rows.Close()
						return nil, summarySinceOutput{}, err
					}
					qualifyingRevs = append(qualifyingRevs, r)
				}
				rows.Close()
			default:
				return nil, summarySinceOutput{}, errors.New("either since_rev or since_time is required")
			}

			out := summarySinceOutput{ArtifactID: artifactID, Slug: slug, Steps: []summaryStep{}}
			if len(qualifyingRevs) == 0 {
				return nil, out, nil
			}

			// Pair each qualifying rev with its immediate predecessor.
			for _, toRev := range qualifyingRevs {
				fromRev := toRev - 1
				if fromRev < 1 {
					continue
				}
				from, err := loadRev(ctx, deps, artifactID, fromRev)
				if err != nil {
					return nil, summarySinceOutput{}, err
				}
				to, err := loadRev(ctx, deps, artifactID, toRev)
				if err != nil {
					return nil, summarySinceOutput{}, err
				}
				stats, deltas := diff.Summary(from.body, to.body)
				out.Steps = append(out.Steps, summaryStep{
					From:          from.meta,
					To:            to.meta,
					Stats:         stats,
					SectionDeltas: deltas,
				})
				out.TotalStats.LinesAdded += stats.LinesAdded
				out.TotalStats.LinesRemoved += stats.LinesRemoved
				out.TotalStats.BytesAdded += stats.BytesAdded
				out.TotalStats.BytesRemoved += stats.BytesRemoved
			}
			return nil, out, nil
		},
	)
}
