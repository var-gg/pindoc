package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

const (
	artifactAuditDefaultLimit = 50
	artifactAuditMaxLimit     = 500

	artifactAuditKindHygiene            = "hygiene"
	artifactAuditKindMetadata           = "metadata"
	artifactAuditKindStale              = "stale"
	artifactAuditKindTaskLifecycle      = "task_lifecycle"
	artifactAuditKindSupersedeCandidate = "supersede_candidate"
)

type artifactAuditInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`

	AreaSlug string   `json:"area_slug,omitempty" jsonschema:"optional area slug filter; alias of area"`
	Area     string   `json:"area,omitempty" jsonschema:"optional area slug filter"`
	Areas    []string `json:"areas,omitempty" jsonschema:"optional area slug filters"`

	Type  string   `json:"type,omitempty" jsonschema:"optional artifact type filter"`
	Types []string `json:"types,omitempty" jsonschema:"optional artifact type filters"`

	Status   string   `json:"status,omitempty" jsonschema:"optional artifact status filter: published | stale | superseded | archived"`
	Statuses []string `json:"statuses,omitempty" jsonschema:"optional artifact status filters: published | stale | superseded | archived"`

	Kind  string   `json:"kind,omitempty" jsonschema:"optional finding kind filter: hygiene | metadata | stale | task_lifecycle | supersede_candidate"`
	Kinds []string `json:"kinds,omitempty" jsonschema:"optional finding kind filters"`

	Limit             int  `json:"limit,omitempty" jsonschema:"default 50, max 500"`
	IncludeSuperseded bool `json:"include_superseded,omitempty" jsonschema:"include superseded artifacts; default false hides replaced artifacts"`
}

type artifactAuditFinding struct {
	ArtifactID        string    `json:"artifact_id"`
	Slug              string    `json:"slug"`
	Title             string    `json:"title"`
	Type              string    `json:"type"`
	AreaSlug          string    `json:"area_slug"`
	Status            string    `json:"status"`
	FindingKind       string    `json:"finding_kind"`
	Code              string    `json:"code"`
	Severity          string    `json:"severity"`
	Reason            string    `json:"reason"`
	RecommendedAction string    `json:"recommended_action"`
	RevisionNumber    int       `json:"revision_number,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
	AgentRef          string    `json:"agent_ref"`
	HumanURL          string    `json:"human_url"`
	HumanURLAbs       string    `json:"human_url_abs"`
}

type artifactAuditOutput struct {
	ProjectSlug string                 `json:"project_slug"`
	Filters     artifactAuditFilterOut `json:"filters"`
	Findings    []artifactAuditFinding `json:"findings"`
	Count       int                    `json:"count"`
	Truncated   bool                   `json:"truncated,omitempty"`
	Notice      string                 `json:"notice"`

	ToolsetVersion string `json:"toolset_version,omitempty"`
}

type artifactAuditFilterOut struct {
	Areas               []string `json:"areas,omitempty"`
	Types               []string `json:"types,omitempty"`
	Statuses            []string `json:"statuses,omitempty"`
	Kinds               []string `json:"kinds,omitempty"`
	Limit               int      `json:"limit"`
	IncludeSuperseded   bool     `json:"include_superseded"`
	DefaultStatusPolicy string   `json:"default_status_policy,omitempty"`
}

type artifactAuditRow struct {
	ArtifactID        string
	Slug              string
	Type              string
	Title             string
	AreaSlug          string
	Status            string
	BodyMarkdown      string
	BodyLocale        string
	TaskMetaRaw       []byte
	WarningPayloadRaw []byte
	RevisionNumber    int
	UpdatedAt         time.Time
}

