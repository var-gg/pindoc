package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

const (
	taskQueueDefaultLimit = 50
	taskQueueMaxLimit     = 500

	taskQueueSemantics    = "reader_tasks_queue_v1"
	taskQueueDefaultFocus = "assignee_open_count"
	taskStatusMissing     = "missing_status"
	taskStatusOther       = "other"

	taskWarningStatusMissing              = "TASK_STATUS_MISSING"
	taskWarningAcceptanceReconcilePending = "TASK_ACCEPTANCE_DONE_RECONCILE_PENDING"
	taskWarningMultiProjectWorkspace      = "MULTI_PROJECT_WORKSPACE"
)

type taskQueueInput struct {
	ProjectSlug    string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	AcrossProjects bool   `json:"across_projects,omitempty" jsonschema:"optional - true lists the caller-visible workspace project queues in one response; defaults assignee to the calling agent when assignee is omitted"`

	// Status selects the task lifecycle bucket to return. The default
	// "pending" intentionally matches the Reader header count:
	// task_meta.status missing OR task_meta.status == "open".
	Status string `json:"status,omitempty" jsonschema:"pending (default = open + missing_status) | all | open | missing_status | missing | claimed_done | blocked | cancelled"`

	AreaSlug string `json:"area_slug,omitempty" jsonschema:"optional - restrict to one area slug"`
	Priority string `json:"priority,omitempty" jsonschema:"optional - p0 | p1 | p2 | p3"`
	Assignee string `json:"assignee,omitempty" jsonschema:"optional - exact task_meta.assignee match, e.g. agent:codex; pair with compact=true for an assigned-only view"`

	// Limit caps returned items after status filtering. Counts are still
	// computed across every matching Task before the item limit is applied.
	Limit int `json:"limit,omitempty" jsonschema:"default 50, max 500"`

	// Compact omits the aggregate fields (status_counts,
	// area_counts, priority_counts, warning_counts) from the response so
	// callers viewing "what is on my plate" do not have to scroll past
	// breakdowns. Items, totals, and notice are still returned.
	// Decision mcp-dx-외부-리뷰-codex-1차-피드백-6항목 발견 5.
	Compact bool `json:"compact,omitempty" jsonschema:"omit aggregate counts (status_counts/area_counts/priority_counts/warning_counts) — items+counts preserved"`
}

