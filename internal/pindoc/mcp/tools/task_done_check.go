package tools

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type taskDoneCheckInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	Assignee    string `json:"assignee,omitempty" jsonschema:"optional exact task_meta.assignee; defaults to the calling agent id, e.g. agent:codex"`
}

type taskDoneCheckTaskRef struct {
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	AreaSlug   string `json:"area_slug"`
	Priority   string `json:"priority,omitempty"`
	Status     string `json:"status"`

	AcceptanceCheckboxesTotal  int                  `json:"acceptance_checkboxes_total"`
	UnresolvedCheckboxes       int                  `json:"unresolved_checkboxes"`
	PartialCheckboxes          int                  `json:"partial_checkboxes"`
	UnresolvedAcceptanceLabels []AcceptanceLabelRef `json:"unresolved_acceptance_labels,omitempty"`

	AgentRef    string `json:"agent_ref"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

type taskDoneCheckOutput struct {
	ProjectSlug string `json:"project_slug"`
	Assignee    string `json:"assignee"`
	IsDone      bool   `json:"is_done"`
	Summary     string `json:"summary"`

	OpenTasks                     []taskDoneCheckTaskRef `json:"open_tasks"`
	UnresolvedAcceptanceTasks     []taskDoneCheckTaskRef `json:"unresolved_acceptance_tasks"`
	OpenTaskCount                 int                    `json:"open_task_count"`
	UnresolvedAcceptanceTaskCount int                    `json:"unresolved_acceptance_task_count"`

	ToolsetVersion string `json:"toolset_version,omitempty"`
}

type taskDoneCheckRecord struct {
	ArtifactID string
	Slug       string
	Title      string
	AreaSlug   string
	Priority   string
	RawStatus  string
	Body       string
}

func RegisterTaskDoneCheck(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.task.done_check",
			Description: "Single-call finishing check for an assignee. Call before final handoff or after claim_done: returns is_done=false when assigned Tasks are still open/missing_status or when claimed_done Tasks still have unresolved [ ]/[~] acceptance labels. Read-only operational metadata lane; no search_receipt required.",
		},
		func(ctx context.Context, p *auth.Principal, in taskDoneCheckInput) (*sdk.CallToolResult, taskDoneCheckOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskDoneCheckOutput{}, fmt.Errorf("task.done_check: %w", err)
			}
			assignee := strings.TrimSpace(in.Assignee)
			if assignee == "" && p != nil {
				assignee = strings.TrimSpace(p.AgentID)
			}
			if assignee == "" {
				return nil, taskDoneCheckOutput{}, fmt.Errorf("assignee is required when caller has no agent id")
			}

			rows, err := deps.DB.Query(ctx, `
				SELECT a.id::text, a.slug, a.title, ar.slug,
				       COALESCE(a.task_meta->>'priority', ''),
				       COALESCE(a.task_meta->>'status', ''),
				       a.body_markdown
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				JOIN areas    ar ON ar.id = a.area_id
				WHERE p.slug = $1
				  AND a.type = 'Task'
				  AND a.status <> 'archived'
				  AND a.status <> 'superseded'
				  AND NOT starts_with(a.slug, '_template_')
				  AND COALESCE(a.task_meta->>'assignee', '') = $2
				ORDER BY a.updated_at DESC
			`, scope.ProjectSlug, assignee)
			if err != nil {
				return nil, taskDoneCheckOutput{}, fmt.Errorf("task.done_check query: %w", err)
			}
			defer rows.Close()

			var records []taskDoneCheckRecord
			for rows.Next() {
				var rec taskDoneCheckRecord
				if err := rows.Scan(&rec.ArtifactID, &rec.Slug, &rec.Title, &rec.AreaSlug, &rec.Priority, &rec.RawStatus, &rec.Body); err != nil {
					return nil, taskDoneCheckOutput{}, fmt.Errorf("task.done_check scan: %w", err)
				}
				records = append(records, rec)
			}
			if err := rows.Err(); err != nil {
				return nil, taskDoneCheckOutput{}, fmt.Errorf("task.done_check rows: %w", err)
			}

			return nil, buildTaskDoneCheckOutput(scope, deps, assignee, records), nil
		},
	)
}

func buildTaskDoneCheckOutput(scope *auth.ProjectScope, deps Deps, assignee string, records []taskDoneCheckRecord) taskDoneCheckOutput {
	out := taskDoneCheckOutput{
		ProjectSlug:               scope.ProjectSlug,
		Assignee:                  assignee,
		OpenTasks:                 []taskDoneCheckTaskRef{},
		UnresolvedAcceptanceTasks: []taskDoneCheckTaskRef{},
	}
	for _, rec := range records {
		bucket := taskStatusBucket(rec.RawStatus)
		ref := taskDoneCheckRef(scope, deps, rec, bucket)
		switch bucket {
		case "open", taskStatusMissing:
			out.OpenTasks = append(out.OpenTasks, ref)
		case "claimed_done":
			if len(ref.UnresolvedAcceptanceLabels) > 0 {
				out.UnresolvedAcceptanceTasks = append(out.UnresolvedAcceptanceTasks, ref)
			}
		}
	}
	out.OpenTaskCount = len(out.OpenTasks)
	out.UnresolvedAcceptanceTaskCount = len(out.UnresolvedAcceptanceTasks)
	out.IsDone = out.OpenTaskCount == 0 && out.UnresolvedAcceptanceTaskCount == 0
	out.Summary = taskDoneCheckSummary(assignee, out.OpenTaskCount, out.UnresolvedAcceptanceTaskCount)
	return out
}

func taskDoneCheckRef(scope *auth.ProjectScope, deps Deps, rec taskDoneCheckRecord, status string) taskDoneCheckTaskRef {
	counts := countTaskQueueAcceptance(rec.Body)
	labels := unresolvedAcceptanceLabels(rec.Body)
	return taskDoneCheckTaskRef{
		ArtifactID:                 rec.ArtifactID,
		Slug:                       rec.Slug,
		Title:                      rec.Title,
		AreaSlug:                   rec.AreaSlug,
		Priority:                   rec.Priority,
		Status:                     status,
		AcceptanceCheckboxesTotal:  counts.total,
		UnresolvedCheckboxes:       counts.unresolved,
		PartialCheckboxes:          counts.partial,
		UnresolvedAcceptanceLabels: labels,
		AgentRef:                   "pindoc://" + rec.Slug,
		HumanURL:                   HumanURL(scope.ProjectSlug, scope.ProjectLocale, rec.Slug),
		HumanURLAbs:                AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, rec.Slug),
	}
}

func taskDoneCheckSummary(assignee string, openCount, unresolvedCount int) string {
	if openCount == 0 && unresolvedCount == 0 {
		return fmt.Sprintf("All done for %s: no open Tasks and no claimed_done Tasks with unresolved acceptance.", assignee)
	}
	parts := []string{}
	if openCount > 0 {
		parts = append(parts, fmt.Sprintf("%d open/missing_status Task(s)", openCount))
	}
	if unresolvedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d claimed_done Task(s) with unresolved acceptance", unresolvedCount))
	}
	return fmt.Sprintf("Not done for %s: %s remain.", assignee, strings.Join(parts, " + "))
}