// RegisterArtifactAudit wires pindoc.artifact.audit. It is a read-only
// hygiene scanner over existing artifacts and warning events.
func RegisterArtifactAudit(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.artifact.audit",
			Description: strings.TrimSpace(`
Read-only audit candidate scanner for existing artifacts. Filters by
area/type/status/kind/limit/include_superseded and returns actionable
hygiene, metadata, stale, task_lifecycle, and supersede_candidate findings.
It re-runs the current title-locale detector, reads latest-revision
artifact.warning_raised events, uses the existing age-based stale signal,
and never mutates or reopens artifacts.
`),
		},
		func(ctx context.Context, p *auth.Principal, in artifactAuditInput) (*sdk.CallToolResult, artifactAuditOutput, error) {
			readScope, err := resolveMCPReadProjectScope(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactAuditOutput{}, fmt.Errorf("artifact.audit: %w", err)
			}
			filter, err := normalizeArtifactAuditFilter(in)
			if err != nil {
				return nil, artifactAuditOutput{}, err
			}
			out, err := buildArtifactAudit(ctx, deps, readScope, filter)
			if err != nil {
				return nil, artifactAuditOutput{}, err
			}
			return nil, out, nil
		},
	)
}

type artifactAuditFilter struct {
	Areas             []string
	Types             []string
	Statuses          []string
	Kinds             map[string]struct{}
	KindList          []string
	Limit             int
	IncludeSuperseded bool
	DefaultStatuses   bool
}

func normalizeArtifactAuditFilter(in artifactAuditInput) (artifactAuditFilter, error) {
	areas := normalizeAuditStringFilters(append(append([]string{in.AreaSlug, in.Area}, in.Areas...), ""))

	types, err := normalizeAuditTypeFilters(append([]string{in.Type}, in.Types...))
	if err != nil {
		return artifactAuditFilter{}, err
	}
	statuses, defaultStatuses, err := normalizeAuditStatusFilters(append([]string{in.Status}, in.Statuses...), in.IncludeSuperseded)
	if err != nil {
		return artifactAuditFilter{}, err
	}
	kindList, err := normalizeAuditKindFilters(append([]string{in.Kind}, in.Kinds...))
	if err != nil {
		return artifactAuditFilter{}, err
	}
	kinds := map[string]struct{}{}
	for _, kind := range kindList {
		kinds[kind] = struct{}{}
	}

	limit := in.Limit
	if limit <= 0 {
		limit = artifactAuditDefaultLimit
	}
	if limit > artifactAuditMaxLimit {
		limit = artifactAuditMaxLimit
	}

	return artifactAuditFilter{
		Areas:             areas,
		Types:             types,
		Statuses:          statuses,
		Kinds:             kinds,
		KindList:          kindList,
		Limit:             limit,
		IncludeSuperseded: in.IncludeSuperseded,
		DefaultStatuses:   defaultStatuses,
	}, nil
}

func normalizeAuditStringFilters(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func normalizeAuditTypeFilters(values []string) ([]string, error) {
	raw := normalizeAuditStringFilters(values)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		match := ""
		for valid := range validArtifactTypes {
			if strings.EqualFold(v, valid) {
				match = valid
				break
			}
		}
		if match == "" {
			return nil, fmt.Errorf("type must be one of: Decision | Analysis | Debug | Flow | Task | TC | Glossary | Feature | APIEndpoint | Screen | DataModel")
		}
		out = append(out, match)
	}
	return out, nil
}

func normalizeAuditStatusFilters(values []string, includeSuperseded bool) ([]string, bool, error) {
	raw := normalizeAuditStringFilters(values)
	if len(raw) == 0 {
		out := []string{"published", "stale"}
		if includeSuperseded {
			out = append(out, "superseded")
		}
		return out, true, nil
	}
	valid := map[string]struct{}{
		"published":  {},
		"stale":      {},
		"superseded": {},
		"archived":   {},
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.ToLower(v)
		if _, ok := valid[v]; !ok {
			return nil, false, fmt.Errorf("status must be one of: published | stale | superseded | archived")
		}
		out = append(out, v)
	}
	return out, false, nil
}

func normalizeAuditKindFilters(values []string) ([]string, error) {
	raw := normalizeAuditStringFilters(values)
	valid := map[string]struct{}{
		artifactAuditKindHygiene:            {},
		artifactAuditKindMetadata:           {},
		artifactAuditKindStale:              {},
		artifactAuditKindTaskLifecycle:      {},
		artifactAuditKindSupersedeCandidate: {},
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.ToLower(v)
		if _, ok := valid[v]; !ok {
			return nil, fmt.Errorf("kind must be one of: hygiene | metadata | stale | task_lifecycle | supersede_candidate")
		}
		out = append(out, v)
	}
	return out, nil
}