type taskQueueItem struct {
	ArtifactID  string `json:"artifact_id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	ProjectSlug string `json:"project_slug,omitempty"`
	AreaSlug    string `json:"area_slug"`

	// Status is the normalized lifecycle bucket used by the Reader queue.
	// Missing task_meta.status is surfaced explicitly as "missing_status"
	// so agents do not mistake it for completed work.
	Status        string `json:"status"`
	StatusBucket  string `json:"status_bucket"`
	RawStatus     string `json:"raw_status,omitempty"`
	MissingStatus bool   `json:"missing_status,omitempty"`

	Priority string `json:"priority,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	DueAt    string `json:"due_at,omitempty"`

	AcceptanceCheckboxesTotal int    `json:"acceptance_checkboxes_total"`
	ResolvedCheckboxes        int    `json:"resolved_checkboxes"`
	UnresolvedCheckboxes      int    `json:"unresolved_checkboxes"`
	PartialCheckboxes         int    `json:"partial_checkboxes"`
	DeferredCheckboxes        int    `json:"deferred_checkboxes"`
	ReadyToClose              bool   `json:"ready_to_close"`
	ReadyToCloseStatus        string `json:"ready_to_close_status"`

	ParentSlug string    `json:"parent_slug,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
	Warnings   []string  `json:"warnings,omitempty"`

	AgentRef    string `json:"agent_ref"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

type taskQueueOutput struct {
	SourceSemantics string `json:"source_semantics"`
	StatusFilter    string `json:"status_filter"`
	DefaultFocus    string `json:"default_focus"`
	AcrossProjects  bool   `json:"across_projects,omitempty"`
	WorkspaceRoot   string `json:"workspace_root,omitempty"`

	// AssigneeFilteredCount is computed after area / priority /
	// assignee filters and before status filtering. AssigneeOpenCount is
	// the open+missing subset after those same filters. ProjectTotalCount
	// is the unfiltered active Task count for the project. The legacy
	// total_count / pending_count fields remain for old clients.
	AssigneeFilteredCount int               `json:"assignee_filtered_count"`
	AssigneeOpenCount     int               `json:"assignee_open_count"`
	ProjectTotalCount     int               `json:"project_total_count"`
	TotalCount            int               `json:"total_count"`
	PendingCount          int               `json:"pending_count"`
	CountDeprecationNote  string            `json:"count_deprecation_notice,omitempty"`
	CountLegend           map[string]string `json:"count_legend,omitempty"`

	StatusCounts   map[string]int `json:"status_counts,omitempty"`
	AreaCounts     map[string]int `json:"area_counts,omitempty"`
	PriorityCounts map[string]int `json:"priority_counts,omitempty"`
	WarningCounts  map[string]int `json:"warning_counts,omitempty"`

	Projects               map[string]taskQueueProjectOutput `json:"projects,omitempty"`
	TotalAssigneeOpenCount *int                              `json:"total_assignee_open_count,omitempty"`
	Warnings               []taskQueueWarning                `json:"warnings,omitempty"`

	// Compact mirrors the input flag back so callers / telemetry can tell
	// at a glance whether the omitted aggregates are intentional.
	Compact bool `json:"compact,omitempty"`

	Items          []taskQueueItem     `json:"items"`
	Truncated      bool                `json:"truncated,omitempty"`
	Notice         string              `json:"notice"`
	NextTools      []NextToolHint      `json:"next_tools,omitempty"`
	Attention      *TaskQueueAttention `json:"attention,omitempty"`
	ToolsetVersion string              `json:"toolset_version,omitempty"`
}

type taskQueueProjectOutput struct {
	ProjectSlug           string          `json:"project_slug"`
	AssigneeFilteredCount int             `json:"assignee_filtered_count"`
	AssigneeOpenCount     int             `json:"assignee_open_count"`
	ProjectTotalCount     int             `json:"project_total_count"`
	TotalCount            int             `json:"total_count"`
	PendingCount          int             `json:"pending_count"`
	Items                 []taskQueueItem `json:"items"`
	Truncated             bool            `json:"truncated,omitempty"`
}

type taskQueueWarning struct {
	Code             string   `json:"code"`
	Message          string   `json:"message"`
	DetectedProjects []string `json:"detected_projects,omitempty"`
	Hint             string   `json:"hint,omitempty"`
}

// RegisterTaskQueue wires pindoc.task.queue. This is the MCP counterpart
// to the Reader Tasks surface: type=Task rows grouped by task_meta.status,
// with missing status treated as pending/no_status. It is deliberately
// not derived from acceptance checkboxes; pindoc.scope.in_flight covers
// that separate question.
func RegisterTaskQueue(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.task.queue",
			Description: strings.TrimSpace(`
List Tasks using the same pending semantics as the Reader Tasks board.
Default status="pending" means task_meta.status is missing OR "open".
Counts are grouped by task_meta.status, area, and priority. This is the
canonical MCP pre-flight before saying "the Task queue is done"; it is
not the same as pindoc.scope.in_flight, which lists unresolved acceptance
checkboxes. Each item includes ready_to_close fields for the acceptance
checklist, but queue counts remain lifecycle counts. For agent dogfood,
default_focus="assignee_open_count" is the current open-work view;
historical totals are reference context. When the caller is an agent
querying its own assignee queue, the response may include attention for
Tasks idle longer than PINDOC_STUCK_THRESHOLD_HOURS (default 24).
Pass across_projects=true during session pre-flight to return every
caller-visible project queue in projects{slug:{items, assignee_open_count}}
with total_assignee_open_count and workspace_root; when assignee is omitted
in this mode, the caller agent's assignee id is used. If project_slug is
omitted in a multi-project workspace without across_projects, the response
includes a MULTI_PROJECT_WORKSPACE warning with detected_projects and a
hint to rerun the sweep or pin project_slug explicitly. Count
fields are explicit:
assignee_filtered_count is after optional area/priority/assignee filters,
assignee_open_count is the filtered open+missing_status queue (same as
legacy pending_count), and project_total_count is the active Task total
for the whole project. Legacy total_count and pending_count remain for
backward compatibility; count_legend repeats these meanings in the
response.
When multiple open Tasks match, queue may include a pindoc.task.next
next_tool: queue is the lifecycle/count surface, while task.next chooses
likely next executable work from dependency and blocker ordering.
`),
		},
		func(ctx context.Context, p *auth.Principal, in taskQueueInput) (*sdk.CallToolResult, taskQueueOutput, error) {
			statusFilter, ok := normalizeTaskQueueStatusFilter(in.Status)
			if !ok {
				return nil, taskQueueOutput{}, fmt.Errorf("status must be one of: pending | all | open | missing_status | missing | claimed_done | blocked | cancelled")
			}
			priority := strings.TrimSpace(strings.ToLower(in.Priority))
			if priority != "" {
				if _, ok := validTaskPriorities[priority]; !ok {
					return nil, taskQueueOutput{}, fmt.Errorf("priority must be one of: p0 | p1 | p2 | p3")
				}
			}

			limit := in.Limit
			if limit <= 0 {
				limit = taskQueueDefaultLimit
			}
			if limit > taskQueueMaxLimit {
				limit = taskQueueMaxLimit
			}

			if in.AcrossProjects {
				out, err := handleTaskQueueAcrossProjects(ctx, deps, p, in, statusFilter, priority, limit)
				if err != nil {
					return nil, taskQueueOutput{}, err
				}
				applyTaskQueueCompact(&out, in.Compact)
				return nil, out, nil
			}
			if strings.TrimSpace(in.ProjectSlug) == "" {
				if warning, ok := taskQueueMultiProjectWorkspaceWarning(ctx, deps, p); ok {
					out := emptyTaskQueueOutput(statusFilter)
					out.Warnings = append(out.Warnings, warning)
					applyTaskQueueCompact(&out, in.Compact)
					return nil, out, nil
				}
			}

			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskQueueOutput{}, fmt.Errorf("task.queue: %w", err)
			}
			out, err := buildTaskQueueForProject(ctx, deps, p, scope, in, statusFilter, priority, limit, strings.TrimSpace(in.Assignee), false)
			if err != nil {
				return nil, taskQueueOutput{}, err
			}
			out.Attention = buildTaskQueueAttention(ctx, deps, p, scope.ProjectSlug, strings.TrimSpace(in.Assignee), deps.UserLanguage)
			if warning, ok := taskQueueMultiProjectWorkspaceWarning(ctx, deps, p); ok {
				out.Warnings = append(out.Warnings, warning)
			}
			applyTaskQueueCompact(&out, in.Compact)
			return nil, out, nil
		},
	)
}

