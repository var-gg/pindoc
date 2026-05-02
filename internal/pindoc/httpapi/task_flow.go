package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
)

const (
	taskFlowHTTPDefaultLimit = 100
	taskFlowHTTPMaxLimit     = 500

	taskFlowHTTPProjectCurrent = "current"
	taskFlowHTTPProjectList    = "list"
	taskFlowHTTPProjectVisible = "visible"

	taskFlowHTTPActorAllVisible = "all_visible"
	taskFlowHTTPActorAssignee   = "assignee"
	taskFlowHTTPActorAgent      = "agent"
	taskFlowHTTPActorUser       = "user"
	taskFlowHTTPActorRequester  = "requester"
	taskFlowHTTPActorTeam       = "team"

	taskFlowHTTPFlowActive  = "active"
	taskFlowHTTPFlowAll     = "all"
	taskFlowHTTPFlowReady   = "ready"
	taskFlowHTTPFlowBlocked = "blocked"

	taskFlowHTTPReadinessReady         = "ready"
	taskFlowHTTPReadinessBlocked       = "blocked"
	taskFlowHTTPReadinessBlockedStatus = "blocked_status"
	taskFlowHTTPReadinessDone          = "done"
	taskFlowHTTPReadinessOther         = "other"

	taskFlowHTTPStatusMissing = "missing_status"
	taskFlowHTTPStatusOther   = "other"
)

var errTaskFlowHiddenProject = errors.New("task-flow: hidden project")

type taskFlowHTTPRequest struct {
	ProjectSlug       string
	ProjectSlugs      []string
	ProjectScope      string
	ActorScope        string
	ActorID           string
	ActorIDs          []string
	IncludeUnassigned bool
	FlowScope         string
	AreaSlug          string
	Priority          string
	Status            string
	Limit             int
	IncludeHidden     bool
}

type taskFlowHTTPOutput struct {
	Mode              string            `json:"mode"`
	ProjectScope      string            `json:"project_scope"`
	ProjectSlugs      []string          `json:"project_slugs"`
	ActorScope        string            `json:"actor_scope"`
	ActorIDs          []string          `json:"actor_ids,omitempty"`
	IncludeUnassigned bool              `json:"include_unassigned,omitempty"`
	FlowScope         string            `json:"flow_scope"`
	Items             []taskFlowHTTPRow `json:"items"`
	Count             int               `json:"count"`
	Truncated         bool              `json:"truncated,omitempty"`
	Notice            string            `json:"notice"`
}

type taskFlowHTTPRow struct {
	ProjectSlug string                `json:"project_slug"`
	ArtifactID  string                `json:"artifact_id"`
	Slug        string                `json:"slug"`
	Title       string                `json:"title"`
	AreaSlug    string                `json:"area_slug"`
	Status      string                `json:"status"`
	RawStatus   string                `json:"raw_status,omitempty"`
	Priority    string                `json:"priority,omitempty"`
	Assignee    string                `json:"assignee,omitempty"`
	DueAt       string                `json:"due_at,omitempty"`
	Stage       string                `json:"stage"`
	Ordinal     int                   `json:"ordinal"`
	Readiness   string                `json:"readiness"`
	Blockers    []taskFlowHTTPBlocker `json:"blockers"`
	UpdatedAt   time.Time             `json:"updated_at"`
	AgentRef    string                `json:"agent_ref"`
	HumanURL    string                `json:"human_url"`
	HumanURLAbs string                `json:"human_url_abs,omitempty"`
}

