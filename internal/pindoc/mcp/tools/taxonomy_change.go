package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// taxonomyTopLevelCap is the upper bound on top-level areas a project
// may hold. Decision area-taxonomy-profiled-skeleton: profiles curate
// 8-11 top-level areas, and controlled extensions stay within ~12 so the
// navigation shelf does not sprawl (Larson/Czerwinski depth-breadth).
const taxonomyTopLevelCap = 12

// taxonomyForbiddenFacetSlugs maps a slug that names a non-concern facet
// (document form, workflow status, actor) to the facet it belongs to.
// Decision area-taxonomy-reform-path-a froze Area as a concern axis;
// area-taxonomy-profiled-skeleton keeps that guard on the proposal gate
// so a profile extension cannot smuggle a facet back onto the shelf.
var taxonomyForbiddenFacetSlugs = map[string]string{
	"decision":    "document type",
	"task":        "document type",
	"analysis":    "document type",
	"debug":       "document type",
	"flow":        "document type",
	"apiendpoint": "document type",
	"screen":      "document type",
	"feature":     "document type",
	"glossary":    "document type",
	"tc":          "document type",
	"todo":        "workflow status",
	"in-progress": "workflow status",
	"wip":         "workflow status",
	"review":      "workflow status",
	"done":        "workflow status",
	"blocked":     "workflow status",
	"draft":       "workflow status",
	"codex":       "agent name",
	"claude":      "agent name",
	"cursor":      "agent name",
}

// taxonomyOneOffSlugRe flags slugs shaped like a one-off initiative
// (phase-3, april-patch, mvp-week, 2026-launch) rather than a stable
// recurring concern that earns a permanent top-level shelf.
var taxonomyOneOffSlugRe = regexp.MustCompile(`(^phase-?\d|^mvp|-patch$|-fix$|-hotfix$|-week$|-day$|-launch$|\d{4})`)

type taxonomyChangeProposeInput struct {
	ProjectSlug   string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	CandidateSlug string `json:"candidate_slug" jsonschema:"proposed top-level area slug, lowercase kebab-case, 2-40 chars"`
	Name          string `json:"name" jsonschema:"display name, 2-60 chars"`
	Description   string `json:"description" jsonschema:"the concern this top-level area holds"`
	Includes      string `json:"includes,omitempty" jsonschema:"what artifacts belong in this area"`
	Excludes      string `json:"excludes,omitempty" jsonschema:"what does NOT belong here and which area takes it instead"`
	Evidence      string `json:"evidence" jsonschema:"why a new top-level is needed: recurring tags, artifact counts, the generic areas it is currently mis-filed under"`
}