func buildArtifactAudit(ctx context.Context, deps Deps, readScope *mcpReadProjectScope, filter artifactAuditFilter) (artifactAuditOutput, error) {
	scope := readScope.ProjectScope
	var typesArg, areasArg any
	if len(filter.Types) > 0 {
		typesArg = filter.Types
	}
	if len(filter.Areas) > 0 {
		areasArg = filter.Areas
	}

	args := []any{scope.ProjectSlug, filter.Statuses, typesArg, areasArg}
	visibilityWhere, visibilityArgs := mcpReadArtifactVisibilityWhere(readScope, "a", len(args)+1)
	args = append(args, visibilityArgs...)
	rows, err := deps.DB.Query(ctx, fmt.Sprintf(`
		WITH base AS (
			SELECT
				a.id AS artifact_uuid,
				a.id::text AS artifact_id,
				a.project_id,
				a.slug,
				a.type,
				a.title,
				ar.slug AS area_slug,
				a.status,
				a.body_markdown,
				COALESCE(a.body_locale, '') AS body_locale,
				COALESCE(a.task_meta, '{}'::jsonb) AS task_meta,
				a.updated_at,
				COALESCE((
					SELECT max(r.revision_number)
					FROM artifact_revisions r
					WHERE r.artifact_id = a.id
				), 0)::int AS revision_number
			FROM artifacts a
			JOIN projects p ON p.id = a.project_id
			JOIN areas    ar ON ar.id = a.area_id
			WHERE p.slug = $1
			  AND a.status = ANY($2::text[])
			  AND ($3::text[] IS NULL OR a.type = ANY($3))
			  AND ($4::text[] IS NULL OR ar.slug = ANY($4))
			  AND NOT starts_with(a.slug, '_template_')
			  AND %s
			ORDER BY a.updated_at DESC, a.slug
		)
		SELECT
			b.artifact_id,
			b.slug,
			b.type,
			b.title,
			b.area_slug,
			b.status,
			b.body_markdown,
			b.body_locale,
			b.task_meta,
			COALESCE(w.payload, '{}'::jsonb) AS warning_payload,
			b.revision_number,
			b.updated_at
		FROM base b
		LEFT JOIN LATERAL (
			SELECT e.payload
			FROM events e
			WHERE e.project_id = b.project_id
			  AND e.subject_id = b.artifact_uuid
			  AND e.kind = 'artifact.warning_raised'
			  AND COALESCE(NULLIF(e.payload->>'revision_number', '')::int, b.revision_number) = b.revision_number
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		) w ON true
		ORDER BY b.updated_at DESC, b.slug
	`, visibilityWhere), args...)
	if err != nil {
		return artifactAuditOutput{}, fmt.Errorf("artifact.audit query: %w", err)
	}
	defer rows.Close()

	findings := []artifactAuditFinding{}
	truncated := false
	for rows.Next() {
		var row artifactAuditRow
		if err := rows.Scan(
			&row.ArtifactID,
			&row.Slug,
			&row.Type,
			&row.Title,
			&row.AreaSlug,
			&row.Status,
			&row.BodyMarkdown,
			&row.BodyLocale,
			&row.TaskMetaRaw,
			&row.WarningPayloadRaw,
			&row.RevisionNumber,
			&row.UpdatedAt,
		); err != nil {
			return artifactAuditOutput{}, fmt.Errorf("artifact.audit scan: %w", err)
		}
		for _, finding := range artifactAuditFindingsForRow(deps, scope, row, filter) {
			if len(findings) >= filter.Limit {
				truncated = true
				continue
			}
			findings = append(findings, finding)
		}
	}
	if err := rows.Err(); err != nil {
		return artifactAuditOutput{}, fmt.Errorf("artifact.audit rows: %w", err)
	}

	statusPolicy := ""
	if filter.DefaultStatuses {
		statusPolicy = "default excludes archived and superseded; set include_superseded=true to include superseded"
	}
	return artifactAuditOutput{
		ProjectSlug: scope.ProjectSlug,
		Filters: artifactAuditFilterOut{
			Areas:               filter.Areas,
			Types:               filter.Types,
			Statuses:            filter.Statuses,
			Kinds:               filter.KindList,
			Limit:               filter.Limit,
			IncludeSuperseded:   filter.IncludeSuperseded,
			DefaultStatusPolicy: statusPolicy,
		},
		Findings:  findings,
		Count:     len(findings),
		Truncated: truncated,
		Notice:    "Read-only audit candidates only. Stale findings are age-based advisories; completed Tasks are never recommended for reopen.",
	}, nil
}

