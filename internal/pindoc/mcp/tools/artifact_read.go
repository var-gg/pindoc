package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type artifactReadInput struct {
	// ProjectSlug picks which project to look the artifact up in
	// (account-level scope, Decision mcp-scope-account-level-industry-
	// standard). Required — empty surfaces PROJECT_SLUG_REQUIRED.
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`

	// One of IDOrSlug (UUID or project-scoped slug) must be set.
	// URLs coming from Wiki Reader share links (pindoc://... or
	// /p/{project}/wiki/{slug}) are accepted here too and normalized
	// server-side — agents shouldn't have to parse Pindoc's URL shape.
	// Legacy /p/{project}/{locale}/wiki/{slug} links are also accepted.
	IDOrSlug string `json:"id_or_slug" jsonschema:"artifact UUID, slug, or share URL (pindoc://... or /p/{project}/wiki/{slug}; legacy /p/{project}/{locale}/wiki/{slug} also accepted)"`

	// View controls how much the server returns. Default "full" matches
	// Phase 1–11 behaviour for backward compat. "brief" omits
	// body_markdown and adds a short summary + pins + stale flag; useful
	// when scanning many artifacts. "continuation" = brief + recent
	// revision delta + typed edges so the next session can land quickly
	// without pulling full bodies for neighbours.
	View string `json:"view,omitempty" jsonschema:"brief | full | continuation; default full"`
}

type artifactReadOutput struct {
	ID            string    `json:"id"`
	ProjectSlug   string    `json:"project_slug"`
	AreaSlug      string    `json:"area_slug"`
	Slug          string    `json:"slug"`
	Type          string    `json:"type"`
	Title         string    `json:"title"`
	BodyMarkdown  string    `json:"body_markdown,omitempty"` // omitted on view=brief
	BodyLocale    string    `json:"body_locale,omitempty"`
	Tags          []string  `json:"tags"`
	Completeness  string    `json:"completeness"`
	Status        string    `json:"status"`
	ReviewState   string    `json:"review_state"`
	AuthorKind    string    `json:"author_kind"`
	AuthorID      string    `json:"author_id"`
	AuthorVersion string    `json:"author_version,omitempty"`
	SupersededBy  string    `json:"superseded_by,omitempty"`
	AgentRef      string    `json:"agent_ref"`
	HumanURL      string    `json:"human_url"`
	HumanURLAbs   string    `json:"human_url_abs,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	PublishedAt   time.Time `json:"published_at,omitzero"`

	// View: populated on brief + continuation.
	View    string       `json:"view"`
	Summary string       `json:"summary,omitempty"`
	Pins    []PinRef     `json:"pins,omitempty"`
	Stale   *StaleSignal `json:"stale,omitempty"`

	// View: populated on continuation only.
	RecentRevisions            []RevisionSummaryRef    `json:"recent_revisions,omitempty"`
	RelatesTo                  []EdgeRef               `json:"relates_to,omitempty"`
	RelatedBy                  []EdgeRef               `json:"related_by,omitempty"`
	UnresolvedAcceptanceLabels []AcceptanceLabelRef    `json:"unresolved_acceptance_labels,omitempty"`
	VerificationNotes          []VerificationNoteInput `json:"verification_notes,omitempty"`
	VerificationReceipts       []string                `json:"verification_receipts,omitempty"`

	// ArtifactMeta echoes the epistemic axes persisted on the artifact.
	// Populated on every view (brief / full / continuation) because the
	// 6-axis trust summary is the cheapest signal a caller needs to decide
	// whether this artifact is usable as-is, which is the same question on
	// every view. Empty object when the row predates migration 0012.
	ArtifactMeta ResolvedArtifactMeta `json:"artifact_meta"`

	// TaskAttention is an agent-only lifecycle reminder for open Task
	// artifacts. It is deliberately carried out-of-band from body_markdown
	// so retrieval reads and human Reader views do not accumulate prompt
	// noise. Server-side gates below decide whether it appears at all.
	TaskAttention  *TaskAttention `json:"task_attention,omitempty"`
	ToolsetVersion string         `json:"toolset_version,omitempty"`
}

type TaskAttention struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	NextTools []NextToolHint `json:"next_tools,omitempty"`
	Level     string         `json:"level"`
}