type taxonomyChangeProposeOutput struct {
	Status    string   `json:"status"` // proposed | not_ready
	ErrorCode string   `json:"error_code,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	Checklist []string `json:"checklist,omitempty"`

	ProjectSlug    string `json:"project_slug,omitempty"`
	CandidateSlug  string `json:"candidate_slug,omitempty"`
	Message        string `json:"message,omitempty"`
	ToolsetVersion string `json:"toolset_version,omitempty"`
}

type normalizedTaxonomyChangePropose struct {
	CandidateSlug string
	Name          string
	Description   string
	Includes      string
	Excludes      string
	Evidence      string
}

// RegisterTaxonomyChangePropose wires pindoc.taxonomy.change.propose.
// Decision area-taxonomy-profiled-skeleton: top-level areas are
// profile-governed, and a project-specific top-level is allowed only as
// a controlled extension. pindoc.area.create deliberately refuses
// top-level rows; this tool is the evidence-based gate. It NEVER creates
// an area — it validates the candidate (facet check, slug collision,
// count cap) and records a taxonomy.top_level_proposed event for the
// project owner to review. Applying an approved proposal is a separate
// owner action, not something an agent self-approves.
func RegisterTaxonomyChangePropose(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.taxonomy.change.propose",
			Description: strings.TrimSpace(`
Propose a new project-specific top-level area. This tool never creates an area: it validates the candidate (rejects document-form / workflow-status / actor slugs, slug collisions, and proposals past the top-level count cap) and records the proposal for the project owner to review and apply. Sub-areas use pindoc.area.create instead; top-level areas are profile-governed (Decision area-taxonomy-profiled-skeleton).
`),
		},
		func(ctx context.Context, p *auth.Principal, in taxonomyChangeProposeInput) (*sdk.CallToolResult, taxonomyChangeProposeOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("taxonomy.change.propose: %w", err)
			}

			norm, notReady := validateTaxonomyChangePropose(in)
			if notReady != nil {
				return nil, *notReady, nil
			}

			rows, err := deps.DB.Query(ctx, `
				SELECT a.slug
				  FROM areas a
				  JOIN projects p ON p.id = a.project_id
				 WHERE p.slug = $1 AND a.parent_id IS NULL
			`, scope.ProjectSlug)
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("load top-level areas: %w", err)
			}
			topLevel := []string{}
			for rows.Next() {
				var s string
				if err := rows.Scan(&s); err != nil {
					rows.Close()
					return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("scan top-level area: %w", err)
				}
				topLevel = append(topLevel, s)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("top-level area rows: %w", err)
			}

			for _, s := range topLevel {
				if s == norm.CandidateSlug {
					return nil, taxonomyChangeNotReady("CANDIDATE_SLUG_EXISTS",
						fmt.Sprintf("Top-level area %q already exists in this project.", norm.CandidateSlug)), nil
				}
			}
			if len(topLevel)+1 > taxonomyTopLevelCap {
				return nil, taxonomyChangeNotReady("TOP_LEVEL_CAP_EXCEEDED",
					fmt.Sprintf("Project already has %d top-level areas; the cap is %d. Rehome or merge existing areas before proposing another.", len(topLevel), taxonomyTopLevelCap)), nil
			}

			actorID := strings.TrimSpace(p.AgentID)
			if actorID == "" {
				actorID = "unassigned"
			}
			if _, err := deps.DB.Exec(ctx, `
				INSERT INTO events (project_id, kind, payload)
				SELECT p.id, 'taxonomy.top_level_proposed', jsonb_build_object(
					'candidate_slug', $2::text,
					'name',           $3::text,
					'description',    $4::text,
					'includes',       $5::text,
					'excludes',       $6::text,
					'evidence',       $7::text,
					'proposed_by',    $8::text
				)
				FROM projects p WHERE p.slug = $1
			`, scope.ProjectSlug, norm.CandidateSlug, norm.Name, norm.Description, norm.Includes, norm.Excludes, norm.Evidence, actorID); err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record proposal event: %w", err)
			}

			return nil, taxonomyChangeProposeOutput{
				Status:        "proposed",
				ProjectSlug:   scope.ProjectSlug,
				CandidateSlug: norm.CandidateSlug,
				Message: fmt.Sprintf(
					"Recorded a top-level area proposal for %q. NOT applied — no area was created. Surface this proposal to the project owner; an approved top-level area is added by the owner through a taxonomy profile or migration update, not by this tool.",
					norm.CandidateSlug),
			}, nil
		},
	)
}

// validateTaxonomyChangePropose normalizes input and runs the static
// gate: slug shape, required fields, facet rejection, and the one-off
// initiative heuristic. Collision and count-cap checks need the project
// row and run in the handler.
func validateTaxonomyChangePropose(in taxonomyChangeProposeInput) (normalizedTaxonomyChangePropose, *taxonomyChangeProposeOutput) {
	norm := normalizedTaxonomyChangePropose{
		CandidateSlug: strings.ToLower(strings.TrimSpace(in.CandidateSlug)),
		Name:          strings.TrimSpace(in.Name),
		Description:   strings.TrimSpace(in.Description),
		Includes:      strings.TrimSpace(in.Includes),
		Excludes:      strings.TrimSpace(in.Excludes),
		Evidence:      strings.TrimSpace(in.Evidence),
	}
	switch {
	case !areaSlugRe.MatchString(norm.CandidateSlug):
		return norm, ptrTaxonomyChangeNotReady("CANDIDATE_SLUG_INVALID",
			"candidate_slug must be lowercase kebab-case, 2-40 chars, starting with a letter.")
	case len([]rune(norm.Name)) < 2 || len([]rune(norm.Name)) > 60:
		return norm, ptrTaxonomyChangeNotReady("NAME_INVALID", "name must be 2-60 characters.")
	case norm.Description == "":
		return norm, ptrTaxonomyChangeNotReady("DESCRIPTION_REQUIRED",
			"description is required: state the concern this top-level area holds.")
	case norm.Evidence == "":
		return norm, ptrTaxonomyChangeNotReady("EVIDENCE_REQUIRED",
			"evidence is required: cite recurring tags, artifact counts, or mis-filed cases that justify a new top-level area.")
	}
	if facet, bad := taxonomyForbiddenFacetSlugs[norm.CandidateSlug]; bad {
		return norm, ptrTaxonomyChangeNotReady("CANDIDATE_SLUG_IS_FACET",
			fmt.Sprintf("%q names a %s, not a subject concern. Area is a concern axis — use Type or Tag for that facet.", norm.CandidateSlug, facet))
	}
	if taxonomyOneOffSlugRe.MatchString(norm.CandidateSlug) {
		return norm, ptrTaxonomyChangeNotReady("CANDIDATE_SLUG_ONE_OFF",
			fmt.Sprintf("%q looks like a one-off initiative (phase / patch / dated), not a stable recurring concern. Top-level areas must be durable.", norm.CandidateSlug))
	}
	return norm, nil
}

func ptrTaxonomyChangeNotReady(code, msg string) *taxonomyChangeProposeOutput {
	out := taxonomyChangeNotReady(code, msg)
	return &out
}

func taxonomyChangeNotReady(code, msg string) taxonomyChangeProposeOutput {
	return taxonomyChangeProposeOutput{
		Status:    "not_ready",
		ErrorCode: code,
		Failed:    []string{code},
		Checklist: []string{msg},
	}
}