func artifactAuditFindingsForRow(deps Deps, scope *auth.ProjectScope, row artifactAuditRow, filter artifactAuditFilter) []artifactAuditFinding {
	out := []artifactAuditFinding{}
	seen := map[string]struct{}{}
	add := func(kind, code, reason, action string) {
		kind = strings.TrimSpace(kind)
		code = strings.TrimSpace(code)
		if kind == "" || code == "" {
			return
		}
		if len(filter.Kinds) > 0 {
			if _, ok := filter.Kinds[kind]; !ok {
				return
			}
		}
		key := kind + "\x00" + code
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, artifactAuditFinding{
			ArtifactID:        row.ArtifactID,
			Slug:              row.Slug,
			Title:             row.Title,
			Type:              row.Type,
			AreaSlug:          row.AreaSlug,
			Status:            row.Status,
			FindingKind:       kind,
			Code:              code,
			Severity:          artifactAuditSeverity(code),
			Reason:            reason,
			RecommendedAction: action,
			RevisionNumber:    row.RevisionNumber,
			UpdatedAt:         row.UpdatedAt,
			AgentRef:          "pindoc://" + row.Slug,
			HumanURL:          HumanURL(scope.ProjectSlug, scope.ProjectLocale, row.Slug),
			HumanURLAbs:       AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, row.Slug),
		})
	}

	for _, code := range titleLocaleMismatchWarnings(scope.ProjectLocale, row.Title) {
		add(artifactAuditKindHygiene, code,
			"title contains Latin letters but lacks a project-language title anchor",
			"wording_fix")
	}

	if locale := strings.TrimSpace(row.BodyLocale); locale != "" && !validBodyLocale(locale) {
		add(artifactAuditKindMetadata, "BODY_LOCALE_INVALID",
			fmt.Sprintf("body_locale %q is outside the safe BCP 47 subset", locale),
			"create_followup_task")
	}

	for _, code := range artifactAuditWarningEventCodes(row.WarningPayloadRaw) {
		add(artifactAuditKindForWarningCode(code), code,
			"latest revision has stored artifact.warning_raised event for this code",
			artifactAuditRecommendedAction(code, row))
	}

	artifactAuditTaskLifecycleFindings(row, add)

	if stale := staleFromAge(row.Slug, row.UpdatedAt); stale != nil {
		add(artifactAuditKindStale, "ARTIFACT_STALE_AGE",
			stale.Reason+"; semantic staleness is not inferred by this audit",
			"create_followup_task")
	}

	if row.Status == "superseded" {
		add(artifactAuditKindSupersedeCandidate, "ARTIFACT_SUPERSEDED",
			"artifact is superseded and hidden from default retrieval unless include_superseded=true",
			"ignore")
	}

	return out
}

