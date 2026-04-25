package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type contextForTaskInput struct {
	ProjectSlug     string   `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	TaskDescription string   `json:"task_description" jsonschema:"free-form natural language description of what the agent is about to do"`
	TopK            int      `json:"top_k,omitempty" jsonschema:"number of artifacts to return; default 3, max 10"`
	Areas           []string `json:"areas,omitempty" jsonschema:"optional area_slug filter"`
	// IncludeTemplates surfaces _template_* artifacts in landings. Default
	// false — templates are meta-docs meant to be read via artifact.read
	// before proposing, not Fast-Landing candidates. The previous default
	// (templates always surfaced) let an empty "연관" section in a template
	// outrank real content on short task descriptions.
	IncludeTemplates bool `json:"include_templates,omitempty" jsonschema:"surface _template_* artifacts in landings; default false matches artifact.search/list and the Reader UI"`
}

type ContextLanding struct {
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	AreaSlug   string `json:"area_slug"`
	Rationale  string `json:"rationale"` // why this is relevant — picked from best chunk heading/text
	// AgentRef for re-feeding into artifact.read; HumanURL for chat share.
	// HumanURLAbs is populated only when server_settings.public_base_url
	// is configured.
	AgentRef    string  `json:"agent_ref"`
	HumanURL    string  `json:"human_url"`
	HumanURLAbs string  `json:"human_url_abs,omitempty"`
	Distance    float64 `json:"distance"`

	// TrustSummary is a three-axis epistemic snapshot of the landing so
	// the agent can decide whether to treat the artifact as authority or
	// as a reference before reading the full body. Mirrors the subset
	// agreed in Task `artifact-meta-jsonb-스키마-추가-6축-epistemic-metadata-도입`
	// (source_type · confidence · next_context_policy). Empty fields when
	// the artifact predates migration 0012.
	TrustSummary LandingTrustSummary `json:"trust_summary"`
}

// LandingTrustSummary is the compact projection of artifact_meta emitted on
// every context.for_task landing. Dedicated type (not ResolvedArtifactMeta)
// so the retrieval surface stays stable as more axes ship — callers depend
// on these three fields for framing, the rest belong on artifact.read.
type LandingTrustSummary struct {
	SourceType        string `json:"source_type,omitempty"`
	Confidence        string `json:"confidence,omitempty"`
	NextContextPolicy string `json:"next_context_policy,omitempty"`
}

// CandidateUpdate is a landing-shaped hint that an existing artifact is
// likely the right target for update_of instead of a fresh create. Emitted
// when the top vector hit is very close (distance <=
// candidateUpdateThreshold). Agents should artifact.read → decide →
// propose(update_of=...) rather than creating a near-duplicate.
type CandidateUpdate struct {
	ArtifactID  string  `json:"artifact_id"`
	Slug        string  `json:"slug"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	AgentRef    string  `json:"agent_ref"`
	HumanURL    string  `json:"human_url"`
	HumanURLAbs string  `json:"human_url_abs,omitempty"`
	Distance    float64 `json:"distance"`
	Reason      string  `json:"reason"`
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

type AreaSuggestion struct {
	AreaSlug string  `json:"area_slug"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
}

type contextForTaskOutput struct {
	TaskDescription string           `json:"task_description"`
	Landings        []ContextLanding `json:"landings"`
	Notice          string           `json:"notice,omitempty"`
	// SuggestedAreas proposes landing areas for a future artifact. It is
	// advisory and omitted only by older servers; low-confidence runs return
	// an empty array so existing callers stay backward-compatible.
	SuggestedAreas []AreaSuggestion `json:"suggested_areas"`
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
	// EmbedderUsed echoes which provider/model served the landings. Added
	// Phase 17 follow-up so agents detect the silent stub-fallback case.
	EmbedderUsed EmbedderInfo `json:"embedder_used"`
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
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.context.for_task",
			Description: "Given a natural-language task description, return the 1–3 most relevant artifacts in this project. Call this at the start of any non-trivial task before grepping code or writing new artifacts. Tuning: smaller TopK than artifact.search because this optimises for first-hop precision, not recall.",
		},
		func(ctx context.Context, p *auth.Principal, in contextForTaskInput) (*sdk.CallToolResult, contextForTaskOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, contextForTaskOutput{}, fmt.Errorf("context.for_task: %w", err)
			}
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

			// artifact_meta filter: skip landings the owner has excluded
			// from default next-session context. opt_in stays visible here
			// because opt_in means "show when asked" — and context.for_task
			// IS the asking surface. Only `excluded` is silently filtered.
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
					  AND ($5::bool OR NOT starts_with(a.slug, '_template_'))
					  AND COALESCE(a.artifact_meta->>'next_context_policy', '') <> 'excluded'
					ORDER BY c.artifact_id, distance
				)
				SELECT
					s.artifact_id::text, a.slug, a.type, a.title, ar.slug,
					s.best_heading, s.best_text, s.distance, a.updated_at,
					a.artifact_meta
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
			rows, err := deps.DB.Query(ctx, sql, qVec, scope.ProjectSlug, areasArg, in.TopK, in.IncludeTemplates)
			if err != nil {
				return nil, contextForTaskOutput{}, fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			out := contextForTaskOutput{
				TaskDescription: in.TaskDescription,
				Landings:        []ContextLanding{},
				SuggestedAreas:  []AreaSuggestion{},
			}
			now := time.Now()
			for rows.Next() {
				var l ContextLanding
				var bestHeading, bestText string
				var updatedAt time.Time
				var metaRaw []byte
				if err := rows.Scan(
					&l.ArtifactID, &l.Slug, &l.Type, &l.Title, &l.AreaSlug,
					&bestHeading, &bestText, &l.Distance, &updatedAt, &metaRaw,
				); err != nil {
					return nil, contextForTaskOutput{}, fmt.Errorf("scan: %w", err)
				}
				l.AgentRef = "pindoc://" + l.Slug
				l.HumanURL = HumanURL(scope.ProjectSlug, scope.ProjectLocale, l.Slug)
				l.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, l.Slug)
				if bestHeading != "" {
					l.Rationale = "Best-matching section: " + bestHeading
				} else {
					l.Rationale = trimSnippet(bestText, 160)
				}
				if len(metaRaw) > 0 {
					var meta ResolvedArtifactMeta
					if err := json.Unmarshal(metaRaw, &meta); err == nil {
						l.TrustSummary = LandingTrustSummary{
							SourceType:        meta.SourceType,
							Confidence:        meta.Confidence,
							NextContextPolicy: meta.NextContextPolicy,
						}
					}
				}
				out.Landings = append(out.Landings, l)

				// Flag this landing as a likely update target when the
				// vector distance says it's very close. Stop before stub
				// embedder to avoid flooding the list with false signals.
				if deps.Embedder.Info().Name != "stub" && l.Distance < candidateUpdateThreshold {
					out.CandidateUpdates = append(out.CandidateUpdates, CandidateUpdate{
						ArtifactID:  l.ArtifactID,
						Slug:        l.Slug,
						Type:        l.Type,
						Title:       l.Title,
						AgentRef:    l.AgentRef,
						HumanURL:    l.HumanURL,
						HumanURLAbs: l.HumanURLAbs,
						Distance:    l.Distance,
						Reason:      fmt.Sprintf("cosine distance %.3f is below update threshold %.2f — consider update_of before creating new", l.Distance, candidateUpdateThreshold),
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
			info := deps.Embedder.Info()
			out.EmbedderUsed = EmbedderInfo{Name: info.Name, ModelID: info.ModelID, Dimension: info.Dimension}
			if info.Name == "stub" {
				out.Notice = "stub embedder active — landings are hash-ranked, not semantic."
			}
			if areaCounts, err := areaArtifactCounts(ctx, deps, scope.ProjectSlug); err == nil {
				out.SuggestedAreas = suggestAreasForTaskDescription(in.TaskDescription, out.Landings, areaCounts)
			}
			if deps.Receipts != nil {
				// Phase E — bind the receipt to the landings' current head
				// revisions. propose-time verifier flags drift instead of
				// trusting a 30-min clock.
				ids := make([]string, 0, len(out.Landings))
				for _, l := range out.Landings {
					ids = append(ids, l.ArtifactID)
				}
				out.SearchReceipt = deps.Receipts.Issue(scope.ProjectSlug, in.TaskDescription,
					headSnapshotsForArtifacts(ctx, deps, ids),
				)
			}
			if err := recordAreaSuggestionEvent(ctx, deps, scope.ProjectSlug, out.SearchReceipt, in.TaskDescription, out.SuggestedAreas); err != nil && deps.Logger != nil {
				deps.Logger.Warn("area suggestion event failed", "err", err)
			}
			return nil, out, rows.Err()
		},
	)
}

type areaSuggestionRule struct {
	Slug     string
	Keywords []string
	Label    string
}

var areaSuggestionRules = []areaSuggestionRule{
	{"users", []string{"user", "persona", "job to be done", "사용자", "페르소나"}, "user research"},
	{"competitors", []string{"competitor", "competitive", "경쟁"}, "competitive context"},
	{"literature", []string{"literature", "paper", "research review", "survey", "문헌", "논문"}, "literature review"},
	{"external-apis", []string{"external api", "third-party", "vendor api", "provider api"}, "external API context"},
	{"standards", []string{"standard", "specification", "protocol", "표준"}, "standards context"},
	{"glossary", []string{"glossary", "terminology", "용어"}, "glossary context"},
	{"flows", []string{"flow", "workflow", "journey", "플로우"}, "user flow"},
	{"information-architecture", []string{"information architecture", "navigation", "hierarchy", "sidebar"}, "information architecture"},
	{"content", []string{"copy", "content", "message", "documentation copy"}, "content structure"},
	{"developer-experience", []string{"developer experience", "dx", "setup", "onboarding"}, "developer experience"},
	{"campaigns", []string{"campaign", "launch page", "marketing"}, "campaign experience"},
	{"ui", []string{"reader ui", "interface", "screen", "component", "layout", "frontend", "화면"}, "user interface"},
	{"architecture", []string{"architecture", "boundary", "layer", "deployment", "시스템 아키텍처"}, "system architecture"},
	{"data", []string{"schema", "migration", "data model", "database", "jsonb", "데이터"}, "data contract"},
	{"mechanisms", []string{"mechanism", "runtime behavior", "preflight", "template", "harness"}, "internal mechanism"},
	{"mcp", []string{"mcp", "tool", "artifact.propose", "context.for_task", "server tool"}, "MCP tool surface"},
	{"embedding", []string{"embedding", "vector", "semantic", "retrieval", "similarity"}, "embedding/retrieval"},
	{"api", []string{"api endpoint", "http api", "rest", "endpoint"}, "API contract"},
	{"integrations", []string{"integration", "adapter", "webhook"}, "integration boundary"},
	{"delivery", []string{"delivery", "handoff", "rollout"}, "delivery process"},
	{"release", []string{"release", "version", "changelog"}, "release process"},
	{"launch", []string{"launch", "readiness", "go-live"}, "launch readiness"},
	{"incidents", []string{"incident", "outage", "postmortem", "장애"}, "incident response"},
	{"editorial-ops", []string{"editorial", "docs ops", "publishing"}, "editorial operations"},
	{"community-ops", []string{"community", "moderation", "support"}, "community operations"},
	{"policies", []string{"policy", "rule", "consent", "license", "auth mode"}, "project policy"},
	{"compliance", []string{"compliance", "regulation", "audit"}, "compliance"},
	{"ownership", []string{"owner", "ownership", "assignee", "identity", "accountability"}, "ownership boundary"},
	{"review", []string{"review", "retrospective", "evaluation", "observation", "검토"}, "review/evaluation"},
	{"taxonomy-policy", []string{"taxonomy", "area", "classification", "sub-area", "cross-cutting"}, "taxonomy governance"},
	{"security", []string{"security", "auth", "permission", "token", "보안"}, "security concern"},
	{"privacy", []string{"privacy", "pii", "personal data"}, "privacy concern"},
	{"accessibility", []string{"accessibility", "a11y"}, "accessibility concern"},
	{"reliability", []string{"reliability", "sla", "resilience"}, "reliability concern"},
	{"observability", []string{"telemetry", "metric", "observability", "logging", "monitoring"}, "observability concern"},
	{"localization", []string{"localization", "i18n", "locale", "translation"}, "localization concern"},
	{"roadmap", []string{"roadmap", "milestone", "phase", "launch criteria"}, "roadmap"},
	{"strategy", []string{"strategy", "vision", "goal", "scope", "hypothesis"}, "strategy"},
}

type areaSuggestionScore struct {
	slug    string
	score   float64
	reasons []string
}

func suggestAreasForTaskDescription(desc string, landings []ContextLanding, areaCounts map[string]int) []AreaSuggestion {
	lower := strings.ToLower(strings.TrimSpace(desc))
	if lower == "" || len(areaCounts) == 0 {
		return []AreaSuggestion{}
	}
	valid := func(slug string) bool {
		_, ok := areaCounts[slug]
		return ok
	}
	scores := map[string]*areaSuggestionScore{}
	add := func(slug string, delta float64, reason string) {
		if !valid(slug) || delta <= 0 {
			return
		}
		s, ok := scores[slug]
		if !ok {
			s = &areaSuggestionScore{slug: slug}
			scores[slug] = s
		}
		s.score += delta
		if reason != "" {
			s.reasons = append(s.reasons, reason)
		}
	}

	for _, rule := range areaSuggestionRules {
		if !valid(rule.Slug) {
			continue
		}
		var matches []string
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				matches = append(matches, kw)
			}
		}
		if len(matches) > 0 {
			score := 0.72 + float64(len(matches)-1)*0.06
			if score > 0.96 {
				score = 0.96
			}
			add(rule.Slug, score, "matched "+rule.Label+": "+strings.Join(matches, ", "))
		}
	}

	for _, l := range landings {
		if l.Distance > 0.55 {
			continue
		}
		add(l.AreaSlug, 0.12+(0.55-l.Distance)*0.18, fmt.Sprintf("nearby artifact %s in %s", l.Slug, l.AreaSlug))
	}

	for slug, s := range scores {
		if areaCounts[slug] > 0 {
			s.score += 0.03
			s.reasons = append(s.reasons, fmt.Sprintf("project already has %d artifact(s) in %s", areaCounts[slug], slug))
		}
		if s.score > 0.99 {
			s.score = 0.99
		}
	}

	ranked := make([]*areaSuggestionScore, 0, len(scores))
	for _, s := range scores {
		if s.score >= 0.50 {
			ranked = append(ranked, s)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].slug < ranked[j].slug
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > 3 {
		ranked = ranked[:3]
	}
	out := make([]AreaSuggestion, 0, len(ranked))
	for _, s := range ranked {
		out = append(out, AreaSuggestion{
			AreaSlug: s.slug,
			Score:    s.score,
			Reason:   strings.Join(s.reasons, "; "),
		})
	}
	return out
}

func areaArtifactCounts(ctx context.Context, deps Deps, projectSlug string) (map[string]int, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT ar.slug, count(a.id)::int
		FROM areas ar
		JOIN projects p ON p.id = ar.project_id
		LEFT JOIN artifacts a ON a.area_id = ar.id AND a.status <> 'archived'
		WHERE p.slug = $1
		GROUP BY ar.slug
	`, projectSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var slug string
		var count int
		if err := rows.Scan(&slug, &count); err != nil {
			return nil, err
		}
		out[slug] = count
	}
	return out, rows.Err()
}

func recordAreaSuggestionEvent(ctx context.Context, deps Deps, projectSlug, correlationID, taskDescription string, suggestions []AreaSuggestion) error {
	if deps.DB == nil {
		return nil
	}
	if correlationID == "" {
		correlationID = fmt.Sprintf("context:%d", time.Now().UnixNano())
	}
	payload, err := json.Marshal(map[string]any{
		"correlation_id":   correlationID,
		"task_description": taskDescription,
		"suggested_areas":  suggestions,
	})
	if err != nil {
		return err
	}
	_, err = deps.DB.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		SELECT p.id, 'agent.area_suggestion_proposed', NULL, $2::jsonb
		FROM projects p
		WHERE p.slug = $1
	`, projectSlug, string(payload))
	return err
}