func handleTaskQueueAcrossProjects(ctx context.Context, deps Deps, p *auth.Principal, in taskQueueInput, statusFilter, priority string, limit int) (taskQueueOutput, error) {
	slugs, err := visibleProjectSlugs(ctx, deps, p)
	if err != nil {
		return taskQueueOutput{}, fmt.Errorf("task.queue across_projects: %w", err)
	}
	assignee := strings.TrimSpace(in.Assignee)
	if assignee == "" {
		assignee = callerAgentAssignee(p)
	}

	projects := make(map[string]taskQueueProjectOutput, len(slugs))
	statusCounts := newTaskStatusCounts()
	areaCounts := map[string]int{}
	priorityCounts := map[string]int{}
	warningCounts := map[string]int{}
	items := []taskQueueItem{}
	totalAssigneeOpen := 0
	assigneeFilteredTotal := 0
	projectTotal := 0
	truncated := false

	for _, slug := range slugs {
		scope, err := auth.ResolveProject(ctx, deps.DB, p, slug)
		if err != nil {
			return taskQueueOutput{}, fmt.Errorf("task.queue across_projects resolve %q: %w", slug, err)
		}
		projectOut, err := buildTaskQueueForProject(ctx, deps, p, scope, in, statusFilter, priority, limit, assignee, true)
		if err != nil {
			return taskQueueOutput{}, err
		}
		projects[slug] = taskQueueProjectOutput{
			ProjectSlug:           slug,
			AssigneeFilteredCount: projectOut.AssigneeFilteredCount,
			AssigneeOpenCount:     projectOut.AssigneeOpenCount,
			ProjectTotalCount:     projectOut.ProjectTotalCount,
			TotalCount:            projectOut.TotalCount,
			PendingCount:          projectOut.PendingCount,
			Items:                 projectOut.Items,
			Truncated:             projectOut.Truncated,
		}
		mergeTaskQueueCounts(statusCounts, projectOut.StatusCounts)
		mergeTaskQueueCounts(areaCounts, projectOut.AreaCounts)
		mergeTaskQueueCounts(priorityCounts, projectOut.PriorityCounts)
		mergeTaskQueueCounts(warningCounts, projectOut.WarningCounts)
		items = append(items, projectOut.Items...)
		totalAssigneeOpen += projectOut.AssigneeOpenCount
		assigneeFilteredTotal += projectOut.AssigneeFilteredCount
		projectTotal += projectOut.ProjectTotalCount
		truncated = truncated || projectOut.Truncated
	}

	totalOpenCopy := totalAssigneeOpen
	return taskQueueOutput{
		SourceSemantics:        taskQueueSemantics,
		StatusFilter:           statusFilter,
		DefaultFocus:           taskQueueDefaultFocus,
		AcrossProjects:         true,
		WorkspaceRoot:          strings.TrimSpace(deps.RepoRoot),
		AssigneeFilteredCount:  assigneeFilteredTotal,
		AssigneeOpenCount:      totalAssigneeOpen,
		ProjectTotalCount:      projectTotal,
		TotalCount:             assigneeFilteredTotal,
		PendingCount:           totalAssigneeOpen,
		CountDeprecationNote:   "total_count is kept as an alias for assignee_filtered_count; pending_count is kept as an alias for assignee_open_count.",
		CountLegend:            taskQueueCountLegend(),
		StatusCounts:           statusCounts,
		AreaCounts:             areaCounts,
		PriorityCounts:         priorityCounts,
		WarningCounts:          warningCounts,
		Projects:               projects,
		TotalAssigneeOpenCount: &totalOpenCopy,
		Items:                  items,
		Truncated:              truncated,
		Notice:                 taskQueueNotice(),
		NextTools:              taskQueueAcrossProjectsNextTools(assignee, strings.TrimSpace(in.AreaSlug), priority, statusFilter, totalAssigneeOpen, priorityCounts),
	}, nil
}

