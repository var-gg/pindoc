package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/i18n"
)

// ValidArtifactTypes are the types Phase 2 accepts. Tier A (7) + Tier B
// Web-SaaS pack (4). When a Tier B pack is activated for a project in
// Phase 4+ this set becomes project-scoped, but for M1 a single flat
// whitelist is enough.
var validArtifactTypes = map[string]struct{}{
	// Tier A core
	"Decision": {}, "Analysis": {}, "Debug": {}, "Flow": {},
	"Task": {}, "TC": {}, "Glossary": {},
	// Tier A — task verification (migration 0013).
	// A VerificationReport is a typed artifact a *verifier* agent files to
	// move a Task from claimed_done → verified via pindoc.artifact.verify.
	// Kept as a first-class type (rather than task_meta subfield) so re-
	// verification chains flow naturally through supersede and the Reader
	// can surface the report as its own document.
	"VerificationReport": {},
	// Tier B Web SaaS
	"Feature": {}, "APIEndpoint": {}, "Screen": {}, "DataModel": {},
}

type artifactProposeInput struct {
	Type          string   `json:"type" jsonschema:"one of Decision|Analysis|Debug|Flow|Task|TC|Glossary|Feature|APIEndpoint|Screen|DataModel"`
	AreaSlug      string   `json:"area_slug" jsonschema:"slug from pindoc.area.list; use 'misc' or '_unsorted' if unsure"`
	Title         string   `json:"title"`
	BodyMarkdown  string   `json:"body_markdown" jsonschema:"main content in markdown"`
	Slug          string   `json:"slug,omitempty" jsonschema:"optional; auto-generated from title if absent"`
	Tags          []string `json:"tags,omitempty"`
	Completeness  string   `json:"completeness,omitempty" jsonschema:"draft|partial|settled; default partial"`
	AuthorID      string   `json:"author_id" jsonschema:"'claude-code', 'cursor', 'codex', etc."`
	AuthorVersion string   `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`

	// UpdateOf switches this call from "create a new artifact" to "append a
	// revision to an existing one". Accepts artifact UUID, project-scoped
	// slug, or pindoc://slug URL. When set, exact-title conflict is
	// skipped and a new artifact_revisions row is written.
	UpdateOf string `json:"update_of,omitempty" jsonschema:"id, slug, or pindoc:// URL of the artifact to revise"`

	// CommitMsg is a short one-liner stored on the revision row so diff
	// views and history lists can explain why the body changed. Required
	// when update_of is set; ignored otherwise.
	CommitMsg string `json:"commit_msg,omitempty" jsonschema:"required for updates; one line rationale"`

	// ExpectedVersion gates an update_of call against concurrent revision
	// writers. When update_of is set the server requires this field
	// (NEED_VER) and then compares the value to the artifact's current
	// max(revision_number); mismatch → not_ready with VER_CONFLICT.
	// Pointer type so a legitimate zero value (seeded artifact with no
	// revision rows yet) is distinguishable from "field omitted."
	ExpectedVersion *int `json:"expected_version,omitempty" jsonschema:"required for update_of; current revision number (use 0 for freshly seeded artifacts)"`

	// SupersedeOf marks the target artifact as superseded by this new one.
	// Creates a NEW artifact (like a no-update_of call), then flips the
	// target's status to 'superseded' and sets superseded_by to the new id.
	// Different from update_of: update appends a revision to the same
	// artifact; supersede creates a replacement and archives the old one.
	SupersedeOf string `json:"supersede_of,omitempty" jsonschema:"id, slug, or pindoc:// URL of the artifact being replaced"`

	// Pins attach code references to the artifact. All optional — the
	// server stores whatever is provided. path is the only required field
	// in each pin (enforced by DB check). Phase 11a stores them; stale
	// detection (comparing commit_sha to current HEAD) lands V1.x.
	Pins []ArtifactPinInput `json:"pins,omitempty" jsonschema:"code references tying this artifact to files/commits"`

	// RelatesTo records typed edges to other artifacts in the same project.
	// Valid relations: implements | references | blocks | relates_to.
	// Target may be id, slug, or pindoc:// URL — resolved server-side.
	// Unknown targets fail the whole call with RELATES_TARGET_NOT_FOUND.
	RelatesTo []ArtifactRelationInput `json:"relates_to,omitempty" jsonschema:"typed edges to other artifacts"`

	// ArtifactMeta carries epistemic axes that classify the artifact's
	// trustworthiness and memory scope. All fields optional — server
	// resolves defaults via resolveArtifactMeta based on pins, update path,
	// and body heuristics. See docs/04-data-model.md for axis definitions.
	//
	// Axes:
	//   source_type         — code | artifact | user_chat | external | mixed
	//   consent_state       — not_needed | requested | granted | denied
	//   confidence          — low | medium | high
	//   audience            — owner_only | approvers | project_readers
	//   next_context_policy — default | opt_in | excluded
	//   verification_state  — verified | partially_verified | unverified
	//
	// On update_of the supplied meta MERGES with existing meta (unset keys
	// keep previous values). Fully replacing requires explicit empty strings
	// which resolveArtifactMeta treats as "caller cleared this axis".
	ArtifactMeta *ArtifactMetaInput `json:"artifact_meta,omitempty" jsonschema:"epistemic axes — source_type, consent_state, confidence, audience, next_context_policy, verification_state (all optional)"`

	// TaskMeta carries typed tracker dimensions for type=Task artifacts
	// (Phase 15b). Ignored for any other type. All fields optional:
	//
	//   status      — todo | in_progress | blocked | done | cancelled
	//   priority    — p0 | p1 | p2 | p3
	//   assignee    — free-form string; e.g. "@alice", "agent:claude-code"
	//   due_at      — RFC3339 timestamp
	//   parent_slug — another Task artifact's slug (for epic→task→subtask)
	//
	// On update_of path, TaskMeta (when present) REPLACES the previous
	// task_meta entirely — there is no merge. Agents that want to change
	// one field must include the full desired state.
	TaskMeta *TaskMetaInput `json:"task_meta,omitempty" jsonschema:"tracker dims for Task artifacts"`

	// Basis records the evidence the agent gathered before proposing.
	// Phase 11b makes basis.search_receipt REQUIRED on the create path
	// (new artifact, no update_of, no supersede_of): the server refuses
	// the write with NO_SRCH if a valid receipt from artifact.search or
	// context.for_task isn't provided. Update/supersede paths skip the
	// gate because reading/targeting an existing artifact is already
	// proof of context.
	Basis *artifactProposeBasis `json:"basis,omitempty"`
}

type artifactProposeBasis struct {
	// SearchReceipt is the opaque token returned by artifact.search or
	// context.for_task in the same session. TTL 10 minutes.
	SearchReceipt string `json:"search_receipt,omitempty" jsonschema:"receipt from artifact.search or context.for_task"`
	// SourceSession is a free-form string identifying the agent session
	// that produced this artifact — stored on the revision row for
	// audit. Not validated.
	SourceSession string `json:"source_session,omitempty"`
}

// ArtifactPinInput is the agent-facing shape for a single pin. `path` is
// always mandatory (DB CHECK); the other fields depend on `kind`:
//
//	kind="code" (default) — repo, commit_sha, path (file path),
//	                        lines_start/lines_end. Phase 11a original.
//	kind="resource"       — path holds a typed resource reference like
//	                        "aws:vpc:vpc-0c6bff25" or "k8s:ns:pod-123";
//	                        repo/commit/lines are ignored.
//	kind="url"            — path holds an absolute URL ("https://…");
//	                        repo/commit/lines are ignored.
//
// Agents that don't set kind get "code" — preserves all Phase 11a call
// sites without rewrite.
type ArtifactPinInput struct {
	Kind       string `json:"kind,omitempty" jsonschema:"one of code | resource | url; default code"`
	Repo       string `json:"repo,omitempty" jsonschema:"'origin' default; named remote when multi-repo; code kind only"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"code kind only"`
	Path       string `json:"path" jsonschema:"code: file path; resource: typed resource ref; url: absolute URL"`
	LinesStart int    `json:"lines_start,omitempty" jsonschema:"code kind only"`
	LinesEnd   int    `json:"lines_end,omitempty" jsonschema:"code kind only"`
}

var validPinKinds = map[string]struct{}{
	"code": {}, "resource": {}, "url": {},
}

