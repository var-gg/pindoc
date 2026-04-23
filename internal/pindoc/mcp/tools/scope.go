package tools

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type scopeInFlightInput struct {
	AreaSlug string `json:"area_slug,omitempty" jsonschema:"optional — restrict to one area; default is project-wide"`
	// StateFilter: which acceptance states to include. "open" = [ ] +
	// [~] (the default — "work still owed"); "unchecked" = [ ] only;
	// "partial" = [~] only; "all" = [ ] + [~] + [-] (includes deferred
	// items with edge context).
	StateFilter string `json:"state_filter,omitempty" jsonschema:"open (default) | unchecked | partial | all"`
	// Limit caps the number of items returned. Defaults to 50 so a
	// project with thousands of acceptance items doesn't flood the agent.
	Limit int `json:"limit,omitempty" jsonschema:"default 50, max 500"`
}

// InFlightItem is one unresolved acceptance checkbox projected through
// the artifact that owns it. Deferred items carry the edge target slug
// so the agent can follow the trail.
type InFlightItem struct {
	ArtifactID    string `json:"artifact_id"`
	ArtifactSlug  string `json:"artifact_slug"`
	ArtifactType  string `json:"artifact_type"`
	ArtifactTitle string `json:"artifact_title"`
	AreaSlug      string `json:"area_slug"`
	CheckboxIndex int    `json:"checkbox_index"`
	State         string `json:"state" jsonschema:"one of '[ ]' | '[~]' | '[-]'"`
	LineText      string `json:"line_text" jsonschema:"the full bullet line for context (trimmed of leading whitespace)"`
	// ForwardedTo is populated only for state=[-]: the slug of the
	// artifact that absorbed this acceptance item (artifact_scope_edges).
	// Empty when no edge exists (e.g. deferred via body edit rather than
	// shape=scope_defer).
	ForwardedToSlug string `json:"forwarded_to_slug,omitempty"`
	ForwardedReason string `json:"forwarded_reason,omitempty"`

	AgentRef    string `json:"agent_ref"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

type scopeInFlightOutput struct {
	Items []InFlightItem `json:"items"`
	// Totals surfaces a project-wide count regardless of state_filter so
	// agents can see "unchecked=17 partial=3 deferred=12" at a glance.
	Totals map[string]int `json:"totals"`
	// Truncated is true if the query had more items than Limit. Agents
	// retry with a tighter area_slug / state_filter to drill in.
	Truncated bool `json:"truncated,omitempty"`
	Notice    string `json:"notice,omitempty"`
}

// RegisterScopeInFlight wires pindoc.scope.in_flight — the Phase F graph
// view of unresolved acceptance items. Surfaces a flat list across every
// Task / Debug / etc. artifact in the project (optionally narrowed by
// area_slug) so agents don't have to grep bodies to answer "what's still
// owed in this project".
func RegisterScopeInFlight(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.scope.in_flight",
			Description: strings.TrimSpace(`
List unresolved acceptance checkboxes ([ ] todo, [~] partial) across the
project, with optional area filter and include-deferred mode. Deferred
items ([-]) show the forwarded-to artifact when a scope_defer edge
exists. Use before claiming_done an epic, or to answer "what's still
owed in this project" without grepping bodies.
`),
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in scopeInFlightInput) (*sdk.CallToolResult, scopeInFlightOutput, error) {
			limit := in.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 500 {
				limit = 500
			}
			filter := strings.TrimSpace(strings.ToLower(in.StateFilter))
			if filter == "" {
				filter = "open"
			}
			switch filter {
			case "open", "unchecked", "partial", "all":
			default:
				return nil, scopeInFlightOutput{}, fmt.Errorf("state_filter must be one of: open | unchecked | partial | all")
			}

			// Pull candidate artifacts (non-archived, non-template-by-
			// default). body_markdown is needed in-memory to walk
			// checkboxes — cheap for V1-sized corpora.
			sql := `
				SELECT a.id::text, a.slug, a.type, a.title, ar.slug, a.body_markdown
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				JOIN areas    ar ON ar.id = a.area_id
				WHERE p.slug = $1
				  AND a.status <> 'archived'
				  AND NOT starts_with(a.slug, '_template_')
				  AND ($2::text IS NULL OR ar.slug = $2)
			`
			var areaArg any
			if s := strings.TrimSpace(in.AreaSlug); s != "" {
				areaArg = s
			}
			rows, err := deps.DB.Query(ctx, sql, deps.ProjectSlug, areaArg)
			if err != nil {
				return nil, scopeInFlightOutput{}, fmt.Errorf("in_flight query: %w", err)
			}
			defer rows.Close()

			// Totals always count every state so the denominator is
			// meaningful even when Items is filtered/truncated.
			totals := map[string]int{"unchecked": 0, "partial": 0, "deferred": 0}
			var items []InFlightItem
			truncated := false

			// deferredByArtifact holds artifact_ids we saw with [-] items,
			// to batch a single edge lookup below.
			deferredByArtifact := map[string]struct{}{}

			for rows.Next() {
				var aid, slug, atype, title, area, body string
				if err := rows.Scan(&aid, &slug, &atype, &title, &area, &body); err != nil {
					return nil, scopeInFlightOutput{}, fmt.Errorf("in_flight scan: %w", err)
				}
				lines := strings.Split(body, "\n")
				for idx, cb := range iterateCheckboxes(body) {
					var state, bucket string
					switch cb.marker {
					case ' ':
						state = "[ ]"
						bucket = "unchecked"
					case 'x', 'X':
						// done — never in-flight, skip
						continue
					case '~':
						state = "[~]"
						bucket = "partial"
					case '-':
						state = "[-]"
						bucket = "deferred"
					default:
						continue
					}
					totals[bucket]++
					// Filter down to requested states for Items.
					include := false
					switch filter {
					case "open":
						include = bucket == "unchecked" || bucket == "partial"
					case "unchecked":
						include = bucket == "unchecked"
					case "partial":
						include = bucket == "partial"
					case "all":
						include = true
					}
					if !include {
						continue
					}
					if len(items) >= limit {
						truncated = true
						continue
					}
					lineText := ""
					if cb.lineIndex < len(lines) {
						lineText = strings.TrimSpace(lines[cb.lineIndex])
					}
					item := InFlightItem{
						ArtifactID:    aid,
						ArtifactSlug:  slug,
						ArtifactType:  atype,
						ArtifactTitle: title,
						AreaSlug:      area,
						CheckboxIndex: idx,
						State:         state,
						LineText:      lineText,
						AgentRef:      "pindoc://" + slug,
						HumanURL:      HumanURL(deps.ProjectSlug, deps.ProjectLocale, slug),
						HumanURLAbs:   AbsHumanURL(deps.Settings, deps.ProjectSlug, deps.ProjectLocale, slug),
					}
					items = append(items, item)
					if bucket == "deferred" {
						deferredByArtifact[aid] = struct{}{}
					}
				}
			}
			if err := rows.Err(); err != nil {
				return nil, scopeInFlightOutput{}, fmt.Errorf("in_flight rows: %w", err)
			}

			// Batch the edge lookup for any deferred items we kept.
			if len(deferredByArtifact) > 0 {
				ids := make([]string, 0, len(deferredByArtifact))
				for id := range deferredByArtifact {
					ids = append(ids, id)
				}
				edgeRows, err := deps.DB.Query(ctx, `
					SELECT e.from_artifact_id::text, e.from_item_ref,
					       a.slug, e.reason
					FROM artifact_scope_edges e
					JOIN artifacts a ON a.id = e.to_artifact_id
					WHERE e.from_artifact_id = ANY($1::uuid[])
				`, ids)
				if err != nil {
					deps.Logger.Warn("in_flight edge lookup failed — deferred rows missing forward slugs", "err", err)
				} else {
					type edgeKey struct{ aid, ref string }
					edges := map[edgeKey]struct {
						slug, reason string
					}{}
					for edgeRows.Next() {
						var aid, ref, toSlug, reason string
						if err := edgeRows.Scan(&aid, &ref, &toSlug, &reason); err != nil {
							continue
						}
						edges[edgeKey{aid, ref}] = struct {
							slug, reason string
						}{toSlug, reason}
					}
					edgeRows.Close()
					for i := range items {
						if items[i].State != "[-]" {
							continue
						}
						k := edgeKey{items[i].ArtifactID, fmt.Sprintf("acceptance[%d]", items[i].CheckboxIndex)}
						if e, ok := edges[k]; ok {
							items[i].ForwardedToSlug = e.slug
							items[i].ForwardedReason = e.reason
						}
					}
				}
			}

			return nil, scopeInFlightOutput{
				Items:     items,
				Totals:    totals,
				Truncated: truncated,
			}, nil
		},
	)
}
