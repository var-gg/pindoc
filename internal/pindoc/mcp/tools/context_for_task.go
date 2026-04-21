package tools

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type contextForTaskInput struct {
	TaskDescription string   `json:"task_description" jsonschema:"free-form natural language description of what the agent is about to do"`
	TopK            int      `json:"top_k,omitempty" jsonschema:"number of artifacts to return; default 3, max 10"`
	Areas           []string `json:"areas,omitempty" jsonschema:"optional area_slug filter"`
}

type ContextLanding struct {
	ArtifactID   string `json:"artifact_id"`
	Slug         string `json:"slug"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	AreaSlug     string `json:"area_slug"`
	Rationale    string `json:"rationale"` // why this is relevant — picked from best chunk heading/text
	URL          string `json:"url"`
	Distance     float64 `json:"distance"`
}

type contextForTaskOutput struct {
	TaskDescription string           `json:"task_description"`
	Landings        []ContextLanding `json:"landings"`
	Notice          string           `json:"notice,omitempty"`
}

// RegisterContextForTask wires pindoc.context.for_task — the Fast Landing
// mechanism from docs/05 §M6. Call this at the start of a task to get
// 1–3 artifacts the agent should read before doing anything else. Lower
// TopK on purpose: Fast Landing is about first-hop precision, not recall.
func RegisterContextForTask(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.context.for_task",
			Description: "Given a natural-language task description, return the 1–3 most relevant artifacts in this project. Call this at the start of any non-trivial task before grepping code or writing new artifacts. Tuning: smaller TopK than artifact.search because this optimises for first-hop precision, not recall.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in contextForTaskInput) (*sdk.CallToolResult, contextForTaskOutput, error) {
			if strings.TrimSpace(in.TaskDescription) == "" {
				return nil, contextForTaskOutput{}, fmt.Errorf("task_description is required")
			}
			if in.TopK <= 0 {
				in.TopK = 3
			}
			if in.TopK > 10 {
				in.TopK = 10
			}
			if deps.Embedder == nil {
				return nil, contextForTaskOutput{
					TaskDescription: in.TaskDescription,
					Notice:          "embedder not configured on this server; context.for_task disabled",
				}, nil
			}

			res, err := deps.Embedder.Embed(ctx, embed.Request{
				Texts: []string{in.TaskDescription}, Kind: embed.KindQuery,
			})
			if err != nil {
				return nil, contextForTaskOutput{}, fmt.Errorf("embed: %w", err)
			}
			qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))

			sql := `
				WITH scored AS (
					SELECT DISTINCT ON (c.artifact_id)
						c.artifact_id,
						COALESCE(c.heading, '') AS best_heading,
						c.text                   AS best_text,
						c.embedding <=> $1::vector AS distance
					FROM artifact_chunks c
					JOIN artifacts a ON a.id = c.artifact_id
					JOIN projects p ON p.id = a.project_id
					JOIN areas    ar ON ar.id = a.area_id
					WHERE p.slug = $2
					  AND a.status <> 'archived'
					  AND ($3::text[] IS NULL OR ar.slug = ANY($3))
					ORDER BY c.artifact_id, distance
				)
				SELECT
					s.artifact_id::text, a.slug, a.type, a.title, ar.slug,
					s.best_heading, s.best_text, s.distance
				FROM scored s
				JOIN artifacts a  ON a.id  = s.artifact_id
				JOIN areas     ar ON ar.id = a.area_id
				ORDER BY s.distance
				LIMIT $4
			`
			var areasArg any
			if len(in.Areas) > 0 {
				areasArg = in.Areas
			}
			rows, err := deps.DB.Query(ctx, sql, qVec, deps.ProjectSlug, areasArg, in.TopK)
			if err != nil {
				return nil, contextForTaskOutput{}, fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			out := contextForTaskOutput{TaskDescription: in.TaskDescription, Landings: []ContextLanding{}}
			for rows.Next() {
				var l ContextLanding
				var bestHeading, bestText string
				if err := rows.Scan(
					&l.ArtifactID, &l.Slug, &l.Type, &l.Title, &l.AreaSlug,
					&bestHeading, &bestText, &l.Distance,
				); err != nil {
					return nil, contextForTaskOutput{}, fmt.Errorf("scan: %w", err)
				}
				l.URL = "pindoc://" + l.Slug
				if bestHeading != "" {
					l.Rationale = "Best-matching section: " + bestHeading
				} else {
					l.Rationale = trimSnippet(bestText, 160)
				}
				out.Landings = append(out.Landings, l)
			}
			if deps.Embedder.Info().Name == "stub" {
				out.Notice = "stub embedder active — landings are hash-ranked, not semantic."
			}
			return nil, out, rows.Err()
		},
	)
}