// TaskMetaInput is the agent-facing shape for a Task artifact's tracker
// dimensions. Every field is optional; the server stores what's provided.
type TaskMetaInput struct {
	// Status is the Task lifecycle v2 enum (migration 0013). Agents set
	// `claimed_done` themselves once acceptance criteria land, but
	// `verified` is controlled exclusively by pindoc.artifact.verify —
	// direct transitions via artifact.propose are rejected (VER_VIA_VERIFY_TOOL_ONLY).
	Status     string `json:"status,omitempty" jsonschema:"open | claimed_done | verified | blocked | cancelled (set via pindoc.artifact.verify, not here)"`
	Priority   string `json:"priority,omitempty" jsonschema:"p0 | p1 | p2 | p3"`
	Assignee   string `json:"assignee,omitempty"`
	DueAt      string `json:"due_at,omitempty" jsonschema:"RFC3339 timestamp"`
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"slug of parent Task artifact"`
}

// validTaskStatuses was rebuilt in migration 0013 around a two-phase
// completion model: agent self-attests claimed_done, then a *different*
// agent files a VerificationReport to reach verified. See
// docs/04-data-model.md (Task status v2 section).
var validTaskStatuses = map[string]struct{}{
	"open": {}, "claimed_done": {}, "verified": {},
	"blocked": {}, "cancelled": {},
}
var validTaskPriorities = map[string]struct{}{
	"p0": {}, "p1": {}, "p2": {}, "p3": {},
}

// ArtifactMetaInput is the agent-facing shape for epistemic axes. Every
// field is optional; resolveArtifactMeta fills defaults based on pins,
// update path, and body heuristics.
type ArtifactMetaInput struct {
	SourceType        string `json:"source_type,omitempty" jsonschema:"code | artifact | user_chat | external | mixed"`
	ConsentState      string `json:"consent_state,omitempty" jsonschema:"not_needed | requested | granted | denied"`
	Confidence        string `json:"confidence,omitempty" jsonschema:"low | medium | high"`
	Audience          string `json:"audience,omitempty" jsonschema:"owner_only | approvers | project_readers"`
	NextContextPolicy string `json:"next_context_policy,omitempty" jsonschema:"default | opt_in | excluded"`
	VerificationState string `json:"verification_state,omitempty" jsonschema:"verified | partially_verified | unverified"`
}

var validSourceTypes = map[string]struct{}{
	"code": {}, "artifact": {}, "user_chat": {}, "external": {}, "mixed": {},
}
var validConsentStates = map[string]struct{}{
	"not_needed": {}, "requested": {}, "granted": {}, "denied": {},
}
var validConfidences = map[string]struct{}{
	"low": {}, "medium": {}, "high": {},
}
var validAudiences = map[string]struct{}{
	"owner_only": {}, "approvers": {}, "project_readers": {},
}
var validNextContextPolicies = map[string]struct{}{
	"default": {}, "opt_in": {}, "excluded": {},
}
var validVerificationStates = map[string]struct{}{
	"verified": {}, "partially_verified": {}, "unverified": {},
}

// ArtifactRelationInput is the agent-facing shape for one edge.
type ArtifactRelationInput struct {
	TargetID string `json:"target_id" jsonschema:"id, slug, or pindoc:// URL of the related artifact"`
	Relation string `json:"relation" jsonschema:"one of implements|references|blocks|relates_to"`
}

var validRelations = map[string]struct{}{
	"implements": {}, "references": {}, "blocks": {}, "relates_to": {},
}

type artifactProposeOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`
	// Failed is the Phase 12-style stable code list. Populated alongside
	// the legacy natural-language Checklist during Phase 11a so agents can
	// start branching on codes now; Checklist becomes optional in Phase 12.
	Failed           []string `json:"failed,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	// Only set on Status == "accepted".
	ArtifactID string `json:"artifact_id,omitempty"`
	Slug       string `json:"slug,omitempty"`
	// AgentRef is the pindoc://<slug> URL an agent re-feeds to
	// artifact.read or embeds in other artifact bodies. Stable across UI
	// route changes.
	AgentRef string `json:"agent_ref,omitempty"`
	// HumanURL is the /p/:project/wiki/:slug path an agent pastes into
	// chat so the user can open the Reader in a browser. Relative because
	// the external origin belongs to the user's deployment.
	HumanURL string `json:"human_url,omitempty"`
	// HumanURLAbs is the absolute URL when server_settings.public_base_url
	// is configured. Empty otherwise. Agents should prefer HumanURLAbs
	// for out-of-context shares (chat / PR / email) and fall back to
	// HumanURL only for same-origin contexts.
	HumanURLAbs    string    `json:"human_url_abs,omitempty"`
	PublishedAt    time.Time `json:"published_at,omitzero"`
	Created        bool      `json:"created"`         // false on updates
	RevisionNumber int       `json:"revision_number"` // 1 on create, N+1 on update

	// Phase 11a: surface what was actually persisted so agents get
	// confirmation of edge/pin storage without a second read.
	PinsStored  int  `json:"pins_stored,omitempty"`
	EdgesStored int  `json:"edges_stored,omitempty"`
	Superseded  bool `json:"superseded,omitempty"` // true if supersede_of was processed

	// Phase 12a: machine-readable continuation hints. NextTools lists the
	// MCP tools the agent should call next to satisfy the failing gate;
	// Related lists artifacts/resources the agent should read to understand
	// why the gate fired. Populated alongside the legacy Checklist/
	// SuggestedActions pair so agents can branch on codes without parsing
	// natural language.
	NextTools []string     `json:"next_tools,omitempty"`
	Related   []RelatedRef `json:"related,omitempty"`
	// PatchableFields (Phase 14b) tells the agent which input fields to
	// change for the retry. Empty = full input needs rework. Maps stable
	// fail codes to the minimum patch surface so agents don't resend
	// entire propose bodies they didn't need to touch.
	PatchableFields []string `json:"patchable_fields,omitempty"`
	// Warnings (Phase 14b) surface advisory flags on otherwise-accepted
	// writes. Current set: RECOMMEND_READ_BEFORE_CREATE when a create
	// passed but a semantic close match existed — the agent did not read
	// it. Agents should log/surface; not a block.
	Warnings []string `json:"warnings,omitempty"`
	// EmbedderUsed (Phase 17 follow-up) echoes which provider served the
	// semantic-conflict check + chunk embedding so the agent can detect
	// silent stub fallback. Empty when the embedder wasn't touched (e.g.
	// pure not_ready on schema validation).
	EmbedderUsed *EmbedderInfo `json:"embedder_used,omitempty"`
	// ArtifactMeta echoes the resolved epistemic axes that were persisted.
	// Populated on accepted paths so agents (and Reader) can confirm which
	// classification actually landed — resolver defaults may override
	// agent-supplied values when a stronger signal was present (e.g. code
	// pins upgrading source_type). Absent on not_ready responses.
	ArtifactMeta *ResolvedArtifactMeta `json:"artifact_meta,omitempty"`
	// CanonicalRewriteWithoutEvidence is true when the update path rewrote
	// a type-specific canonical claim section (Debug.Root cause,
	// Decision.Decision, Analysis.Conclusion) without fresh evidence (new
	// pins, verification_state bump past unverified, or commit_msg
	// evidence keyword). Reader revision badges consume this flag for an
	// "uncertain rewrite" marker. Always false on create paths.
	CanonicalRewriteWithoutEvidence bool `json:"canonical_rewrite_without_evidence,omitempty"`
}

// RelatedRef is a compact pointer to another artifact the caller should
// read. Both agent_ref and human_url are always populated so the agent
// re-feeds one and shares the other with the user.
type RelatedRef struct {
	ID          string `json:"id,omitempty"`
	Slug        string `json:"slug"`
	Type        string `json:"type,omitempty"`
	Title       string `json:"title,omitempty"`
	AgentRef    string `json:"agent_ref"`
	HumanURL    string `json:"human_url"`
	HumanURLAbs string `json:"human_url_abs,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// RegisterArtifactPropose wires pindoc.artifact.propose — the only write
// tool in Phase 2. Implements the Pre-flight Check mechanism (M0.5 in
// docs/05): on failing checks the tool returns Status=not_ready with a
// checklist telling the agent what to fix, not a hard error. The agent
// re-submits after addressing the checklist.
//
// Accepted propose calls auto-publish (review_state=auto_published,
// status=published). Review Queue (sensitive ops) lands in Phase 2.x+.
func RegisterArtifactPropose(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.artifact.propose",
			Description: "Propose a new artifact (the only write path humans use — always via an agent). Returns Status=accepted + artifact_id on success, or Status=not_ready + checklist + suggested_actions if Pre-flight fails. Always read the checklist; never surface the raw error to the user without trying the suggested actions first.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactProposeInput) (*sdk.CallToolResult, artifactProposeOutput, error) {
			// --- Pre-flight ----------------------------------------------
			lang := deps.UserLanguage
			checklist, failed, code := preflight(&in, lang)
			if len(checklist) > 0 {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: code,
					Failed:    failed,
					Checklist: checklist,
					SuggestedActions: []string{
						i18n.T(lang, "suggested.fix_all"),
						i18n.T(lang, "suggested.confirm_types"),
						i18n.T(lang, "suggested.use_misc"),
					},
					NextTools:       defaultNextTools(code),
					PatchableFields: patchFieldsFor(code),
				}, nil
			}

			// Mutual exclusion: update_of and supersede_of are two different
			// revisions-of-truth paths. Agent must pick one.
			if strings.TrimSpace(in.UpdateOf) != "" && strings.TrimSpace(in.SupersedeOf) != "" {
				return nil, artifactProposeOutput{
					Status:           "not_ready",
					ErrorCode:        "UPDATE_SUPERSEDE_EXCLUSIVE",
					Failed:           []string{"UPDATE_SUPERSEDE_EXCLUSIVE"},
					Checklist:        []string{i18n.T(lang, "preflight.update_supersede_exclusive")},
					SuggestedActions: []string{i18n.T(lang, "suggested.pick_one_mode")},
					NextTools:        defaultNextTools("UPDATE_SUPERSEDE_EXCLUSIVE"),
					PatchableFields:  patchFieldsFor("UPDATE_SUPERSEDE_EXCLUSIVE"),
				}, nil
			}

			// --- Update path (update_of set) -----------------------------
			if strings.TrimSpace(in.UpdateOf) != "" {
				return handleUpdate(ctx, deps, in, lang)
			}

			// --- Supersede path (supersede_of set) -----------------------
			// Creates a fresh artifact via the same insert flow as "new",
			// then flips the target artifact's status to 'superseded' and
			// writes superseded_by. We reuse the create path below and do
			// the supersede bookkeeping just before commit.

			// --- Resolve area + project ----------------------------------
			var projectID, areaID string
			err := deps.DB.QueryRow(ctx, `
				SELECT proj.id::text, area.id::text
				FROM projects proj
				JOIN areas area ON area.project_id = proj.id
				WHERE proj.slug = $1 AND area.slug = $2
			`, deps.ProjectSlug, in.AreaSlug).Scan(&projectID, &areaID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: "AREA_NOT_FOUND",
					Failed:    []string{"AREA_NOT_FOUND"},
					Checklist: []string{
						fmt.Sprintf(i18n.T(lang, "preflight.area_not_found"), in.AreaSlug, deps.ProjectSlug),
					},
					SuggestedActions: []string{
						i18n.T(lang, "suggested.list_areas"),
						i18n.T(lang, "suggested.area_or_misc"),
					},
					NextTools:       defaultNextTools("AREA_NOT_FOUND"),
					PatchableFields: patchFieldsFor("AREA_NOT_FOUND"),
				}, nil
			}
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("resolve scope: %w", err)
			}

			// --- search_receipt gate (Phase 11b) -------------------------
			// Create path requires a valid receipt. Update/supersede bypass
			// the gate because they already depend on an existing artifact
			// (read/target proves context). Unset receipts store disables
			// the gate entirely (test fixtures).
			isCreatePath := strings.TrimSpace(in.UpdateOf) == "" && strings.TrimSpace(in.SupersedeOf) == ""
			if isCreatePath && deps.Receipts != nil {
				receipt := ""
				if in.Basis != nil {
					receipt = strings.TrimSpace(in.Basis.SearchReceipt)
				}
				if receipt == "" {
					return nil, artifactProposeOutput{
						Status:           "not_ready",
						ErrorCode:        "NO_SRCH",
						Failed:           []string{"NO_SRCH"},
						Checklist:        []string{i18n.T(lang, "preflight.no_search_receipt")},
						SuggestedActions: []string{i18n.T(lang, "suggested.call_search_first")},
						NextTools:        defaultNextTools("NO_SRCH"),
						PatchableFields:  patchFieldsFor("NO_SRCH"),
					}, nil
				}
				res := deps.Receipts.Verify(receipt, deps.ProjectSlug)
				switch {
				case res.Unknown:
					return nil, artifactProposeOutput{
						Status:           "not_ready",
						ErrorCode:        "RECEIPT_UNKNOWN",
						Failed:           []string{"RECEIPT_UNKNOWN"},
						Checklist:        []string{i18n.T(lang, "preflight.receipt_unknown")},
						SuggestedActions: []string{i18n.T(lang, "suggested.call_search_first")},
						NextTools:        defaultNextTools("RECEIPT_UNKNOWN"),
						PatchableFields:  patchFieldsFor("RECEIPT_UNKNOWN"),
					}, nil
				case res.Expired:
					return nil, artifactProposeOutput{
						Status:           "not_ready",
						ErrorCode:        "RECEIPT_EXPIRED",
						Failed:           []string{"RECEIPT_EXPIRED"},
						Checklist:        []string{i18n.T(lang, "preflight.receipt_expired")},
						SuggestedActions: []string{i18n.T(lang, "suggested.call_search_first")},
						NextTools:        defaultNextTools("RECEIPT_EXPIRED"),
						PatchableFields:  patchFieldsFor("RECEIPT_EXPIRED"),
					}, nil
				case res.WrongProject:
					return nil, artifactProposeOutput{
						Status:           "not_ready",
						ErrorCode:        "RECEIPT_WRONG_PROJECT",
						Failed:           []string{"RECEIPT_WRONG_PROJECT"},
						Checklist:        []string{i18n.T(lang, "preflight.receipt_wrong_project")},
						SuggestedActions: []string{i18n.T(lang, "suggested.call_search_first")},
						NextTools:        defaultNextTools("RECEIPT_WRONG_PROJECT"),
						PatchableFields:  patchFieldsFor("RECEIPT_WRONG_PROJECT"),
					}, nil
				}
			}

			// --- Resolve supersede_of target first (if set) --------------
			// The supersede path creates a new artifact and flips the old
			// one's status to 'superseded'. We resolve the target upfront
			// so we can skip the exact-title conflict check when the
			// replacement keeps the same title (common pattern).
			var supersedeTargetID string
			if strings.TrimSpace(in.SupersedeOf) != "" {
				ref := normalizeRef(in.SupersedeOf)
				err := deps.DB.QueryRow(ctx, `
					SELECT a.id::text FROM artifacts a
					JOIN projects p ON p.id = a.project_id
					WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
					  AND a.status <> 'archived'
					LIMIT 1
				`, deps.ProjectSlug, ref).Scan(&supersedeTargetID)
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, artifactProposeOutput{
						Status:    "not_ready",
						ErrorCode: "SUPERSEDE_TARGET_NOT_FOUND",
						Failed:    []string{"SUPERSEDE_TARGET_NOT_FOUND"},
						Checklist: []string{
							fmt.Sprintf(i18n.T(lang, "preflight.supersede_target_missing"), in.SupersedeOf),
						},
						NextTools:       defaultNextTools("SUPERSEDE_TARGET_NOT_FOUND"),
						PatchableFields: patchFieldsFor("SUPERSEDE_TARGET_NOT_FOUND"),
					}, nil
				}
				if err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("resolve supersede target: %w", err)
				}
			}

			// --- Exact-title conflict check (embedding-based lands Phase 3) ---
			// Skipped when superseding: the replacement commonly keeps the
			// same title; the old one is about to be archived anyway, so it
			// would no longer match status<>'archived' either.
			if supersedeTargetID == "" {
				var existingID, existingSlug string
				err = deps.DB.QueryRow(ctx, `
					SELECT id::text, slug FROM artifacts
					WHERE project_id = $1
					  AND lower(title) = lower($2)
					  AND status <> 'archived'
					LIMIT 1
				`, projectID, in.Title).Scan(&existingID, &existingSlug)
				if err == nil {
					return nil, artifactProposeOutput{
						Status:    "not_ready",
						ErrorCode: "CONFLICT_EXACT_TITLE",
						Failed:    []string{"CONFLICT_EXACT_TITLE"},
						Checklist: []string{
							fmt.Sprintf(i18n.T(lang, "preflight.conflict_exact"), existingID, existingSlug),
						},
						SuggestedActions: []string{
							fmt.Sprintf(i18n.T(lang, "suggested.update_of_hint"), existingSlug),
							fmt.Sprintf(i18n.T(lang, "suggested.read_existing"), existingSlug),
							i18n.T(lang, "suggested.pick_title"),
						},
						NextTools:       defaultNextTools("CONFLICT_EXACT_TITLE"),
						PatchableFields: patchFieldsFor("CONFLICT_EXACT_TITLE"),
						Related: []RelatedRef{
							makeRelated(deps, existingSlug, existingID, "", in.Title, "exact title match"),
						},
					}, nil
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return nil, artifactProposeOutput{}, fmt.Errorf("conflict check: %w", err)
				}

				// --- Semantic conflict (Phase 11b) ------------------------
				// Embed the proposed title + first body slice, vector-search
				// the existing corpus, block if top hit is suspiciously close.
				// Threshold 0.18 cosine distance roughly corresponds to "this
				// is a near-duplicate of an existing artifact in the same
				// embedding space". Only gates when provider is non-stub —
				// stub hash-ranking would false-positive everything.
				if deps.Embedder != nil && deps.Embedder.Info().Name != "stub" {
					candidates, err := findSemanticConflicts(ctx, deps, projectID, in.Title, in.BodyMarkdown)
					if err != nil {
						deps.Logger.Warn("semantic conflict check failed — skipping gate", "err", err)
					} else if len(candidates) > 0 {
						rel := make([]string, 0, len(candidates))
						related := make([]RelatedRef, 0, len(candidates))
						for _, c := range candidates {
							rel = append(rel, fmt.Sprintf("[%s] %s — /p/%s/wiki/%s (distance %.3f)", c.Type, c.Title, deps.ProjectSlug, c.Slug, c.Distance))
							related = append(related, makeRelated(
								deps, c.Slug, c.ArtifactID, c.Type, c.Title,
								fmt.Sprintf("cosine distance %.3f", c.Distance),
							))
						}
						return nil, artifactProposeOutput{
							Status:    "not_ready",
							ErrorCode: "POSSIBLE_DUP",
							Failed:    []string{"POSSIBLE_DUP"},
							Checklist: []string{
								fmt.Sprintf(i18n.T(lang, "preflight.possible_dup"), candidates[0].Slug, candidates[0].Distance),
							},
							SuggestedActions: append(
								[]string{i18n.T(lang, "suggested.read_similar")},
								rel...,
							),
							NextTools:       defaultNextTools("POSSIBLE_DUP"),
							PatchableFields: patchFieldsFor("POSSIBLE_DUP"),
							Related:         related,
						}, nil
					}
				}
			}

			// --- Slug: either the explicit one or a generated one. Retry on
			//     unique-constraint violation with a -N suffix until we settle.
			baseSlug := in.Slug
			if baseSlug == "" {
				baseSlug = slugify(in.Title)
			}
			if baseSlug == "" {
				baseSlug = strings.ToLower(in.Type) + "-" + time.Now().UTC().Format("20060102150405")
			}

			// Resolve artifact_meta and classify conversation-derived writes
			// BEFORE committing to completeness — consent-granted chat writes
			// step down to draft by default (see Task
			// `conversation-derived-write-기본-draft-라우팅-...`).
			resolvedMeta := resolveArtifactMeta(in.ArtifactMeta, in.Pins, in.BodyMarkdown, false)
			isConvDerived := classifyConversationDerived(&in)
			completeness := applyConversationDerivedDefaults(&in, &resolvedMeta, in.Completeness)
			if completeness == "" {
				completeness = "partial"
			}
			if in.Tags == nil {
				in.Tags = []string{}
			}

			// --- INSERT + event in one tx --------------------------------
			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			finalSlug := baseSlug
			var newID string
			var publishedAt time.Time
			taskMetaJSON := taskMetaToJSON(in.Type, in.TaskMeta)
			artifactMetaJSON := artifactMetaToJSON(resolvedMeta)
			for attempt := 0; attempt < 10; attempt++ {
				err = tx.QueryRow(ctx, `
					INSERT INTO artifacts (
						project_id, area_id, slug, type, title, body_markdown, tags,
						completeness, status, review_state,
						author_kind, author_id, author_version, author_user_id,
						task_meta, artifact_meta, published_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'published', 'auto_published', 'agent', $9, $10, NULLIF($11, '')::uuid, $12, $13::jsonb, now())
					RETURNING id::text, published_at
				`, projectID, areaID, finalSlug, in.Type, in.Title, in.BodyMarkdown, in.Tags,
					completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion), deps.UserID, taskMetaJSON, artifactMetaJSON).Scan(&newID, &publishedAt)
				if err == nil {
					break
				}
				if isUniqueViolation(err, "artifacts_project_id_slug_key") {
					finalSlug = fmt.Sprintf("%s-%d", baseSlug, attempt+2)
					continue
				}
				return nil, artifactProposeOutput{}, fmt.Errorf("insert: %w", err)
			}
			if newID == "" {
				return nil, artifactProposeOutput{}, errors.New("could not allocate a unique slug after 10 attempts")
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1, 'artifact.published', $2, jsonb_build_object(
					'area_slug', $3::text,
					'type', $4::text,
					'slug', $5::text,
					'author_id', $6::text
				))
			`, projectID, newID, in.AreaSlug, in.Type, finalSlug, in.AuthorID); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("event insert: %w", err)
			}

			// Embed title + body chunks in the same transaction so search
			// never observes a half-indexed artifact. If the embedder fails
			// we still keep the artifact — search becomes keyword-only for
			// that row until re-embedding lands in Phase 3.x.
			if deps.Embedder != nil {
				if err := embedAndStoreChunks(ctx, tx, deps.Embedder, newID, in.Title, in.BodyMarkdown); err != nil {
					deps.Logger.Warn("chunk/embed failed — artifact saved without vectors",
						"artifact_id", newID, "err", err)
				}
			}

			// --- relates_to: resolve targets, then insert edges ----------
			relTargets, relErr := resolveRelatesTo(ctx, tx, deps.ProjectSlug, in.RelatesTo, lang)
			if relErr != nil {
				return nil, *relErr, nil
			}
			edgesStored, err := insertEdges(ctx, tx, newID, relTargets, in.RelatesTo)
			if err != nil {
				return nil, artifactProposeOutput{}, err
			}

			// --- pins ---------------------------------------------------
			pinsStored, err := insertPins(ctx, tx, newID, in.Pins)
			if err != nil {
				return nil, artifactProposeOutput{}, err
			}

			// --- supersede bookkeeping: archive target + record edge ----
			supersededFlag := false
			if supersedeTargetID != "" {
				if _, err := tx.Exec(ctx, `
					UPDATE artifacts
					   SET status         = 'superseded',
					       superseded_by  = $2::uuid,
					       updated_at     = now()
					 WHERE id = $1
				`, supersedeTargetID, newID); err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("supersede update: %w", err)
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO events (project_id, kind, subject_id, payload)
					VALUES ($1, 'artifact.superseded', $2::uuid, jsonb_build_object(
						'superseded_by', $3::text,
						'author_id', $4::text
					))
				`, projectID, supersedeTargetID, newID, in.AuthorID); err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("supersede event: %w", err)
				}
				supersededFlag = true
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
			}

			// First revision — keep the invariant that every artifact has
			// at least one artifact_revisions row. Done outside the tx
			// (commit just happened) which is safe because a missing
			// revision row still has the artifact intact; a background
			// backfill (future) can repair it.
			if _, err := deps.DB.Exec(ctx, `
				INSERT INTO artifact_revisions (
					artifact_id, revision_number, title, body_markdown, body_hash, tags,
					completeness, author_kind, author_id, author_version, commit_msg,
					source_session_ref
				) VALUES ($1, 1, $2, $3, $4, $5, $6, 'agent', $7, $8, 'initial', $9)
			`, newID, in.Title, in.BodyMarkdown, bodyHash(in.BodyMarkdown), in.Tags,
				completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion),
				buildSourceSessionRef(deps, in),
			); err != nil {
				deps.Logger.Warn("initial revision insert failed — head row still present",
					"artifact_id", newID, "err", err)
			}

			warnings := createWarnings(ctx, deps, projectID, in.Title, in.BodyMarkdown)
			warnings = append(warnings, pinPathWarnings(deps, in.Pins)...)
			warnings = append(warnings, titleLengthWarnings(in.Title)...)
			warnings = append(warnings, bodyH1Warnings(in.BodyMarkdown)...)
			warnings = append(warnings, requiredH2Warnings(in.BodyMarkdown, in.Type)...)
			if detectUnclassifiedUserChat(resolvedMeta, in.Pins, in.BodyMarkdown) {
				warnings = append(warnings, "SOURCE_TYPE_UNCLASSIFIED")
			}
			if isConvDerived && resolvedMeta.ConsentState == "" {
				warnings = append(warnings, "CONSENT_REQUIRED_FOR_USER_CHAT")
			}
			metaOut := resolvedMeta
			return nil, artifactProposeOutput{
				Status:         "accepted",
				ArtifactID:     newID,
				Slug:           finalSlug,
				AgentRef:       "pindoc://" + finalSlug,
				HumanURL:       HumanURL(deps.ProjectSlug, finalSlug),
				HumanURLAbs:    AbsHumanURL(deps.Settings, deps.ProjectSlug, finalSlug),
				PublishedAt:    publishedAt,
				Created:        true,
				RevisionNumber: 1,
				PinsStored:     pinsStored,
				EdgesStored:    edgesStored,
				Superseded:     supersededFlag,
				Warnings:       warnings,
				EmbedderUsed:   embedderInfo(deps),
				ArtifactMeta:   &metaOut,
			}, nil
		},
	)
}

