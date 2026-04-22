package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type contextForTaskInput struct {
	TaskDescription string   `json:"task_description" jsonschema:"free-form natural language description of what the agent is about to do"`
	TopK            int      `json:"top_k,omitempty" jsonschema:"number of artifacts to return; default 3, max 10"`
	Areas           []string `json:"areas,omitempty" jsonschema:"optional area_slug filter"`
}

type ContextLanding struct {
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	AreaSlug   string `json:"area_slug"`
	Rationale  string `json:"rationale"` // why this is relevant — picked from best chunk heading/text
	// AgentRef for re-feeding into artifact.read; HumanURL for chat share.
	AgentRef string  `json:"agent_ref"`
	HumanURL string  `json:"human_url"`
	Distance float64 `json:"distance"`
}

// CandidateUpdate is a landing-shaped hint that an existing artifact is
// likely the right target for update_of instead of a fresh create. Emitted
// when the top vector hit is very close (distance <=
// candidateUpdateThreshold). Agents should artifact.read → decide →
// propose(update_of=...) rather than creating a near-duplicate.
type CandidateUpdate struct {
	ArtifactID string  `json:"artifact_id"`
	Slug       string  `json:"slug"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	AgentRef   string  `json:"agent_ref"`
	HumanURL   string  `json:"human_url"`
	Distance   float64 `json:"distance"`
	Reason     string  `json:"reason"`
}

// StaleSignal flags a landing as potentially out-of-date. Phase 11c
// implements the simplest heuristic: `updated_at` older than
// staleAgeThreshold. Later phases add pin-diff-vs-HEAD and explicit
// supersede chain checks.
type StaleSignal struct {
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Reason     string `json:"reason"`
	DaysOld    int    `json:"days_old"`
}

type contextForTaskOutput struct {
	TaskDescription string           `json:"task_description"`
	Landings        []ContextLanding `json:"landings"`
	Notice          string           `json:"notice,omitempty"`
	// SearchReceipt mirrors artifact.search — same opaque token, same TTL,
	// same downstream effect on artifact.propose. Agents that Fast-Land
	// with context.for_task satisfy the search-before-propose gate without
	// also calling artifact.search.
	SearchReceipt string `json:"search_receipt,omitempty"`
	// CandidateUpdates surfaces landings that are close enough to the task
	// description that the agent should probably update them instead of
	// creating a new artifact. Empty when nothing is that close.
	CandidateUpdates []CandidateUpdate `json:"candidate_updates,omitempty"`
	// Stale flags landings that may be out-of-date. Phase 11c uses a
	// simple updated_at age heuristic; later phases add pin-diff checks.
	Stale []StaleSignal `json:"stale,omitempty"`
}

// candidateUpdateThreshold: landings under this cosine distance prompt an
// "update instead of create?" hint. Looser than semanticConflictThreshold
// (0.18) because this is advisory, not a block.
const candidateUpdateThreshold = 0.22

// staleAgeThreshold: 60 days without an update is our simple "may be
// stale" proxy. Arbitrary but operational; tune with real dogfood data.
const staleAgeThreshold = 60 * 24 * time.Hour

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
					s.best_heading, s.best_text, s.distance, a.updated_at
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
			now := time.Now()
			for rows.Next() {
				var l ContextLanding
				var bestHeading, bestText string
				var updatedAt time.Time
				if err := rows.Scan(
					&l.ArtifactID, &l.Slug, &l.Type, &l.Title, &l.AreaSlug,
					&bestHeading, &bestText, &l.Distance, &updatedAt,
				); err != nil {
					return nil, contextForTaskOutput{}, fmt.Errorf("scan: %w", err)
				}
				l.AgentRef = "pindoc://" + l.Slug
				l.HumanURL = HumanURL(deps.ProjectSlug, l.Slug)
				if bestHeading != "" {
					l.Rationale = "Best-matching section: " + bestHeading
				} else {
					l.Rationale = trimSnippet(bestText, 160)
				}
				out.Landings = append(out.Landings, l)

				// Flag this landing as a likely update target when the
				// vector distance says it's very close. Stop before stub
				// embedder to avoid flooding the list with false signals.
				if deps.Embedder.Info().Name != "stub" && l.Distance < candidateUpdateThreshold {
					out.CandidateUpdates = append(out.CandidateUpdates, CandidateUpdate{
						ArtifactID: l.ArtifactID,
						Slug:       l.Slug,
						Type:       l.Type,
						Title:      l.Title,
						AgentRef:   l.AgentRef,
						HumanURL:   l.HumanURL,
						Distance:   l.Distance,
						Reason:     fmt.Sprintf("cosine distance %.3f is below update threshold %.2f — consider update_of before creating new", l.Distance, candidateUpdateThreshold),
					})
				}

				// Flag stale landings. Phase 11c: simple age heuristic.
				// Phase V1.x replaces this with pin-diff-vs-HEAD.
				if age := now.Sub(updatedAt); age > staleAgeThreshold {
					out.Stale = append(out.Stale, StaleSignal{
						ArtifactID: l.ArtifactID,
						Slug:       l.Slug,
						DaysOld:    int(age.Hours() / 24),
						Reason:     fmt.Sprintf("not updated in %d days — verify pins/facts before reuse", int(age.Hours()/24)),
					})
				}
			}
			if deps.Embedder.Info().Name == "stub" {
				out.Notice = "stub embedder active — landings are hash-ranked, not semantic."
			}
			if deps.Receipts != nil {
				out.SearchReceipt = deps.Receipts.Issue(deps.ProjectSlug, in.TaskDescription)
			}
			return nil, out, rows.Err()
		},
	)
}
