package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

const (
	taskFlowDefaultLimit = 100
	taskFlowMaxLimit     = 500

	taskFlowProjectScopeCurrent = "current"
	taskFlowProjectScopeList    = "list"
	taskFlowProjectScopeVisible = "visible"

	taskFlowActorAllVisible = "all_visible"
	taskFlowActorAssignee   = "assignee"
	taskFlowActorAgent      = "agent"
	taskFlowActorUser       = "user"
	taskFlowActorRequester  = "requester"
	taskFlowActorTeam       = "team"

	taskFlowScopeActive  = "active"
	taskFlowScopeAll     = "all"
	taskFlowScopeReady   = "ready"
	taskFlowScopeBlocked = "blocked"

	taskReadinessReady         = "ready"
	taskReadinessBlocked       = "blocked"
	taskReadinessBlockedStatus = "blocked_status"
	taskReadinessDone          = "done"
	taskReadinessOther         = "other"
)

type taskFlowInput struct {
	ProjectSlug  string   `json:"project_slug,omitempty" jsonschema:"optional single projects.slug; omitted uses explicit session/default resolver unless project_scope=visible or project_slugs is set"`
	ProjectSlugs []string `json:"project_slugs,omitempty" jsonschema:"optional explicit visible project slug list"`
	ProjectScope string   `json:"project_scope,omitempty" jsonschema:"current (default) | list | visible | caller_visible"`

	ActorScope        string   `json:"actor_scope,omitempty" jsonschema:"all_visible (default) | assignee | agent | user | requester | team"`
	ActorID           string   `json:"actor_id,omitempty" jsonschema:"actor identifier; assignee values are exact strings such as agent:codex, user:<id>, @handle"`
	ActorIDs          []string `json:"actor_ids,omitempty" jsonschema:"team or multi-actor exact assignee identifiers"`
	IncludeUnassigned bool     `json:"include_unassigned,omitempty" jsonschema:"include Tasks with empty task_meta.assignee in actor-filtered views"`

	FlowScope string `json:"flow_scope,omitempty" jsonschema:"active (default) | all | ready | blocked"`
	AreaSlug  string `json:"area_slug,omitempty" jsonschema:"optional area slug filter"`
	Priority  string `json:"priority,omitempty" jsonschema:"optional p0 | p1 | p2 | p3"`
	Status    string `json:"status,omitempty" jsonschema:"optional pending | all | open | missing_status | missing | claimed_done | blocked | cancelled"`
	Limit     int    `json:"limit,omitempty" jsonschema:"default 100, max 500"`
}

type taskNextInput struct {
	ProjectSlug  string   `json:"project_slug,omitempty" jsonschema:"optional single projects.slug; omitted uses explicit session/default resolver unless project_scope=visible or project_slugs is set"`
	ProjectSlugs []string `json:"project_slugs,omitempty" jsonschema:"optional explicit visible project slug list"`
	ProjectScope string   `json:"project_scope,omitempty" jsonschema:"current (default) | list | visible | caller_visible"`

	ActorScope        string   `json:"actor_scope,omitempty" jsonschema:"assignee (default) | agent | user | requester | team | all_visible"`
	ActorID           string   `json:"actor_id,omitempty" jsonschema:"actor identifier; defaults to the calling agent for assignee/agent scope"`
	ActorIDs          []string `json:"actor_ids,omitempty" jsonschema:"team or multi-actor exact assignee identifiers"`
	IncludeUnassigned *bool    `json:"include_unassigned,omitempty" jsonschema:"default true; include unassigned ready Tasks as claim candidates"`

	AreaSlug string `json:"area_slug,omitempty" jsonschema:"optional area slug filter"`
	Priority string `json:"priority,omitempty" jsonschema:"optional p0 | p1 | p2 | p3"`
	Limit    int    `json:"limit,omitempty" jsonschema:"default 5, max 50"`
}

type taskFlowOutput struct {
	Mode              string        `json:"mode"`
	ProjectScope      string        `json:"project_scope"`
	ProjectSlugs      []string      `json:"project_slugs"`
	ActorScope        string        `json:"actor_scope"`
	ActorIDs          []string      `json:"actor_ids,omitempty"`
	IncludeUnassigned bool          `json:"include_unassigned,omitempty"`
	FlowScope         string        `json:"flow_scope"`
	Items             []taskFlowRow `json:"items"`
	Count             int           `json:"count"`
	Truncated         bool          `json:"truncated,omitempty"`
	Notice            string        `json:"notice"`
	ToolsetVersion    string        `json:"toolset_version,omitempty"`
}