// handleUpdate writes a new revision for an existing artifact, updates the
// head row, re-chunks embeddings, and emits an event. Runs in a single
// transaction so search never sees a half-indexed update.
func handleUpdate(ctx context.Context, deps Deps, in artifactProposeInput, lang string) (*sdk.CallToolResult, artifactProposeOutput, error) {
	if strings.TrimSpace(in.CommitMsg) == "" {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "MISSING_COMMIT_MSG",
			Failed:    []string{"MISSING_COMMIT_MSG"},
			Checklist: []string{i18n.T(lang, "preflight.update_needs_commit")},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.commit_msg_hint"),
			},
			PatchableFields: patchFieldsFor("MISSING_COMMIT_MSG"),
		}, nil
	}

	ref := normalizeRef(in.UpdateOf)

	var artifactID, projectID, currentBody, currentTitle, currentType string
	var currentMetaRaw []byte
	var lastRev int
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.body_markdown, a.title, a.type, a.artifact_meta,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, deps.ProjectSlug, ref).Scan(&artifactID, &projectID, &currentBody, &currentTitle, &currentType, &currentMetaRaw, &lastRev)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "UPDATE_TARGET_NOT_FOUND",
			Failed:    []string{"UPDATE_TARGET_NOT_FOUND"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.update_target_missing"), in.UpdateOf),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.list_areas"),
			},
			NextTools:       defaultNextTools("UPDATE_TARGET_NOT_FOUND"),
			PatchableFields: patchFieldsFor("UPDATE_TARGET_NOT_FOUND"),
		}, nil
	}
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("resolve update target: %w", err)
	}

	// Phase 14b: expected_version is HARD REQUIRED on the update path.
	// Absence = NEED_VER. Rationale: reading the artifact to discover the
	// current version is an implicit "agent read this target before
	// updating it" gate. Without the requirement an over-confident agent
	// can overwrite stale context.
	if in.ExpectedVersion == nil {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "NEED_VER",
			Failed:    []string{"NEED_VER"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.need_ver"), lastRev),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.reread_before_update"),
			},
			NextTools:       defaultNextTools("UPDATE_TARGET_NOT_FOUND"), // artifact.revisions / artifact.search
			PatchableFields: patchFieldsFor("NEED_VER"),
			Related: []RelatedRef{
				makeRelated(deps, ref, artifactID, "", currentTitle, fmt.Sprintf("current revision = %d; pass expected_version = %d", lastRev, lastRev)),
			},
		}, nil
	}

	// Optimistic lock: version provided but stale → VER_CONFLICT.
	if *in.ExpectedVersion != lastRev {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "VER_CONFLICT",
			Failed:    []string{"VER_CONFLICT"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.ver_conflict"), *in.ExpectedVersion, lastRev),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.reread_before_update"),
			},
			NextTools:       defaultNextTools("VER_CONFLICT"),
			PatchableFields: patchFieldsFor("VER_CONFLICT"),
			Related: []RelatedRef{
				makeRelated(deps, ref, artifactID, "", currentTitle, fmt.Sprintf("current revision = %d, not %d", lastRev, *in.ExpectedVersion)),
			},
		}, nil
	}

	// No-op detection: identical body + title → reject so history stays
	// clean. Agents that hit this should stop retrying.
	if currentBody == in.BodyMarkdown && currentTitle == in.Title {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "NO_CHANGES",
			Failed:    []string{"NO_CHANGES"},
			Checklist: []string{i18n.T(lang, "preflight.no_changes")},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.verify_diff"),
			},
			PatchableFields: []string{"body_markdown", "title"},
		}, nil
	}

	// Area is preserved on update — moving across areas is a supersede,
	// not a revision. Treat area_slug as a reconfirm-of-current.
	var areaID string
	if err := deps.DB.QueryRow(ctx, `
		SELECT area.id::text FROM areas area
		JOIN projects p ON p.id = area.project_id
		WHERE p.slug = $1 AND area.slug = $2
	`, deps.ProjectSlug, in.AreaSlug).Scan(&areaID); err != nil {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "AREA_NOT_FOUND",
			Failed:    []string{"AREA_NOT_FOUND"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.area_not_found"), in.AreaSlug, deps.ProjectSlug),
			},
			NextTools:       defaultNextTools("AREA_NOT_FOUND"),
			PatchableFields: patchFieldsFor("AREA_NOT_FOUND"),
		}, nil
	}

	if in.Tags == nil {
		in.Tags = []string{}
	}
	completeness := in.Completeness
	if completeness == "" {
		completeness = "partial"
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	newRev := lastRev + 1
	var revID string
	err = tx.QueryRow(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, author_version, commit_msg,
			source_session_ref
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'agent', $8, $9, $10, $11)
		RETURNING id::text
	`, artifactID, newRev, in.Title, in.BodyMarkdown, bodyHash(in.BodyMarkdown),
		in.Tags, completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion), in.CommitMsg,
		buildSourceSessionRef(deps, in),
	).Scan(&revID)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("insert revision: %w", err)
	}

	var publishedAt time.Time
	var slug string
	// Decide whether to overwrite task_meta. Only overwrite when the
	// caller explicitly sent a new TaskMeta — omitting it preserves the
	// prior value so you can revise a Task's body without re-specifying
	// status/priority every time.
	taskMetaPatch := taskMetaToJSON(in.Type, in.TaskMeta)
	// artifact_meta follows the same "send-to-overwrite" rule: agents that
	// do not include artifact_meta on a revision keep the artifact's prior
	// classification. When they do send it, the resolved payload fully
	// replaces the stored JSONB — no server-side merge.
	var artifactMetaPatch any
	var resolvedUpdateMeta ResolvedArtifactMeta
	if in.ArtifactMeta != nil {
		resolvedUpdateMeta = resolveArtifactMeta(in.ArtifactMeta, in.Pins, in.BodyMarkdown, true)
		artifactMetaPatch = artifactMetaToJSON(resolvedUpdateMeta)
	}
	err = tx.QueryRow(ctx, `
		UPDATE artifacts
		   SET title          = $2,
		       body_markdown  = $3,
		       tags           = $4,
		       completeness   = $5,
		       author_id      = $6,
		       author_version = $7,
		       task_meta      = COALESCE($8, task_meta),
		       artifact_meta  = COALESCE($9::jsonb, artifact_meta),
		       updated_at     = now()
		 WHERE id = $1
		RETURNING slug, COALESCE(published_at, now())
	`, artifactID, in.Title, in.BodyMarkdown, in.Tags, completeness,
		in.AuthorID, nullIfEmpty(in.AuthorVersion), taskMetaPatch, artifactMetaPatch,
	).Scan(&slug, &publishedAt)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("update head: %w", err)
	}

	// Re-chunk: drop old chunks, generate new.
	if _, err := tx.Exec(ctx, `DELETE FROM artifact_chunks WHERE artifact_id = $1`, artifactID); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("purge chunks: %w", err)
	}
	if deps.Embedder != nil {
		if err := embedAndStoreChunks(ctx, tx, deps.Embedder, artifactID, in.Title, in.BodyMarkdown); err != nil {
			deps.Logger.Warn("re-embed failed — artifact updated without vectors",
				"artifact_id", artifactID, "err", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.revised', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'commit_msg',      $6::text
		))
	`, projectID, artifactID, newRev, slug, in.AuthorID, in.CommitMsg); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("event: %w", err)
	}

	// pins and edges are additive on update (append new pins; edges are
	// idempotent on (source, target, relation)). If an agent needs to
	// "replace" all pins they should supersede rather than update.
	relTargets, relErr := resolveRelatesTo(ctx, tx, deps.ProjectSlug, in.RelatesTo, lang)
	if relErr != nil {
		return nil, *relErr, nil
	}
	edgesStored, err := insertEdges(ctx, tx, artifactID, relTargets, in.RelatesTo)
	if err != nil {
		return nil, artifactProposeOutput{}, err
	}
	pinsStored, err := insertPins(ctx, tx, artifactID, in.Pins)
	if err != nil {
		return nil, artifactProposeOutput{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
	}

	var updateMetaOut *ResolvedArtifactMeta
	if in.ArtifactMeta != nil {
		m := resolvedUpdateMeta
		updateMetaOut = &m
	}

	// Canonical-claim rewrite guard — compare prev/new H2 sections for
	// types that carry a canonical truth claim (Debug, Decision, Analysis)
	// and require fresh evidence when that section's content shifts.
	warnings := updatePathWarnings(deps, in)
	var suggested []string
	var prevMeta ResolvedArtifactMeta
	if len(currentMetaRaw) > 0 {
		_ = json.Unmarshal(currentMetaRaw, &prevMeta)
	}
	rewrittenSections := detectCanonicalClaimRewrite(currentBody, in.BodyMarkdown, currentType)
	canonicalRewriteFlag := false
	if len(rewrittenSections) > 0 && !hasEvidenceDelta(prevMeta, &in) {
		canonicalRewriteFlag = true
		warnings = append(warnings, "CANONICAL_REWRITE_WITHOUT_EVIDENCE:"+strings.Join(rewrittenSections, "+"))
		suggested = []string{
			"If the new content is a hypothesis, file it as a fresh Analysis or Debug draft rather than rewriting the canonical claim.",
			"If verified, set artifact_meta.verification_state=verified and attach the evidence pin on the same propose.",
			"If this is wording cleanup only, mention 'wording cleanup' (or 'verified') in commit_msg so the warning stays quiet next time.",
		}
	}

	return nil, artifactProposeOutput{
		Status:                          "accepted",
		ArtifactID:                      artifactID,
		Slug:                            slug,
		AgentRef:                        "pindoc://" + slug,
		HumanURL:                        HumanURL(deps.ProjectSlug, slug),
		HumanURLAbs:                     AbsHumanURL(deps.Settings, deps.ProjectSlug, slug),
		PublishedAt:                     publishedAt,
		Created:                         false,
		RevisionNumber:                  newRev,
		PinsStored:                      pinsStored,
		EdgesStored:                     edgesStored,
		Warnings:                        warnings,
		SuggestedActions:                suggested,
		EmbedderUsed:                    embedderInfo(deps),
		ArtifactMeta:                    updateMetaOut,
		CanonicalRewriteWithoutEvidence: canonicalRewriteFlag,
	}, nil
}

