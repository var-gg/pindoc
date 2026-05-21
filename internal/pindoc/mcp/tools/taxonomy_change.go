package tools

import (
	"context"
	"encoding/json"
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
	CandidateSlug string `json:"candidate_slug,omitempty" jsonschema:"required for kind=top_level.add: proposed top-level area slug, lowercase kebab-case, 2-40 chars"`
	Name          string `json:"name,omitempty" jsonschema:"required for kind=top_level.add: display name, 2-60 chars"`
	Description   string `json:"description,omitempty" jsonschema:"required for kind=top_level.add: the concern this top-level area holds"`
	Includes      string `json:"includes,omitempty" jsonschema:"what artifacts belong in this area"`
	Excludes      string `json:"excludes,omitempty" jsonschema:"what does NOT belong here and which area takes it instead"`
	Evidence      string `json:"evidence,omitempty" jsonschema:"required for kind=top_level.add: why a new top-level is needed: recurring tags, artifact counts, the generic areas it is currently mis-filed under"`

	// Fileable / MaxDepth / IsCrossCutting are the area spec the change-set
	// applies. Decision taxonomy-change-operation: a custom top-level must
	// declare fileable and max_depth explicitly — no implicit default.
	Fileable       bool `json:"fileable,omitempty" jsonschema:"true if artifacts may be filed directly into this top-level area; false for a pure structural shelf"`
	MaxDepth       int  `json:"max_depth,omitempty" jsonschema:"sub-area nesting cap under this top-level: 1 (depth-1 only) or 2; 0 is treated as 1"`
	IsCrossCutting bool `json:"is_cross_cutting,omitempty" jsonschema:"true if this top-level holds reusable concerns spanning other areas"`

	// Kind selects the change-set kind; empty defaults to top_level.add.
	// kind=area.retire_empty uses AreaSlugs instead of the candidate fields.
	Kind      string   `json:"kind,omitempty" jsonschema:"change-set kind: top_level.add (default), area.retire_empty, or profile.adopt"`
	AreaSlugs []string `json:"area_slugs,omitempty" jsonschema:"for kind=area.retire_empty: existing area slugs to archive once empty"`

	// profile.adopt: the target taxonomy profile and an optional agent
	// relocation map (Decision taxonomy-change-operation T13).
	TargetProfileSlug string                `json:"target_profile_slug,omitempty" jsonschema:"for kind=profile.adopt: the taxonomy profile to adopt"`
	RelocationMap     []relocationMoveInput `json:"relocation_map,omitempty" jsonschema:"for kind=profile.adopt: optional artifact relocation map applied during adoption"`
}