type taskNextOutput struct {
	Mode              string                    `json:"mode"`
	ProjectScope      string                    `json:"project_scope"`
	ProjectSlugs      []string                  `json:"project_slugs"`
	ActorScope        string                    `json:"actor_scope"`
	ActorIDs          []string                  `json:"actor_ids,omitempty"`
	IncludeUnassigned bool                      `json:"include_unassigned"`
	Candidates        []taskNextCandidate       `json:"candidates"`
	ExcludedBlockers  []taskNextExcludedBlocker `json:"excluded_blockers"`
	BlockerSummary    string                    `json:"blocker_summary,omitempty"`
	NoReadyReason     string                    `json:"no_ready_reason,omitempty"`
	ClaimPolicy       taskNextClaimPolicy       `json:"claim_policy"`
	NextTools         []NextToolHint            `json:"next_tools,omitempty"`
	ToolsetVersion    string                    `json:"toolset_version,omitempty"`
}

type taskFlowRow struct {
	ProjectSlug string            `json:"project_slug"`
	ArtifactID  string            `json:"artifact_id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	AreaSlug    string            `json:"area_slug"`
	Status      string            `json:"status"`
	RawStatus   string            `json:"raw_status,omitempty"`
	Priority    string            `json:"priority,omitempty"`
	Assignee    string            `json:"assignee,omitempty"`
	DueAt       string            `json:"due_at,omitempty"`
	Stage       string            `json:"stage"`
	Ordinal     int               `json:"ordinal"`
	Readiness   string            `json:"readiness"`
	Blockers    []taskFlowBlocker `json:"blockers"`
	UpdatedAt   time.Time         `json:"updated_at"`
	AgentRef    string            `json:"agent_ref"`
	HumanURL    string            `json:"human_url"`
	HumanURLAbs string            `json:"human_url_abs"`
}

type taskFlowBlocker struct {
	ProjectSlug string `json:"project_slug"`
	ArtifactID  string `json:"artifact_id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    string `json:"priority,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	HumanURLAbs string `json:"human_url_abs"`
}

type taskNextCandidate struct {
	taskFlowRow
	SelectionReason string `json:"selection_reason"`
	ClaimRequired   bool   `json:"claim_required,omitempty"`
}

type taskNextExcludedBlocker struct {
	taskFlowRow
	BlockerCount int `json:"blocker_count"`
}

type taskNextClaimPolicy struct {
	Mode            string `json:"mode"`
	LeaseSupported  bool   `json:"lease_supported"`
	ClaimBeforeWork bool   `json:"claim_before_work"`
	Reason          string `json:"reason"`
	ClaimTool       string `json:"claim_tool,omitempty"`
}

type taskFlowProjectSelection struct {
	Scopes []auth.ProjectScope
	Mode   string
}

type taskFlowActorSelection struct {
	Scope             string
	IDs               []string
	IncludeUnassigned bool
}

// RegisterTaskFlow wires pindoc.task.flow. It is a derived read model for
// sequencing Tasks; task.queue remains the lifecycle count surface.
func RegisterTaskFlow(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.task.flow",
			Description: strings.TrimSpace(`
Return a derived Task sequence across one project, an explicit project list,
or every caller-visible project. This is not task.queue: task.queue reports
lifecycle counts, while task.flow computes readiness from blocks edges,
priority, and stable updated_at/slug ordering for agents and automation.
`),
		},
		func(ctx context.Context, p *auth.Principal, in taskFlowInput) (*sdk.CallToolResult, taskFlowOutput, error) {
			out, err := buildTaskFlow(ctx, deps, p, in)
			return nil, out, err
		},
	)
}

// RegisterTaskNext wires pindoc.task.next. It returns the next ready Task
// candidates for a specific actor without claiming or leasing them.
func RegisterTaskNext(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.task.next",
			Description: strings.TrimSpace(`
Return ready Task candidates for an actor using the same derived ordering as
task.flow. The tool is read-only and does not claim work; unassigned
candidates include a task.assign next-tool hint so automation workers can
claim immediately before executing.
`),
		},
		func(ctx context.Context, p *auth.Principal, in taskNextInput) (*sdk.CallToolResult, taskNextOutput, error) {
			out, err := buildTaskNext(ctx, deps, p, in)
			return nil, out, err
		},
	)
}