// PinRef mirrors artifact_pins rows. Empty repo defaults to "origin" in
// the migration, so we always have a non-empty value here. Kind is
// "code" | "doc" | "config" | "asset" | "resource" | "url"; repo_id is
// the canonical project_repos coordinate when the server can resolve it.
type PinRef struct {
	Kind       string `json:"kind"`
	RepoID     string `json:"repo_id,omitempty"`
	Repo       string `json:"repo,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Path       string `json:"path"`
	LinesStart int    `json:"lines_start,omitempty"`
	LinesEnd   int    `json:"lines_end,omitempty"`
}

// RevisionSummaryRef is a trimmed revision row for continuation view.
type RevisionSummaryRef struct {
	RevisionNumber int       `json:"revision_number"`
	CommitMsg      string    `json:"commit_msg,omitempty"`
	AuthorID       string    `json:"author_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// EdgeRef describes one artifact_edges row from the target's perspective.
// For "relates_to" view the target's ID/slug is filled; for "related_by"
// the source is.
type EdgeRef struct {
	ArtifactID  string `json:"artifact_id"`
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Relation    string `json:"relation"`
	AgentRef    string `json:"agent_ref"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
}

// RegisterArtifactRead wires pindoc.artifact.read.
func RegisterArtifactRead(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.read",
			Description: "Fetch an artifact by UUID, slug, or share URL, including /p/{project}/wiki/{slug} paths and their absolute URLs. Legacy /p/{project}/{locale}/wiki/{slug} paths are accepted. view=brief returns title/summary/pins/stale without the full body; view=continuation adds recent revisions and typed edges; view=full (default) returns everything. Responses include unresolved_acceptance_labels for [ ]/[~] checkbox selector context. For open Task artifacts, full/continuation reads may include task_attention only when the caller is the assignee or latest revision author.",
		},
		func(ctx context.Context, p *auth.Principal, in artifactReadInput) (*sdk.CallToolResult, artifactReadOutput, error) {
			readScope, err := resolveMCPReadProjectScope(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactReadOutput{}, fmt.Errorf("artifact.read: %w", err)
			}
			scope := readScope.ProjectScope
			ref := normalizeArtifactReadRef(in.IDOrSlug, scope.ProjectSlug)
			idOrSlug := ref.Value
			if idOrSlug == "" {
				return nil, artifactReadOutput{}, errors.New("id_or_slug is required")
			}
			if ref.ScopeMismatch {
				return nil, artifactReadOutput{}, artifactReadNotFoundError(in.IDOrSlug, scope, ref)
			}
			view := strings.ToLower(strings.TrimSpace(in.View))
			if view == "" {
				view = "full"
			}
			if view != "brief" && view != "full" && view != "continuation" {
				return nil, artifactReadOutput{}, fmt.Errorf("view %q invalid; use brief | full | continuation", in.View)
			}

			var out artifactReadOutput
			var desc, authorVer, superseded *string
			var publishedAt *time.Time
			var metaRaw, taskMetaRaw []byte
			var lastRevisionAuthor string
			readArgs := []any{scope.ProjectSlug, idOrSlug}
			visibilityWhere, visibilityArgs := mcpReadArtifactVisibilityWhere(readScope, "a", len(readArgs)+1)
			readArgs = append(readArgs, visibilityArgs...)
			err = deps.DB.QueryRow(ctx, fmt.Sprintf(`
				SELECT
					a.id::text,
					proj.slug,
					area.slug,
					a.slug,
					a.type,
					a.title,
					a.body_markdown,
					COALESCE(NULLIF(a.body_locale, ''), NULLIF(proj.primary_language, ''), 'en'),
					a.tags,
					a.completeness,
					a.status,
					a.review_state,
					a.author_kind,
					a.author_id,
					a.author_version,
					a.superseded_by::text,
					a.created_at,
					a.updated_at,
					a.published_at,
					a.artifact_meta,
					COALESCE(a.task_meta, '{}'::jsonb),
					COALESCE((
						SELECT r.author_id
						FROM artifact_revisions r
						WHERE r.artifact_id = a.id
						ORDER BY r.revision_number DESC
						LIMIT 1
					), a.author_id)
				FROM artifacts a
				JOIN projects proj ON proj.id = a.project_id
				JOIN areas    area ON area.id = a.area_id
				WHERE proj.slug = $1
				  AND (
				       a.id::text = $2 OR a.slug = $2 OR
				       a.id = (
				          SELECT asa.artifact_id
				            FROM artifact_slug_aliases asa
				           WHERE asa.project_id = proj.id AND asa.old_slug = $2
				           LIMIT 1
				       )
				  )
				  AND %s
				LIMIT 1
			`, visibilityWhere), readArgs...).Scan(
				&out.ID, &out.ProjectSlug, &out.AreaSlug, &out.Slug,
				&out.Type, &out.Title, &out.BodyMarkdown, &out.BodyLocale, &out.Tags,
				&out.Completeness, &out.Status, &out.ReviewState,
				&out.AuthorKind, &out.AuthorID, &authorVer, &superseded,
				&out.CreatedAt, &out.UpdatedAt, &publishedAt, &metaRaw,
				&taskMetaRaw, &lastRevisionAuthor,
			)
			_ = desc // reserved; project.description not part of read response
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactReadOutput{}, artifactReadNotFoundError(in.IDOrSlug, scope, ref)
			}
			if err != nil {
				return nil, artifactReadOutput{}, fmt.Errorf("read: %w", err)
			}
			if authorVer != nil {
				out.AuthorVersion = *authorVer
			}
			if superseded != nil {
				out.SupersededBy = *superseded
			}
			if publishedAt != nil {
				out.PublishedAt = *publishedAt
			}
			if len(metaRaw) > 0 {
				if err := json.Unmarshal(metaRaw, &out.ArtifactMeta); err != nil {
					deps.Logger.Warn("artifact_meta unmarshal failed",
						"artifact_id", out.ID, "err", err)
				}
			}
			out.AgentRef = "pindoc://" + out.Slug
			out.HumanURL = HumanURL(out.ProjectSlug, scope.ProjectLocale, out.Slug)
			out.HumanURLAbs = AbsHumanURL(deps.Settings, out.ProjectSlug, scope.ProjectLocale, out.Slug)
			out.View = view
			out.UnresolvedAcceptanceLabels = unresolvedAcceptanceLabels(out.BodyMarkdown)
			if notes, receipts, err := loadLatestVerificationEvidence(ctx, deps, out.ID); err != nil {
				deps.Logger.Warn("verification evidence lookup failed", "artifact_id", out.ID, "err", err)
			} else {
				out.VerificationNotes = notes
				out.VerificationReceipts = receipts
			}
			taskStatus, taskAssignee := taskAttentionTaskMetaFields(taskMetaRaw)
			out.TaskAttention = buildTaskAttention(
				out.Type,
				taskStatus,
				taskAssignee,
				lastRevisionAuthor,
				p,
				view,
				out.BodyLocale,
				out.ProjectSlug,
				out.Slug,
			)

			// view=brief / continuation: drop the heavy body, add summary.
			if view == "brief" || view == "continuation" {
				out.Summary = summarizeBody(out.BodyMarkdown)
				if view == "brief" {
					out.BodyMarkdown = ""
				}
			}

			// pins + stale are cheap; attach on brief and continuation.
			if view == "brief" || view == "continuation" {
				pins, err := loadPins(ctx, deps, out.ID)
				if err != nil {
					deps.Logger.Warn("pin lookup failed", "artifact_id", out.ID, "err", err)
				}
				out.Pins = pins

				if stale := staleFromAge(out.Slug, out.UpdatedAt); stale != nil {
					out.Stale = stale
				}
			}

			// continuation: recent revisions + edges.
			if view == "continuation" {
				revs, err := loadRecentRevisions(ctx, deps, out.ID, 3)
				if err != nil {
					deps.Logger.Warn("revisions lookup failed", "artifact_id", out.ID, "err", err)
				}
				out.RecentRevisions = revs

				rel, relBy, err := loadEdges(ctx, deps, readScope, out.ID)
				if err != nil {
					deps.Logger.Warn("edges lookup failed", "artifact_id", out.ID, "err", err)
				}
				out.RelatesTo = rel
				out.RelatedBy = relBy
			}

			return nil, out, nil
		},
	)
}

func taskAttentionTaskMetaFields(raw []byte) (status, assignee string) {
	if len(raw) == 0 {
		return "", ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", ""
	}
	if s, ok := m["status"].(string); ok {
		status = strings.TrimSpace(s)
	}
	if a, ok := m["assignee"].(string); ok {
		assignee = strings.TrimSpace(a)
	}
	return status, assignee
}

func buildTaskAttention(artifactType, taskStatus, taskAssignee, lastRevisionAuthor string, p *auth.Principal, view, locale, projectSlug, slug string) *TaskAttention {
	if artifactType != "Task" {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(view), "brief") {
		return nil
	}
	status := strings.TrimSpace(strings.ToLower(taskStatus))
	if status == "" {
		status = "open"
	}
	switch status {
	case "claimed_done", "blocked", "cancelled":
		return nil
	}

	caller := taskAttentionCallerID(p)
	if taskAttentionHumanCaller(caller) {
		return nil
	}
	if !taskAttentionCallerMatches(caller, lastRevisionAuthor) &&
		!taskAttentionCallerMatches(caller, taskAssignee) {
		return nil
	}

	return &TaskAttention{
		Code:    "task_still_open",
		Message: taskAttentionMessage(locale),
		Level:   "info",
		NextTools: []NextToolHint{
			{
				Tool: "pindoc.artifact.propose",
				Args: map[string]any{
					"project_slug": projectSlug,
					"update_of":    "pindoc://" + slug,
					"shape":        string(ShapeAcceptanceTransition),
				},
				Reason: "update acceptance checks",
			},
			{
				Tool: "pindoc.task.claim_done",
				Args: map[string]any{
					"project_slug": projectSlug,
					"slug_or_id":   "pindoc://" + slug,
				},
				Reason: "claim completion atomically, attach evidence, then run pindoc.task.done_check before final handoff",
			},
		},
	}
}

func taskAttentionCallerID(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.AgentID)
}

func taskAttentionHumanCaller(caller string) bool {
	caller = strings.TrimSpace(caller)
	return caller == "" || strings.HasPrefix(caller, "user:") || strings.HasPrefix(caller, "@")
}

func taskAttentionCallerMatches(caller, candidate string) bool {
	caller = strings.TrimSpace(caller)
	candidate = strings.TrimSpace(candidate)
	if caller == "" || candidate == "" {
		return false
	}
	if caller == candidate {
		return true
	}
	return stripAgentPrefix(caller) == stripAgentPrefix(candidate)
}

func stripAgentPrefix(id string) string {
	id = strings.TrimSpace(id)
	return strings.TrimPrefix(id, "agent:")
}

func taskAttentionMessage(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "ko") {
		return "이 Task는 status=open. 작업이 끝났으면 acceptance 체크와 evidence를 정리한 뒤 pindoc.task.claim_done을 호출하고, 최종 응답 전 pindoc.task.done_check로 닫힘을 확인하세요."
	}
	return "This Task is still open. If you're done, update acceptance/evidence, call pindoc.task.claim_done, then run pindoc.task.done_check before final handoff."
}

// summarizeBody returns up to ~240 chars, preferring the first paragraph
// break. Agents rarely need more than that to decide "is this the artifact
// I want?" before calling view=full.
func summarizeBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	// Prefer first paragraph.
	if idx := strings.Index(body, "\n\n"); idx >= 0 && idx < 400 {
		return strings.TrimSpace(body[:idx])
	}
	if len(body) <= 240 {
		return body
	}
	// Word boundary trim.
	cut := 240
	for cut > 0 && body[cut] != ' ' && body[cut] != '\n' {
		cut--
	}
	if cut == 0 {
		cut = 240
	}
	return strings.TrimSpace(body[:cut]) + "…"
}

func loadPins(ctx context.Context, deps Deps, artifactID string) ([]PinRef, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT kind, repo_id::text, repo, commit_sha, path, lines_start, lines_end
		FROM artifact_pins
		WHERE artifact_id = $1
		ORDER BY id
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PinRef
	for rows.Next() {
		var p PinRef
		var repoID, commitSHA *string
		var linesStart, linesEnd *int
		if err := rows.Scan(&p.Kind, &repoID, &p.Repo, &commitSHA, &p.Path, &linesStart, &linesEnd); err != nil {
			return nil, err
		}
		if repoID != nil {
			p.RepoID = *repoID
		}
		if commitSHA != nil {
			p.CommitSHA = *commitSHA
		}
		if linesStart != nil {
			p.LinesStart = *linesStart
		}
		if linesEnd != nil {
			p.LinesEnd = *linesEnd
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadLatestVerificationEvidence(ctx context.Context, deps Deps, artifactID string) ([]VerificationNoteInput, []string, error) {
	var raw []byte
	err := deps.DB.QueryRow(ctx, `
		SELECT shape_payload
		  FROM artifact_revisions
		 WHERE artifact_id = $1
		   AND shape_payload->>'kind' = 'claim_done'
		   AND (
		        shape_payload ? 'verification_notes'
		     OR shape_payload ? 'verification_receipts'
		   )
		 ORDER BY revision_number DESC
		 LIMIT 1
	`, artifactID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		VerificationNotes    []VerificationNoteInput `json:"verification_notes"`
		VerificationReceipts []string                `json:"verification_receipts"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, nil, err
	}
	return parsed.VerificationNotes, normalizeClaimDoneEvidenceArtifacts(parsed.VerificationReceipts), nil
}

