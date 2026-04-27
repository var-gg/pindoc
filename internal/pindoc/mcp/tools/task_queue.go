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

	taskQueueSemantics = "reader_tasks_queue_v1"
	taskStatusMissing  = "missing_status"
	taskStatusOther    = "other"

	taskWarningStatusMissing              = "TASK_STATUS_MISSING"
	taskWarningAcceptanceReconcilePending = "TASK_ACCEPTANCE_DONE_RECONCILE_PENDING"
)

type taskQueueInput struct {
	ProjectSlug string `json:"project_slug" jsonschema:"projects.slug to scope this call to"`

	// Status selects the task lifecycle bucket to return. The default
	// "pending" intentionally matches the Reader header count:
	// task_meta.status missing OR task_meta.status == "open".
	Status string `json:"status,omitempty" jsonschema:"pending (default = open + missing_status) | all | open | missing_status | missing | claimed_done | verified | blocked | cancelled"`

	AreaSlug string `json:"area_slug,omitempty" jsonschema:"optional - restrict to one area slug"`
	Priority string `json:"priority,omitempty" jsonschema:"optional - p0 | p1 | p2 | p3"`
	Assignee string `json:"assignee,omitempty" jsonschema:"optional - exact task_meta.assignee match, e.g. agent:codex; pair with compact=true for an assigned-only view"`

	// Limit caps returned items after status filtering. Counts are still
	// computed across every matching Task before the item limit is applied.
	Limit int `json:"limit,omitempty" jsonschema:"default 50, max 500"`

	// Compact omits the project-wide aggregate fields (status_counts,
	// area_counts, priority_counts, warning_counts) from the response so
	// callers viewing "what is on my plate" do not have to scroll past
	// project-wide noise. Items, totals, and notice are still returned.
	// Decision mcp-dx-외부-리뷰-codex-1차-피드백-6항목 발견 5.
	Compact bool `json:"compact,omitempty" jsonschema:"omit project-wide aggregate counts (status_counts/area_counts/priority_counts/warning_counts) — items+total preserved"`
}

type taskQueueItem struct {
	ArtifactID string `json:"artifact_id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	AreaSlug   string `json:"area_slug"`

	// Status is the normalized lifecycle bucket used by the Reader queue.
	// Missing task_meta.status is surfaced explicitly as "missing_status"
	// so agents do not mistake it for completed work.
	Status        string `json:"status"`
	RawStatus     string `json:"raw_status,omitempty"`
	MissingStatus bool   `json:"missing_status,omitempty"`

	Priority   string    `json:"priority,omitempty"`
	Assignee   string    `json:"assignee,omitempty"`
	DueAt      string    `json:"due_at,omitempty"`
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

	// Counts are computed after area / priority / assignee filters and
	// before status filtering. This lets agents see "pending 13 / total
	// 69" while asking for just one bucket of Items.
	TotalCount     int            `json:"total_count"`
	PendingCount   int            `json:"pending_count"`
	StatusCounts   map[string]int `json:"status_counts,omitempty"`
	AreaCounts     map[string]int `json:"area_counts,omitempty"`
	PriorityCounts map[string]int `json:"priority_counts,omitempty"`
	WarningCounts  map[string]int `json:"warning_counts,omitempty"`

	// Compact mirrors the input flag back so callers / telemetry can tell
	// at a glance whether the omitted aggregates are intentional.
	Compact bool `json:"compact,omitempty"`

	Items     []taskQueueItem `json:"items"`
	Truncated bool            `json:"truncated,omitempty"`
	Notice    string          `json:"notice"`
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
checkboxes.
`),
		},
		func(ctx context.Context, p *auth.Principal, in taskQueueInput) (*sdk.CallToolResult, taskQueueOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, taskQueueOutput{}, fmt.Errorf("task.queue: %w", err)
			}
			statusFilter, ok := normalizeTaskQueueStatusFilter(in.Status)
			if !ok {
				return nil, taskQueueOutput{}, fmt.Errorf("status must be one of: pending | all | open | missing_status | missing | claimed_done | verified | blocked | cancelled")
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
				  AND NOT starts_with(a.slug, '_template_')
				  AND ($2::text = '' OR ar.slug = $2)
				  AND ($3::text = '' OR a.task_meta->>'priority' = $3)
				  AND ($4::text = '' OR a.task_meta->>'assignee' = $4)
				ORDER BY a.updated_at DESC
			`, scope.ProjectSlug, strings.TrimSpace(in.AreaSlug), priority, strings.TrimSpace(in.Assignee))
			if err != nil {
				return nil, taskQueueOutput{}, fmt.Errorf("task.queue query: %w", err)
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
					return nil, taskQueueOutput{}, fmt.Errorf("task.queue scan: %w", err)
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
				if bucket == taskStatusOther {
					item.RawStatus = strings.TrimSpace(rawStatus)
				}
				if bucket == taskStatusMissing {
					item.MissingStatus = true
				}
				item.Warnings = itemWarnings
				item.AgentRef = "pindoc://" + item.Slug
				item.HumanURL = HumanURL(scope.ProjectSlug, scope.ProjectLocale, item.Slug)
				item.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, item.Slug)
				items = append(items, item)
			}
			if err := rows.Err(); err != nil {
				return nil, taskQueueOutput{}, fmt.Errorf("task.queue rows: %w", err)
			}

			total := 0
			for _, n := range statusCounts {
				total += n
			}

			out := taskQueueOutput{
				SourceSemantics: taskQueueSemantics,
				StatusFilter:    statusFilter,
				TotalCount:      total,
				PendingCount:    statusCounts["open"] + statusCounts[taskStatusMissing],
				StatusCounts:    statusCounts,
				AreaCounts:      areaCounts,
				PriorityCounts:  priorityCounts,
				WarningCounts:   warningCounts,
				Items:           items,
				Truncated:       truncated,
				Notice:          taskQueueNotice(),
			}
			applyTaskQueueCompact(&out, in.Compact)
			return nil, out, nil
		},
	)
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
	case "pending", "all", "open", taskStatusMissing, "claimed_done", "verified", "blocked", "cancelled":
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
		"verified":        0,
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

func taskQueueNotice() string {
	return "Reader parity: pending means task_meta.status is missing or open. Acceptance-complete open Tasks are transient reconcile candidates; pindoc.ping auto-transitions them to claimed_done. Use pindoc.scope.in_flight for unresolved [ ]/[~] checklist items."
}

// applyTaskQueueCompact drops the project-wide aggregate maps when the
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