type taskFlowHTTPBlocker struct {
	ProjectSlug string `json:"project_slug"`
	ArtifactID  string `json:"artifact_id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    string `json:"priority,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

type taskFlowHTTPProjectSelection struct {
	Scopes []pauth.ProjectScope
	Mode   string
}

type taskFlowHTTPActorSelection struct {
	Scope             string
	IDs               []string
	IncludeUnassigned bool
}

type taskFlowHTTPQueryFilter struct {
	FlowScope       string
	AreaSlug        string
	Priority        string
	StatusFilter    string
	HasStatusFilter bool
}

type taskFlowHTTPError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func (d Deps) handleTaskFlow(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeTaskFlowHTTPError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	principal := d.principalForRequest(r)
	if principal == nil {
		writeTaskFlowHTTPError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authenticated reader principal is required")
		return
	}

	pathProject := projectSlugFrom(r)
	in, err := parseTaskFlowHTTPRequest(pathProject, r.URL.Query(), includeReaderHiddenProjects(r))
	if err != nil {
		writeTaskFlowHTTPError(w, http.StatusBadRequest, "BAD_QUERY", err.Error())
		return
	}
	projects, err := d.resolveTaskFlowHTTPProjects(r.Context(), principal, in)
	if err != nil {
		writeTaskFlowResolveError(w, err)
		return
	}
	actor, err := normalizeTaskFlowHTTPActor(principal, in.ActorScope, in.ActorID, in.ActorIDs, in.IncludeUnassigned, false)
	if err != nil {
		writeTaskFlowHTTPError(w, http.StatusBadRequest, "BAD_ACTOR", err.Error())
		return
	}
	flowScope, err := normalizeTaskFlowHTTPScope(in.FlowScope)
	if err != nil {
		writeTaskFlowHTTPError(w, http.StatusBadRequest, "BAD_FLOW_SCOPE", err.Error())
		return
	}
	statusFilter, hasStatusFilter, err := normalizeTaskFlowHTTPStatusFilter(in.Status)
	if err != nil {
		writeTaskFlowHTTPError(w, http.StatusBadRequest, "BAD_STATUS", err.Error())
		return
	}
	priority, err := normalizeTaskFlowHTTPPriority(in.Priority)
	if err != nil {
		writeTaskFlowHTTPError(w, http.StatusBadRequest, "BAD_PRIORITY", err.Error())
		return
	}

	limit := clampTaskFlowHTTPLimit(in.Limit, taskFlowHTTPDefaultLimit, taskFlowHTTPMaxLimit)
	rows, err := d.loadTaskFlowHTTPRows(r.Context(), projects.Scopes, actor, taskFlowHTTPQueryFilter{
		FlowScope:       flowScope,
		AreaSlug:        strings.TrimSpace(in.AreaSlug),
		Priority:        priority,
		StatusFilter:    statusFilter,
		HasStatusFilter: hasStatusFilter,
	}, limit)
	if err != nil {
		d.Logger.Error("task flow query", "err", err)
		writeTaskFlowHTTPError(w, http.StatusInternalServerError, "QUERY_FAILED", "task flow query failed")
		return
	}
	truncated := false
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}

	writeJSON(w, http.StatusOK, taskFlowHTTPOutput{
		Mode:              "derived",
		ProjectScope:      projects.Mode,
		ProjectSlugs:      taskFlowHTTPProjectSlugs(projects.Scopes),
		ActorScope:        actor.Scope,
		ActorIDs:          actor.IDs,
		IncludeUnassigned: actor.IncludeUnassigned,
		FlowScope:         flowScope,
		Items:             rows,
		Count:             len(rows),
		Truncated:         truncated,
		Notice:            taskFlowHTTPNotice(),
	})
}

func parseTaskFlowHTTPRequest(pathProject string, q url.Values, includeHidden bool) (taskFlowHTTPRequest, error) {
	limit := 0
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return taskFlowHTTPRequest{}, fmt.Errorf("limit must be an integer")
		}
		limit = parsed
	}
	return taskFlowHTTPRequest{
		ProjectSlug:       firstNonEmpty(q.Get("project_slug"), pathProject),
		ProjectSlugs:      taskFlowHTTPStringList(q["project_slugs"]),
		ProjectScope:      q.Get("project_scope"),
		ActorScope:        q.Get("actor_scope"),
		ActorID:           q.Get("actor_id"),
		ActorIDs:          taskFlowHTTPStringList(q["actor_ids"]),
		IncludeUnassigned: parseBoolQuery(q.Get("include_unassigned")),
		FlowScope:         q.Get("flow_scope"),
		AreaSlug:          q.Get("area_slug"),
		Priority:          q.Get("priority"),
		Status:            q.Get("status"),
		Limit:             limit,
		IncludeHidden:     includeHidden,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeTaskFlowHTTPError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, taskFlowHTTPError{ErrorCode: code, Message: msg})
}

func writeTaskFlowResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errTaskFlowHiddenProject), errors.Is(err, pauth.ErrProjectNotFound):
		writeTaskFlowHTTPError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found")
	case errors.Is(err, pauth.ErrProjectSlugRequired):
		writeTaskFlowHTTPError(w, http.StatusBadRequest, "PROJECT_SLUG_REQUIRED", "project slug is required")
	case errors.Is(err, pauth.ErrProjectAccessDenied):
		writeTaskFlowHTTPError(w, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "project access denied")
	default:
		writeTaskFlowHTTPError(w, http.StatusInternalServerError, "PROJECT_RESOLVE_FAILED", "project resolution failed")
	}
}