func buildTaskQueueForProject(ctx context.Context, deps Deps, p *auth.Principal, scope *auth.ProjectScope, in taskQueueInput, statusFilter, priority string, limit int, assignee string, includeItemProjectSlug bool) (taskQueueOutput, error) {
	var projectTotalCount int
	if err := deps.DB.QueryRow(ctx, `
		SELECT count(*)::int
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.type = 'Task'
		   AND a.status <> 'archived'
		   AND a.status <> 'superseded'
		   AND NOT starts_with(a.slug, '_template_')
	`, scope.ProjectSlug).Scan(&projectTotalCount); err != nil {
		return taskQueueOutput{}, fmt.Errorf("task.queue project total: %w", err)
	}

	rows, err := deps.DB.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, ar.slug, a.updated_at,
		       a.body_markdown,
		       COALESCE(a.task_meta->>'status', ''),
		       COALESCE(a.task_meta->>'priority', ''),
		       COALESCE(a.task_meta->>'assignee', ''),
		       COALESCE(a.task_meta->>'due_at', ''),
		       COALESCE(a.task_meta->>'parent_slug', '')
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		WHERE p.slug = $1
		  AND a.type = 'Task'
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
		  AND NOT starts_with(a.slug, '_template_')
		  AND ($2::text = '' OR ar.slug = $2)
		  AND ($3::text = '' OR a.task_meta->>'priority' = $3)
		  AND ($4::text = '' OR a.task_meta->>'assignee' = $4)
		ORDER BY a.updated_at DESC
	`, scope.ProjectSlug, strings.TrimSpace(in.AreaSlug), priority, strings.TrimSpace(assignee))
	if err != nil {
		return taskQueueOutput{}, fmt.Errorf("task.queue query: %w", err)
	}
	defer rows.Close()

	statusCounts := newTaskStatusCounts()
	areaCounts := map[string]int{}
	priorityCounts := map[string]int{}
	warningCounts := map[string]int{}
	items := []taskQueueItem{}
	truncated := false

	for rows.Next() {
		var item taskQueueItem
		var body, rawStatus string
		if err := rows.Scan(
			&item.ArtifactID, &item.Slug, &item.Title, &item.AreaSlug, &item.UpdatedAt,
			&body, &rawStatus, &item.Priority, &item.Assignee, &item.DueAt, &item.ParentSlug,
		); err != nil {
			return taskQueueOutput{}, fmt.Errorf("task.queue scan: %w", err)
		}

		bucket := taskStatusBucket(rawStatus)
		statusCounts[bucket]++
		areaCounts[item.AreaSlug]++
		priorityCounts[taskPriorityBucket(item.Priority)]++
		itemWarnings := taskQueueWarnings(bucket, body)
		for _, w := range itemWarnings {
			warningCounts[w]++
		}

		if !taskQueueStatusMatches(rawStatus, statusFilter) {
			continue
		}
		if len(items) >= limit {
			truncated = true
			continue
		}

		item.Status = bucket
		item.StatusBucket = bucket
		if includeItemProjectSlug {
			item.ProjectSlug = scope.ProjectSlug
		}
		if bucket == taskStatusOther {
			item.RawStatus = strings.TrimSpace(rawStatus)
		}
		if bucket == taskStatusMissing {
			item.MissingStatus = true
		}
		applyTaskQueueReadySignal(&item, bucket, body)
		item.Warnings = itemWarnings
		item.AgentRef = "pindoc://" + item.Slug
		item.HumanURL = HumanURL(scope.ProjectSlug, scope.ProjectLocale, item.Slug)
		item.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, item.Slug)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return taskQueueOutput{}, fmt.Errorf("task.queue rows: %w", err)
	}

	total := 0
	for _, n := range statusCounts {
		total += n
	}
	assigneeOpenCount := statusCounts["open"] + statusCounts[taskStatusMissing]

	return taskQueueOutput{
		SourceSemantics:       taskQueueSemantics,
		StatusFilter:          statusFilter,
		DefaultFocus:          taskQueueDefaultFocus,
		AssigneeFilteredCount: total,
		AssigneeOpenCount:     assigneeOpenCount,
		ProjectTotalCount:     projectTotalCount,
		TotalCount:            total,
		PendingCount:          assigneeOpenCount,
		CountDeprecationNote:  "total_count is kept as an alias for assignee_filtered_count; pending_count is kept as an alias for assignee_open_count.",
		CountLegend:           taskQueueCountLegend(),
		StatusCounts:          statusCounts,
		AreaCounts:            areaCounts,
		PriorityCounts:        priorityCounts,
		WarningCounts:         warningCounts,
		Items:                 items,
		Truncated:             truncated,
		Notice:                taskQueueNotice(),
		NextTools: taskQueueNextTools(
			scope.ProjectSlug,
			strings.TrimSpace(assignee),
			strings.TrimSpace(in.AreaSlug),
			priority,
			statusFilter,
			assigneeOpenCount,
			priorityCounts,
		),
	}, nil
}

func emptyTaskQueueOutput(statusFilter string) taskQueueOutput {
	return taskQueueOutput{
		SourceSemantics:      taskQueueSemantics,
		StatusFilter:         statusFilter,
		DefaultFocus:         taskQueueDefaultFocus,
		CountDeprecationNote: "total_count is kept as an alias for assignee_filtered_count; pending_count is kept as an alias for assignee_open_count.",
		CountLegend:          taskQueueCountLegend(),
		StatusCounts:         newTaskStatusCounts(),
		AreaCounts:           map[string]int{},
		PriorityCounts:       map[string]int{},
		WarningCounts:        map[string]int{},
		Items:                []taskQueueItem{},
		Notice:               taskQueueNotice(),
	}
}

func mergeTaskQueueCounts(dst, src map[string]int) {
	for k, v := range src {
		dst[k] += v
	}
}

func taskQueueMultiProjectWorkspaceWarning(ctx context.Context, deps Deps, p *auth.Principal) (taskQueueWarning, bool) {
	defaultRes, ok := projectSlugDefaultResultFromContext(ctx)
	if !ok || defaultRes.Via == "" {
		return taskQueueWarning{}, false
	}
	slugs, err := visibleProjectSlugs(ctx, deps, p)
	if err != nil {
		return taskQueueWarning{}, false
	}
	return buildTaskQueueMultiProjectWorkspaceWarning(defaultRes, slugs)
}

func buildTaskQueueMultiProjectWorkspaceWarning(defaultRes projectSlugDefaultResult, detectedProjects []string) (taskQueueWarning, bool) {
	detectedProjects = normalizedSlugList(detectedProjects)
	if defaultRes.Via == "" || len(detectedProjects) <= 1 {
		return taskQueueWarning{}, false
	}
	return taskQueueWarning{
		Code:             taskWarningMultiProjectWorkspace,
		Message:          "project_slug was omitted in a multi-project workspace; this response is scoped to the default project only.",
		DetectedProjects: detectedProjects,
		Hint:             `Run pindoc.task.queue with across_projects=true for the session-start sweep, or pass project_slug explicitly for a pinned project queue.`,
	}, true
}

func normalizeTaskQueueStatusFilter(raw string) (string, bool) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return "pending", true
	}
	if s == "missing" {
		return taskStatusMissing, true
	}
	switch s {
	case "pending", "all", "open", taskStatusMissing, "claimed_done", "blocked", "cancelled":
		return s, true
	default:
		return "", false
	}
}

func taskStatusBucket(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return taskStatusMissing
	}
	if _, ok := validTaskStatuses[s]; ok {
		return s
	}
	return taskStatusOther
}

func taskPriorityBucket(raw string) string {
	p := strings.TrimSpace(strings.ToLower(raw))
	if p == "" {
		return "missing_priority"
	}
	if _, ok := validTaskPriorities[p]; ok {
		return p
	}
	return "other_priority"
}

func taskQueueStatusMatches(rawStatus, filter string) bool {
	bucket := taskStatusBucket(rawStatus)
	switch filter {
	case "all":
		return true
	case "pending":
		return bucket == "open" || bucket == taskStatusMissing
	default:
		return bucket == filter
	}
}

func newTaskStatusCounts() map[string]int {
	return map[string]int{
		"open":            0,
		"claimed_done":    0,
		"blocked":         0,
		"cancelled":       0,
		taskStatusMissing: 0,
		taskStatusOther:   0,
	}
}

func taskQueueWarnings(statusBucket, body string) []string {
	warnings := []string{}
	if statusBucket == taskStatusMissing {
		warnings = append(warnings, taskWarningStatusMissing)
	}
	if statusBucket == "open" || statusBucket == taskStatusMissing {
		done, total := countAcceptanceCheckboxes(body)
		if total > 0 && done == total {
			warnings = append(warnings, taskWarningAcceptanceReconcilePending)
		}
	}
	return warnings
}

type taskQueueAcceptanceCounts struct {
	total      int
	resolved   int
	unresolved int
	partial    int
	deferred   int
}

func countTaskQueueAcceptance(body string) taskQueueAcceptanceCounts {
	var out taskQueueAcceptanceCounts
	for _, cb := range iterateCheckboxes(body) {
		out.total++
		switch cb.marker {
		case ' ', 0:
			out.unresolved++
		case '~':
			out.partial++
			out.resolved++
		case '-':
			out.deferred++
			out.resolved++
		case 'x', 'X':
			out.resolved++
		default:
			out.unresolved++
		}
	}
	return out
}

func applyTaskQueueReadySignal(item *taskQueueItem, statusBucket, body string) {
	if item == nil {
		return
	}
	counts := countTaskQueueAcceptance(body)
	item.AcceptanceCheckboxesTotal = counts.total
	item.ResolvedCheckboxes = counts.resolved
	item.UnresolvedCheckboxes = counts.unresolved
	item.PartialCheckboxes = counts.partial
	item.DeferredCheckboxes = counts.deferred

	switch {
	case statusBucket == "claimed_done" || statusBucket == "cancelled":
		item.ReadyToCloseStatus = "terminal_status"
	case statusBucket == "blocked":
		item.ReadyToCloseStatus = "blocked"
	case statusBucket != "open" && statusBucket != taskStatusMissing:
		item.ReadyToCloseStatus = "not_open"
	case counts.total == 0:
		item.ReadyToCloseStatus = "no_acceptance_checkboxes"
	case counts.unresolved == 0:
		item.ReadyToClose = true
		item.ReadyToCloseStatus = "ready"
	default:
		item.ReadyToCloseStatus = "unresolved_acceptance"
	}
}

func taskQueueNotice() string {
	return "Reader parity: pending means task_meta.status is missing or open. Counts are lifecycle counts; item-level ready_to_close is the acceptance checklist signal. Acceptance-complete open Tasks are transient reconcile candidates; pindoc.ping auto-transitions them to claimed_done. Use pindoc.scope.in_flight for unresolved [ ]/[~] checklist items."
}

func taskQueueCloseoutNextTools(projectSlug, assignee string) []NextToolHint {
	if strings.TrimSpace(assignee) == "" {
		return nil
	}
	return []NextToolHint{
		{
			Tool: "pindoc.task.done_check",
			Args: map[string]any{
				"project_slug": projectSlug,
				"assignee":     strings.TrimSpace(assignee),
				"mode":         taskDoneCheckModeCurrentOpenOnly,
			},
			Reason: "final current-open-work closeout check before telling the user the assigned queue is complete; historical acceptance debt is reported separately",
		},
	}
}

func taskQueueNextTools(projectSlug, assignee, areaSlug, priority, statusFilter string, assigneeOpenCount int, priorityCounts map[string]int) []NextToolHint {
	out := taskQueueRecommendationNextTools(projectSlug, assignee, areaSlug, priority, statusFilter, assigneeOpenCount, priorityCounts)
	return appendUniqueNextTools(out, taskQueueCloseoutNextTools(projectSlug, assignee)...)
}

func taskQueueRecommendationNextTools(projectSlug, assignee, areaSlug, priority, statusFilter string, assigneeOpenCount int, priorityCounts map[string]int) []NextToolHint {
	if statusFilter != "pending" && statusFilter != "open" && statusFilter != taskStatusMissing && statusFilter != "all" {
		return nil
	}
	if assigneeOpenCount <= 1 {
		return nil
	}
	p1Count := 0
	if priorityCounts != nil {
		p1Count = priorityCounts["p1"]
	}
	reason := "Use task.next when queue has multiple open Tasks; it orders ready work by dependency/blocker state instead of priority alone."
	if p1Count > 1 {
		reason = "Multiple p1 Tasks are open; use task.next to choose the next ready Task from dependency/blocker ordering instead of priority alone."
	}
	args := map[string]any{
		"project_slug": projectSlug,
		"limit":        5,
	}
	if areaSlug != "" {
		args["area_slug"] = areaSlug
	}
	if priority != "" {
		args["priority"] = priority
	}
	if assignee != "" {
		args["actor_scope"] = "assignee"
		args["actor_id"] = assignee
	} else {
		args["actor_scope"] = "all_visible"
	}
	return []NextToolHint{{
		Tool:   "pindoc.task.next",
		Args:   args,
		Reason: reason,
	}}
}

func taskQueueAcrossProjectsNextTools(assignee, areaSlug, priority, statusFilter string, assigneeOpenCount int, priorityCounts map[string]int) []NextToolHint {
	if statusFilter != "pending" && statusFilter != "open" && statusFilter != taskStatusMissing && statusFilter != "all" {
		return nil
	}
	if assigneeOpenCount <= 1 {
		return nil
	}
	p1Count := 0
	if priorityCounts != nil {
		p1Count = priorityCounts["p1"]
	}
	reason := "Use task.next with project_scope=visible when multiple open Tasks span projects; it orders ready work by dependency/blocker state."
	if p1Count > 1 {
		reason = "Multiple p1 Tasks are open across visible projects; use task.next with project_scope=visible to choose ready work by dependency/blocker ordering."
	}
	args := map[string]any{
		"project_scope": "visible",
		"limit":         5,
	}
	if areaSlug != "" {
		args["area_slug"] = areaSlug
	}
	if priority != "" {
		args["priority"] = priority
	}
	if assignee != "" {
		args["actor_scope"] = "assignee"
		args["actor_id"] = assignee
	} else {
		args["actor_scope"] = "all_visible"
	}
	return []NextToolHint{{
		Tool:   "pindoc.task.next",
		Args:   args,
		Reason: reason,
	}}
}

func taskQueueCountLegend() map[string]string {
	return map[string]string{
		"default_focus":           "The field agents should read first for their current open-work view: assignee_open_count.",
		"assignee_filtered_count": "Task count after area, priority, and assignee filters, before status filtering.",
		"assignee_open_count":     "Open queue count after area, priority, and assignee filters: task_meta.status missing or open. Same value as legacy pending_count.",
		"project_total_count":     "All active Task artifacts in the project, ignoring area, priority, assignee, and status filters.",
		"total_count":             "Legacy alias for assignee_filtered_count.",
		"pending_count":           "Legacy alias for assignee_open_count.",
		"items":                   "Returned rows after status filtering and limit.",
		"ready_to_close":          "Per-item acceptance checklist signal. true only for open/missing_status Tasks with at least one acceptance checkbox and zero unresolved [ ] items.",
	}
}

// applyTaskQueueCompact drops the aggregate maps when the
// caller asked for the compact view. Totals (TotalCount / PendingCount)
// are computed before this fires so they stay honest — the user sees
// "12 pending of 47 total" without scrolling past per-area / per-status
// breakdowns. The maps go to nil rather than empty {} so the json
// encoder omits them via the omitempty contract.
func applyTaskQueueCompact(out *taskQueueOutput, compact bool) {
	if out == nil {
		return
	}
	out.Compact = compact
	if !compact {
		return
	}
	out.StatusCounts = nil
	out.AreaCounts = nil
	out.PriorityCounts = nil
	out.WarningCounts = nil
}