func buildTaskFlow(ctx context.Context, deps Deps, p *auth.Principal, in taskFlowInput) (taskFlowOutput, error) {
	projects, err := resolveTaskFlowProjects(ctx, deps, p, in.ProjectSlug, in.ProjectSlugs, in.ProjectScope)
	if err != nil {
		return taskFlowOutput{}, err
	}
	actor, err := normalizeTaskFlowActor(p, in.ActorScope, in.ActorID, in.ActorIDs, in.IncludeUnassigned, false)
	if err != nil {
		return taskFlowOutput{}, err
	}
	flowScope, err := normalizeTaskFlowScope(in.FlowScope)
	if err != nil {
		return taskFlowOutput{}, err
	}
	statusFilter, hasStatusFilter, err := normalizeTaskFlowStatusFilter(in.Status)
	if err != nil {
		return taskFlowOutput{}, err
	}
	priority, err := normalizeTaskFlowPriority(in.Priority)
	if err != nil {
		return taskFlowOutput{}, err
	}
	limit := clampTaskFlowLimit(in.Limit, taskFlowDefaultLimit, taskFlowMaxLimit)

	rows, err := loadTaskFlowRows(ctx, deps, projects.Scopes, actor, taskFlowQueryFilter{
		FlowScope:       flowScope,
		AreaSlug:        strings.TrimSpace(in.AreaSlug),
		Priority:        priority,
		StatusFilter:    statusFilter,
		HasStatusFilter: hasStatusFilter,
	}, limit)
	if err != nil {
		return taskFlowOutput{}, err
	}
	truncated := false
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}
	return taskFlowOutput{
		Mode:              "derived",
		ProjectScope:      projects.Mode,
		ProjectSlugs:      taskFlowProjectSlugs(projects.Scopes),
		ActorScope:        actor.Scope,
		ActorIDs:          actor.IDs,
		IncludeUnassigned: actor.IncludeUnassigned,
		FlowScope:         flowScope,
		Items:             rows,
		Count:             len(rows),
		Truncated:         truncated,
		Notice:            taskFlowNotice(),
	}, nil
}

func buildTaskNext(ctx context.Context, deps Deps, p *auth.Principal, in taskNextInput) (taskNextOutput, error) {
	includeUnassigned := true
	if in.IncludeUnassigned != nil {
		includeUnassigned = *in.IncludeUnassigned
	}
	projects, err := resolveTaskFlowProjects(ctx, deps, p, in.ProjectSlug, in.ProjectSlugs, in.ProjectScope)
	if err != nil {
		return taskNextOutput{}, err
	}
	actorScope := strings.TrimSpace(in.ActorScope)
	if actorScope == "" {
		actorScope = taskFlowActorAssignee
	}
	actor, err := normalizeTaskFlowActor(p, actorScope, in.ActorID, in.ActorIDs, includeUnassigned, true)
	if err != nil {
		return taskNextOutput{}, err
	}
	priority, err := normalizeTaskFlowPriority(in.Priority)
	if err != nil {
		return taskNextOutput{}, err
	}
	limit := clampTaskFlowLimit(in.Limit, 5, 50)

	rows, err := loadTaskFlowRows(ctx, deps, projects.Scopes, actor, taskFlowQueryFilter{
		FlowScope: taskFlowScopeActive,
		AreaSlug:  strings.TrimSpace(in.AreaSlug),
		Priority:  priority,
	}, taskFlowMaxLimit)
	if err != nil {
		return taskNextOutput{}, err
	}

	candidates := []taskNextCandidate{}
	excluded := []taskNextExcludedBlocker{}
	for _, row := range rows {
		switch row.Readiness {
		case taskReadinessReady:
			if len(candidates) >= limit {
				continue
			}
			candidates = append(candidates, taskNextCandidate{
				taskFlowRow:     row,
				SelectionReason: taskNextSelectionReason(row, actor),
				ClaimRequired:   taskNextClaimRequired(row, actor),
			})
		case taskReadinessBlocked, taskReadinessBlockedStatus:
			excluded = append(excluded, taskNextExcludedBlocker{
				taskFlowRow:  row,
				BlockerCount: len(row.Blockers),
			})
		}
	}

	out := taskNextOutput{
		Mode:              "derived",
		ProjectScope:      projects.Mode,
		ProjectSlugs:      taskFlowProjectSlugs(projects.Scopes),
		ActorScope:        actor.Scope,
		ActorIDs:          actor.IDs,
		IncludeUnassigned: actor.IncludeUnassigned,
		Candidates:        candidates,
		ExcludedBlockers:  excluded,
		BlockerSummary:    taskNextBlockerSummary(excluded),
		ClaimPolicy: taskNextClaimPolicy{
			Mode:            "read_only_claim_via_task_assign",
			LeaseSupported:  false,
			ClaimBeforeWork: true,
			Reason:          "task.next does not mutate or lease; automation workers should claim unassigned work with pindoc.task.assign immediately before executing.",
			ClaimTool:       "pindoc.task.assign",
		},
	}
	if len(out.Candidates) == 0 {
		out.NoReadyReason = "no ready Task matched the project, actor, and filter scope"
	}
	out.NextTools = taskNextTools(out, actor)
	return out, nil
}

