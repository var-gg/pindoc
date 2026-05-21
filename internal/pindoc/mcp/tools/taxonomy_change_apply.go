package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// Decision taxonomy-change-operation T10: a taxonomy change-set moves
// proposed -> approved -> applied. propose (taxonomy_change.go) records
// the taxonomy_changes row; approve is owner-only; apply executes an
// owner-approved change-set. This file holds approve + apply; propose
// stays in taxonomy_change.go.

// topLevelAddPlan is the plan_json shape for a kind=top_level.add
// change-set. It is the exact area spec apply hands to
// projects.CreateTopLevelArea — immutable between propose and apply, so
// plan_hash is a spec-integrity digest. Drift that matters for this kind
// (slug now taken, active cap now full) is re-checked directly at apply.
type topLevelAddPlan struct {
	Kind           string `json:"kind"`
	ProjectID      string `json:"project_id"`
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	IsCrossCutting bool   `json:"is_cross_cutting"`
	Fileable       bool   `json:"fileable"`
	MaxDepth       int    `json:"max_depth"`
}

// taxonomyChangeActor resolves the audit actor label for an approve/apply
// call: the principal's user id when present, else the agent id.
func taxonomyChangeActor(p *auth.Principal) string {
	if p == nil {
		return "unassigned"
	}
	if uid := principalUserID(p); uid != "" {
		return uid
	}
	if agent := strings.TrimSpace(p.AgentID); agent != "" {
		return agent
	}
	return "unassigned"
}

// --- approve ---------------------------------------------------------

type taxonomyChangeApproveInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	ChangeID    string `json:"change_id" jsonschema:"taxonomy_changes id returned by pindoc.taxonomy.change.propose"`
	Reason      string `json:"reason,omitempty" jsonschema:"optional owner note stored on the approval event"`
}

type taxonomyChangeApproveOutput struct {
	Status         string   `json:"status"` // approved | not_ready
	ErrorCode      string   `json:"error_code,omitempty"`
	Failed         []string `json:"failed,omitempty"`
	ChangeID       string   `json:"change_id,omitempty"`
	ChangeStatus   string   `json:"change_status,omitempty"`
	PlanHash       string   `json:"plan_hash,omitempty"`
	Message        string   `json:"message,omitempty"`
	ToolsetVersion string   `json:"toolset_version,omitempty"`
}

// RegisterTaxonomyChangeApprove wires pindoc.taxonomy.change.approve.
// Owner-only: an agent proposes and applies, but only a project owner
// approves — that is the human gate in pindoc's "agent works, owner
// approves" lifecycle.
func RegisterTaxonomyChangeApprove(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.taxonomy.change.approve",
			Description: strings.TrimSpace(`
Approve a proposed taxonomy change-set so it can be applied. Owner-only: an agent can propose and apply, but only a project owner approves. The change_id comes from the pindoc.taxonomy.change.propose response. Applying an approved change-set is a separate pindoc.taxonomy.change.apply call.
`),
		},
		func(ctx context.Context, p *auth.Principal, in taxonomyChangeApproveInput) (*sdk.CallToolResult, taxonomyChangeApproveOutput, error) {
			changeID := strings.TrimSpace(in.ChangeID)
			if changeID == "" {
				return nil, taxonomyChangeApproveNotReady("CHANGE_ID_REQUIRED", ""), nil
			}
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taxonomyChangeApproveOutput{}, fmt.Errorf("taxonomy.change.approve: %w", err)
			}
			if !scope.Can("write.project") {
				return nil, taxonomyChangeApproveNotReady("PROJECT_OWNER_REQUIRED", changeID), nil
			}
			change, err := getTaxonomyChange(ctx, deps.DB, changeID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, taxonomyChangeApproveNotReady("CHANGE_NOT_FOUND", changeID), nil
			}
			if err != nil {
				return nil, taxonomyChangeApproveOutput{}, fmt.Errorf("load taxonomy change: %w", err)
			}
			if change.ProjectID != scope.ProjectID {
				return nil, taxonomyChangeApproveNotReady("CHANGE_NOT_FOUND", changeID), nil
			}
			if change.Status != taxonomyChangeStatusProposed {
				out := taxonomyChangeApproveNotReady("CHANGE_NOT_PROPOSED", changeID)
				out.ChangeStatus = change.Status
				return nil, out, nil
			}

			approver := taxonomyChangeActor(p)
			if err := markTaxonomyChangeApproved(ctx, deps.DB, changeID, approver); err != nil {
				return nil, taxonomyChangeApproveOutput{}, fmt.Errorf("approve taxonomy change: %w", err)
			}
			if _, err := deps.DB.Exec(ctx, `
				INSERT INTO events (project_id, kind, payload)
				VALUES ($1::uuid, 'taxonomy.change_approved', jsonb_build_object(
					'change_id',   $2::text,
					'plan_hash',   $3::text,
					'kind',        $4::text,
					'approved_by', $5::text,
					'reason',      $6::text
				))
			`, change.ProjectID, changeID, change.PlanHash, change.Kind, approver, strings.TrimSpace(in.Reason)); err != nil {
				return nil, taxonomyChangeApproveOutput{}, fmt.Errorf("record approval event: %w", err)
			}
			return nil, taxonomyChangeApproveOutput{
				Status:       "approved",
				ChangeID:     changeID,
				ChangeStatus: taxonomyChangeStatusApproved,
				PlanHash:     change.PlanHash,
				Message:      fmt.Sprintf("Approved change-set %s. Call pindoc.taxonomy.change.apply to execute it.", changeID),
			}, nil
		},
	)
}