func (d Deps) resolveTaskFlowHTTPProjects(ctx context.Context, principal *pauth.Principal, in taskFlowHTTPRequest) (taskFlowHTTPProjectSelection, error) {
	scope := strings.ToLower(strings.TrimSpace(in.ProjectScope))
	if scope == "caller_visible" || scope == "all_visible" {
		scope = taskFlowHTTPProjectVisible
	}
	if len(in.ProjectSlugs) > 0 {
		scope = taskFlowHTTPProjectList
	}
	if scope == "" {
		scope = taskFlowHTTPProjectCurrent
	}

	var slugs []string
	switch scope {
	case taskFlowHTTPProjectCurrent:
		resolved, err := d.resolveTaskFlowHTTPScope(ctx, principal, in.ProjectSlug, in.IncludeHidden)
		if err != nil {
			return taskFlowHTTPProjectSelection{}, err
		}
		return taskFlowHTTPProjectSelection{Scopes: []pauth.ProjectScope{*resolved}, Mode: taskFlowHTTPProjectCurrent}, nil
	case taskFlowHTTPProjectList:
		slugs = in.ProjectSlugs
		if len(slugs) == 0 && strings.TrimSpace(in.ProjectSlug) != "" {
			slugs = []string{strings.TrimSpace(in.ProjectSlug)}
		}
		if len(slugs) == 0 {
			return taskFlowHTTPProjectSelection{}, fmt.Errorf("%w: project_slugs", pauth.ErrProjectSlugRequired)
		}
	case taskFlowHTTPProjectVisible:
		visible, err := d.visibleTaskFlowHTTPScopes(ctx, principal, in.IncludeHidden)
		if err != nil {
			return taskFlowHTTPProjectSelection{}, err
		}
		return taskFlowHTTPProjectSelection{Scopes: visible, Mode: taskFlowHTTPProjectVisible}, nil
	default:
		return taskFlowHTTPProjectSelection{}, fmt.Errorf("project_scope must be current | list | visible | caller_visible")
	}

	out := make([]pauth.ProjectScope, 0, len(slugs))
	for _, slug := range slugs {
		resolved, err := d.resolveTaskFlowHTTPScope(ctx, principal, slug, in.IncludeHidden)
		if err != nil {
			return taskFlowHTTPProjectSelection{}, err
		}
		out = append(out, *resolved)
	}
	return taskFlowHTTPProjectSelection{Scopes: out, Mode: scope}, nil
}

func (d Deps) resolveTaskFlowHTTPScope(ctx context.Context, principal *pauth.Principal, slug string, includeHidden bool) (*pauth.ProjectScope, error) {
	slug = strings.TrimSpace(slug)
	if slug != "" && readerHiddenProjectSlug(slug) && !includeHidden {
		return nil, errTaskFlowHiddenProject
	}
	scope, err := pauth.ResolveProject(ctx, d.DB, principal, slug)
	if err != nil {
		return nil, err
	}
	if !scope.Can("read.artifact") {
		return nil, fmt.Errorf("%w: %q", pauth.ErrProjectAccessDenied, slug)
	}
	return scope, nil
}