type taskFlowQueryFilter struct {
	FlowScope       string
	AreaSlug        string
	Priority        string
	StatusFilter    string
	HasStatusFilter bool
}

func resolveTaskFlowProjects(ctx context.Context, deps Deps, p *auth.Principal, projectSlug string, projectSlugs []string, projectScope string) (taskFlowProjectSelection, error) {
	scope := strings.ToLower(strings.TrimSpace(projectScope))
	if scope == "caller_visible" || scope == "all_visible" {
		scope = taskFlowProjectScopeVisible
	}
	if len(normalizeAuditStringFilters(projectSlugs)) > 0 {
		scope = taskFlowProjectScopeList
	}
	if scope == "" {
		scope = taskFlowProjectScopeCurrent
	}

	var slugs []string
	switch scope {
	case taskFlowProjectScopeCurrent:
		resolved, err := auth.ResolveProject(ctx, deps.DB, p, projectSlug)
		if err != nil {
			return taskFlowProjectSelection{}, fmt.Errorf("task.flow project: %w", err)
		}
		return taskFlowProjectSelection{Scopes: []auth.ProjectScope{*resolved}, Mode: taskFlowProjectScopeCurrent}, nil
	case taskFlowProjectScopeList:
		slugs = normalizeAuditStringFilters(projectSlugs)
		if len(slugs) == 0 {
			if strings.TrimSpace(projectSlug) != "" {
				slugs = []string{strings.TrimSpace(projectSlug)}
			} else {
				return taskFlowProjectSelection{}, fmt.Errorf("project_slugs is required when project_scope=list")
			}
		}
	case taskFlowProjectScopeVisible:
		visible, err := visibleProjectSlugs(ctx, deps, p)
		if err != nil {
			return taskFlowProjectSelection{}, fmt.Errorf("task.flow visible projects: %w", err)
		}
		slugs = visible
	default:
		return taskFlowProjectSelection{}, fmt.Errorf("project_scope must be one of: current | list | visible | caller_visible")
	}

	out := make([]auth.ProjectScope, 0, len(slugs))
	for _, slug := range slugs {
		resolved, err := auth.ResolveProject(ctx, deps.DB, p, slug)
		if err != nil {
			return taskFlowProjectSelection{}, fmt.Errorf("task.flow project %q: %w", slug, err)
		}
		out = append(out, *resolved)
	}
	return taskFlowProjectSelection{Scopes: out, Mode: scope}, nil
}