func taxonomyChangeApproveNotReady(code, changeID string) taxonomyChangeApproveOutput {
	return taxonomyChangeApproveOutput{
		Status: "not_ready", ErrorCode: code, Failed: []string{code}, ChangeID: changeID,
	}
}

// --- apply -----------------------------------------------------------

type taxonomyChangeApplyInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	ChangeID    string `json:"change_id" jsonschema:"approved taxonomy_changes id to apply"`
}

type taxonomyChangeApplyOutput struct {
	Status         string   `json:"status"` // applied | not_ready
	ErrorCode      string   `json:"error_code,omitempty"`
	Failed         []string `json:"failed,omitempty"`
	ChangeID       string   `json:"change_id,omitempty"`
	ChangeStatus   string   `json:"change_status,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	AreaSlug       string   `json:"area_slug,omitempty"`
	AreaID         string   `json:"area_id,omitempty"`
	ArchivedCount  int      `json:"archived_count,omitempty"`
	BlockedCount   int      `json:"blocked_count,omitempty"`
	Message        string   `json:"message,omitempty"`
	ToolsetVersion string   `json:"toolset_version,omitempty"`
}

// RegisterTaxonomyChangeApply wires pindoc.taxonomy.change.apply. An agent
// may call it, but only an owner-approved change-set executes. apply
// re-checks the plan against current state and marks the change-set stale
// if the world drifted since approval.
func RegisterTaxonomyChangeApply(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.taxonomy.change.apply",
			Description: strings.TrimSpace(`
Apply an owner-approved taxonomy change-set. An agent may call this — the approval (owner-only) is the gate. apply re-checks the plan against current state and refuses, marking the change-set stale, if the world drifted since approval. For kind=top_level.add it creates the proposed top-level area in one transaction.
`),
		},
		func(ctx context.Context, p *auth.Principal, in taxonomyChangeApplyInput) (*sdk.CallToolResult, taxonomyChangeApplyOutput, error) {
			changeID := strings.TrimSpace(in.ChangeID)
			if changeID == "" {
				return nil, taxonomyChangeApplyNotReady("CHANGE_ID_REQUIRED", ""), nil
			}
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("taxonomy.change.apply: %w", err)
			}
			change, err := getTaxonomyChange(ctx, deps.DB, changeID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, taxonomyChangeApplyNotReady("CHANGE_NOT_FOUND", changeID), nil
			}
			if err != nil {
				return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("load taxonomy change: %w", err)
			}
			if change.ProjectID != scope.ProjectID {
				return nil, taxonomyChangeApplyNotReady("CHANGE_NOT_FOUND", changeID), nil
			}
			if change.Status != taxonomyChangeStatusApproved {
				out := taxonomyChangeApplyNotReady("CHANGE_NOT_APPROVED", changeID)
				out.ChangeStatus = change.Status
				return nil, out, nil
			}

			switch change.Kind {
			case taxonomyChangeKindTopLevelAdd:
				return applyTopLevelAdd(ctx, deps, p, scope.ProjectID, change)
			case taxonomyChangeKindAreaRetire:
				return applyAreaRetireEmpty(ctx, deps, p, scope.ProjectID, change)
			case taxonomyChangeKindProfileAdopt:
				return applyProfileAdopt(ctx, deps, p, scope.ProjectID, change)
			default:
				out := taxonomyChangeApplyNotReady("KIND_NOT_SUPPORTED", changeID)
				out.Kind = change.Kind
				return nil, out, nil
			}
		},
	)
}

// applyTopLevelAdd executes a kind=top_level.add change-set: it re-checks
// for drift, then creates the top-level area, marks the change applied,
// and records audit events — all in one transaction.
func applyTopLevelAdd(ctx context.Context, deps Deps, p *auth.Principal, projectID string, change taxonomyChange) (*sdk.CallToolResult, taxonomyChangeApplyOutput, error) {
	var plan topLevelAddPlan
	if err := json.Unmarshal(change.PlanJSON, &plan); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("parse top_level.add plan: %w", err)
	}

	// Drift check: the candidate slug may have been taken since approval
	// (another change-set, a profile.adopt). A taken slug means the
	// change-set can no longer apply — mark it stale.
	var slugTaken bool
	if err := deps.DB.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM areas WHERE project_id = $1::uuid AND slug = $2)
	`, projectID, plan.Slug).Scan(&slugTaken); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("recheck slug: %w", err)
	}
	if slugTaken {
		return nil, taxonomyChangeApplyStale(deps, ctx, change,
			fmt.Sprintf("slug %q is no longer free", plan.Slug)), nil
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("begin apply tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	areaID, err := projects.CreateTopLevelArea(ctx, tx, projectID, projects.TopLevelAreaSpec{
		Slug:           plan.Slug,
		Name:           plan.Name,
		Description:    plan.Description,
		IsCrossCutting: plan.IsCrossCutting,
		Fileable:       plan.Fileable,
		MaxDepth:       plan.MaxDepth,
	}, "", change.ID)
	if err != nil {
		if errors.Is(err, projects.ErrTopLevelAreaCapExceeded) || errors.Is(err, projects.ErrTopLevelAreaSlugTaken) {
			return nil, taxonomyChangeApplyStale(deps, ctx, change, err.Error()), nil
		}
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("create top-level area: %w", err)
	}

	actor := taxonomyChangeActor(p)
	if err := markTaxonomyChangeApplied(ctx, tx, change.ID, actor); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("mark applied: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1::uuid, 'taxonomy.area_created', $2::uuid, jsonb_build_object(
			'change_id', $3::text, 'plan_hash', $4::text, 'kind', $5::text, 'slug', $6::text
		))
	`, projectID, areaID, change.ID, change.PlanHash, change.Kind, plan.Slug); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("record area_created event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, payload)
		VALUES ($1::uuid, 'taxonomy.change_applied', jsonb_build_object(
			'change_id', $2::text, 'plan_hash', $3::text, 'kind', $4::text, 'applied_by', $5::text
		))
	`, projectID, change.ID, change.PlanHash, change.Kind, actor); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("record change_applied event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("commit apply: %w", err)
	}
	return nil, taxonomyChangeApplyOutput{
		Status:       "applied",
		ChangeID:     change.ID,
		ChangeStatus: taxonomyChangeStatusApplied,
		Kind:         change.Kind,
		AreaSlug:     plan.Slug,
		AreaID:       areaID,
		Message:      fmt.Sprintf("Applied change-set %s — created top-level area %q.", change.ID, plan.Slug),
	}, nil
}

// taxonomyChangeApplyStale marks a drifted change-set stale and builds the
// not_ready output. The stale UPDATE runs outside any apply transaction so
// it is durable even though the apply itself does not proceed.
func taxonomyChangeApplyStale(deps Deps, ctx context.Context, change taxonomyChange, detail string) taxonomyChangeApplyOutput {
	_ = setTaxonomyChangeTerminal(ctx, deps.DB, change.ID, taxonomyChangeStatusStale)
	out := taxonomyChangeApplyNotReady("CHANGE_STALE", change.ID)
	out.ChangeStatus = taxonomyChangeStatusStale
	out.Kind = change.Kind
	out.Message = fmt.Sprintf("Change-set %s drifted since approval (%s) — marked stale. Re-propose.", change.ID, detail)
	return out
}

func taxonomyChangeApplyNotReady(code, changeID string) taxonomyChangeApplyOutput {
	return taxonomyChangeApplyOutput{
		Status: "not_ready", ErrorCode: code, Failed: []string{code}, ChangeID: changeID,
	}
}