// updatePathWarnings aggregates non-blocking advisories for the update
// (revision) path. Skips the semantic near-duplicate probe — updating in
// place can't produce a duplicate — but runs the structural/pin gates so
// the agent learns about title length / heading / pin-path issues on
// revised artifacts too.
func updatePathWarnings(deps Deps, in artifactProposeInput) []string {
	var out []string
	out = append(out, pinPathWarnings(deps, in.Pins)...)
	out = append(out, titleLengthWarnings(in.Title)...)
	out = append(out, bodyH1Warnings(in.BodyMarkdown)...)
	out = append(out, requiredH2Warnings(in.BodyMarkdown, in.Type)...)
	return out
}

// embedderInfo returns a pointer-typed EmbedderInfo ready for the propose
// response. Pointer-typed so omitempty drops the field entirely when the
// propose path never touched the embedder (pure schema not_ready). Called
// only on accepted paths after chunks are computed, which is where the
// field carries real information.
func embedderInfo(deps Deps) *EmbedderInfo {
	if deps.Embedder == nil {
		return nil
	}
	info := deps.Embedder.Info()
	return &EmbedderInfo{Name: info.Name, ModelID: info.ModelID, Dimension: info.Dimension}
}

func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// buildSourceSessionRef assembles the JSONB payload stored on
// artifact_revisions.source_session_ref. Fields:
//   - agent_id: server-issued identity (Phase 12c) — trusted
//   - reported_author_id: client-reported string — untrusted label
//   - source_session: agent-supplied free-form session id (basis)
//
// Returns nil when there's nothing useful to record so the column stays
// NULL rather than storing {} everywhere.
func buildSourceSessionRef(deps Deps, in artifactProposeInput) any {
	payload := map[string]any{}
	if strings.TrimSpace(deps.AgentID) != "" {
		payload["agent_id"] = deps.AgentID
	}
	if strings.TrimSpace(in.AuthorID) != "" {
		payload["reported_author_id"] = in.AuthorID
	}
	if in.Basis != nil {
		if s := strings.TrimSpace(in.Basis.SourceSession); s != "" {
			payload["source_session"] = s
		}
	}
	if len(payload) == 0 {
		return nil
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return string(buf)
}

// preflight runs the cheap synchronous checks. Returns a list of ✗-prefixed
// lines (legacy natural-language) + parallel stable-code list + short
// ErrorCode for the first failure. Empty lists mean clean.
//
// Phase 11a change: every check now emits both a natural-language line
// (legacy) AND a stable code (Phase 12-style). Phase 12 will make codes
// primary and prose optional.
func preflight(in *artifactProposeInput, lang string) (checklist []string, failed []string, code string) {
	push := func(line, failCode string) {
		checklist = append(checklist, line)
		failed = append(failed, failCode)
		if code == "" {
			code = failCode
		}
	}

	if _, ok := validArtifactTypes[in.Type]; !ok {
		push(fmt.Sprintf(i18n.T(lang, "preflight.type_invalid"), in.Type), "TYPE_INVALID")
	}
	if strings.TrimSpace(in.Title) == "" {
		push(i18n.T(lang, "preflight.title_empty"), "TITLE_EMPTY")
	}
	if strings.TrimSpace(in.BodyMarkdown) == "" {
		push(i18n.T(lang, "preflight.body_empty"), "BODY_EMPTY")
	}
	if strings.TrimSpace(in.AreaSlug) == "" {
		push(i18n.T(lang, "preflight.area_empty"), "AREA_EMPTY")
	}
	if strings.TrimSpace(in.AuthorID) == "" {
		push(i18n.T(lang, "preflight.author_empty"), "AUTHOR_EMPTY")
	}
	if in.Completeness != "" {
		switch in.Completeness {
		case "draft", "partial", "settled":
		default:
			push(fmt.Sprintf(i18n.T(lang, "preflight.completeness_invalid"), in.Completeness), "COMPLETENESS_INVALID")
		}
	}

	// artifact_meta enum validation. Each axis is optional; when set it
	// must match the enum. Unknown values fail the whole call so agents
	// can't quietly smuggle free-form strings into the JSONB.
	if in.ArtifactMeta != nil {
		m := in.ArtifactMeta
		if v := strings.TrimSpace(m.SourceType); v != "" {
			if _, ok := validSourceTypes[v]; !ok {
				push(fmt.Sprintf("artifact_meta.source_type %q is not one of code|artifact|user_chat|external|mixed", v), "META_SOURCE_TYPE_INVALID")
			}
		}
		if v := strings.TrimSpace(m.ConsentState); v != "" {
			if _, ok := validConsentStates[v]; !ok {
				push(fmt.Sprintf("artifact_meta.consent_state %q is not one of not_needed|requested|granted|denied", v), "META_CONSENT_STATE_INVALID")
			}
		}
		if v := strings.TrimSpace(m.Confidence); v != "" {
			if _, ok := validConfidences[v]; !ok {
				push(fmt.Sprintf("artifact_meta.confidence %q is not one of low|medium|high", v), "META_CONFIDENCE_INVALID")
			}
		}
		if v := strings.TrimSpace(m.Audience); v != "" {
			if _, ok := validAudiences[v]; !ok {
				push(fmt.Sprintf("artifact_meta.audience %q is not one of owner_only|approvers|project_readers", v), "META_AUDIENCE_INVALID")
			}
		}
		if v := strings.TrimSpace(m.NextContextPolicy); v != "" {
			if _, ok := validNextContextPolicies[v]; !ok {
				push(fmt.Sprintf("artifact_meta.next_context_policy %q is not one of default|opt_in|excluded", v), "META_NEXT_CONTEXT_INVALID")
			}
		}
		if v := strings.TrimSpace(m.VerificationState); v != "" {
			if _, ok := validVerificationStates[v]; !ok {
				push(fmt.Sprintf("artifact_meta.verification_state %q is not one of verified|partially_verified|unverified", v), "META_VERIFICATION_INVALID")
			}
		}
	}

	// Type-specific guardrails. Minimal keyword checks — Phase 13 brings
	// in template artifacts so agents get structured exemplars instead of
	// these ad-hoc tripwires.
	switch in.Type {
	case "Task":
		if !strings.Contains(strings.ToLower(in.BodyMarkdown), "acceptance") {
			push(i18n.T(lang, "preflight.task_acceptance"), "TASK_NO_ACCEPTANCE")
		}
	case "Decision":
		lower := strings.ToLower(in.BodyMarkdown)
		if !strings.Contains(lower, "decision") || !strings.Contains(lower, "context") {
			push(i18n.T(lang, "preflight.adr_sections"), "DEC_NO_SECTIONS")
		}
	case "Debug":
		// Expect at least one of the repro/cause anchors so debug artifacts
		// don't devolve into summaries. Korean + English keywords to match
		// both user languages; lowercasing Korean is a no-op but harmless.
		lower := strings.ToLower(in.BodyMarkdown)
		hasRepro := strings.Contains(lower, "reproduction") || strings.Contains(lower, "repro") ||
			strings.Contains(lower, "재현") || strings.Contains(lower, "증상") ||
			strings.Contains(lower, "symptom")
		if !hasRepro {
			push(i18n.T(lang, "preflight.debug_no_repro"), "DBG_NO_REPRO")
		}
		hasResolution := strings.Contains(lower, "resolution") || strings.Contains(lower, "root cause") ||
			strings.Contains(lower, "원인") || strings.Contains(lower, "해결")
		if !hasResolution {
			push(i18n.T(lang, "preflight.debug_no_resolution"), "DBG_NO_RESOLUTION")
		}
	case "VerificationReport":
		// Migration 0013: VerificationReport artifacts must carry an explicit
		// pass/partial/fail judgement so the downstream verify tool can
		// parse the final verdict without re-running LLM classification.
		// Keyword check is structural-minimum only; richer verdict schema
		// (json body_json column or body_markdown structured section) is a
		// V1.x follow-up.
		lower := strings.ToLower(in.BodyMarkdown)
		hasVerdict := strings.Contains(lower, "pass") || strings.Contains(lower, "partial") ||
			strings.Contains(lower, "fail") || strings.Contains(lower, "합격") ||
			strings.Contains(lower, "부분") || strings.Contains(lower, "불합격")
		if !hasVerdict {
			push(i18n.T(lang, "preflight.verify_no_verdict"), "VER_NO_VERDICT")
		}
	}

	// Phase 11a + 15c: shape-check pins + relates_to. Hard-blocks only on
	// structurally invalid input (empty path, unknown relation/kind).
	// Missing entirely = soft (future escalation NEED_PIN for code-linked
	// types once search_receipt is in place).
	for i, p := range in.Pins {
		kind := strings.TrimSpace(p.Kind)
		if kind == "" {
			kind = "code"
		}
		if _, ok := validPinKinds[kind]; !ok {
			push(fmt.Sprintf(i18n.T(lang, "preflight.pin_kind_invalid"), i, p.Kind), "PIN_KIND_INVALID")
		}
		if strings.TrimSpace(p.Path) == "" {
			push(fmt.Sprintf(i18n.T(lang, "preflight.pin_path_empty"), i), "PIN_PATH_EMPTY")
		}
		if kind == "code" {
			if p.LinesStart < 0 || p.LinesEnd < 0 {
				push(fmt.Sprintf(i18n.T(lang, "preflight.pin_lines_invalid"), i), "PIN_LINES_INVALID")
			}
			if p.LinesStart > 0 && p.LinesEnd > 0 && p.LinesEnd < p.LinesStart {
				push(fmt.Sprintf(i18n.T(lang, "preflight.pin_lines_range"), i), "PIN_LINES_INVALID")
			}
		}
		if kind == "url" && !strings.Contains(p.Path, "://") {
			push(fmt.Sprintf(i18n.T(lang, "preflight.pin_url_invalid"), i), "PIN_URL_INVALID")
		}
	}
	for i, r := range in.RelatesTo {
		if strings.TrimSpace(r.TargetID) == "" {
			push(fmt.Sprintf(i18n.T(lang, "preflight.rel_target_empty"), i), "REL_TARGET_EMPTY")
		}
		if _, ok := validRelations[strings.TrimSpace(r.Relation)]; !ok {
			push(fmt.Sprintf(i18n.T(lang, "preflight.rel_invalid"), i, r.Relation), "REL_INVALID")
		}
	}
	if in.ExpectedVersion != nil && *in.ExpectedVersion < 0 {
		push(i18n.T(lang, "preflight.expected_version_negative"), "VER_INVALID")
	}

	// Phase 15b: task_meta shape. Only validated when provided and only
	// meaningful on type=Task. Non-Task with task_meta earns a soft
	// rejection so agents don't accidentally attach tracker dims to
	// Decision/Analysis/etc. — that would make retrieval confusing.
	if in.TaskMeta != nil {
		if in.Type != "Task" {
			push(i18n.T(lang, "preflight.task_meta_wrong_type"), "TASK_META_WRONG_TYPE")
		}
		tm := in.TaskMeta
		if s := strings.TrimSpace(tm.Status); s != "" {
			if _, ok := validTaskStatuses[s]; !ok {
				push(fmt.Sprintf(i18n.T(lang, "preflight.task_status_invalid"), tm.Status), "TASK_STATUS_INVALID")
			}
			// Task lifecycle v2 (migration 0013): `verified` is reserved
			// for pindoc.artifact.verify. Propose path can only set
			// open / claimed_done / blocked / cancelled. Direct
			// transition to verified is rejected so a single agent
			// cannot ship self-verification in one call — the
			// Implementer ≠ Verifier invariant is structurally enforced,
			// not prompt-guided.
			if s == "verified" {
				push(i18n.T(lang, "preflight.verified_via_verify_tool_only"), "VER_VIA_VERIFY_TOOL_ONLY")
			}
			// `claimed_done` requires acceptance checkboxes to be 100%
			// checked. Minimum evidence gate so agents cannot flip status
			// while acceptance criteria still sit at `- [ ]`. Body with
			// no checkboxes at all passes (not every Task uses the
			// checklist format).
			if s == "claimed_done" {
				done, total := countAcceptanceCheckboxes(in.BodyMarkdown)
				if total > 0 && done != total {
					push(
						fmt.Sprintf(i18n.T(lang, "preflight.claimed_done_incomplete"), done, total),
						"CLAIMED_DONE_INCOMPLETE",
					)
				}
			}
		}
		if p := strings.TrimSpace(tm.Priority); p != "" {
			if _, ok := validTaskPriorities[p]; !ok {
				push(fmt.Sprintf(i18n.T(lang, "preflight.task_priority_invalid"), tm.Priority), "TASK_PRIORITY_INVALID")
			}
		}
		if d := strings.TrimSpace(tm.DueAt); d != "" {
			if _, err := time.Parse(time.RFC3339, d); err != nil {
				push(fmt.Sprintf(i18n.T(lang, "preflight.task_due_at_invalid"), tm.DueAt), "TASK_DUE_AT_INVALID")
			}
		}
	}

	return checklist, failed, code
}

// slugRegex replaces any run of characters that are NOT Unicode letters
// or numbers with a single hyphen. This preserves Hangul / Kana / CJK /
// Latin-ext / Cyrillic / Arabic verbatim — URL path components accept
// all of these when percent-encoded, browsers show the decoded form in
// the address bar, and our pgvector-based lookup is byte-exact.
var slugRegex = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// slugify produces a URL-safe, human-legible slug from a title.
//
// Policy (revised 2026-04-22 Phase 17 follow-up):
//   - ASCII letters lowercased.
//   - Unicode letters (Hangul, Kana, CJK, Cyrillic, Arabic, …) preserved
//     as-is. The earlier policy stripped them, which turned Korean titles
//     like "Pindoc 시스템 아키텍처 — URL 스코프" into "pindoc-url" — 2
//     tokens of meaning lost.
//   - Any run of non-letter/non-digit characters collapses to a single "-".
//   - Trimmed of leading/trailing hyphens.
//   - Capped at 60 runes (not bytes) so UTF-8 doesn't get chopped mid-
//     character; then re-trimmed in case the cut left a trailing hyphen.
//   - Still empty after all that (e.g. title was only punctuation) → the
//     caller falls back to a type+timestamp slug.
//
// Agents that want full control can pass an explicit `slug` on propose;
// this function only runs when `slug` is omitted.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if utf8.RuneCountInString(s) > 60 {
		runes := []rune(s)
		s = strings.Trim(string(runes[:60]), "-")
	}
	return s
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// embedAndStoreChunks computes vectors for the title and each body chunk
// and inserts them into artifact_chunks inside the caller's transaction.
// All vectors pad to the DB column width (768) — see embed/vector.go for
// the rationale.
func embedAndStoreChunks(ctx context.Context, tx pgx.Tx, provider embed.Provider, artifactID, title, body string) error {
	info := provider.Info()

	// Title vector (always one, kind='title').
	titleRes, err := provider.Embed(ctx, embed.Request{Texts: []string{title}, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed title: %w", err)
	}
	if len(titleRes.Vectors) != 1 {
		return fmt.Errorf("embed title: got %d vectors", len(titleRes.Vectors))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, span_start, span_end,
			text, embedding, model_name, model_dim
		) VALUES ($1, 'title', 0, NULL, 0, 0, $2, $3::vector, $4, $5)
	`,
		artifactID,
		title,
		embed.VectorString(embed.PadTo768(titleRes.Vectors[0])),
		info.Name+":"+info.ModelID,
		info.Dimension,
	); err != nil {
		return fmt.Errorf("store title chunk: %w", err)
	}

	// Body chunks (kind='body').
	chunks := embed.ChunkBody(title, body, 600)
	if len(chunks) == 0 {
		return nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	bodyRes, err := provider.Embed(ctx, embed.Request{Texts: texts, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed body: %w", err)
	}
	if len(bodyRes.Vectors) != len(chunks) {
		return fmt.Errorf("embed body: got %d vectors want %d", len(bodyRes.Vectors), len(chunks))
	}
	for i, c := range chunks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_chunks (
				artifact_id, kind, chunk_index, heading, span_start, span_end,
				text, embedding, model_name, model_dim
			) VALUES ($1, 'body', $2, $3, $4, $5, $6, $7::vector, $8, $9)
		`,
			artifactID,
			c.Index,
			nullIfEmpty(c.Heading),
			c.SpanStart, c.SpanEnd,
			c.Text,
			embed.VectorString(embed.PadTo768(bodyRes.Vectors[i])),
			info.Name+":"+info.ModelID,
			info.Dimension,
		); err != nil {
			return fmt.Errorf("store body chunk %d: %w", c.Index, err)
		}
	}
	return nil
}