func normalizeTaskFlowActor(p *auth.Principal, actorScope, actorID string, actorIDs []string, includeUnassigned bool, requireActor bool) (taskFlowActorSelection, error) {
	scope := strings.ToLower(strings.TrimSpace(actorScope))
	if scope == "" {
		scope = taskFlowActorAllVisible
	}
	if scope == "all" {
		scope = taskFlowActorAllVisible
	}
	switch scope {
	case taskFlowActorAllVisible, taskFlowActorAssignee, taskFlowActorAgent, taskFlowActorUser, taskFlowActorRequester, taskFlowActorTeam:
	default:
		return taskFlowActorSelection{}, fmt.Errorf("actor_scope must be one of: all_visible | assignee | agent | user | requester | team")
	}

	ids := normalizeAuditStringFilters(append([]string{actorID}, actorIDs...))
	if len(ids) == 0 {
		switch scope {
		case taskFlowActorAssignee, taskFlowActorAgent:
			if caller := callerAgentAssignee(p); caller != "" {
				ids = []string{caller}
			}
		case taskFlowActorUser, taskFlowActorRequester:
			if p != nil && strings.TrimSpace(p.UserID) != "" {
				ids = []string{"user:" + strings.TrimSpace(p.UserID)}
			}
		}
	}
	for i, id := range ids {
		ids[i] = normalizeTaskFlowActorID(scope, id)
	}
	if requireActor && scope != taskFlowActorAllVisible && len(ids) == 0 {
		return taskFlowActorSelection{}, fmt.Errorf("actor_id is required for actor_scope=%s when caller identity cannot supply one", scope)
	}
	return taskFlowActorSelection{Scope: scope, IDs: ids, IncludeUnassigned: includeUnassigned}, nil
}

func normalizeTaskFlowActorID(scope, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	switch scope {
	case taskFlowActorAgent:
		if !strings.HasPrefix(id, "agent:") {
			return "agent:" + stripAgentPrefix(id)
		}
	case taskFlowActorUser, taskFlowActorRequester:
		if !strings.HasPrefix(id, "user:") && !strings.HasPrefix(id, "@") {
			return "user:" + id
		}
	}
	return id
}

func normalizeTaskFlowScope(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		return taskFlowScopeActive, nil
	}
	switch scope {
	case taskFlowScopeActive, taskFlowScopeAll, taskFlowScopeReady, taskFlowScopeBlocked:
		return scope, nil
	default:
		return "", fmt.Errorf("flow_scope must be one of: active | all | ready | blocked")
	}
}

func normalizeTaskFlowStatusFilter(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	status, ok := normalizeTaskQueueStatusFilter(raw)
	if !ok {
		return "", false, fmt.Errorf("status must be one of: pending | all | open | missing_status | missing | claimed_done | blocked | cancelled")
	}
	return status, true, nil
}

func normalizeTaskFlowPriority(raw string) (string, error) {
	priority := strings.ToLower(strings.TrimSpace(raw))
	if priority == "" {
		return "", nil
	}
	if _, ok := validTaskPriorities[priority]; !ok {
		return "", fmt.Errorf("priority must be one of: p0 | p1 | p2 | p3")
	}
	return priority, nil
}

func clampTaskFlowLimit(raw, defaultValue, maxValue int) int {
	if raw <= 0 {
		return defaultValue
	}
	if raw > maxValue {
		return maxValue
	}
	return raw
}

func loadTaskFlowRows(ctx context.Context, deps Deps, scopes []auth.ProjectScope, actor taskFlowActorSelection, filter taskFlowQueryFilter, limit int) ([]taskFlowRow, error) {
	rows := []taskFlowRow{}
	for _, scope := range scopes {
		projectRows, err := loadTaskFlowProjectRows(ctx, deps, scope, actor, filter)
		if err != nil {
			return nil, err
		}
		rows = append(rows, projectRows...)
	}
	sortTaskFlowRows(rows)
	for i := range rows {
		rows[i].Ordinal = i + 1
	}
	if limit > 0 && len(rows) > limit+1 {
		return rows[:limit+1], nil
	}
	return rows, nil
}