type taxonomyChangeProposeOutput struct {
	Status    string   `json:"status"` // proposed | not_ready
	ErrorCode string   `json:"error_code,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	Checklist []string `json:"checklist,omitempty"`

	ProjectSlug    string            `json:"project_slug,omitempty"`
	CandidateSlug  string            `json:"candidate_slug,omitempty"`
	AreaSlugs      []string          `json:"area_slugs,omitempty"`
	ChangeID       string            `json:"change_id,omitempty"`
	PlanHash       string            `json:"plan_hash,omitempty"`
	Diff           *profileAdoptDiff `json:"diff,omitempty"`
	Message        string            `json:"message,omitempty"`
	ToolsetVersion string            `json:"toolset_version,omitempty"`
}

type normalizedTaxonomyChangePropose struct {
	CandidateSlug  string
	Name           string
	Description    string
	Includes       string
	Excludes       string
	Evidence       string
	Fileable       bool
	MaxDepth       int
	IsCrossCutting bool
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

			// Decision taxonomy-change-operation T11: area.retire_empty is
			// a distinct change-set kind with its own input shape, so it
			// dispatches before the top_level.add validation below.
			if strings.TrimSpace(in.Kind) == taxonomyChangeKindAreaRetire {
				return proposeAreaRetireEmpty(ctx, deps, p, scope.ProjectID, scope.ProjectSlug, in)
			}
			if strings.TrimSpace(in.Kind) == taxonomyChangeKindProfileAdopt {
				return proposeProfileAdopt(ctx, deps, p, scope.ProjectID, scope.ProjectSlug, in)
			}

			norm, notReady := validateTaxonomyChangePropose(in)
			if notReady != nil {
				return nil, *notReady, nil
			}

			rows, err := deps.DB.Query(ctx, `
				SELECT a.slug, a.lifecycle
				  FROM areas a
				  JOIN projects p ON p.id = a.project_id
				 WHERE p.slug = $1 AND a.parent_id IS NULL
			`, scope.ProjectSlug)
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("load top-level areas: %w", err)
			}
			type topLevelAreaRow struct{ slug, lifecycle string }
			topLevel := []topLevelAreaRow{}
			for rows.Next() {
				var r topLevelAreaRow
				if err := rows.Scan(&r.slug, &r.lifecycle); err != nil {
					rows.Close()
					return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("scan top-level area: %w", err)
				}
				topLevel = append(topLevel, r)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("top-level area rows: %w", err)
			}

			// A candidate colliding with ANY existing top-level slug —
			// active, retiring, or archived — is rejected; reusing a slug
			// is ambiguous. The cap (Decision taxonomy-change-operation T8)
			// counts only active top-levels, so a profile.adopt that leaves
			// old areas retiring does not exhaust the budget for the new
			// skeleton.
			activeTopLevel := 0
			for _, r := range topLevel {
				if r.slug == norm.CandidateSlug {
					return nil, taxonomyChangeNotReady("CANDIDATE_SLUG_EXISTS",
						fmt.Sprintf("Top-level area %q already exists in this project.", norm.CandidateSlug)), nil
				}
				if r.lifecycle == "active" {
					activeTopLevel++
				}
			}
			if activeTopLevel+1 > taxonomyTopLevelCap {
				return nil, taxonomyChangeNotReady("TOP_LEVEL_CAP_EXCEEDED",
					fmt.Sprintf("Project already has %d active top-level areas; the cap is %d. Retire or rehome existing areas before proposing another.", activeTopLevel, taxonomyTopLevelCap)), nil
			}

			actorID := strings.TrimSpace(p.AgentID)
			if actorID == "" {
				actorID = "unassigned"
			}

			// Decision taxonomy-change-operation T10: a proposal is now a
			// persisted change-set (taxonomy_changes row), not just an
			// event. propose/approve/apply all act on that row; the event
			// is an audit copy carrying change_id and plan_hash.
			plan := topLevelAddPlan{
				Kind:           taxonomyChangeKindTopLevelAdd,
				ProjectID:      scope.ProjectID,
				Slug:           norm.CandidateSlug,
				Name:           norm.Name,
				Description:    norm.Description,
				IsCrossCutting: norm.IsCrossCutting,
				Fileable:       norm.Fileable,
				MaxDepth:       norm.MaxDepth,
			}
			planJSON, err := json.Marshal(plan)
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("marshal top_level.add plan: %w", err)
			}
			planHash, err := computeTaxonomyPlanHash(plan)
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("hash top_level.add plan: %w", err)
			}
			diffJSON, err := json.Marshal(map[string]any{"to_create": []string{norm.CandidateSlug}})
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("marshal top_level.add diff: %w", err)
			}

			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("begin propose tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			changeID, err := insertTaxonomyChange(ctx, tx, taxonomyChange{
				ProjectID: scope.ProjectID,
				Kind:      taxonomyChangeKindTopLevelAdd,
				PlanJSON:  planJSON,
				DiffJSON:  diffJSON,
				PlanHash:  planHash,
				CreatedBy: actorID,
			})
			if err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record taxonomy change: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, payload)
				VALUES ($1::uuid, 'taxonomy.top_level_proposed', jsonb_build_object(
					'change_id',      $2::text,
					'plan_hash',      $3::text,
					'kind',           $4::text,
					'candidate_slug', $5::text,
					'name',           $6::text,
					'description',    $7::text,
					'includes',       $8::text,
					'excludes',       $9::text,
					'evidence',       $10::text,
					'proposed_by',    $11::text
				))
			`, scope.ProjectID, changeID, planHash, taxonomyChangeKindTopLevelAdd,
				norm.CandidateSlug, norm.Name, norm.Description, norm.Includes, norm.Excludes, norm.Evidence, actorID); err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record proposal event: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("commit propose: %w", err)
			}

			return nil, taxonomyChangeProposeOutput{
				Status:        "proposed",
				ProjectSlug:   scope.ProjectSlug,
				CandidateSlug: norm.CandidateSlug,
				ChangeID:      changeID,
				PlanHash:      planHash,
				Message: fmt.Sprintf(
					"Recorded change-set %s (top_level.add %q). NOT applied — surface it to the project owner: pindoc.taxonomy.change.approve, then pindoc.taxonomy.change.apply.",
					changeID, norm.CandidateSlug),
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
		CandidateSlug:  strings.ToLower(strings.TrimSpace(in.CandidateSlug)),
		Name:           strings.TrimSpace(in.Name),
		Description:    strings.TrimSpace(in.Description),
		Includes:       strings.TrimSpace(in.Includes),
		Excludes:       strings.TrimSpace(in.Excludes),
		Evidence:       strings.TrimSpace(in.Evidence),
		Fileable:       in.Fileable,
		MaxDepth:       in.MaxDepth,
		IsCrossCutting: in.IsCrossCutting,
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