// defaultNextTools maps a stable fail code to the MCP tools an agent
// should call next to unblock itself. Returning nil means "no suggestion;
// agent should re-read the checklist" — schema-level failures mostly fall
// into that bucket because the fix is "fill in the missing field".
func defaultNextTools(code string) []string {
	switch code {
	case "NO_SRCH", "RECEIPT_UNKNOWN", "RECEIPT_EXPIRED", "RECEIPT_WRONG_PROJECT":
		return []string{"pindoc.artifact.search", "pindoc.context.for_task"}
	case "CONFLICT_EXACT_TITLE", "POSSIBLE_DUP":
		return []string{"pindoc.artifact.read", "pindoc.artifact.propose"}
	case "VER_CONFLICT":
		return []string{"pindoc.artifact.revisions", "pindoc.artifact.diff"}
	case "UPDATE_TARGET_NOT_FOUND", "SUPERSEDE_TARGET_NOT_FOUND", "REL_TARGET_NOT_FOUND":
		return []string{"pindoc.artifact.search", "pindoc.area.list"}
	case "AREA_NOT_FOUND", "AREA_EMPTY":
		return []string{"pindoc.area.list"}
	case "TASK_NO_ACCEPTANCE", "DEC_NO_SECTIONS", "DBG_NO_REPRO", "DBG_NO_RESOLUTION":
		return []string{"pindoc.harness.install"}
	case "UPDATE_SUPERSEDE_EXCLUSIVE":
		return []string{"pindoc.artifact.read"}
	default:
		return nil
	}
}