func artifactAuditTaskLifecycleFindings(row artifactAuditRow, add func(kind, code, reason, action string)) {
	if row.Type != "Task" {
		return
	}
	rawStatus, _ := taskAttentionTaskMetaFields(row.TaskMetaRaw)
	bucket := taskStatusBucket(rawStatus)
	counts := countTaskQueueAcceptance(row.BodyMarkdown)
	switch {
	case (bucket == "open" || bucket == taskStatusMissing) && counts.total > 0 && counts.unresolved == 0:
		add(artifactAuditKindTaskLifecycle, taskWarningAcceptanceReconcilePending,
			"Task is open or missing status but all acceptance checkboxes are resolved",
			"meta_patch")
	case bucket == "claimed_done" && len(unresolvedAcceptanceLabels(row.BodyMarkdown)) > 0:
		add(artifactAuditKindTaskLifecycle, "TASK_CLAIMED_DONE_UNRESOLVED_ACCEPTANCE",
			"Task is claimed_done but still has unresolved [ ]/[~] acceptance labels",
			"create_followup_task")
	}
}

func artifactAuditWarningEventCodes(raw []byte) []string {
	if len(raw) == 0 || string(raw) == "{}" {
		return nil
	}
	var payload struct {
		Codes []string `json:"codes"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return uniqueStrings(payload.Codes)
}

func artifactAuditSeverity(code string) string {
	switch artifactAuditCodePrefix(code) {
	case "BODY_LOCALE_INVALID":
		return SeverityError
	case "ARTIFACT_STALE_AGE", "ARTIFACT_SUPERSEDED":
		return SeverityInfo
	default:
		return warningSeverity(code)
	}
}

func artifactAuditKindForWarningCode(code string) string {
	switch artifactAuditCodePrefix(code) {
	case "SOURCE_TYPE_UNCLASSIFIED", "CONSENT_REQUIRED_FOR_USER_CHAT",
		"PIN_PATH_NONEXISTENT", "PIN_PATH_OUTSIDE_REPO", "PIN_PATH_NOT_FOUND",
		"PIN_PATH_REJECTED", "PIN_REPO_ID_NOT_FOUND", "PIN_REPO_LOCAL_PATHS_MISSING",
		"PIN_REPO_MAPPING_DIAGNOSTIC_FAILED", "PIN_REPO_MAPPING_UNRESOLVED",
		"PIN_REPO_NOT_REGISTERED", "RECOMMEND_REPO_REGISTRATION":
		return artifactAuditKindMetadata
	default:
		return artifactAuditKindHygiene
	}
}

func artifactAuditRecommendedAction(code string, row artifactAuditRow) string {
	switch artifactAuditCodePrefix(code) {
	case "TITLE_LOCALE_MISMATCH", "TITLE_TOO_SHORT", "TITLE_TOO_LONG",
		"TITLE_GENERIC_TOKENS", "BODY_HAS_H1_REDUNDANT", "MISSING_H2",
		"SECTION_DUPLICATES_EDGES", "DECISION_AREA_MUST_BE_SUBJECT",
		"SLUG_VERBOSE", "MISSING_COMMIT_MSG_ON_CREATE":
		return "wording_fix"
	case "SOURCE_TYPE_UNCLASSIFIED", "CONSENT_REQUIRED_FOR_USER_CHAT",
		"PIN_PATH_NONEXISTENT", "PIN_PATH_OUTSIDE_REPO", "PIN_PATH_NOT_FOUND",
		"PIN_PATH_REJECTED", "PIN_REPO_ID_NOT_FOUND", "PIN_REPO_LOCAL_PATHS_MISSING",
		"PIN_REPO_MAPPING_DIAGNOSTIC_FAILED", "PIN_REPO_MAPPING_UNRESOLVED",
		"PIN_REPO_NOT_REGISTERED", "RECOMMEND_REPO_REGISTRATION":
		return "meta_patch"
	case "CANONICAL_REWRITE_WITHOUT_EVIDENCE", "RECOMMEND_READ_BEFORE_CREATE":
		return "create_followup_task"
	default:
		if row.Type == "Task" && taskStatusBucket(taskStatusFromRawJSON(row.TaskMetaRaw)) == "claimed_done" {
			return "create_followup_task"
		}
		return "create_followup_task"
	}
}

func artifactAuditCodePrefix(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.IndexByte(code, ':'); i > 0 {
		return code[:i]
	}
	return code
}

func taskStatusFromRawJSON(raw []byte) string {
	status, _ := taskAttentionTaskMetaFields(raw)
	return status
}