func loadTaskFlowProjectRows(ctx context.Context, deps Deps, scope auth.ProjectScope, actor taskFlowActorSelection, filter taskFlowQueryFilter) ([]taskFlowRow, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, ar.slug, a.updated_at,
		       COALESCE(a.task_meta->>'status', ''),
		       COALESCE(a.task_meta->>'priority', ''),
		       COALESCE(a.task_meta->>'assignee', ''),
		       COALESCE(a.task_meta->>'due_at', '')
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
	`, scope.ProjectSlug, filter.AreaSlug, filter.Priority)
	if err != nil {
		return nil, fmt.Errorf("task.flow query %s: %w", scope.ProjectSlug, err)
	}
	defer rows.Close()

	projectRows := []taskFlowRow{}
	for rows.Next() {
		var row taskFlowRow
		var rawStatus string
		if err := rows.Scan(
			&row.ArtifactID, &row.Slug, &row.Title, &row.AreaSlug, &row.UpdatedAt,
			&rawStatus, &row.Priority, &row.Assignee, &row.DueAt,
		); err != nil {
			return nil, fmt.Errorf("task.flow scan %s: %w", scope.ProjectSlug, err)
		}
		if !taskFlowActorMatches(row.Assignee, actor) {
			continue
		}
		bucket := taskStatusBucket(rawStatus)
		if filter.HasStatusFilter && !taskQueueStatusMatches(rawStatus, filter.StatusFilter) {
			continue
		}
		row.ProjectSlug = scope.ProjectSlug
		row.Status = bucket
		if bucket == taskStatusOther {
			row.RawStatus = strings.TrimSpace(rawStatus)
		}
		row.AgentRef = "pindoc://" + row.Slug
		row.HumanURL = HumanURL(scope.ProjectSlug, scope.ProjectLocale, row.Slug)
		row.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, row.Slug)
		projectRows = append(projectRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("task.flow rows %s: %w", scope.ProjectSlug, err)
	}

	blockers, err := loadTaskFlowBlockers(ctx, deps, scope, taskFlowArtifactIDs(projectRows))
	if err != nil {
		return nil, err
	}
	out := make([]taskFlowRow, 0, len(projectRows))
	for _, row := range projectRows {
		row.Blockers = blockers[row.ArtifactID]
		if row.Blockers == nil {
			row.Blockers = []taskFlowBlocker{}
		}
		row.Readiness = taskFlowReadiness(row.Status, row.Blockers)
		row.Stage = taskFlowStage(row.Readiness)
		if !taskFlowScopeMatches(row, filter.FlowScope) {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func loadTaskFlowBlockers(ctx context.Context, deps Deps, scope auth.ProjectScope, taskIDs []string) (map[string][]taskFlowBlocker, error) {
	out := map[string][]taskFlowBlocker{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT
			e.target_id::text,
			b.id::text,
			b.slug,
			b.title,
			COALESCE(b.task_meta->>'status', ''),
			COALESCE(b.task_meta->>'priority', ''),
			COALESCE(b.task_meta->>'assignee', '')
		FROM artifact_edges e
		JOIN artifacts b ON b.id = e.source_id
		WHERE e.target_id::text = ANY($1::text[])
		  AND e.relation = 'blocks'
		  AND b.type = 'Task'
		  AND b.status <> 'archived'
		  AND b.status <> 'superseded'
		ORDER BY b.slug
	`, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("task.flow blockers %s: %w", scope.ProjectSlug, err)
	}
	defer rows.Close()
	for rows.Next() {
		var targetID, rawStatus string
		var blocker taskFlowBlocker
		if err := rows.Scan(&targetID, &blocker.ArtifactID, &blocker.Slug, &blocker.Title, &rawStatus, &blocker.Priority, &blocker.Assignee); err != nil {
			return nil, fmt.Errorf("task.flow blockers scan %s: %w", scope.ProjectSlug, err)
		}
		blocker.ProjectSlug = scope.ProjectSlug
		blocker.Status = taskStatusBucket(rawStatus)
		if blocker.Status == "claimed_done" || blocker.Status == "cancelled" {
			continue
		}
		blocker.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, blocker.Slug)
		out[targetID] = append(out[targetID], blocker)
	}
	return out, rows.Err()
}

func taskFlowArtifactIDs(rows []taskFlowRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ArtifactID)
	}
	return out
}

func taskFlowActorMatches(assignee string, actor taskFlowActorSelection) bool {
	assignee = strings.TrimSpace(assignee)
	if actor.Scope == taskFlowActorAllVisible {
		return true
	}
	if assignee == "" {
		return actor.IncludeUnassigned
	}
	for _, id := range actor.IDs {
		if assignee == id {
			return true
		}
	}
	return false
}

func taskFlowReadiness(status string, blockers []taskFlowBlocker) string {
	switch status {
	case "claimed_done", "cancelled":
		return taskReadinessDone
	case "blocked":
		return taskReadinessBlockedStatus
	case "open", taskStatusMissing:
		if len(blockers) > 0 {
			return taskReadinessBlocked
		}
		return taskReadinessReady
	default:
		return taskReadinessOther
	}
}

func taskFlowStage(readiness string) string {
	switch readiness {
	case taskReadinessReady:
		return "ready"
	case taskReadinessBlocked, taskReadinessBlockedStatus:
		return "blocked"
	case taskReadinessDone:
		return "done"
	default:
		return "other"
	}
}