// staleFromAge reuses the Phase 11c heuristic: over 60 days without an
// update → stale. Later phases swap in pin-diff-vs-HEAD.
func staleFromAge(slug string, updatedAt time.Time) *StaleSignal {
	age := time.Since(updatedAt)
	if age <= staleAgeThreshold {
		return nil
	}
	return &StaleSignal{
		Slug:    slug,
		DaysOld: int(age.Hours() / 24),
		Reason:  fmt.Sprintf("not updated in %d days", int(age.Hours()/24)),
	}
}

func loadRecentRevisions(ctx context.Context, deps Deps, artifactID string, limit int) ([]RevisionSummaryRef, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT revision_number, commit_msg, author_id, created_at
		FROM artifact_revisions
		WHERE artifact_id = $1
		ORDER BY revision_number DESC
		LIMIT $2
	`, artifactID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RevisionSummaryRef
	for rows.Next() {
		var r RevisionSummaryRef
		var msg *string
		if err := rows.Scan(&r.RevisionNumber, &msg, &r.AuthorID, &r.CreatedAt); err != nil {
			return nil, err
		}
		if msg != nil {
			r.CommitMsg = *msg
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadEdges(ctx context.Context, deps Deps, readScope *mcpReadProjectScope, artifactID string) ([]EdgeRef, []EdgeRef, error) {
	out := []EdgeRef{}
	outBy := []EdgeRef{}
	scope := readScope.ProjectScope

	// Outgoing: this artifact → others.
	args := []any{artifactID}
	visibilityWhere, visibilityArgs := mcpReadArtifactVisibilityWhere(readScope, "a", len(args)+1)
	args = append(args, visibilityArgs...)
	rows, err := deps.DB.Query(ctx, fmt.Sprintf(`
		SELECT e.target_id::text, a.slug, a.type, a.title, e.relation
		FROM artifact_edges e
		JOIN artifacts a ON a.id = e.target_id
		WHERE e.source_id = $1
		  AND %s
		ORDER BY e.created_at
	`, visibilityWhere), args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e EdgeRef
		if err := rows.Scan(&e.ArtifactID, &e.Slug, &e.Type, &e.Title, &e.Relation); err != nil {
			return nil, nil, err
		}
		e.AgentRef = "pindoc://" + e.Slug
		e.HumanURL = HumanURL(scope.ProjectSlug, scope.ProjectLocale, e.Slug)
		e.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, e.Slug)
		out = append(out, e)
	}
	rows.Close()

	// Incoming: others → this artifact.
	args = []any{artifactID}
	visibilityWhere, visibilityArgs = mcpReadArtifactVisibilityWhere(readScope, "a", len(args)+1)
	args = append(args, visibilityArgs...)
	rows2, err := deps.DB.Query(ctx, fmt.Sprintf(`
		SELECT e.source_id::text, a.slug, a.type, a.title, e.relation
		FROM artifact_edges e
		JOIN artifacts a ON a.id = e.source_id
		WHERE e.target_id = $1
		  AND %s
		ORDER BY e.created_at
	`, visibilityWhere), args...)
	if err != nil {
		return out, nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var e EdgeRef
		if err := rows2.Scan(&e.ArtifactID, &e.Slug, &e.Type, &e.Title, &e.Relation); err != nil {
			return out, nil, err
		}
		e.AgentRef = "pindoc://" + e.Slug
		e.HumanURL = HumanURL(scope.ProjectSlug, scope.ProjectLocale, e.Slug)
		e.HumanURLAbs = AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, e.Slug)
		outBy = append(outBy, e)
	}

	return out, outBy, nil
}

// normalizeRef strips a Pindoc share URL down to the ID/slug the caller
// actually wanted. Plain IDs/slugs pass through unchanged.
//
// Recognised shapes:
//
//	pindoc://<id_or_slug>
//	https://<host>/a/<id_or_slug>
//	http://<host>/a/<id_or_slug>
//	<id_or_slug>
func normalizeRef(raw string) string {
	s := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(s, "pindoc://"):
		return strings.TrimPrefix(s, "pindoc://")
	case strings.Contains(s, "://"):
		// http(s)://host/a/<tail>
		if i := strings.LastIndex(s, "/a/"); i >= 0 {
			return s[i+3:]
		}
	}
	return s
}

type artifactReadRef struct {
	Value             string
	LooksLikeShareURL bool
	ScopeMismatch     bool
}

// normalizeArtifactReadRef handles the Reader-facing share URL shape. It is
// intentionally stricter than normalizeRef: a /p/<project>/wiki/...
// URL from another fixed-session project scope must not be allowed to
// collapse to a bare slug that could accidentally match the active project.
func normalizeArtifactReadRef(raw, projectSlug string) artifactReadRef {
	s := strings.TrimSpace(raw)
	if s == "" {
		return artifactReadRef{}
	}
	if strings.HasPrefix(s, "pindoc://") {
		return artifactReadRef{Value: strings.TrimPrefix(s, "pindoc://")}
	}

	if strings.HasPrefix(s, "/") || strings.Contains(s, "://") {
		if parsed, err := url.Parse(s); err == nil {
			if ref, ok := normalizeArtifactReadPath(parsed.Path, projectSlug); ok {
				return ref
			}
		}
	}

	return artifactReadRef{Value: s}
}

func normalizeArtifactReadPath(path, projectSlug string) (artifactReadRef, bool) {
	segments := splitPathSegments(path)
	if len(segments) == 0 {
		return artifactReadRef{}, false
	}

	if segments[0] == "a" && len(segments) >= 2 {
		return artifactReadRef{
			Value:             strings.Join(segments[1:], "/"),
			LooksLikeShareURL: true,
		}, true
	}

	if len(segments) >= 4 && segments[0] == "p" && segments[2] == "wiki" {
		ref := artifactReadRef{LooksLikeShareURL: true}
		if len(segments) < 4 || segments[3] == "" {
			ref.Value = strings.TrimSpace(path)
			return ref, true
		}
		ref.Value = segments[3]
		ref.ScopeMismatch = segments[1] != projectSlug
		return ref, true
	}

	if len(segments) >= 5 && segments[0] == "p" && segments[3] == "wiki" {
		ref := artifactReadRef{LooksLikeShareURL: true}
		if len(segments) < 5 || segments[4] == "" {
			ref.Value = strings.TrimSpace(path)
			return ref, true
		}
		ref.Value = segments[4]
		ref.ScopeMismatch = segments[1] != projectSlug
		return ref, true
	}

	return artifactReadRef{}, false
}

func splitPathSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	rawSegments := strings.Split(path, "/")
	segments := make([]string, 0, len(rawSegments))
	for _, raw := range rawSegments {
		segment, err := url.PathUnescape(raw)
		if err != nil {
			segment = raw
		}
		segments = append(segments, segment)
	}
	return segments
}

func artifactReadNotFoundError(raw string, scope *auth.ProjectScope, ref artifactReadRef) error {
	projectScope := ""
	if scope != nil {
		projectScope = scope.ProjectSlug
	}
	msg := fmt.Sprintf("artifact %q not found in project %q", raw, projectScope)
	if ref.LooksLikeShareURL {
		if ref.Value != "" {
			msg += fmt.Sprintf("; input looks like a share URL, retry with only the extracted slug %q if this project scope is intended", ref.Value)
		} else {
			msg += "; input looks like a share URL, extract the wiki slug and retry with id_or_slug"
		}
	}
	return errors.New(msg)
}