// makeRelated builds a RelatedRef from the minimal fields most not_ready
// sites have on hand. Empty ID is fine — slug alone is stable for URL
// construction.
func makeRelated(deps Deps, slug, id, artType, title, reason string) RelatedRef {
	return RelatedRef{
		ID:          id,
		Slug:        slug,
		Type:        artType,
		Title:       title,
		AgentRef:    "pindoc://" + slug,
		HumanURL:    HumanURL(deps.ProjectSlug, slug),
		HumanURLAbs: AbsHumanURL(deps.Settings, deps.ProjectSlug, slug),
		Reason:      reason,
	}
}

// patchFieldsFor maps a stable fail code to the minimum set of input
// fields an agent should change to pass the retry. Empty set means
// "rework the whole submission" (schema problems, etc.). Mirrors the 3rd
// peer review's `patchable_fields[]` proposal.
func patchFieldsFor(code string) []string {
	switch code {
	case "NO_SRCH", "RECEIPT_UNKNOWN", "RECEIPT_EXPIRED", "RECEIPT_WRONG_PROJECT":
		return []string{"basis.search_receipt"}
	case "NEED_VER":
		return []string{"expected_version"}
	case "VER_CONFLICT":
		return []string{"expected_version", "body_markdown", "title"}
	case "MISSING_COMMIT_MSG":
		return []string{"commit_msg"}
	case "POSSIBLE_DUP":
		return []string{"update_of", "title"}
	case "CONFLICT_EXACT_TITLE":
		return []string{"title", "update_of"}
	case "REL_TARGET_NOT_FOUND":
		return []string{"relates_to"}
	case "SUPERSEDE_TARGET_NOT_FOUND":
		return []string{"supersede_of"}
	case "UPDATE_SUPERSEDE_EXCLUSIVE":
		return []string{"update_of", "supersede_of"}
	case "UPDATE_TARGET_NOT_FOUND":
		return []string{"update_of"}
	case "AREA_NOT_FOUND", "AREA_EMPTY":
		return []string{"area_slug"}
	case "TASK_NO_ACCEPTANCE", "DEC_NO_SECTIONS", "DBG_NO_REPRO", "DBG_NO_RESOLUTION":
		return []string{"body_markdown"}
	case "TITLE_EMPTY":
		return []string{"title"}
	case "BODY_EMPTY":
		return []string{"body_markdown"}
	case "AUTHOR_EMPTY":
		return []string{"author_id"}
	case "TYPE_INVALID":
		return []string{"type"}
	case "PIN_PATH_EMPTY", "PIN_LINES_INVALID", "PIN_KIND_INVALID", "PIN_URL_INVALID":
		return []string{"pins"}
	case "TASK_META_WRONG_TYPE", "TASK_STATUS_INVALID", "TASK_PRIORITY_INVALID", "TASK_DUE_AT_INVALID":
		return []string{"task_meta"}
	case "REL_TARGET_EMPTY", "REL_INVALID":
		return []string{"relates_to"}
	default:
		return nil
	}
}

// isUniqueViolation is a best-effort check against pgx's error message.
// The typed error route requires importing pgconn; until we add that we
// string-match on the known constraint name. Good enough for one retry
// loop.
func isUniqueViolation(err error, constraint string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") &&
		(constraint == "" || strings.Contains(err.Error(), constraint))
}

// resolveRelatesTo looks up each relates_to target (by id, slug, or
// pindoc:// URL) inside the given project and returns the resolved UUIDs
// aligned with the input order. Returns (nil, notReadyOutput, true) when
// a target can't be found — the caller surfaces it as REL_TARGET_NOT_FOUND.
func resolveRelatesTo(ctx context.Context, tx pgx.Tx, projectSlug string, relates []ArtifactRelationInput, lang string) ([]string, *artifactProposeOutput) {
	if len(relates) == 0 {
		return nil, nil
	}
	resolved := make([]string, len(relates))
	for i, r := range relates {
		ref := normalizeRef(r.TargetID)
		var id string
		err := tx.QueryRow(ctx, `
			SELECT a.id::text FROM artifacts a
			JOIN projects p ON p.id = a.project_id
			WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
			LIMIT 1
		`, projectSlug, ref).Scan(&id)
		if err != nil {
			return nil, &artifactProposeOutput{
				Status:    "not_ready",
				ErrorCode: "REL_TARGET_NOT_FOUND",
				Failed:    []string{"REL_TARGET_NOT_FOUND"},
				Checklist: []string{
					fmt.Sprintf(i18n.T(lang, "preflight.rel_target_missing"), i, r.TargetID),
				},
				SuggestedActions: []string{
					i18n.T(lang, "suggested.read_existing_rel"),
				},
				NextTools:       defaultNextTools("REL_TARGET_NOT_FOUND"),
				PatchableFields: patchFieldsFor("REL_TARGET_NOT_FOUND"),
			}
		}
		resolved[i] = id
	}
	return resolved, nil
}

// insertPins writes each validated pin to artifact_pins. Returns how many
// rows landed (caller echoes this in the output).
func insertPins(ctx context.Context, tx pgx.Tx, artifactID string, pins []ArtifactPinInput) (int, error) {
	n := 0
	for _, p := range pins {
		kind := strings.TrimSpace(p.Kind)
		if kind == "" {
			kind = "code"
		}
		repo := strings.TrimSpace(p.Repo)
		if repo == "" {
			repo = "origin"
		}
		// Non-code kinds don't use line ranges or commit_sha; null them
		// out so the row is consistent with the kind semantics.
		var commit any = nullIfEmpty(p.CommitSHA)
		var linesStart any = nullIfZero(p.LinesStart)
		var linesEnd any = nullIfZero(p.LinesEnd)
		if kind != "code" {
			commit, linesStart, linesEnd = nil, nil, nil
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_pins (artifact_id, kind, repo, commit_sha, path, lines_start, lines_end)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, artifactID, kind, repo, commit, p.Path, linesStart, linesEnd,
		); err != nil {
			return n, fmt.Errorf("pin insert: %w", err)
		}
		n++
	}
	return n, nil
}