func (d Deps) visibleTaskFlowHTTPScopes(ctx context.Context, principal *pauth.Principal, includeHidden bool) ([]pauth.ProjectScope, error) {
	rows, err := d.DB.Query(ctx, `SELECT slug FROM projects ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []pauth.ProjectScope{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		if readerHiddenProjectSlug(slug) && !includeHidden {
			continue
		}
		scope, err := d.resolveTaskFlowHTTPScope(ctx, principal, slug, includeHidden)
		if err == nil {
			out = append(out, *scope)
			continue
		}
		if !errors.Is(err, pauth.ErrProjectAccessDenied) && !errors.Is(err, pauth.ErrProjectNotFound) {
			return nil, err
		}
	}
	return out, rows.Err()
}

func normalizeTaskFlowHTTPActor(principal *pauth.Principal, actorScope, actorID string, actorIDs []string, includeUnassigned bool, requireActor bool) (taskFlowHTTPActorSelection, error) {
	scope := strings.ToLower(strings.TrimSpace(actorScope))
	if scope == "" || scope == "all" {
		scope = taskFlowHTTPActorAllVisible
	}
	switch scope {
	case taskFlowHTTPActorAllVisible, taskFlowHTTPActorAssignee, taskFlowHTTPActorAgent, taskFlowHTTPActorUser, taskFlowHTTPActorRequester, taskFlowHTTPActorTeam:
	default:
		return taskFlowHTTPActorSelection{}, fmt.Errorf("actor_scope must be all_visible | assignee | agent | user | requester | team")
	}

	ids := taskFlowHTTPStringList(append([]string{actorID}, actorIDs...))
	if len(ids) == 0 {
		switch scope {
		case taskFlowHTTPActorAssignee, taskFlowHTTPActorAgent:
			if caller := taskFlowHTTPCallerAgentAssignee(principal); caller != "" {
				ids = []string{caller}
			}
		case taskFlowHTTPActorUser, taskFlowHTTPActorRequester:
			if principal != nil && strings.TrimSpace(principal.UserID) != "" {
				ids = []string{"user:" + strings.TrimSpace(principal.UserID)}
			}
		}
	}
	for i, id := range ids {
		ids[i] = normalizeTaskFlowHTTPActorID(scope, id)
	}
	if requireActor && scope != taskFlowHTTPActorAllVisible && len(ids) == 0 {
		return taskFlowHTTPActorSelection{}, fmt.Errorf("actor_id is required for actor_scope=%s", scope)
	}
	return taskFlowHTTPActorSelection{Scope: scope, IDs: ids, IncludeUnassigned: includeUnassigned}, nil
}

func normalizeTaskFlowHTTPActorID(scope, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	switch scope {
	case taskFlowHTTPActorAgent:
		if !strings.HasPrefix(id, "agent:") {
			return "agent:" + strings.TrimPrefix(id, "agent:")
		}
	case taskFlowHTTPActorUser, taskFlowHTTPActorRequester:
		if !strings.HasPrefix(id, "user:") && !strings.HasPrefix(id, "@") {
			return "user:" + id
		}
	}
	return id
}

func taskFlowHTTPCallerAgentAssignee(principal *pauth.Principal) string {
	if principal == nil {
		return ""
	}
	caller := strings.TrimSpace(principal.AgentID)
	if caller == "" || strings.HasPrefix(caller, "user:") || strings.HasPrefix(caller, "@") {
		return ""
	}
	return "agent:" + strings.TrimPrefix(caller, "agent:")
}

func normalizeTaskFlowHTTPScope(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		return taskFlowHTTPFlowActive, nil
	}
	switch scope {
	case taskFlowHTTPFlowActive, taskFlowHTTPFlowAll, taskFlowHTTPFlowReady, taskFlowHTTPFlowBlocked:
		return scope, nil
	default:
		return "", fmt.Errorf("flow_scope must be active | all | ready | blocked")
	}
}

func normalizeTaskFlowHTTPStatusFilter(raw string) (string, bool, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	if status == "" {
		return "", false, nil
	}
	switch status {
	case "pending", "all", "open", taskFlowHTTPStatusMissing, "missing", "claimed_done", "blocked", "cancelled":
		if status == "missing" {
			status = taskFlowHTTPStatusMissing
		}
		return status, true, nil
	default:
		return "", false, fmt.Errorf("status must be pending | all | open | missing_status | missing | claimed_done | blocked | cancelled")
	}
}

func normalizeTaskFlowHTTPPriority(raw string) (string, error) {
	priority := strings.ToLower(strings.TrimSpace(raw))
	if priority == "" {
		return "", nil
	}
	if _, ok := validTaskPriorities[priority]; !ok {
		return "", fmt.Errorf("priority must be p0 | p1 | p2 | p3")
	}
	return priority, nil
}

func clampTaskFlowHTTPLimit(raw, defaultValue, maxValue int) int {
	if raw <= 0 {
		return defaultValue
	}
	if raw > maxValue {
		return maxValue
	}
	return raw
}

func (d Deps) loadTaskFlowHTTPRows(ctx context.Context, scopes []pauth.ProjectScope, actor taskFlowHTTPActorSelection, filter taskFlowHTTPQueryFilter, limit int) ([]taskFlowHTTPRow, error) {
	rows := []taskFlowHTTPRow{}
	for _, scope := range scopes {
		projectRows, err := d.loadTaskFlowHTTPProjectRows(ctx, scope, actor, filter)
		if err != nil {
			return nil, err
		}
		rows = append(rows, projectRows...)
	}
	sortTaskFlowHTTPRows(rows)
	for i := range rows {
		rows[i].Ordinal = i + 1
	}
	if limit > 0 && len(rows) > limit+1 {
		return rows[:limit+1], nil
	}
	return rows, nil
}

func (d Deps) loadTaskFlowHTTPProjectRows(ctx context.Context, scope pauth.ProjectScope, actor taskFlowHTTPActorSelection, filter taskFlowHTTPQueryFilter) ([]taskFlowHTTPRow, error) {
	rows, err := d.DB.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, ar.slug, a.updated_at,
		       COALESCE(a.task_meta->>'status', ''),
		       COALESCE(a.task_meta->>'priority', ''),
		       COALESCE(a.task_meta->>'assignee', ''),
		       COALESCE(a.task_meta->>'due_at', '')
		  FROM artifacts a
		  JOIN areas ar ON ar.id = a.area_id
		 WHERE a.project_id = $1::uuid
		   AND a.type = 'Task'
		   AND a.status <> 'archived'
		   AND a.status <> 'superseded'
		   AND NOT starts_with(a.slug, '_template_')
		   AND ($2::text = '' OR ar.slug = $2)
		   AND ($3::text = '' OR a.task_meta->>'priority' = $3)
	`, scope.ProjectID, filter.AreaSlug, filter.Priority)
	if err != nil {
		return nil, fmt.Errorf("task flow project query %s: %w", scope.ProjectSlug, err)
	}
	defer rows.Close()

	projectRows := []taskFlowHTTPRow{}
	for rows.Next() {
		var row taskFlowHTTPRow
		var rawStatus string
		if err := rows.Scan(
			&row.ArtifactID, &row.Slug, &row.Title, &row.AreaSlug, &row.UpdatedAt,
			&rawStatus, &row.Priority, &row.Assignee, &row.DueAt,
		); err != nil {
			return nil, fmt.Errorf("task flow project scan %s: %w", scope.ProjectSlug, err)
		}
		if !taskFlowHTTPActorMatches(row.Assignee, actor) {
			continue
		}
		bucket := taskFlowHTTPStatusBucket(rawStatus)
		if filter.HasStatusFilter && !taskFlowHTTPStatusMatches(rawStatus, filter.StatusFilter) {
			continue
		}
		row.ProjectSlug = scope.ProjectSlug
		row.Status = bucket
		if bucket == taskFlowHTTPStatusOther {
			row.RawStatus = strings.TrimSpace(rawStatus)
		}
		row.AgentRef = "pindoc://" + row.Slug
		row.HumanURL = httpArtifactURL(scope.ProjectSlug, row.Slug)
		row.HumanURLAbs = d.httpArtifactURLAbs(scope.ProjectSlug, row.Slug)
		projectRows = append(projectRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	blockers, err := d.loadTaskFlowHTTPBlockers(ctx, scope, taskFlowHTTPArtifactIDs(projectRows))
	if err != nil {
		return nil, err
	}
	out := make([]taskFlowHTTPRow, 0, len(projectRows))
	for _, row := range projectRows {
		row.Blockers = blockers[row.ArtifactID]
		if row.Blockers == nil {
			row.Blockers = []taskFlowHTTPBlocker{}
		}
		row.Readiness = taskFlowHTTPReadiness(row.Status, row.Blockers)
		row.Stage = taskFlowHTTPStage(row.Readiness)
		if !taskFlowHTTPScopeMatches(row, filter.FlowScope) {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (d Deps) loadTaskFlowHTTPBlockers(ctx context.Context, scope pauth.ProjectScope, taskIDs []string) (map[string][]taskFlowHTTPBlocker, error) {
	out := map[string][]taskFlowHTTPBlocker{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	rows, err := d.DB.Query(ctx, `
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
		   AND b.project_id = $2::uuid
		   AND b.type = 'Task'
		   AND b.status <> 'archived'
		   AND b.status <> 'superseded'
		 ORDER BY b.slug
	`, taskIDs, scope.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("task flow blockers %s: %w", scope.ProjectSlug, err)
	}
	defer rows.Close()
	for rows.Next() {
		var targetID, rawStatus string
		var blocker taskFlowHTTPBlocker
		if err := rows.Scan(&targetID, &blocker.ArtifactID, &blocker.Slug, &blocker.Title, &rawStatus, &blocker.Priority, &blocker.Assignee); err != nil {
			return nil, fmt.Errorf("task flow blockers scan %s: %w", scope.ProjectSlug, err)
		}
		blocker.ProjectSlug = scope.ProjectSlug
		blocker.Status = taskFlowHTTPStatusBucket(rawStatus)
		if blocker.Status == "claimed_done" || blocker.Status == "cancelled" {
			continue
		}
		blocker.HumanURLAbs = d.httpArtifactURLAbs(scope.ProjectSlug, blocker.Slug)
		out[targetID] = append(out[targetID], blocker)
	}
	return out, rows.Err()
}

func taskFlowHTTPArtifactIDs(rows []taskFlowHTTPRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ArtifactID)
	}
	return out
}

func taskFlowHTTPActorMatches(assignee string, actor taskFlowHTTPActorSelection) bool {
	assignee = strings.TrimSpace(assignee)
	if actor.Scope == taskFlowHTTPActorAllVisible {
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

func taskFlowHTTPStatusBucket(raw string) string {
	status := strings.ToLower(strings.TrimSpace(raw))
	if status == "" {
		return taskFlowHTTPStatusMissing
	}
	if _, ok := validTaskStatuses[status]; ok {
		return status
	}
	return taskFlowHTTPStatusOther
}

func taskFlowHTTPStatusMatches(raw, filter string) bool {
	bucket := taskFlowHTTPStatusBucket(raw)
	switch filter {
	case "", "all":
		return true
	case "pending":
		return bucket == taskFlowHTTPStatusMissing || bucket == "open"
	default:
		return bucket == filter
	}
}

func taskFlowHTTPReadiness(status string, blockers []taskFlowHTTPBlocker) string {
	switch status {
	case "claimed_done", "cancelled":
		return taskFlowHTTPReadinessDone
	case "blocked":
		return taskFlowHTTPReadinessBlockedStatus
	case "open", taskFlowHTTPStatusMissing:
		if len(blockers) > 0 {
			return taskFlowHTTPReadinessBlocked
		}
		return taskFlowHTTPReadinessReady
	default:
		return taskFlowHTTPReadinessOther
	}
}

func taskFlowHTTPStage(readiness string) string {
	switch readiness {
	case taskFlowHTTPReadinessReady:
		return "ready"
	case taskFlowHTTPReadinessBlocked, taskFlowHTTPReadinessBlockedStatus:
		return "blocked"
	case taskFlowHTTPReadinessDone:
		return "done"
	default:
		return "other"
	}
}

func taskFlowHTTPScopeMatches(row taskFlowHTTPRow, flowScope string) bool {
	switch flowScope {
	case taskFlowHTTPFlowAll:
		return true
	case taskFlowHTTPFlowReady:
		return row.Readiness == taskFlowHTTPReadinessReady
	case taskFlowHTTPFlowBlocked:
		return row.Readiness == taskFlowHTTPReadinessBlocked || row.Readiness == taskFlowHTTPReadinessBlockedStatus
	default:
		return row.Status == "open" || row.Status == taskFlowHTTPStatusMissing || row.Status == "blocked"
	}
}

func sortTaskFlowHTTPRows(rows []taskFlowHTTPRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if ra, rb := taskFlowHTTPReadinessRank(a.Readiness), taskFlowHTTPReadinessRank(b.Readiness); ra != rb {
			return ra < rb
		}
		if pa, pb := taskFlowHTTPPriorityRank(a.Priority), taskFlowHTTPPriorityRank(b.Priority); pa != pb {
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

func taskFlowHTTPReadinessRank(readiness string) int {
	switch readiness {
	case taskFlowHTTPReadinessReady:
		return 0
	case taskFlowHTTPReadinessBlocked:
		return 1
	case taskFlowHTTPReadinessBlockedStatus:
		return 2
	case taskFlowHTTPReadinessOther:
		return 3
	case taskFlowHTTPReadinessDone:
		return 4
	default:
		return 5
	}
}

func taskFlowHTTPPriorityRank(priority string) int {
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

func taskFlowHTTPProjectSlugs(scopes []pauth.ProjectScope) []string {
	slugs := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		slug := strings.ToLower(strings.TrimSpace(scope.ProjectSlug))
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs
}

func taskFlowHTTPStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	return out
}

func taskFlowHTTPNotice() string {
	return "task-flow HTTP is the Reader bridge for the same derived sequence model as pindoc.task.flow. due_at is a deadline marker; ordering is readiness, priority, updated_at, project, slug."
}
