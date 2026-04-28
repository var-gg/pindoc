package tools

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

const defaultStuckThresholdHours = 24

type CallerInFlightAttention struct {
	Code                string         `json:"code"`
	Message             string         `json:"message"`
	NextTools           []NextToolHint `json:"next_tools,omitempty"`
	Level               string         `json:"level"`
	Count               int            `json:"count"`
	TopSlugs            []string       `json:"top_slugs"`
	StuckCount          int            `json:"stuck_count"`
	StuckThresholdHours int            `json:"stuck_threshold_hours"`
}

type TaskQueueAttention struct {
	Code                string         `json:"code"`
	Message             string         `json:"message"`
	NextTools           []NextToolHint `json:"next_tools,omitempty"`
	Level               string         `json:"level"`
	StuckSlugs          []string       `json:"stuck_slugs"`
	StuckThresholdHours int            `json:"stuck_threshold_hours"`
}

type taskActivityRow struct {
	Slug         string
	LastActivity time.Time
}

func buildCallerInFlightAttention(ctx context.Context, deps Deps, p *auth.Principal, projectSlug, locale string) *CallerInFlightAttention {
	caller := callerAgentAssignee(p)
	if caller == "" {
		return nil
	}
	threshold := stuckThresholdHours()
	rows, err := openTasksForAssignee(ctx, deps, projectSlug, caller)
	if err != nil || len(rows) == 0 {
		if err != nil && deps.Logger != nil {
			deps.Logger.Warn("caller in-flight attention failed", "err", err)
		}
		return nil
	}
	top := make([]string, 0, minInt(5, len(rows)))
	stuck := 0
	cutoff := time.Now().Add(-time.Duration(threshold) * time.Hour)
	for i, row := range rows {
		if i < 5 {
			top = append(top, row.Slug)
		}
		if row.LastActivity.Before(cutoff) {
			stuck++
		}
	}
	return &CallerInFlightAttention{
		Code:                "caller_has_open_tasks",
		Message:             callerInFlightMessage(locale, len(rows), stuck, threshold),
		Level:               "info",
		NextTools:           lifecycleNextTools(projectSlug, firstString(top)),
		Count:               len(rows),
		TopSlugs:            top,
		StuckCount:          stuck,
		StuckThresholdHours: threshold,
	}
}

func buildTaskQueueAttention(ctx context.Context, deps Deps, p *auth.Principal, projectSlug, queryAssignee, locale string) *TaskQueueAttention {
	caller := callerAgentAssignee(p)
	if caller == "" || !taskAttentionCallerMatches(caller, strings.TrimSpace(queryAssignee)) {
		return nil
	}
	threshold := stuckThresholdHours()
	rows, err := openTasksForAssignee(ctx, deps, projectSlug, caller)
	if err != nil || len(rows) == 0 {
		if err != nil && deps.Logger != nil {
			deps.Logger.Warn("task.queue attention failed", "err", err)
		}
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(threshold) * time.Hour)
	var stuck []string
	for _, row := range rows {
		if row.LastActivity.Before(cutoff) {
			stuck = append(stuck, row.Slug)
		}
	}
	if len(stuck) == 0 {
		return nil
	}
	return &TaskQueueAttention{
		Code:                "task_queue_stuck",
		Message:             taskQueueAttentionMessage(locale, len(stuck), threshold),
		Level:               "info",
		NextTools:           lifecycleNextTools(projectSlug, stuck[0]),
		StuckSlugs:          stuck,
		StuckThresholdHours: threshold,
	}
}

func openTasksForAssignee(ctx context.Context, deps Deps, projectSlug, assignee string) ([]taskActivityRow, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT a.slug,
		       COALESCE((
		           SELECT max(r.created_at)
		             FROM artifact_revisions r
		            WHERE r.artifact_id = a.id
		       ), a.updated_at) AS last_activity
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.type = 'Task'
		   AND a.status <> 'archived'
		   AND NOT starts_with(a.slug, '_template_')
		   AND COALESCE(a.task_meta->>'status', 'open') = 'open'
		   AND a.task_meta->>'assignee' = $2
		 ORDER BY last_activity DESC, a.slug
	`, projectSlug, assignee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []taskActivityRow
	for rows.Next() {
		var row taskActivityRow
		if err := rows.Scan(&row.Slug, &row.LastActivity); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func callerAgentAssignee(p *auth.Principal) string {
	caller := taskAttentionCallerID(p)
	if taskAttentionHumanCaller(caller) {
		return ""
	}
	return "agent:" + stripAgentPrefix(caller)
}

func stuckThresholdHours() int {
	raw := strings.TrimSpace(os.Getenv("PINDOC_STUCK_THRESHOLD_HOURS"))
	if raw == "" {
		return defaultStuckThresholdHours
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultStuckThresholdHours
	}
	return n
}

func lifecycleNextTools(projectSlug, slug string) []NextToolHint {
	args := map[string]any{"project_slug": projectSlug, "view": "full"}
	if slug != "" {
		args["id_or_slug"] = "pindoc://" + slug
	}
	return []NextToolHint{
		{Tool: "pindoc.artifact.read", Args: args, Reason: "inspect the open Task before continuing"},
		{
			Tool: "pindoc.artifact.propose",
			Args: map[string]any{
				"project_slug": projectSlug,
				"shape":        string(ShapeAcceptanceTransition),
			},
			Reason: "update acceptance checks or move lifecycle when complete",
		},
	}
}

func callerInFlightMessage(locale string, count, stuck, threshold int) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "ko") {
		return fmt.Sprintf("이 호출자에게 status=open인 Task가 %d개 있습니다(그중 %d개는 %dh+ 정체). 새 작업 시작 전에 닫을 수 있는 Task가 있는지 검토하세요.", count, stuck, threshold)
	}
	return fmt.Sprintf("Caller has %d open Tasks (%d stuck > %dh). Check whether any can be closed before starting new work.", count, stuck, threshold)
}

func taskQueueAttentionMessage(locale string, count, threshold int) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "ko") {
		return fmt.Sprintf("큐의 %d개 Task가 %d시간 이상 정체 상태입니다. acceptance 갱신 또는 status 전환을 검토하세요.", count, threshold)
	}
	return fmt.Sprintf("%d Tasks in queue have been idle > %dh. Update acceptance or transition status.", count, threshold)
}

func firstString(in []string) string {
	if len(in) == 0 {
		return ""
	}
	return in[0]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