func taskFlowScopeMatches(row taskFlowRow, flowScope string) bool {
	switch flowScope {
	case taskFlowScopeAll:
		return true
	case taskFlowScopeReady:
		return row.Readiness == taskReadinessReady
	case taskFlowScopeBlocked:
		return row.Readiness == taskReadinessBlocked || row.Readiness == taskReadinessBlockedStatus
	default:
		return row.Status == "open" || row.Status == taskStatusMissing || row.Status == "blocked"
	}
}

func sortTaskFlowRows(rows []taskFlowRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if ra, rb := taskFlowReadinessRank(a.Readiness), taskFlowReadinessRank(b.Readiness); ra != rb {
			return ra < rb
		}
		if pa, pb := taskFlowPriorityRank(a.Priority), taskFlowPriorityRank(b.Priority); pa != pb {
			return pa < pb
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.Before(b.UpdatedAt)
		}
		if a.ProjectSlug != b.ProjectSlug {
			return a.ProjectSlug < b.ProjectSlug
		}
		return a.Slug < b.Slug
	})
}

func taskFlowReadinessRank(readiness string) int {
	switch readiness {
	case taskReadinessReady:
		return 0
	case taskReadinessBlocked:
		return 1
	case taskReadinessBlockedStatus:
		return 2
	case taskReadinessOther:
		return 3
	case taskReadinessDone:
		return 4
	default:
		return 5
	}
}

func taskFlowPriorityRank(priority string) int {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "p0":
		return 0
	case "p1":
		return 1
	case "p2":
		return 2
	case "p3":
		return 3
	default:
		return 4
	}
}

func taskFlowProjectSlugs(scopes []auth.ProjectScope) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, scope.ProjectSlug)
	}
	return normalizedSlugList(out)
}

func taskNextSelectionReason(row taskFlowRow, actor taskFlowActorSelection) string {
	if strings.TrimSpace(row.Assignee) == "" {
		return "ready unassigned Task included for actor claim; ordered by dependency, priority, updated_at, slug"
	}
	for _, id := range actor.IDs {
		if row.Assignee == id {
			return "ready Task assigned to actor; ordered by dependency, priority, updated_at, slug"
		}
	}
	return "ready Task matched actor scope; ordered by dependency, priority, updated_at, slug"
}

func taskNextClaimRequired(row taskFlowRow, actor taskFlowActorSelection) bool {
	if strings.TrimSpace(row.Assignee) == "" {
		return true
	}
	for _, id := range actor.IDs {
		if row.Assignee == id {
			return false
		}
	}
	return actor.Scope != taskFlowActorAllVisible
}

func taskNextBlockerSummary(excluded []taskNextExcludedBlocker) string {
	if len(excluded) == 0 {
		return ""
	}
	blockerCount := 0
	for _, row := range excluded {
		blockerCount += row.BlockerCount
	}
	return fmt.Sprintf("%d Task(s) excluded because %d blocker(s) or blocked status remain", len(excluded), blockerCount)
}

func taskNextTools(out taskNextOutput, actor taskFlowActorSelection) []NextToolHint {
	if len(out.Candidates) == 0 {
		return nil
	}
	first := out.Candidates[0]
	tools := []NextToolHint{{
		Tool: "pindoc.artifact.read",
		Args: map[string]any{
			"project_slug": first.ProjectSlug,
			"id_or_slug":   "pindoc://" + first.Slug,
			"view":         "full",
		},
		Reason: "read the selected Task before execution",
	}}
	if first.ClaimRequired {
		assignee := ""
		if len(actor.IDs) > 0 {
			assignee = actor.IDs[0]
		}
		if assignee != "" {
			tools = append([]NextToolHint{{
				Tool: "pindoc.task.assign",
				Args: map[string]any{
					"project_slug": first.ProjectSlug,
					"slug_or_id":   "pindoc://" + first.Slug,
					"assignee":     assignee,
					"reason":       "claim ready Task returned by pindoc.task.next",
				},
				Reason: "claim unassigned ready work before an automation worker starts",
			}}, tools...)
		}
	}
	return tools
}

func taskFlowNotice() string {
	return "task.flow is a derived sequence read model. task.queue remains the lifecycle count surface; task.next is read-only and uses task.assign for explicit claims instead of leases."
}
