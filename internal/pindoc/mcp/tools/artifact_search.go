package tools

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type artifactSearchInput struct {
	Query  string   `json:"query" jsonschema:"user's natural-language question"`
	TopK   int      `json:"top_k,omitempty" jsonschema:"default 5, max 20"`
	Types  []string `json:"types,omitempty" jsonschema:"filter by artifact type (Decision, Debug, ...)"`
	Areas  []string `json:"areas,omitempty" jsonschema:"filter by area slug"`
}

type SearchHit struct {
	ArtifactID   string  `json:"artifact_id"`
	Slug         string  `json:"slug"`
	Type         string  `json:"type"`
	Title        string  `json:"title"`
	AreaSlug     string  `json:"area_slug"`
	ChunkKind    string  `json:"chunk_kind"`
	ChunkHeading string  `json:"chunk_heading,omitempty"`
	Snippet      string  `json:"snippet"`
	Distance     float64 `json:"distance"`
	// AgentRef + HumanURL: agent feeds AgentRef into artifact.read, shares
	// HumanURL in chat. Both always populated on a hit. HumanURLAbs is
	// populated only when server_settings.public_base_url is configured.
	AgentRef    string `json:"agent_ref"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

type artifactSearchOutput struct {
	Query  string      `json:"query"`
	Hits   []SearchHit `json:"hits"`
	Notice string      `json:"notice,omitempty"`
	// SearchReceipt is a short-lived opaque token (TTL 10 min). Agents
	// pass it back as basis.search_receipt on the next artifact.propose
	// to satisfy Phase 11b's server-enforced "search before write" rule.
	SearchReceipt string `json:"search_receipt,omitempty"`
}

// RegisterArtifactSearch wires pindoc.artifact.search. Does a vector
// similarity query over artifact_chunks, groups hits per artifact, and
// returns the best chunk per artifact. Filters on type/area happen in SQL.
func RegisterArtifactSearch(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.artifact.search",
			Description: "Semantic search over Pindoc artifacts. Returns the best matching chunk per artifact with distance (lower = closer). Use before writing a new artifact to avoid duplicates. Filters on type and area_slug.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactSearchInput) (*sdk.CallToolResult, artifactSearchOutput, error) {
			if strings.TrimSpace(in.Query) == "" {
				return nil, artifactSearchOutput{}, fmt.Errorf("query is required")
			}
			if in.TopK <= 0 {
				in.TopK = 5
			}
			if in.TopK > 20 {
				in.TopK = 20
			}
			if deps.Embedder == nil {
				return nil, artifactSearchOutput{
					Query:  in.Query,
					Notice: "embedder not configured on this server; search disabled",
				}, nil
			}

			res, err := deps.Embedder.Embed(ctx, embed.Request{
				Texts: []string{in.Query}, Kind: embed.KindQuery,
			})
			if err != nil {
				return nil, artifactSearchOutput{}, fmt.Errorf("embed query: %w", err)
			}
			if len(res.Vectors) != 1 {
				return nil, artifactSearchOutput{}, fmt.Errorf("embed query returned %d vectors", len(res.Vectors))
			}
			qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))

			// DISTINCT ON picks the best (closest) chunk per artifact after
			// ORDER BY artifact_id, distance. Then we re-sort by distance
			// and clamp to TopK. Cheaper than two-phase aggregate for
			// V1 dataset sizes.
			sql := `
				WITH scored AS (
					SELECT DISTINCT ON (c.artifact_id)
						c.artifact_id,
						c.kind           AS chunk_kind,
						c.heading        AS chunk_heading,
						c.text           AS snippet,
						c.embedding <=> $1::vector AS distance
					FROM artifact_chunks c
					JOIN artifacts a ON a.id = c.artifact_id
					JOIN projects p ON p.id = a.project_id
					JOIN areas    ar ON ar.id = a.area_id
					WHERE p.slug = $2
					  AND a.status <> 'archived'
					  AND ($3::text[] IS NULL OR a.type   = ANY($3))
					  AND ($4::text[] IS NULL OR ar.slug  = ANY($4))
					ORDER BY c.artifact_id, distance
				)
				SELECT
					s.artifact_id::text,
					a.slug,
					a.type,
					a.title,
					ar.slug,
					s.chunk_kind,
					s.chunk_heading,
					s.snippet,
					s.distance
				FROM scored s
				JOIN artifacts a  ON a.id  = s.artifact_id
				JOIN areas     ar ON ar.id = a.area_id
				ORDER BY s.distance
				LIMIT $5
			`

			var typesArg, areasArg any
			if len(in.Types) > 0 {
				typesArg = in.Types
			}
			if len(in.Areas) > 0 {
				areasArg = in.Areas
			}

			rows, err := deps.DB.Query(ctx, sql, qVec, deps.ProjectSlug, typesArg, areasArg, in.TopK)
			if err != nil {
				return nil, artifactSearchOutput{}, fmt.Errorf("search query: %w", err)
			}
			defer rows.Close()

			out := artifactSearchOutput{Query: in.Query, Hits: []SearchHit{}}
			for rows.Next() {
				var h SearchHit
				var heading *string
				if err := rows.Scan(
					&h.ArtifactID, &h.Slug, &h.Type, &h.Title,
					&h.AreaSlug, &h.ChunkKind, &heading,
					&h.Snippet, &h.Distance,
				); err != nil {
					return nil, artifactSearchOutput{}, fmt.Errorf("scan: %w", err)
				}
				if heading != nil {
					h.ChunkHeading = *heading
				}
				// Trim long snippets for transport efficiency. Agent can
				// fetch full body via artifact.read if needed.
				h.Snippet = trimSnippet(h.Snippet, 400)
				h.AgentRef = "pindoc://" + h.Slug
				h.HumanURL = HumanURL(deps.ProjectSlug, h.Slug)
				h.HumanURLAbs = AbsHumanURL(deps.Settings, deps.ProjectSlug, h.Slug)
				out.Hits = append(out.Hits, h)
			}

			if deps.Embedder.Info().Name == "stub" {
				out.Notice = "stub embedder — ranking is hash-based, not semantic. Swap to a real embedding provider to get meaningful results."
			}
			if deps.Receipts != nil {
				out.SearchReceipt = deps.Receipts.Issue(deps.ProjectSlug, in.Query)
			}
			return nil, out, rows.Err()
		},
	)
}

func trimSnippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}