// insertEdges writes artifact_edges rows for each resolved (target, relation)
// pair. Idempotent on (source, target, relation) via UNIQUE index; duplicates
// are silently treated as success so re-propose with the same relates_to
// list doesn't blow up.
func insertEdges(ctx context.Context, tx pgx.Tx, sourceID string, targetIDs []string, relates []ArtifactRelationInput) (int, error) {
	n := 0
	for i, tgt := range targetIDs {
		if tgt == sourceID {
			// DB check also catches this, but skip silently to give a
			// cleaner error path.
			continue
		}
		rel := strings.TrimSpace(relates[i].Relation)
		tag, err := tx.Exec(ctx, `
			INSERT INTO artifact_edges (source_id, target_id, relation)
			VALUES ($1, $2, $3)
			ON CONFLICT (source_id, target_id, relation) DO NOTHING
		`, sourceID, tgt, rel)
		if err != nil {
			return n, fmt.Errorf("edge insert: %w", err)
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// taskMetaToJSON serialises input into JSONB for the artifacts.task_meta
// column. Returns nil (= no overwrite) when the caller didn't send
// task_meta at all. Returns '{}'-level empty object when all fields are
// empty — lets the agent explicitly "clear" a Task's tracker dims.
// Returns nil when the artifact isn't a Task so we never store task_meta
// on, say, a Decision row.
func taskMetaToJSON(artifactType string, tm *TaskMetaInput) any {
	if tm == nil || artifactType != "Task" {
		return nil
	}
	payload := map[string]any{}
	if s := strings.TrimSpace(tm.Status); s != "" {
		payload["status"] = s
	}
	if p := strings.TrimSpace(tm.Priority); p != "" {
		payload["priority"] = p
	}
	if a := strings.TrimSpace(tm.Assignee); a != "" {
		payload["assignee"] = a
	}
	if d := strings.TrimSpace(tm.DueAt); d != "" {
		payload["due_at"] = d
	}
	if ps := strings.TrimSpace(tm.ParentSlug); ps != "" {
		payload["parent_slug"] = ps
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return string(buf)
}

// ResolvedArtifactMeta is the server-resolved epistemic metadata that
// actually lands on the artifact row. Mirrors ArtifactMetaInput plus a
// warnings slice describing heuristic decisions so agents can see why a
// default was chosen (e.g. "source_type inferred from pins").
type ResolvedArtifactMeta struct {
	SourceType        string   `json:"source_type,omitempty"`
	ConsentState      string   `json:"consent_state,omitempty"`
	Confidence        string   `json:"confidence,omitempty"`
	Audience          string   `json:"audience,omitempty"`
	NextContextPolicy string   `json:"next_context_policy,omitempty"`
	VerificationState string   `json:"verification_state,omitempty"`
	Warnings          []string `json:"-"`
}

// userChatQuotePattern matches body substrates likely derived from a user
// chat turn: a quoted phrase that is preceded or followed by an "said"
// marker in English or Korean. Intentionally conservative — the goal is
// SOURCE_TYPE_UNCLASSIFIED warning (advisory), not blocking code-grounded
// writes that happen to include user quotes as evidence.
var userChatQuoteMarkers = []string{
	// English markers
	" said", "user said", "user says", " says:", "user:", "the user",
	// Korean markers
	"사용자는", "사용자가", "사용자 말", "말했다", "얘기했", "라고 하", "라고 했",
}

// resolveArtifactMeta combines agent-declared meta with server heuristics
// to produce the ResolvedArtifactMeta that will be persisted. Rules (first
// rule that applies wins per axis):
//
//   - source_type: caller value > pins with code kind ⇒ "code" > otherwise ""
//   - verification_state: caller > (source_type=code ⇒ "partially_verified") > ""
//   - next_context_policy: caller > (source_type=user_chat ⇒ "opt_in") > ""
//   - audience: caller > (source_type=user_chat + PII heuristic ⇒ "owner_only") > ""
//   - consent_state: caller only (no server default — agent must classify)
//   - confidence: caller only (no server default — agent must classify)
//
// Empty strings on return keep the JSONB slot absent (server omits). This
// lets the column stay {} for legacy rows and gracefully accept partial
// classifications without forcing agents to pick every axis.
func resolveArtifactMeta(in *ArtifactMetaInput, pins []ArtifactPinInput, body string, isUpdate bool) ResolvedArtifactMeta {
	out := ResolvedArtifactMeta{}
	if in != nil {
		out.SourceType = strings.TrimSpace(in.SourceType)
		out.ConsentState = strings.TrimSpace(in.ConsentState)
		out.Confidence = strings.TrimSpace(in.Confidence)
		out.Audience = strings.TrimSpace(in.Audience)
		out.NextContextPolicy = strings.TrimSpace(in.NextContextPolicy)
		out.VerificationState = strings.TrimSpace(in.VerificationState)
	}

	hasCodePin := false
	for _, p := range pins {
		kind := strings.TrimSpace(p.Kind)
		if kind == "" {
			kind = "code"
		}
		if kind == "code" {
			hasCodePin = true
			break
		}
	}

	if out.SourceType == "" && hasCodePin {
		out.SourceType = "code"
		out.Warnings = append(out.Warnings, "source_type inferred from code pins")
	}
	if out.VerificationState == "" && out.SourceType == "code" {
		out.VerificationState = "partially_verified"
	}
	if out.NextContextPolicy == "" && out.SourceType == "user_chat" {
		out.NextContextPolicy = "opt_in"
	}
	if out.Audience == "" && out.SourceType == "user_chat" && hasPIISignal(body) {
		out.Audience = "owner_only"
		out.Warnings = append(out.Warnings, "audience downgraded to owner_only — PII-like pattern in body")
	}
	_ = isUpdate // reserved: update path currently does not shift defaults;
	// Task 4 (canonical rewrite guard) will consume this flag to clamp
	// verification_state to "unverified" on detected rewrites.

	return out
}

// hasPIISignal is a coarse regex-free detector that fires on common PII
// surface forms inside artifact bodies (email addresses, tokens, user IDs).
// Deliberately over-eager: if anything resembling a secret shows up in a
// user_chat-derived artifact the audience should start narrow.
func hasPIISignal(body string) bool {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "@") && strings.Contains(lower, ".") {
		// tiny structural check for "x@y.z" anywhere
		at := strings.Index(lower, "@")
		dot := strings.Index(lower[at:], ".")
		if dot > 0 {
			return true
		}
	}
	for _, marker := range []string{"bearer ", "api_key", "api-key", "authorization:", "token="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// artifactMetaToJSON serialises the resolved meta for the JSONB column.
// Returns "{}" when nothing is set so the column stays non-null and the
// NOT NULL DEFAULT invariant is preserved on the Go side too (DB would
// fall back to '{}' anyway, but explicit is safer across the update path).
func artifactMetaToJSON(r ResolvedArtifactMeta) string {
	payload := map[string]any{}
	if r.SourceType != "" {
		payload["source_type"] = r.SourceType
	}
	if r.ConsentState != "" {
		payload["consent_state"] = r.ConsentState
	}
	if r.Confidence != "" {
		payload["confidence"] = r.Confidence
	}
	if r.Audience != "" {
		payload["audience"] = r.Audience
	}
	if r.NextContextPolicy != "" {
		payload["next_context_policy"] = r.NextContextPolicy
	}
	if r.VerificationState != "" {
		payload["verification_state"] = r.VerificationState
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(buf)
}

// classifyConversationDerived returns true when the agent has explicitly
// declared the artifact's substrate is a user chat (source_type = user_chat
// or mixed). This is distinct from detectUnclassifiedUserChat: the latter
// fires on *missing* classification, this one fires on *declared* chat
// substrate. Server policy (Task `conversation-derived-write-...`) treats
// declared chat writes as a new sensitive_ops class
// `conversation_derived_canonical` — they get default draft + opt_in
// context unless the caller explicitly overrides.
func classifyConversationDerived(in *artifactProposeInput) bool {
	if in == nil || in.ArtifactMeta == nil {
		return false
	}
	switch strings.TrimSpace(in.ArtifactMeta.SourceType) {
	case "user_chat", "mixed":
		return true
	}
	return false
}

// applyConversationDerivedDefaults mutates the resolved meta and returned
// completeness when the caller declared a conversation-derived substrate
// AND provided consent_state=granted. In that case default to draft body
// status and opt_in next-session context unless the caller explicitly
// set the corresponding axis (caller override always wins). Returns the
// adjusted completeness value; caller assigns it back to the local var.
func applyConversationDerivedDefaults(in *artifactProposeInput, meta *ResolvedArtifactMeta, completeness string) string {
	if !classifyConversationDerived(in) {
		return completeness
	}
	if meta.ConsentState != "granted" {
		return completeness
	}
	// completeness: when caller left it blank, step down to draft.
	if strings.TrimSpace(in.Completeness) == "" {
		completeness = "draft"
	}
	// next_context_policy: when caller (and resolver) left it blank,
	// default to opt_in. resolveArtifactMeta already applies this for
	// source_type=user_chat; redundant here guards against mixed where
	// the resolver stays silent.
	if meta.NextContextPolicy == "" {
		meta.NextContextPolicy = "opt_in"
	}
	return completeness
}

// detectUnclassifiedUserChat returns true when source_type is unset AND
// there are no pins AND the body contains patterns typical of user-chat
// paraphrase (quote marks plus an "said"-style marker). Used to raise
// SOURCE_TYPE_UNCLASSIFIED warnings on accepted writes — not a block,
// a nudge for the agent to classify explicitly.
func detectUnclassifiedUserChat(meta ResolvedArtifactMeta, pins []ArtifactPinInput, body string) bool {
	if meta.SourceType != "" {
		return false
	}
	if len(pins) > 0 {
		return false
	}
	lower := strings.ToLower(body)
	hasQuote := strings.Contains(body, "\"") || strings.Contains(body, "「") || strings.Contains(body, "\u201c") || strings.Contains(body, "\u201d")
	if !hasQuote {
		return false
	}
	for _, marker := range userChatQuoteMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// semanticConflictThreshold is the cosine-distance ceiling below which we
// treat a vector hit as "likely the same artifact". Recalibrated 2026-04-22
// against gemma + 13-artifact Tier 2 corpus (docs/16-tier2-preflight.md
// Part C). Prior 0.18 was too tight — 4 legitimate near-matches
// (concept→mechanism→spec, Phase chain→Roadmap) fell in 0.17-0.19 band.
// 0.13 keeps only genuine duplicates. Raise if real dupes slip through;
// lower if false positives persist.
const semanticConflictThreshold = 0.13

// semanticAdvisoryThreshold is the softer ceiling: hits between this and
// semanticConflictThreshold earn a RECOMMEND_READ_BEFORE_CREATE warning on
// the accepted response but don't block. Widened from 0.25 to 0.30 on the
// same 2026-04-22 calibration — near-match advisory band should catch the
// 0.17-0.28 "same topic, different depth" cluster a single product's
// corpus produces.
const semanticAdvisoryThreshold = 0.30

// semanticConflictLimit caps how many near-matches we surface in the
// POSSIBLE_DUP response. Two is usually enough — the top hit is the main
// suspect, the runner-up is a tiebreak. More would bloat the response.
const semanticConflictLimit = 2

type semanticCandidate struct {
	ArtifactID string
	Slug       string
	Type       string
	Title      string
	Distance   float64
}

// createWarnings runs a best-effort advisory vector check after an accepted
// create and returns any soft warnings. Non-blocking — failure here never
// rejects the write, since the hard gate (findSemanticConflicts) already
// ran before insert. We only report when a close-but-not-dupe neighbour
// exists, to nudge "you might want to supersede this next time".
func createWarnings(ctx context.Context, deps Deps, projectID, title, body string) []string {
	if deps.Embedder == nil || deps.Embedder.Info().Name == "stub" {
		return nil
	}
	cands, err := findSemanticAdvisories(ctx, deps, projectID, title, body)
	if err != nil || len(cands) == 0 {
		return nil
	}
	return []string{"RECOMMEND_READ_BEFORE_CREATE"}
}

// pinPathWarnings checks every kind="code" pin path against the configured
// repo root and returns one PIN_PATH_NOT_FOUND:<path> warning per miss.
// No-op when deps.RepoRoot is empty (V1 default — the V1.5 git-pinner
// takes over once it lands). Traversal-escape attempts (..) are rejected
// as warnings too; the current commit_sha-less pin flow is trust-on-report
// from the agent, and this validation is the cheapest defence we can run
// without a git checkout on hand.
func pinPathWarnings(deps Deps, pins []ArtifactPinInput) []string {
	if strings.TrimSpace(deps.RepoRoot) == "" || len(pins) == 0 {
		return nil
	}
	var out []string
	for _, p := range pins {
		kind := strings.TrimSpace(p.Kind)
		if kind != "" && kind != "code" {
			// Non-code kinds (resource, url) don't point at a local path.
			continue
		}
		path := strings.TrimSpace(p.Path)
		if path == "" {
			continue
		}
		// Refuse traversal. Pin paths are repo-relative; absolute or parent-
		// escaping paths are a mistake the agent should see.
		if strings.Contains(path, "..") || filepath.IsAbs(path) {
			out = append(out, "PIN_PATH_REJECTED:"+path)
			continue
		}
		full := filepath.Join(deps.RepoRoot, filepath.FromSlash(path))
		if _, err := os.Stat(full); err != nil {
			out = append(out, "PIN_PATH_NOT_FOUND:"+path)
		}
	}
	return out
}

// titleLengthWarnings flags title rune count outside the human-readable
// band (Decision decision-title-heading-rule-preflight). rune-based so the
// gate is language-neutral — 한글/영문/CJK/아랍 동등. Warning-level only.
func titleLengthWarnings(title string) []string {
	n := len([]rune(strings.TrimSpace(title)))
	var out []string
	if n < 15 {
		out = append(out, fmt.Sprintf("TITLE_TOO_SHORT:%d_runes", n))
	}
	if n > 80 {
		out = append(out, fmt.Sprintf("TITLE_TOO_LONG:%d_runes", n))
	}
	return out
}

// bodyH1Warnings flags an H1 inside body_markdown. Reader renders
// artifact.title as the H1 already; a second H1 fragments the TOC.
func bodyH1Warnings(body string) []string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return []string{"BODY_HAS_H1_REDUNDANT"}
		}
	}
	return nil
}

// countAcceptanceCheckboxes walks the body and returns (done, total) where
// done counts `- [x]` lines and total counts `- [x]` + `- [ ]` lines. Both
// are case-insensitive on the fill marker ("x" vs "X"). Used by the Task
// claimed_done evidence gate (migration 0013): without at least one
// checkbox the gate stays quiet (not every Task uses the checklist form),
// but when checkboxes exist they must all be checked.
func countAcceptanceCheckboxes(body string) (done, total int) {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		// Accept "- [ ] x", "* [ ] x", "+ [ ] x" as bullet list checkboxes.
		if len(trimmed) < 5 {
			continue
		}
		marker := trimmed[0]
		if marker != '-' && marker != '*' && marker != '+' {
			continue
		}
		rest := strings.TrimSpace(trimmed[1:])
		if len(rest) < 3 || rest[0] != '[' || rest[2] != ']' {
			continue
		}
		fill := rest[1]
		switch fill {
		case ' ':
			total++
		case 'x', 'X':
			total++
			done++
		}
	}
	return done, total
}

// canonicalClaimSectionsByType returns the H2 headings whose rewrite on the
// update path counts as a canonical truth change (Task
// `update-of-canonical-claim-rewrite-guard-...`). First entry per slot is
// the label surfaced in `warnings[]`. Alternatives section is deliberately
// omitted for Decision — reshaping alternatives is narrative tuning, not a
// canonical claim rewrite.
func canonicalClaimSectionsByType(t string) [][]string {
	switch t {
	case "Debug":
		return [][]string{
			{"Root cause", "원인"},
			{"Resolution", "해결"},
		}
	case "Decision":
		return [][]string{
			{"Decision", "결정"},
		}
	case "Analysis":
		return [][]string{
			{"Conclusion", "결론"},
		}
	default:
		return nil
	}
}

// parseH2Sections splits a markdown body by `## ` headings and returns a
// map keyed by lowercased heading → the raw section body (all lines up to
// the next H2). Non-H2 content above the first H2 is discarded — we only
// care about the named sections. Deliberately does not descend into H3+
// headings so sub-headings don't fragment their parent section.
func parseH2Sections(body string) map[string]string {
	out := map[string]string{}
	var currentHeading string
	var buf strings.Builder
	flush := func() {
		if currentHeading != "" {
			out[currentHeading] = buf.String()
		}
		buf.Reset()
	}
	for _, line := range strings.Split(body, "\n") {
		if after, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			currentHeading = strings.ToLower(strings.TrimSpace(strings.TrimRight(after, "#")))
			continue
		}
		if currentHeading != "" {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	flush()
	return out
}

// normalizeSectionContent strips runs of whitespace so "same content,
// different formatting" doesn't register as a rewrite. Trailing whitespace
// on lines, blank-line runs, and trailing newlines collapse; everything
// else stays intact. Intentionally does NOT strip markdown syntax — bolding
// a phrase or changing bullet markers IS worth flagging as a rewrite
// because it can change emphasis of the canonical claim.
func normalizeSectionContent(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevBlank := false
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			if prevBlank {
				continue
			}
			prevBlank = true
		} else {
			prevBlank = false
		}
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// detectCanonicalClaimRewrite compares the canonical-claim H2 sections of
// two bodies and returns the (canonical-label) names that changed in
// non-whitespace content. Returns nil when the artifact type has no
// canonical claim sections defined or nothing changed.
func detectCanonicalClaimRewrite(prevBody, newBody, artifactType string) []string {
	slots := canonicalClaimSectionsByType(artifactType)
	if len(slots) == 0 {
		return nil
	}
	prevSections := parseH2Sections(prevBody)
	newSections := parseH2Sections(newBody)
	var changed []string
	for _, slot := range slots {
		canonicalLabel := slot[0]
		// Collect prev/new content across synonyms — a rewrite that changes
		// the heading from "Root cause" to "원인" shouldn't read as "absent".
		var prevVal, newVal string
		for _, alt := range slot {
			key := strings.ToLower(alt)
			if v, ok := prevSections[key]; ok && prevVal == "" {
				prevVal = v
			}
			if v, ok := newSections[key]; ok && newVal == "" {
				newVal = v
			}
		}
		prevN := normalizeSectionContent(prevVal)
		newN := normalizeSectionContent(newVal)
		// Absent in both → nothing to guard.
		if prevN == "" && newN == "" {
			continue
		}
		if prevN != newN {
			changed = append(changed, canonicalLabel)
		}
	}
	return changed
}

// evidenceKeywordMarkers matches commit_msg blurbs that hint the rewrite
// was backed by fresh investigation (reproduction, verification). Weak
// signal — resolveArtifactMeta.verification_state and new pins are the
// primary signals.
var evidenceKeywordMarkers = []string{
	"evidence", "verified", "verify", "tested", "test added",
	"reproduced", "repro", "confirmed", "근거", "재현", "검증", "확인",
}

// hasEvidenceDelta returns true when the update carries at least one signal
// that backs a canonical-claim rewrite: new pins attached, verification
// state advanced past `unverified`, or commit_msg references an evidence
// keyword. Explicitly permissive on weak signals so a legitimate "fixed
// typo, re-read the docs" rewrite doesn't earn a warning it doesn't merit.
func hasEvidenceDelta(prevMeta ResolvedArtifactMeta, in *artifactProposeInput) bool {
	if len(in.Pins) > 0 {
		return true
	}
	if in.ArtifactMeta != nil {
		newVer := strings.TrimSpace(in.ArtifactMeta.VerificationState)
		if newVer != "" && newVer != "unverified" && prevMeta.VerificationState != newVer {
			return true
		}
	}
	commit := strings.ToLower(in.CommitMsg)
	for _, marker := range evidenceKeywordMarkers {
		if strings.Contains(commit, marker) {
			return true
		}
	}
	return false
}

// requiredH2Warnings flags missing mandatory H2 sections per Type. Fuzzy
// match: a slot is satisfied if any of its synonyms appear as an H2
// (case-insensitive). en/ko synonyms tolerate the current bilingual
// template corpus; Phase 18 template locale-split will normalise them.
func requiredH2Warnings(body, artifactType string) []string {
	slots := requiredH2ByType(artifactType)
	if len(slots) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(body, "\n") {
		after, ok := strings.CutPrefix(line, "## ")
		if !ok {
			continue
		}
		heading := strings.ToLower(strings.TrimSpace(strings.TrimRight(after, "#")))
		seen[heading] = true
	}
	var out []string
	for _, syn := range slots {
		matched := false
		for _, alt := range syn {
			if seen[strings.ToLower(alt)] {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, "MISSING_H2:"+syn[0])
		}
	}
	return out
}

// requiredH2ByType returns canonical H2 slots per Type plus accepted
// synonyms (first entry = canonical English used for the warning label).
// Debug keeps its existing keyword-based pre-flight check (debug_no_repro /
// debug_no_resolution) — unification deferred to Phase 18.
func requiredH2ByType(t string) [][]string {
	switch t {
	case "Decision":
		return [][]string{
			{"Context", "맥락", "컨텍스트"},
			{"Decision", "결정"},
			{"Rationale", "근거"},
			{"Alternatives considered", "Alternatives", "대안"},
			{"Consequences", "영향", "결과"},
		}
	case "Analysis":
		return [][]string{
			{"TL;DR", "요약"},
		}
	case "Task":
		return [][]string{
			{"Purpose", "목적"},
			{"Scope", "범위"},
			{"Acceptance criteria", "Acceptance", "완료 기준", "완료기준"},
		}
	default:
		return nil
	}
}

// findSemanticAdvisories is findSemanticConflicts' soft cousin: returns
// hits in the [semanticConflictThreshold, semanticAdvisoryThreshold)
// band — close enough to be worth mentioning, far enough not to block.
func findSemanticAdvisories(ctx context.Context, deps Deps, projectID, title, body string) ([]semanticCandidate, error) {
	probe := title
	if trimmed := strings.TrimSpace(body); trimmed != "" {
		cut := 800
		if len(trimmed) < cut {
			cut = len(trimmed)
		}
		probe = probe + "\n\n" + trimmed[:cut]
	}
	res, err := deps.Embedder.Embed(ctx, embed.Request{
		Texts: []string{probe},
		Kind:  embed.KindQuery,
	})
	if err != nil {
		return nil, err
	}
	if len(res.Vectors) != 1 {
		return nil, fmt.Errorf("embed probe: got %d vectors", len(res.Vectors))
	}
	qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))
	rows, err := deps.DB.Query(ctx, `
		SELECT DISTINCT ON (c.artifact_id)
			c.artifact_id::text, a.slug, a.type, a.title,
			c.embedding <=> $1::vector AS distance
		FROM artifact_chunks c
		JOIN artifacts a ON a.id = c.artifact_id
		WHERE a.project_id = $2 AND a.status <> 'archived'
		ORDER BY c.artifact_id, distance
	`, qVec, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []semanticCandidate
	for rows.Next() {
		var c semanticCandidate
		if err := rows.Scan(&c.ArtifactID, &c.Slug, &c.Type, &c.Title, &c.Distance); err != nil {
			return nil, err
		}
		if c.Distance >= semanticConflictThreshold && c.Distance < semanticAdvisoryThreshold {
			out = append(out, c)
			if len(out) >= 2 {
				break
			}
		}
	}
	return out, rows.Err()
}

// findSemanticConflicts embeds (title + first ~800 chars of body) and runs
// a pgvector distance query against existing (non-archived) artifacts in
// the same project. Returns the suspects sorted by distance ascending,
// only if the best one is within semanticConflictThreshold. Empty slice
// means "no suspect" — caller proceeds to insert.
func findSemanticConflicts(ctx context.Context, deps Deps, projectID, title, body string) ([]semanticCandidate, error) {
	// Use a compact "query probe": title + first chunk of body gives the
	// embedding provider enough signal without being so long the vector
	// averages toward the corpus mean.
	probe := title
	if trimmed := strings.TrimSpace(body); trimmed != "" {
		cut := 800
		if len(trimmed) < cut {
			cut = len(trimmed)
		}
		probe = probe + "\n\n" + trimmed[:cut]
	}
	res, err := deps.Embedder.Embed(ctx, embed.Request{
		Texts: []string{probe},
		Kind:  embed.KindQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("embed probe: %w", err)
	}
	if len(res.Vectors) != 1 {
		return nil, fmt.Errorf("embed probe: got %d vectors", len(res.Vectors))
	}
	qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))

	// DISTINCT ON picks the best chunk per artifact; we then filter by
	// threshold and limit in Go. Keeps the SQL simple and lets us tune
	// the threshold without redeploying a query.
	rows, err := deps.DB.Query(ctx, `
		SELECT DISTINCT ON (c.artifact_id)
			c.artifact_id::text, a.slug, a.type, a.title,
			c.embedding <=> $1::vector AS distance
		FROM artifact_chunks c
		JOIN artifacts a ON a.id = c.artifact_id
		WHERE a.project_id = $2 AND a.status <> 'archived'
		ORDER BY c.artifact_id, distance
	`, qVec, projectID)
	if err != nil {
		return nil, fmt.Errorf("vector query: %w", err)
	}
	defer rows.Close()

	var all []semanticCandidate
	for rows.Next() {
		var c semanticCandidate
		if err := rows.Scan(&c.ArtifactID, &c.Slug, &c.Type, &c.Title, &c.Distance); err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort ascending by distance; keep only those under the threshold.
	// The DISTINCT ON didn't give a stable global ordering.
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if all[j].Distance < all[i].Distance {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	var out []semanticCandidate
	for _, c := range all {
		if c.Distance >= semanticConflictThreshold {
			break
		}
		out = append(out, c)
		if len(out) >= semanticConflictLimit {
			break
		}
	}
	return out, nil
}
