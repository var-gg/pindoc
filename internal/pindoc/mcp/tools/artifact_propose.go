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

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	pgit "github.com/var-gg/pindoc/internal/pindoc/git"
	"github.com/var-gg/pindoc/internal/pindoc/i18n"
	pinmodel "github.com/var-gg/pindoc/internal/pindoc/pins"
	"github.com/var-gg/pindoc/internal/pindoc/policy"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/titleguide"
)

// ValidArtifactTypes are the types Phase 2 accepts. Tier A (7) + Tier B
// Web-SaaS pack (4). When a Tier B pack is activated for a project in
// Phase 4+ this set becomes project-scoped, but for M1 a single flat
// whitelist is enough.
var validArtifactTypes = map[string]struct{}{
	// Tier A core
	"Decision": {}, "Analysis": {}, "Debug": {}, "Flow": {},
	"Task": {}, "TC": {}, "Glossary": {},
	// Tier B Web SaaS
	"Feature": {}, "APIEndpoint": {}, "Screen": {}, "DataModel": {},
}

type artifactProposeInput struct {
	// ProjectSlug picks which project this artifact lands in (account-
	// level scope, Decision mcp-scope-account-level-industry-standard).
	// Required.
	ProjectSlug   string   `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	Type          string   `json:"type" jsonschema:"one of Decision|Analysis|Debug|Flow|Task|TC|Glossary|Feature|APIEndpoint|Screen|DataModel"`
	AreaSlug      string   `json:"area_slug" jsonschema:"slug from pindoc.area.list; use 'misc' or '_unsorted' if unsure"`
	Title         string   `json:"title"`
	BodyMarkdown  string   `json:"body_markdown" jsonschema:"main content in markdown"`
	BodyLocale    string   `json:"body_locale,omitempty" jsonschema:"BCP 47 body language tag; default = project primary_language"`
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
	// Pointer type preserves the nil / zero distinction: nil = omitted
	// (NEED_VER), 0 = reserved invalid (FIELD_VALUE_RESERVED — migration
	// 0017 guarantees every artifact has revision >= 1), N>=1 = compared
	// to head. Agents should read pindoc.artifact.revisions and pass the
	// current max revision_number.
	ExpectedVersion *int `json:"expected_version,omitempty" jsonschema:"required for update_of; current revision number (>= 1)"`

	// Shape is the discriminator for the revision mutation kind (Phase B
	// revision-shapes refactor). Empty defaults to "body_patch" — the
	// legacy path that re-encodes the whole body. Other shapes (Phase C+)
	// unlock metadata-only / acceptance-transition / scope-defer paths
	// without a full body round-trip. See internal/pindoc/mcp/tools/
	// revision_shape.go for the full enum.
	Shape string `json:"shape,omitempty" jsonschema:"one of body_patch|meta_patch|acceptance_transition|scope_defer; default body_patch"`

	// DryRun validates the same propose payload without persisting an
	// artifact, revision, edge, pin, or event. It does not bypass
	// search_receipt, expected_version, relates_to, or policy gates.
	DryRun bool `json:"dry_run,omitempty" jsonschema:"validate without INSERT/update; does not bypass receipt requirement; use for agent self-validation and MCP regression tests"`

	// SupersedeOf marks the target artifact as superseded by this new one.
	// Creates a NEW artifact (like a no-update_of call), then flips the
	// target's status to 'superseded' and sets superseded_by to the new id.
	// Different from update_of: update appends a revision to the same
	// artifact; supersede creates a replacement and archives the old one.
	SupersedeOf string `json:"supersede_of,omitempty" jsonschema:"id, slug, or pindoc:// URL of the artifact being replaced"`

	// UnrelatedReason is the Task-only escape hatch for the supersede
	// safety gate. When a new Task semantically overlaps active Tasks in
	// the same area, the caller must either set supersede_of or explain why
	// the new Task is intentionally unrelated.
	UnrelatedReason string `json:"unrelated_reason,omitempty" jsonschema:"Task-only escape hatch when semantic conflict candidates are active but not superseded; minimum 20 characters when used"`

	// Pins attach code references to the artifact. All optional — the
	// server stores whatever is provided. path is the only required field
	// in each pin (enforced by DB check). Phase 11a stores them; stale
	// detection (comparing commit_sha to current HEAD) lands V1.x.
	Pins []ArtifactPinInput `json:"pins,omitempty" jsonschema:"code references tying this artifact to files/commits"`

	// RelatesTo records typed edges to other artifacts in the same project.
	// Valid relations: implements | references | blocks | relates_to |
	// translation_of | evidence.
	// Target may be id, slug, or pindoc:// URL — resolved server-side.
	// Unknown targets fail the whole call with RELATES_TARGET_NOT_FOUND.
	RelatesTo []ArtifactRelationInput `json:"relates_to,omitempty" jsonschema:"typed edges to other artifacts"`

	// BodyPatch is an optional light-weight alternative to re-sending the
	// entire BodyMarkdown on update_of. Task
	// artifact-propose-본문-patch-입력-도입 — Decision `mcp-dog-food-1차-
	// 관찰-6-friction-1-validation` (관찰 2·6). Update path only; create
	// and supersede reject it because there is nothing to patch against.
	// Mutually exclusive with BodyMarkdown.
	BodyPatch *BodyPatchInput `json:"body_patch,omitempty" jsonschema:"lightweight patch against the current body — update_of only, exclusive with body_markdown"`

	// AcceptanceTransition is the Phase D payload for shape=
	// acceptance_transition. Flips a single 4-state checkbox (locator +
	// new_state + reason) without resending body_markdown. Only read when
	// shape=acceptance_transition; ignored on other shapes.
	AcceptanceTransition *AcceptanceTransitionInput `json:"acceptance_transition,omitempty" jsonschema:"payload for shape=acceptance_transition — checkbox locator + new_state + reason"`

	// ScopeDefer is the Phase F payload for shape=scope_defer. Moves an
	// acceptance item to a target artifact — server atomically rewrites
	// the source checkbox to [-] and writes an artifact_scope_edges row.
	// Only read when shape=scope_defer.
	ScopeDefer *ScopeDeferInput `json:"scope_defer,omitempty" jsonschema:"payload for shape=scope_defer — checkbox locator + to_artifact + reason"`

	// ArtifactMeta carries epistemic axes that classify the artifact's
	// trustworthiness and memory scope, plus optional rule-scoping fields
	// used by context.for_task's Applicable Rules mechanism. All fields
	// optional — server resolves defaults via resolveArtifactMeta based on
	// pins, update path, and body heuristics. See docs/04-data-model.md for
	// field definitions.
	//
	// Axes:
	//   source_type         — code | artifact | user_chat | external | mixed
	//   consent_state       — not_needed | requested | granted | denied
	//   confidence          — low | medium | high
	//   audience            — owner_only | approvers | project_readers
	//   next_context_policy — default | opt_in | excluded
	//   verification_state  — verified | partially_verified | unverified
	//   applies_to_areas    — area slugs or wildcard scopes such as ui/*
	//   applies_to_types    — artifact types; empty means all types
	//   rule_severity       — binding | guidance | reference
	//   rule_excerpt        — short excerpt surfaced in applicable_rules
	//
	// On update_of the supplied meta replaces the existing artifact_meta
	// JSONB. Omit artifact_meta to preserve the previous value.
	ArtifactMeta *ArtifactMetaInput `json:"artifact_meta,omitempty" jsonschema:"epistemic axes and optional applicable-rule metadata (all fields optional)"`

	// TaskMeta carries typed tracker dimensions for type=Task artifacts
	// (Phase 15b). Ignored for any other type. All fields optional:
	//
	//   status      — open | claimed_done | blocked | cancelled
	//   priority    — p0 | p1 | p2 | p3
	//   assignee    — agent:<id> | user:<id> | @<handle> | "" to clear
	//   due_at      — RFC3339 timestamp
	//   parent_slug — another Task artifact's slug (for epic→task→subtask)
	//
	// On create, omitted assignee means unassigned; set assignee explicitly
	// when claiming ownership. On meta_patch update, omitted assignee is
	// untouched while explicit "" clears it.
	TaskMeta *TaskMetaInput `json:"task_meta,omitempty" jsonschema:"tracker dims for Task artifacts"`

	// Basis records the evidence the agent gathered before proposing.
	// Phase 11b makes basis.search_receipt REQUIRED on the create path
	// (new artifact, no update_of, no supersede_of): the server refuses
	// the write with NO_SRCH if a valid receipt from artifact.search or
	// context.for_task isn't provided. Update/supersede paths skip the
	// gate because reading/targeting an existing artifact is already
	// proof of context.
	Basis *artifactProposeBasis `json:"basis,omitempty"`

	// WordingFix is set by the pindoc.artifact.wording_fix shortcut. It is
	// not part of the public artifact.propose schema; it lets the shared
	// update path suppress canonical rewrite warnings for narrow wording
	// patches.
	WordingFix bool `json:"-"`

	// AddPin is set by the pindoc.artifact.add_pin shortcut. It is not part
	// of the public artifact.propose schema; it documents that the mutation
	// is a narrow pin-only operation and keeps canonical rewrite detection
	// from treating the lane as an evidence-free body rewrite.
	AddPin bool `json:"-"`
}

type artifactProposeBasis struct {
	// SearchReceipt is the opaque token returned by artifact.search or
	// context.for_task in the same session. TTL 10 minutes.
	SearchReceipt string `json:"search_receipt,omitempty" jsonschema:"receipt from artifact.search or context.for_task"`
	// SourceSession is a free-form string identifying the agent session
	// that produced this artifact — stored on the revision row for
	// audit. Not validated.
	SourceSession string `json:"source_session,omitempty"`
	// BulkOpID groups revisions emitted by one bulk operational metadata
	// tool call, such as pindoc.task.bulk_assign.
	BulkOpID string `json:"bulk_op_id,omitempty"`
}

// ArtifactPinInput is the agent-facing shape for a single pin. `path` is
// always mandatory (DB CHECK); the other fields depend on `kind`:
//
//	kind="code" (default) — repo_id/repo, commit_sha, path (file path),
//	                        lines_start/lines_end. Phase 11a original.
//	kind="doc"            — markdown/text docs and README/CHANGELOG-like paths.
//	kind="config"         — JSON/YAML/TOML/Dockerfile/env/config paths.
//	kind="asset"          — image/PDF/media/font paths.
//	kind="resource"       — legacy typed resource references; preserved for
//	                        compatibility with existing callers.
//	kind="url"            — path holds an absolute URL ("https://…");
//	                        repo/commit/lines are ignored.
//
// Agents that don't set kind get a server-inferred value from path. Go/TS/Py
// and other source paths still fall back to code.
type ArtifactPinInput struct {
	Kind       string `json:"kind,omitempty" jsonschema:"one of code | doc | config | asset | resource | url; omitted kind is inferred from path"`
	RepoID     string `json:"repo_id,omitempty" jsonschema:"canonical project_repos.id; optional, server auto-maps when omitted"`
	Repo       string `json:"repo,omitempty" jsonschema:"'origin' default; named remote when multi-repo; code kind only"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"code kind only"`
	Path       string `json:"path" jsonschema:"code: file path; resource: typed resource ref; url: absolute URL"`
	LinesStart int    `json:"lines_start,omitempty" jsonschema:"code kind only"`
	LinesEnd   int    `json:"lines_end,omitempty" jsonschema:"code kind only"`
}

func normalizePinInputs(pins []ArtifactPinInput) {
	for i := range pins {
		pins[i].Kind = pinmodel.NormalizeKind(pins[i].Kind, pins[i].Path)
	}
}

// TaskMetaInput is the agent-facing shape for a Task artifact's tracker
// dimensions. Every field is optional; the server stores what's provided.
type TaskMetaInput struct {
	// Status is the Task lifecycle enum. `claimed_done` is the settled
	// completion state after acceptance criteria land.
	Status     string `json:"status,omitempty" jsonschema:"open | claimed_done | blocked | cancelled"`
	Priority   string `json:"priority,omitempty" jsonschema:"p0 release blocker; p1 must close before release; p2 next round; p3 backlog. Project-specific priority policy wins when present."`
	Assignee   string `json:"assignee,omitempty"`
	DueAt      string `json:"due_at,omitempty" jsonschema:"RFC3339 timestamp"`
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"slug of parent Task artifact"`

	assigneeSet bool
}

// UnmarshalJSON preserves whether the caller supplied task_meta.assignee.
// A plain string field cannot distinguish omitted from "" after decoding,
// but the meta_patch lane needs that distinction: omitted means untouched,
// while explicit "" means clear the assignee.
func (tm *TaskMetaInput) UnmarshalJSON(data []byte) error {
	type wireTaskMetaInput struct {
		Status     string `json:"status,omitempty"`
		Priority   string `json:"priority,omitempty"`
		Assignee   string `json:"assignee,omitempty"`
		DueAt      string `json:"due_at,omitempty"`
		ParentSlug string `json:"parent_slug,omitempty"`
	}
	var wire wireTaskMetaInput
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*tm = TaskMetaInput{
		Status:      wire.Status,
		Priority:    wire.Priority,
		Assignee:    wire.Assignee,
		DueAt:       wire.DueAt,
		ParentSlug:  wire.ParentSlug,
		assigneeSet: raw != nil && raw["assignee"] != nil,
	}
	return nil
}

func (tm TaskMetaInput) MarshalJSON() ([]byte, error) {
	payload := map[string]any{}
	if s := strings.TrimSpace(tm.Status); s != "" {
		payload["status"] = s
	}
	if p := strings.TrimSpace(tm.Priority); p != "" {
		payload["priority"] = p
	}
	if a := strings.TrimSpace(tm.Assignee); a != "" {
		payload["assignee"] = a
	} else if tm.assigneeSet {
		payload["assignee"] = nil
	}
	if d := strings.TrimSpace(tm.DueAt); d != "" {
		payload["due_at"] = d
	}
	if ps := strings.TrimSpace(tm.ParentSlug); ps != "" {
		payload["parent_slug"] = ps
	}
	return json.Marshal(payload)
}

func (tm *TaskMetaInput) markAssigneeSet() {
	if tm != nil {
		tm.assigneeSet = true
	}
}

var validTaskStatuses = map[string]struct{}{
	"open": {}, "claimed_done": {}, "blocked": {}, "cancelled": {},
}
var validTaskPriorities = map[string]struct{}{
	"p0": {}, "p1": {}, "p2": {}, "p3": {},
}

const warningAcceptanceUnchecked = "acceptance_unchecked"

var closeSuggestiveCommitWordRe = regexp.MustCompile(`[a-z]+`)

// ArtifactMetaInput is the agent-facing shape for epistemic axes. Every
// field is optional; resolveArtifactMeta fills defaults based on pins,
// update path, and body heuristics.
type ArtifactMetaInput struct {
	SourceType        string   `json:"source_type,omitempty" jsonschema:"code | artifact | user_chat | external | mixed"`
	ConsentState      string   `json:"consent_state,omitempty" jsonschema:"not_needed | requested | granted | denied"`
	Confidence        string   `json:"confidence,omitempty" jsonschema:"low | medium | high"`
	Audience          string   `json:"audience,omitempty" jsonschema:"owner_only | approvers | project_readers"`
	NextContextPolicy string   `json:"next_context_policy,omitempty" jsonschema:"default | opt_in | excluded"`
	VerificationState string   `json:"verification_state,omitempty" jsonschema:"verified | partially_verified | unverified"`
	AppliesToAreas    []string `json:"applies_to_areas,omitempty" jsonschema:"area_slug list; wildcard scopes like ui/* supported"`
	AppliesToTypes    []string `json:"applies_to_types,omitempty" jsonschema:"artifact type list; omitted or empty means all types"`
	RuleSeverity      string   `json:"rule_severity,omitempty" jsonschema:"binding | guidance | reference; presence marks this artifact as an applicable rule"`
	RuleExcerpt       string   `json:"rule_excerpt,omitempty" jsonschema:"short excerpt returned by context.for_task applicable_rules; default derives from the first H2 section"`
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
var validRuleSeverities = map[string]struct{}{
	"binding": {}, "guidance": {}, "reference": {},
}

func validRuleAreaScope(scope string) bool {
	s := strings.TrimSpace(scope)
	if s == "" {
		return false
	}
	if s == "*" {
		return true
	}
	if strings.ContainsAny(s, " \t\r\n") || strings.Contains(s, "//") {
		return false
	}
	if strings.HasSuffix(s, "/*") {
		base := strings.TrimSuffix(s, "/*")
		return base != "" && !strings.Contains(base, "*")
	}
	return !strings.Contains(s, "*")
}

// ArtifactRelationInput is the agent-facing shape for one edge.
type ArtifactRelationInput struct {
	TargetID string `json:"target_id" jsonschema:"id, slug, or pindoc:// URL of the related artifact"`
	Relation string `json:"relation" jsonschema:"one of implements|references|blocks|relates_to|translation_of|evidence"`
}

var validRelations = map[string]struct{}{
	"implements": {}, "references": {}, "blocks": {}, "relates_to": {},
	// Phase 18 — cross-locale pairing for project-locale composite key
	// (Task task-phase-18-project-locale-implementation).
	"translation_of": {},
	// Evidence links point at another Pindoc artifact used as supporting
	// proof. Use pins for concrete code/file/URL coordinates; use evidence
	// when the support is itself an artifact.
	"evidence": {},
}

type artifactProposeOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`
	// Failed is the Phase 12-style stable code list. Populated alongside
	// the legacy natural-language Checklist during Phase 11a so agents can
	// start branching on codes now; Checklist becomes optional in Phase 12.
	Failed           []string             `json:"failed,omitempty"`
	ErrorCodes       []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist        []string             `json:"checklist,omitempty"`
	ChecklistItems   []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale    string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`
	SuggestedActions []string             `json:"suggested_actions,omitempty"`

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
	DryRun         bool      `json:"dry_run,omitempty"`

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
	NextTools []NextToolHint `json:"next_tools,omitempty"`
	Related   []RelatedRef   `json:"related,omitempty"`
	Expected  *ExpectedShape `json:"expected,omitempty"`
	// PatchableFields (Phase 14b) tells the agent which input fields to
	// change for the retry. Empty = full input needs rework. Maps stable
	// fail codes to the minimum patch surface so agents don't resend
	// entire propose bodies they didn't need to touch.
	PatchableFields []string `json:"patchable_fields,omitempty"`
	// Warnings (Phase 14b) surface advisory flags on otherwise-accepted
	// writes. Current set: RECOMMEND_READ_BEFORE_CREATE when a create
	// passed but a semantic close match existed — the agent did not read
	// it. Agents should log/surface; not a block.
	//
	// Phase G ordering — slice is sorted by severity descending so the
	// first entries are the ones an agent should act on first. The
	// parallel WarningSeverities slice carries the resolved severity
	// ("error" | "warn" | "info") aligned index-by-index so rich clients
	// don't have to hardcode the catalog.
	Warnings          []string `json:"warnings,omitempty"`
	WarningSeverities []string `json:"warning_severities,omitempty" jsonschema:"aligned with warnings[] index-by-index; one of error | warn | info"`
	// ToolsetVersion echoes the current MCP tool catalog hash so agents
	// can detect drift without a dedicated ping — every propose response
	// is enough to notice "server grew a tool between sessions, reconnect".
	// Phase H drift notice.
	ToolsetVersion string `json:"toolset_version,omitempty"`
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
	// ReceiptExempted is populated when create-path search_receipt gating
	// was intentionally skipped by the empty/same-author bootstrap policy.
	ReceiptExempted *ReceiptExemptionSignal `json:"receipt_exempted,omitempty"`
	// CanonicalRewriteWithoutEvidence is true when the update path rewrote
	// a type-specific canonical claim section (Debug.Root cause,
	// Decision.Decision, Analysis.Conclusion) without fresh evidence (new
	// pins, verification_state bump past unverified, or commit_msg
	// evidence keyword). Reader revision badges consume this flag for an
	// "uncertain rewrite" marker. Always false on create paths.
	CanonicalRewriteWithoutEvidence bool `json:"canonical_rewrite_without_evidence,omitempty"`
}

type ReceiptExemptionSignal struct {
	Reason     string `json:"reason"`
	NRemaining int    `json:"n_remaining"`
	Limit      int    `json:"limit,omitempty"`
}

// NextToolHint is a structured continuation hint. It keeps the tool name
// machine-readable and, when useful, carries the exact arguments for the
// next MCP call.
type NextToolHint struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args,omitempty"`
	Reason string         `json:"reason,omitempty"`
}

// ExpectedShape describes the shape the failed propose attempt should
// converge to. For template-backed structure gates this includes the
// exact template slug and H2 slots the agent can copy into its retry.
type ExpectedShape struct {
	ArtifactType string           `json:"artifact_type,omitempty"`
	TemplateSlug string           `json:"template_slug,omitempty"`
	RequiredH2   []ExpectedH2Slot `json:"required_h2,omitempty"`
}

type ExpectedH2Slot struct {
	Label   string   `json:"label"`
	Aliases []string `json:"aliases,omitempty"`
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
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.propose",
			Description: "Propose a new artifact (the only write path humans use — always via an agent). Create path (both update_of and supersede_of omitted) requires basis.search_receipt from pindoc.artifact.search or pindoc.context.for_task in the same session; update/supersede paths do not. dry_run=true validates without INSERT/update and does not bypass receipt, expected_version, relates_to, or policy gates; use it for agent self-validation and MCP regression tests, then retry with dry_run=false to publish. For Task creates, omitted task_meta.assignee means unassigned; explicit assignee claims ownership, and explicit task_meta.assignee=\"\" clears on update. Task priority meanings: p0 release blocker, p1 must close before release, p2 next round, p3 backlog; project-specific priority policy wins when present. Use pins[] for concrete code/file/URL evidence; use relates_to relation=evidence when the supporting source is another Pindoc artifact. Returns Status=accepted + artifact_id on success, or Status=not_ready + checklist + suggested_actions if Pre-flight fails. Always read the checklist; never surface the raw error to the user without trying the suggested actions first.",
		},
		func(ctx context.Context, p *auth.Principal, in artifactProposeInput) (*sdk.CallToolResult, artifactProposeOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("artifact.propose: %w", err)
			}

			// Create / supersede-create Task rows should never land without
			// the lifecycle's baseline metadata. Updates intentionally skip
			// this so existing task_meta is preserved unless the caller
			// explicitly patches it.
			applyTaskCreateDefaults(&in)
			normalizePinInputs(in.Pins)

			// --- Pre-flight ----------------------------------------------
			lang := deps.UserLanguage
			checklist, failed, code := preflight(ctx, deps, scope.ProjectSlug, &in, lang)
			if len(checklist) > 0 {
				expected := expectedForNotReady(ctx, deps, scope.ProjectSlug, in.Type, failed)
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: code,
					Failed:    failed,
					Checklist: checklist,
					SuggestedActions: suggestedActionsForNotReady(lang, in.Type, failed, []string{
						i18n.T(lang, "suggested.fix_all"),
						i18n.T(lang, "suggested.confirm_types"),
						i18n.T(lang, "suggested.use_misc"),
					}),
					NextTools:       nextToolsForNotReady(code, in.Type, failed),
					PatchableFields: patchFieldsFor(code),
					Expected:        expected,
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
				return handleUpdate(ctx, deps, p, scope, in, lang)
			}

			// --- Supersede path (supersede_of set) -----------------------
			// Creates a fresh artifact via the same insert flow as "new",
			// then flips the target artifact's status to 'superseded' and
			// writes superseded_by. We reuse the create path below and do
			// the supersede bookkeeping just before commit.

			// --- Resolve area + project ----------------------------------
			var projectID, areaID, sensitiveOps string
			err = deps.DB.QueryRow(ctx, `
				SELECT proj.id::text, area.id::text, COALESCE(NULLIF(proj.sensitive_ops, ''), 'auto')
				FROM projects proj
				JOIN areas area ON area.project_id = proj.id
				WHERE proj.slug = $1 AND area.slug = $2
			`, scope.ProjectSlug, in.AreaSlug).Scan(&projectID, &areaID, &sensitiveOps)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: "AREA_UNKNOWN",
					Failed:    []string{"AREA_UNKNOWN"},
					Checklist: areaUnknownChecklist(ctx, deps, scope, lang, in.AreaSlug),
					SuggestedActions: []string{
						i18n.T(lang, "suggested.list_areas"),
						i18n.T(lang, "suggested.area_or_misc"),
					},
					NextTools:       defaultNextTools("AREA_UNKNOWN"),
					PatchableFields: patchFieldsFor("AREA_UNKNOWN"),
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
			var receiptExempted *ReceiptExemptionSignal
			if isCreatePath && deps.Receipts != nil {
				receipt := ""
				if in.Basis != nil {
					receipt = strings.TrimSpace(in.Basis.SearchReceipt)
				}
				var receiptSnapshots []receipts.ArtifactRef
				if receipt == "" {
					exemption, ok, err := maybeExemptMissingReceipt(ctx, deps, projectID, areaID, in.AuthorID)
					if err != nil {
						return nil, artifactProposeOutput{}, fmt.Errorf("receipt exemption check: %w", err)
					}
					if !ok {
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
					receiptExempted = exemption
				} else {
					res := deps.Receipts.Verify(receipt, scope.ProjectSlug)
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
					receiptSnapshots = res.Snapshots
					// Phase E — corpus-drift staleness. If the receipt was issued
					// with snapshots (artifact_id, revision_number at search time)
					// and ALL of those artifacts have since moved past their
					// snapshotted revision, the receipt is stale in the one sense
					// that matters: the agent searched a corpus that no longer
					// exists. Partial drift is just a warning (see checkReceiptSupersedes).
					if len(receiptSnapshots) > 0 {
						superseded, err := checkReceiptSupersedes(ctx, deps, receiptSnapshots)
						if err != nil {
							deps.Logger.Warn("receipt supersede check failed — allowing write", "err", err)
						} else if len(superseded) == len(receiptSnapshots) {
							return nil, artifactProposeOutput{
								Status:           "not_ready",
								ErrorCode:        "RECEIPT_SUPERSEDED",
								Failed:           []string{"RECEIPT_SUPERSEDED"},
								Checklist:        []string{i18n.T(lang, "preflight.receipt_superseded")},
								SuggestedActions: []string{i18n.T(lang, "suggested.call_search_first")},
								NextTools:        defaultNextTools("RECEIPT_SUPERSEDED"),
								PatchableFields:  patchFieldsFor("RECEIPT_SUPERSEDED"),
							}, nil
						}
					}
				}
				if in.Type == "Task" {
					activeTasks, err := activeTasksInArea(ctx, deps, projectID, areaID, "")
					if err != nil {
						return nil, artifactProposeOutput{}, fmt.Errorf("active task exposure check: %w", err)
					}
					if len(activeTasks) > 0 && !receiptSnapshotsContainAny(receiptSnapshots, activeTasks) {
						related := make([]RelatedRef, 0, len(activeTasks))
						for _, c := range activeTasks {
							related = append(related, makeRelated(
								deps, scope, c.Slug, c.ArtifactID, "Task", c.Title,
								"active Task in same area; read acceptance before creating a new Task",
							))
							if len(related) >= semanticConflictLimit {
								break
							}
						}
						return nil, artifactProposeOutput{
							Status:    "not_ready",
							ErrorCode: "TASK_ACTIVE_CONTEXT_REQUIRED",
							Failed:    []string{"TASK_ACTIVE_CONTEXT_REQUIRED"},
							Checklist: []string{
								"New Task create path must expose active same-area Task acceptance through the search_receipt snapshot.",
							},
							SuggestedActions: []string{
								"Call pindoc.artifact.search with type=Task and the same area, or read the related Task(s), then retry with a fresh receipt.",
							},
							NextTools:       toolHints("pindoc.artifact.search", "pindoc.artifact.read"),
							PatchableFields: []string{"basis.search_receipt"},
							Related:         related,
						}, nil
					}
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
				`, scope.ProjectSlug, ref).Scan(&supersedeTargetID)
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
					  AND status <> 'superseded'
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
							makeRelated(deps, scope, existingSlug, existingID, "", in.Title, "exact title match"),
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
					} else if in.Type == "Task" && len(candidates) > 0 {
						taskCandidates, err := filterTaskSemanticCandidates(ctx, deps, projectID, areaID, candidates)
						if err != nil {
							return nil, artifactProposeOutput{}, fmt.Errorf("filter task semantic conflicts: %w", err)
						}
						if len(taskCandidates) > 0 {
							unrelatedReason := strings.TrimSpace(in.UnrelatedReason)
							if unrelatedReason != "" && utf8.RuneCountInString(unrelatedReason) < unrelatedReasonMinRunes {
								return nil, artifactProposeOutput{
									Status:    "not_ready",
									ErrorCode: "UNRELATED_REASON_TOO_SHORT",
									Failed:    []string{"UNRELATED_REASON_TOO_SHORT"},
									Checklist: []string{
										fmt.Sprintf("unrelated_reason must be at least %d characters when bypassing active Task semantic conflicts.", unrelatedReasonMinRunes),
									},
									PatchableFields: []string{"unrelated_reason"},
								}, nil
							}
							if unrelatedReason == "" {
								rel := make([]string, 0, len(candidates))
								related := make([]RelatedRef, 0, len(taskCandidates))
								for _, c := range taskCandidates {
									rel = append(rel, fmt.Sprintf("[%s] %s — /p/%s/wiki/%s (distance %.3f)", c.Type, c.Title, scope.ProjectSlug, c.Slug, c.Distance))
									related = append(related, makeRelated(
										deps, scope, c.Slug, c.ArtifactID, c.Type, c.Title,
										fmt.Sprintf("cosine distance %.3f", c.Distance),
									))
								}
								return nil, artifactProposeOutput{
									Status:    "not_ready",
									ErrorCode: "TASK_SUPERSEDE_REQUIRED",
									Failed:    []string{"TASK_SUPERSEDE_REQUIRED"},
									Checklist: []string{
										fmt.Sprintf("New Task semantically overlaps active Task %q (distance %.3f). Use supersede_of or provide unrelated_reason.", taskCandidates[0].Slug, taskCandidates[0].Distance),
									},
									SuggestedActions: append(
										[]string{"Read the candidate Task acceptance, then retry with supersede_of=<old-task-slug> or unrelated_reason=<why it does not overlap>."},
										rel...,
									),
									NextTools:       toolHints("pindoc.artifact.read", "pindoc.artifact.propose"),
									PatchableFields: []string{"supersede_of", "unrelated_reason"},
									Related:         related,
								}, nil
							}
						}
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
			bodyLocale := normalizeBodyLocale(in.BodyLocale)
			if bodyLocale == "" {
				bodyLocale = normalizeBodyLocale(scope.ProjectLocale)
			}
			if bodyLocale == "" {
				bodyLocale = "en"
			}
			commitMsgWarnings := applyCreateCommitMsgFallback(&in)

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
			reviewOp := policy.OpCompletenessWrite
			if supersedeTargetID != "" {
				reviewOp = policy.OpSupersede
			}
			reviewState := policy.ReviewStateFor(sensitiveOps, reviewOp, policy.SensitiveContext{
				ToCompleteness: completeness,
			})
			for attempt := 0; attempt < 10; attempt++ {
				err = tx.QueryRow(ctx, `
					INSERT INTO artifacts (
						project_id, area_id, slug, type, title, body_markdown, tags,
						body_locale,
						completeness, status, review_state,
						author_kind, author_id, author_version, author_user_id,
						task_meta, artifact_meta, published_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'published', $15, 'agent', $10, $11, NULLIF($12, '')::uuid, $13, $14::jsonb, now())
					RETURNING id::text, published_at
				`, projectID, areaID, finalSlug, in.Type, in.Title, in.BodyMarkdown, in.Tags,
					bodyLocale, completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion), p.UserID, taskMetaJSON, artifactMetaJSON, reviewState).Scan(&newID, &publishedAt)
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

			if reviewState == policy.ReviewStatePending {
				if err := recordReviewRequiredEvent(ctx, tx, projectID, newID, string(reviewOp), in.AuthorID); err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("review required event: %w", err)
				}
			} else {
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
			}
			if err := recordAreaSuggestionResolvedEvent(ctx, tx, projectID, newID, areaSuggestionCorrelation(in), in.AreaSlug, finalSlug, in.AuthorID); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("area suggestion resolve event: %w", err)
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
			relTargets, relErr := resolveRelatesTo(ctx, tx, scope.ProjectSlug, in.RelatesTo, lang)
			if relErr != nil {
				return nil, *relErr, nil
			}
			edgesStored, err := insertEdges(ctx, tx, newID, relTargets, in.RelatesTo)
			if err != nil {
				return nil, artifactProposeOutput{}, err
			}

			// --- pins ---------------------------------------------------
			pinsStored, repoWarnings, err := insertPins(ctx, tx, projectID, newID, in.Pins, deps.RepoRoot)
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

			// First revision — keep the invariant that every artifact has
			// at least one artifact_revisions row. Phase A of the revision-
			// shape refactor moves this inside the tx so a best-effort
			// failure can't leave an artifact with head() = 0 ever again
			// (migration 0017 mopped up the pre-fix backlog).
			if _, err := tx.Exec(ctx, `
				INSERT INTO artifact_revisions (
					artifact_id, revision_number, title, body_markdown, body_hash, tags,
					completeness, author_kind, author_id, author_version, commit_msg,
					source_session_ref, revision_shape
				) VALUES ($1, 1, $2, $3, $4, $5, $6, 'agent', $7, $8, $9, $10, 'body_patch')
			`, newID, in.Title, in.BodyMarkdown, bodyHash(in.BodyMarkdown), in.Tags,
				completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion),
				in.CommitMsg,
				buildSourceSessionRef(p, in),
			); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("initial revision insert: %w", err)
			}

			if in.DryRun {
				if err := tx.Rollback(ctx); err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("rollback dry_run create: %w", err)
				}
			} else if err := tx.Commit(ctx); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
			}

			warnings := createWarnings(ctx, deps, projectID, in.Title, in.BodyMarkdown)
			warnings = append(warnings, repoWarnings...)
			warnings = append(warnings, commitMsgWarnings...)
			warnings = append(warnings, pinPathWarnings(deps, in.Pins)...)
			warnings = append(warnings, titleQualityWarnings(in.Title, in.BodyLocale, projectTitleJargon(deps))...)
			warnings = append(warnings, bodyH1Warnings(in.BodyMarkdown)...)
			warnings = append(warnings, requiredH2WarningsFor(ctx, deps, scope.ProjectSlug, in.BodyMarkdown, in.Type)...)
			warnings = append(warnings, sectionDuplicatesEdgesWarnings(in.BodyMarkdown)...)
			warnings = append(warnings, decisionSubjectAreaWarnings(in)...)
			slugWarnings, slugSuggestedActions := slugBrevityAdvisory(in, finalSlug)
			warnings = append(warnings, slugWarnings...)
			suggestedActions := append([]string{}, slugSuggestedActions...)
			suggestedActions = append(suggestedActions, sectionDuplicatesEdgesSuggestedActions(warnings)...)
			if detectUnclassifiedUserChat(resolvedMeta, in.Pins, in.BodyMarkdown) {
				warnings = append(warnings, "SOURCE_TYPE_UNCLASSIFIED")
			}
			if consentRequiredForUserChatWarning(isConvDerived, resolvedMeta, in.BodyMarkdown) {
				warnings = append(warnings, "CONSENT_REQUIRED_FOR_USER_CHAT")
			}
			// Invalidate validator hints when a new `_template_*` row lands
			// — the pre-insert cache may have a negative-cache entry for
			// this type that we need to clear so the next propose picks
			// up the template's meta comment.
			if !in.DryRun {
				invalidateValidatorHints(scope.ProjectSlug, finalSlug)
			}

			// Task propose-경로-warning-영속화: persist the accepted-path
			// warnings into events so Reader Trust Card and future
			// sessions can surface them. Best-effort — event failure
			// doesn't roll back the artifact.
			if !in.DryRun {
				recordWarningEvent(ctx, deps, projectID, newID, 1, warnings, in.AuthorID, false)
			}

			metaOut := resolvedMeta
			sortedWarnings := sortWarningsBySeverity(warnings)
			severities := make([]string, len(sortedWarnings))
			for i, w := range sortedWarnings {
				severities[i] = warningSeverity(w)
			}
			out := artifactProposeOutput{
				Status:            "accepted",
				ArtifactID:        newID,
				Slug:              finalSlug,
				AgentRef:          "pindoc://" + finalSlug,
				HumanURL:          HumanURL(scope.ProjectSlug, scope.ProjectLocale, finalSlug),
				HumanURLAbs:       AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, finalSlug),
				PublishedAt:       publishedAt,
				Created:           true,
				RevisionNumber:    1,
				PinsStored:        pinsStored,
				EdgesStored:       edgesStored,
				Superseded:        supersededFlag,
				SuggestedActions:  suggestedActions,
				Warnings:          sortedWarnings,
				WarningSeverities: severities,
				EmbedderUsed:      embedderInfo(deps),
				ArtifactMeta:      &metaOut,
				ReceiptExempted:   receiptExempted,
				ToolsetVersion:    ToolsetVersion(),
			}
			if in.DryRun {
				out = dryRunProposeOutput(out, false)
			}
			return nil, out, nil
		},
	)
}

// handleUpdate writes a new revision for an existing artifact, updates the
// head row, re-chunks embeddings, and emits an event. Runs in a single
// transaction so search never sees a half-indexed update.
//
// Phase B revision-shapes dispatch: the shape field (validated in
// preflight) decides which writer runs. Phase B wired body_patch only;
// Phase C adds meta_patch. acceptance_transition / scope_defer return
// SHAPE_NOT_IMPLEMENTED until Phase D / F light them up.
func handleUpdate(ctx context.Context, deps Deps, p *auth.Principal, scope *auth.ProjectScope, in artifactProposeInput, lang string) (*sdk.CallToolResult, artifactProposeOutput, error) {
	shape, _ := parseShape(in.Shape) // preflight already validated
	switch shape {
	case ShapeBodyPatch, ShapeAcceptanceTransition, ShapeScopeDefer:
		// Fall through — body_patch is the plain-body path; acceptance_
		// transition materialises its rewritten body below; scope_defer
		// synthesises an acceptance transition to [-] plus an edge insert
		// in the same tx. All three travel the body-update flow.
	case ShapeMetaPatch:
		return handleUpdateMetaPatch(ctx, deps, p, scope, in, lang)
	default:
		return nil, artifactProposeOutput{
			Status:          "not_ready",
			ErrorCode:       "SHAPE_NOT_IMPLEMENTED",
			Failed:          []string{"SHAPE_NOT_IMPLEMENTED"},
			Checklist:       []string{fmt.Sprintf(i18n.T(lang, "preflight.shape_not_implemented"), in.Shape)},
			PatchableFields: patchFieldsFor("SHAPE_NOT_IMPLEMENTED"),
		}, nil
	}
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

	var artifactID, projectID, currentBody, currentTitle, currentType, currentSlug, sensitiveOps string
	var currentTags []string
	var currentCompleteness string
	var currentMetaRaw, currentTaskMetaRaw []byte
	var lastRev int
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.body_markdown, a.title, a.type, a.slug,
		       a.tags, a.completeness, a.artifact_meta, a.task_meta,
		       COALESCE(NULLIF(p.sensitive_ops, ''), 'auto'),
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, scope.ProjectSlug, ref).Scan(
		&artifactID, &projectID, &currentBody, &currentTitle, &currentType, &currentSlug,
		&currentTags, &currentCompleteness, &currentMetaRaw, &currentTaskMetaRaw, &sensitiveOps, &lastRev,
	)
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
				makeRelated(deps, scope, ref, artifactID, "", currentTitle, fmt.Sprintf("current revision = %d; pass expected_version = %d", lastRev, lastRev)),
			},
		}, nil
	}

	// Acceptance-transition materialisation (Phase D) + scope-defer
	// (Phase F). For shape=acceptance_transition the caller sends a
	// locator + new_state + reason. For shape=scope_defer the caller
	// additionally names a target artifact — the server synthesises an
	// AcceptanceTransition to [-] and records an artifact_scope_edges
	// row in the same tx so the graph and body stay in lockstep. Either
	// path ends with in.BodyMarkdown = rewritten body so the rest of
	// handleUpdate proceeds as if the caller had sent body_markdown
	// whole. Canonical-rewrite / body-warning gates are bypassed for
	// both shapes — a single-byte marker flip isn't a claim rewrite.
	var acceptanceShapePayload []byte
	var scopeDeferTargetID string
	var scopeDeferTargetSlug string
	if shape == ShapeAcceptanceTransition || shape == ShapeScopeDefer {
		acceptanceInput := in.AcceptanceTransition
		if shape == ShapeScopeDefer {
			if in.ScopeDefer == nil {
				return nil, artifactProposeOutput{
					Status:          "not_ready",
					ErrorCode:       "SCOPE_DEFER_REQUIRED",
					Failed:          []string{"SCOPE_DEFER_REQUIRED"},
					Checklist:       []string{i18n.T(lang, "preflight.scope_defer_required")},
					PatchableFields: patchFieldsFor("SCOPE_DEFER_REQUIRED"),
				}, nil
			}
			if strings.TrimSpace(in.ScopeDefer.Reason) == "" {
				return nil, artifactProposeOutput{
					Status:          "not_ready",
					ErrorCode:       "SCOPE_DEFER_REASON_REQUIRED",
					Failed:          []string{"SCOPE_DEFER_REASON_REQUIRED"},
					Checklist:       []string{i18n.T(lang, "preflight.scope_defer_reason_required")},
					PatchableFields: patchFieldsFor("SCOPE_DEFER_REASON_REQUIRED"),
				}, nil
			}
			// Resolve target artifact — must exist in same project, not archived.
			targetRef := normalizeRef(in.ScopeDefer.ToArtifact)
			err := deps.DB.QueryRow(ctx, `
				SELECT a.id::text, a.slug FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
				  AND a.status <> 'archived'
				LIMIT 1
			`, scope.ProjectSlug, targetRef).Scan(&scopeDeferTargetID, &scopeDeferTargetSlug)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: "SCOPE_DEFER_TARGET_NOT_FOUND",
					Failed:    []string{"SCOPE_DEFER_TARGET_NOT_FOUND"},
					Checklist: []string{
						fmt.Sprintf(i18n.T(lang, "preflight.scope_defer_target_missing"), in.ScopeDefer.ToArtifact),
					},
					NextTools:       defaultNextTools("SCOPE_DEFER_TARGET_NOT_FOUND"),
					PatchableFields: patchFieldsFor("SCOPE_DEFER_TARGET_NOT_FOUND"),
				}, nil
			}
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("resolve scope-defer target: %w", err)
			}
			if scopeDeferTargetID == artifactID {
				return nil, artifactProposeOutput{
					Status:          "not_ready",
					ErrorCode:       "SCOPE_DEFER_SELF",
					Failed:          []string{"SCOPE_DEFER_SELF"},
					Checklist:       []string{i18n.T(lang, "preflight.scope_defer_self")},
					PatchableFields: patchFieldsFor("SCOPE_DEFER_SELF"),
				}, nil
			}
			acceptanceInput = &AcceptanceTransitionInput{
				CheckboxIndex: in.ScopeDefer.CheckboxIndex,
				NewState:      "[-]",
				Reason:        fmt.Sprintf("moved to %s: %s", scopeDeferTargetSlug, strings.TrimSpace(in.ScopeDefer.Reason)),
			}
		}
		if acceptanceInput == nil {
			return nil, artifactProposeOutput{
				Status:          "not_ready",
				ErrorCode:       "ACCEPT_TRANSITION_REQUIRED",
				Failed:          []string{"ACCEPT_TRANSITION_REQUIRED"},
				Checklist:       []string{acceptanceTransitionChecklist(lang, "ACCEPT_TRANSITION_REQUIRED")},
				PatchableFields: patchFieldsFor("ACCEPT_TRANSITION_REQUIRED"),
			}, nil
		}
		newBody, fromMarker, code := applyAcceptanceTransition(currentBody, acceptanceInput)
		if code != "" {
			return nil, artifactProposeOutput{
				Status:          "not_ready",
				ErrorCode:       code,
				Failed:          []string{code},
				Checklist:       []string{acceptanceTransitionChecklist(lang, code)},
				PatchableFields: patchFieldsFor(code),
			}, nil
		}
		in.BodyMarkdown = newBody
		payload := map[string]any{
			"checkbox_index": *acceptanceInput.CheckboxIndex,
			"from_state":     string([]byte{fromMarker}),
			"new_state":      strings.TrimSpace(acceptanceInput.NewState),
		}
		if r := strings.TrimSpace(acceptanceInput.Reason); r != "" {
			payload["reason"] = r
		}
		if shape == ShapeScopeDefer {
			payload["to_artifact_id"] = scopeDeferTargetID
			payload["to_artifact_slug"] = scopeDeferTargetSlug
			payload["scope_defer_reason"] = strings.TrimSpace(in.ScopeDefer.Reason)
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, artifactProposeOutput{}, fmt.Errorf("marshal acceptance payload: %w", err)
		}
		acceptanceShapePayload = b
	}

	// Body patch materialisation (Task artifact-propose-본문-patch-입력-도입).
	// Runs before the version check's error path but after currentBody has
	// been fetched — we need prev body to apply the patch, and the caller
	// expects the rest of handleUpdate to see the resolved body exactly
	// as if they had sent body_markdown whole. Warnings (e.g. PATCH_NOOP)
	// bubble up into the accepted response alongside other advisories.
	var patchWarnings []string
	if in.BodyPatch != nil {
		newBody, w, patchErr := applyBodyPatch(currentBody, in.BodyPatch)
		if patchErr != "" {
			return nil, artifactProposeOutput{
				Status:    "not_ready",
				ErrorCode: patchErr,
				Failed:    []string{patchErr},
				Checklist: []string{patchExplain(patchErr)},
			}, nil
		}
		in.BodyMarkdown = newBody
		patchWarnings = append(patchWarnings, w...)
	}

	autoClaimedDone := shouldAutoClaimDone(currentType, currentTaskMetaRaw, in.BodyMarkdown)

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
				makeRelated(deps, scope, ref, artifactID, "", currentTitle, fmt.Sprintf("current revision = %d, not %d", lastRev, *in.ExpectedVersion)),
			},
		}, nil
	}

	// No-op detection: identical body + title with no explicit metadata
	// change → reject so history stays clean. Task status transitions
	// intentionally use this body-update lane until pindoc.task.transition
	// exists, so task_meta must count as a meaningful change.
	if currentBody == in.BodyMarkdown && currentTitle == in.Title && !hasExplicitMetadataUpdate(in) {
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
	`, scope.ProjectSlug, in.AreaSlug).Scan(&areaID); err != nil {
		return nil, artifactProposeOutput{
			Status:          "not_ready",
			ErrorCode:       "AREA_UNKNOWN",
			Failed:          []string{"AREA_UNKNOWN"},
			Checklist:       areaUnknownChecklist(ctx, deps, scope, lang, in.AreaSlug),
			NextTools:       defaultNextTools("AREA_UNKNOWN"),
			PatchableFields: patchFieldsFor("AREA_UNKNOWN"),
		}, nil
	}

	if in.Tags == nil {
		in.Tags = currentTags
	}
	completeness := in.Completeness
	if completeness == "" {
		completeness = currentCompleteness
		if completeness == "" {
			completeness = "partial"
		}
	}
	reviewState := policy.ReviewStateFor(sensitiveOps, policy.OpCompletenessWrite, policy.SensitiveContext{
		FromCompleteness: currentCompleteness,
		ToCompleteness:   completeness,
	})

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	newRev := lastRev + 1
	var revID string
	var shapePayloadArg any
	if len(acceptanceShapePayload) > 0 {
		shapePayloadArg = string(acceptanceShapePayload)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, author_version, commit_msg,
			source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'agent', $8, $9, $10, $11, $12, $13::jsonb)
		RETURNING id::text
	`, artifactID, newRev, in.Title, in.BodyMarkdown, bodyHash(in.BodyMarkdown),
		in.Tags, completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion), in.CommitMsg,
		buildSourceSessionRef(p, in), string(shape), shapePayloadArg,
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
		       task_meta      = CASE
		           WHEN $10::bool THEN jsonb_set(COALESCE(task_meta, '{}'::jsonb), '{status}', '"claimed_done"')
		           ELSE COALESCE($8, task_meta)
		       END,
		       artifact_meta  = COALESCE($9::jsonb, artifact_meta),
		       review_state   = CASE
		           WHEN $11::text = 'pending_review' THEN 'pending_review'
		           ELSE review_state
		       END,
		       updated_at     = now()
		 WHERE id = $1
		RETURNING slug, COALESCE(published_at, now())
	`, artifactID, in.Title, in.BodyMarkdown, in.Tags, completeness,
		in.AuthorID, nullIfEmpty(in.AuthorVersion), taskMetaPatch, artifactMetaPatch, autoClaimedDone,
		reviewState,
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
	if err := recordAreaSuggestionResolvedEvent(ctx, tx, projectID, artifactID, areaSuggestionCorrelation(in), in.AreaSlug, slug, in.AuthorID); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("area suggestion resolve event: %w", err)
	}
	if reviewState == policy.ReviewStatePending {
		if err := recordReviewRequiredEvent(ctx, tx, projectID, artifactID, string(policy.OpCompletenessWrite), in.AuthorID); err != nil {
			return nil, artifactProposeOutput{}, fmt.Errorf("review required event: %w", err)
		}
	}

	// Phase F scope-defer edge write. Runs inside the same tx so the
	// graph and the body's [-] marker commit or roll back together.
	// from_item_ref is the agent-facing locator ("acceptance[N]") — Reader
	// resolves it against the current body at query time so subsequent
	// body edits that renumber checkboxes don't break the edge.
	if shape == ShapeScopeDefer && in.ScopeDefer != nil {
		fromItemRef := fmt.Sprintf("acceptance[%d]", *in.ScopeDefer.CheckboxIndex)
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_scope_edges (
				from_artifact_id, from_item_ref, to_artifact_id,
				reason, created_by_user_id, created_by_agent
			) VALUES ($1::uuid, $2, $3::uuid, $4, NULLIF($5, '')::uuid, $6)
			ON CONFLICT (from_artifact_id, from_item_ref, to_artifact_id) DO NOTHING
		`, artifactID, fromItemRef, scopeDeferTargetID,
			strings.TrimSpace(in.ScopeDefer.Reason), p.UserID, in.AuthorID,
		); err != nil {
			return nil, artifactProposeOutput{}, fmt.Errorf("scope edge insert: %w", err)
		}
	}

	// pins and edges are additive on update (append new pins; edges are
	// idempotent on (source, target, relation)). If an agent needs to
	// "replace" all pins they should supersede rather than update.
	relTargets, relErr := resolveRelatesTo(ctx, tx, scope.ProjectSlug, in.RelatesTo, lang)
	if relErr != nil {
		return nil, *relErr, nil
	}
	edgesStored, err := insertEdges(ctx, tx, artifactID, relTargets, in.RelatesTo)
	if err != nil {
		return nil, artifactProposeOutput{}, err
	}
	pinsStored, repoWarnings, err := insertPins(ctx, tx, projectID, artifactID, in.Pins, deps.RepoRoot)
	if err != nil {
		return nil, artifactProposeOutput{}, err
	}

	if in.DryRun {
		if err := tx.Rollback(ctx); err != nil {
			return nil, artifactProposeOutput{}, fmt.Errorf("rollback dry_run update: %w", err)
		}
	} else if err := tx.Commit(ctx); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
	}

	// Template validator cache invalidation — revising `_template_*`
	// should re-anchor the per-type preflight rules without a server
	// restart (Task preflight-template-drift-통합 §cache invalidation).
	if !in.DryRun {
		invalidateValidatorHints(scope.ProjectSlug, slug)
	}

	var updateMetaOut *ResolvedArtifactMeta
	if in.ArtifactMeta != nil {
		m := resolvedUpdateMeta
		updateMetaOut = &m
	}

	// Canonical-claim rewrite guard — compare prev/new H2 sections for
	// types that carry a canonical truth claim (Debug, Decision, Analysis)
	// and require fresh evidence when that section's content shifts.
	warnings := updatePathWarnings(ctx, deps, scope.ProjectSlug, in)
	warnings = append(warnings, repoWarnings...)
	warnings = append(warnings, acceptanceUncheckedNudgeWarnings(currentType, in.BodyMarkdown, in.CommitMsg)...)
	warnings = append(warnings, decisionSubjectAreaWarnings(in)...)
	// Body-patch warnings bubble up here so PATCH_NOOP etc. sit alongside
	// canonical-rewrite / source-type advisories instead of a separate
	// response field.
	warnings = append(warnings, patchWarnings...)
	var suggested []string
	var prevMeta ResolvedArtifactMeta
	if len(currentMetaRaw) > 0 {
		_ = json.Unmarshal(currentMetaRaw, &prevMeta)
	}
	// Phase B/D exclusions — templates ARE the canonical source for their
	// type's section layout, so rewriting ## Decision / ## Root cause on
	// _template_* is the intended edit path (B). Acceptance-transition
	// shape flips a single checkbox marker byte; that can't be a canonical
	// claim rewrite by construction, so skip the guard for that shape (D).
	rewrittenSections := detectCanonicalClaimRewrite(currentBody, in.BodyMarkdown, currentType)
	canonicalRewriteFlag := false
	if len(rewrittenSections) > 0 && !hasEvidenceDelta(prevMeta, &in) && !isTemplateArtifact(currentSlug) && shape != ShapeAcceptanceTransition && shape != ShapeScopeDefer && !in.WordingFix && !in.AddPin {
		canonicalRewriteFlag = true
		warnings = append(warnings, "CANONICAL_REWRITE_WITHOUT_EVIDENCE:"+strings.Join(rewrittenSections, "+"))
		suggested = []string{
			"If the new content is a hypothesis, file it as a fresh Analysis or Debug draft rather than rewriting the canonical claim.",
			"If verified, set artifact_meta.verification_state=verified and attach the evidence pin on the same propose.",
			"If this is wording cleanup only, mention 'wording cleanup' (or 'verified') in commit_msg so the warning stays quiet next time.",
		}
	}
	suggested = append(suggested, sectionDuplicatesEdgesSuggestedActions(warnings)...)

	// Task propose-경로-warning-영속화: persist update-path warnings +
	// canonical-rewrite flag into events so Reader Trust Card and future
	// sessions can surface them. Best-effort — event failure doesn't
	// roll back the revision.
	if !in.DryRun {
		recordWarningEvent(ctx, deps, projectID, artifactID, newRev, warnings, in.AuthorID, canonicalRewriteFlag)
	}

	sortedWarnings := sortWarningsBySeverity(warnings)
	severities := make([]string, len(sortedWarnings))
	for i, w := range sortedWarnings {
		severities[i] = warningSeverity(w)
	}
	out := artifactProposeOutput{
		Status:                          "accepted",
		ArtifactID:                      artifactID,
		Slug:                            slug,
		AgentRef:                        "pindoc://" + slug,
		HumanURL:                        HumanURL(scope.ProjectSlug, scope.ProjectLocale, slug),
		HumanURLAbs:                     AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, slug),
		PublishedAt:                     publishedAt,
		Created:                         false,
		RevisionNumber:                  newRev,
		PinsStored:                      pinsStored,
		EdgesStored:                     edgesStored,
		Warnings:                        sortedWarnings,
		WarningSeverities:               severities,
		SuggestedActions:                suggested,
		EmbedderUsed:                    embedderInfo(deps),
		ArtifactMeta:                    updateMetaOut,
		CanonicalRewriteWithoutEvidence: canonicalRewriteFlag,
		ToolsetVersion:                  ToolsetVersion(),
	}
	if in.DryRun {
		out = dryRunProposeOutput(out, true)
	}
	return nil, out, nil
}

// updatePathWarnings aggregates non-blocking advisories for the update
// (revision) path. Skips the semantic near-duplicate probe — updating in
// place can't produce a duplicate — but runs the structural/pin gates so
// the agent learns about title length / heading / pin-path issues on
// revised artifacts too.
func updatePathWarnings(ctx context.Context, deps Deps, projectSlug string, in artifactProposeInput) []string {
	var out []string
	out = append(out, pinPathWarnings(deps, in.Pins)...)
	out = append(out, titleQualityWarnings(in.Title, in.BodyLocale, projectTitleJargon(deps))...)
	out = append(out, bodyH1Warnings(in.BodyMarkdown)...)
	out = append(out, requiredH2WarningsFor(ctx, deps, projectSlug, in.BodyMarkdown, in.Type)...)
	out = append(out, sectionDuplicatesEdgesWarnings(in.BodyMarkdown)...)
	return out
}

func acceptanceUncheckedNudgeWarnings(artifactType, body, commitMsg string) []string {
	if artifactType != "Task" || !commitMsgSuggestsClose(commitMsg) {
		return nil
	}
	resolved, _ := countAcceptanceCheckboxes(body)
	if resolved == 0 {
		return []string{warningAcceptanceUnchecked}
	}
	return nil
}

func commitMsgSuggestsClose(commitMsg string) bool {
	msg := strings.ToLower(strings.TrimSpace(commitMsg))
	if msg == "" {
		return false
	}
	for _, phrase := range []string{"closes pindoc://", "완료", "해결"} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	for _, word := range closeSuggestiveCommitWordRe.FindAllString(msg, -1) {
		switch word {
		case "fix", "resolve", "close":
			return true
		}
	}
	return false
}

// projectTitleJargon returns the operator-supplied jargon set that
// extends the embedded locale baseline. Today the override path is not
// yet wired into server_settings — this returns nil and the embedded
// LocaleData jargon set is the only signal. The signature is in place
// so the follow-up that lands `server_settings.title_jargon_tokens`
// (and eventually `project_settings`) flips a single helper without
// touching every call site. See docs/CONTRIBUTING_LOCALE.md for the
// extensibility contract this anchors.
func projectTitleJargon(_ Deps) []string {
	return nil
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
//   - bulk_op_id: batch operation correlation key
//
// Returns nil when there's nothing useful to record so the column stays
// NULL rather than storing {} everywhere.
func buildSourceSessionRef(p *auth.Principal, in artifactProposeInput) any {
	payload := map[string]any{}
	if p != nil && strings.TrimSpace(p.AgentID) != "" {
		payload["agent_id"] = p.AgentID
	}
	if strings.TrimSpace(in.AuthorID) != "" {
		payload["reported_author_id"] = in.AuthorID
	}
	if in.Basis != nil {
		if s := strings.TrimSpace(in.Basis.SourceSession); s != "" {
			payload["source_session"] = s
		}
		if b := strings.TrimSpace(in.Basis.BulkOpID); b != "" {
			payload["bulk_op_id"] = b
		}
		if r := strings.TrimSpace(in.Basis.SearchReceipt); r != "" {
			payload["search_receipt"] = r
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

func areaSuggestionCorrelation(in artifactProposeInput) string {
	if in.Basis == nil {
		return ""
	}
	return strings.TrimSpace(in.Basis.SearchReceipt)
}

func recordAreaSuggestionResolvedEvent(ctx context.Context, tx pgx.Tx, projectID, artifactID, correlationID, finalAreaSlug, slug, authorID string) error {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'agent.area_suggestion_resolved', $2, jsonb_build_object(
			'correlation_id',  $3::text,
			'final_area_slug', $4::text,
			'artifact_slug',   $5::text,
			'author_id',       $6::text
		))
	`, projectID, artifactID, correlationID, finalAreaSlug, slug, authorID)
	return err
}

func recordReviewRequiredEvent(ctx context.Context, tx pgx.Tx, projectID, artifactID, op, authorID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'review.required', $2, jsonb_build_object(
			'op',        $3::text,
			'author_id', $4::text
		))
	`, projectID, artifactID, op, authorID)
	return err
}

// preflight runs the cheap synchronous checks. Returns a list of ✗-prefixed
// lines (legacy natural-language) + parallel stable-code list + short
// ErrorCode for the first failure. Empty lists mean clean.
//
// Phase 11a change: every check now emits both a natural-language line
// (legacy) AND a stable code (Phase 12-style). Phase 12 will make codes
// primary and prose optional.
func preflight(ctx context.Context, deps Deps, projectSlug string, in *artifactProposeInput, lang string) (checklist []string, failed []string, code string) {
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
	// body_patch / body_markdown mutual exclusion + path gating.
	// body_patch is an update_of-only convenience — create and supersede
	// reject it because there's no prior body to patch against. Both
	// fields together would be ambiguous, so PATCH_EXCLUSIVE.
	if in.BodyPatch != nil {
		if strings.TrimSpace(in.BodyMarkdown) != "" {
			push("body_patch cannot be combined with body_markdown — pick one.", "PATCH_EXCLUSIVE")
		}
		if in.UpdateOf == "" || in.SupersedeOf != "" {
			push("body_patch is only valid with update_of; create and supersede paths require body_markdown.", "PATCH_UPDATE_ONLY")
		}
	}
	// Phase B+D+F: shape=meta_patch / acceptance_transition / scope_defer
	// derive their final body from server-side state or a dedicated payload,
	// so an empty body_markdown at preflight time is expected — skip the
	// BODY_EMPTY gate for those shapes.
	if shape := strings.TrimSpace(in.Shape); shape == "" || shape == string(ShapeBodyPatch) {
		if strings.TrimSpace(in.BodyMarkdown) == "" && in.BodyPatch == nil {
			push(i18n.T(lang, "preflight.body_empty"), "BODY_EMPTY")
		}
	}
	if strings.TrimSpace(in.AreaSlug) == "" {
		push(i18n.T(lang, "preflight.area_empty"), "AREA_EMPTY")
	}
	if in.Type == "Decision" && strings.TrimSpace(in.AreaSlug) == "decisions" {
		push(i18n.T(lang, "preflight.decision_area_deprecated"), "DECISION_AREA_DEPRECATED")
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
		for _, area := range m.AppliesToAreas {
			if !validRuleAreaScope(area) {
				push(fmt.Sprintf("artifact_meta.applies_to_areas contains invalid scope %q; use area_slug, *, or wildcard scope like ui/*", area), "META_APPLIES_AREA_INVALID")
				break
			}
		}
		for _, artifactType := range m.AppliesToTypes {
			t := strings.TrimSpace(artifactType)
			if _, ok := validArtifactTypes[t]; !ok {
				push(fmt.Sprintf("artifact_meta.applies_to_types contains invalid type %q", artifactType), "META_APPLIES_TYPE_INVALID")
				break
			}
		}
		if v := strings.TrimSpace(m.RuleSeverity); v != "" {
			if _, ok := validRuleSeverities[v]; !ok {
				push(fmt.Sprintf("artifact_meta.rule_severity %q is not one of binding|guidance|reference", v), "META_RULE_SEVERITY_INVALID")
			}
		}
	}

	// Type-specific guardrails. The previous code hard-coded keywords
	// (`"acceptance"`, `"reproduction"`, …) directly in this switch; the
	// drift between those strings and the canonical `_template_*` bodies
	// caused the 1차 dogfood TASK_NO_ACCEPTANCE loop. Now each type reads
	// its required_keywords from the matching `_template_<type>` body's
	// validator meta comment. A nil hint set (no template, no comment,
	// DB unreachable) falls back to the hard-coded defaults below.
	hints := getValidatorHints(ctx, deps, projectSlug, in.Type)
	for _, slot := range missingRequiredH2Slots(in.BodyMarkdown, requiredH2SlotsFromHints(in.Type, hints)) {
		push(fmt.Sprintf(i18n.T(lang, "preflight.h2_missing"), in.Type, slot.Label), "MISSING_H2:"+slot.Label)
	}
	switch in.Type {
	case "Task":
		needed := []string{"acceptance"}
		if hints != nil && len(hints.RequiredKeywords) > 0 {
			needed = hints.RequiredKeywords
		}
		if !bodyContainsAnyKeyword(in.BodyMarkdown, needed) {
			push(i18n.T(lang, "preflight.task_acceptance"), "TASK_NO_ACCEPTANCE")
		}
	case "Decision":
		if hints != nil && len(hints.RequiredKeywords) > 0 {
			if !bodyContainsAllKeywords(in.BodyMarkdown, hints.RequiredKeywords) {
				push(i18n.T(lang, "preflight.adr_sections"), "DEC_NO_SECTIONS")
			}
		} else {
			lower := strings.ToLower(in.BodyMarkdown)
			if !strings.Contains(lower, "decision") || !strings.Contains(lower, "context") {
				push(i18n.T(lang, "preflight.adr_sections"), "DEC_NO_SECTIONS")
			}
		}
	case "Debug":
		// Debug keeps its "at least one repro-ish keyword and at least
		// one resolution-ish keyword" split because that captures the
		// "symptom → resolution" arc better than a single all-or-nothing
		// check. Template hints contribute extra synonyms on top of the
		// hard-coded base set.
		reproKeywords := []string{"reproduction", "repro", "재현", "증상", "symptom"}
		resolutionKeywords := []string{"resolution", "root cause", "원인", "해결"}
		if hints != nil {
			for _, kw := range hints.RequiredKeywords {
				lower := strings.ToLower(kw)
				if strings.Contains(lower, "repro") || strings.Contains(lower, "symptom") ||
					strings.Contains(lower, "증상") || strings.Contains(lower, "재현") {
					reproKeywords = append(reproKeywords, kw)
					continue
				}
				if strings.Contains(lower, "resolution") || strings.Contains(lower, "cause") ||
					strings.Contains(lower, "원인") || strings.Contains(lower, "해결") {
					resolutionKeywords = append(resolutionKeywords, kw)
				}
			}
		}
		if !bodyContainsAnyKeyword(in.BodyMarkdown, reproKeywords) {
			push(i18n.T(lang, "preflight.debug_no_repro"), "DBG_NO_REPRO")
		}
		if !bodyContainsAnyKeyword(in.BodyMarkdown, resolutionKeywords) {
			push(i18n.T(lang, "preflight.debug_no_resolution"), "DBG_NO_RESOLUTION")
		}
	}

	// Phase 11a + 15c: shape-check pins + relates_to. Hard-blocks only on
	// structurally invalid input (empty path, unknown relation/kind).
	// Missing entirely = soft (future escalation NEED_PIN for code-linked
	// types once search_receipt is in place).
	for i, p := range in.Pins {
		kind := pinmodel.NormalizeKind(p.Kind, p.Path)
		if !pinmodel.ValidKind(kind) {
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
	if in.ExpectedVersion != nil {
		switch {
		case *in.ExpectedVersion < 0:
			push(i18n.T(lang, "preflight.expected_version_negative"), "VER_INVALID")
		case *in.ExpectedVersion == 0:
			// Migration 0017 backfilled revision 1 for every seeded artifact
			// and create paths now write the initial revision inside the
			// artifact insert tx — head() = 0 is no longer a legal state.
			push(i18n.T(lang, "preflight.expected_version_reserved"), "FIELD_VALUE_RESERVED")
		}
	}

	// Phase B revision-shapes: validate the optional shape discriminator
	// before the tool dispatches into handleUpdate. Empty = legacy
	// body_patch path; anything else has to be a known enum value. The
	// create path only accepts body_patch (meta-only / acceptance /
	// scope-defer shapes are meaningless without a prior head) and we
	// enforce that here so the error surfaces alongside schema issues
	// rather than mid-handler.
	shape, shapeOK := parseShape(in.Shape)
	if !shapeOK {
		push(fmt.Sprintf(i18n.T(lang, "preflight.shape_invalid"), in.Shape), "SHAPE_INVALID")
	} else if shape != ShapeBodyPatch && strings.TrimSpace(in.UpdateOf) == "" {
		push(i18n.T(lang, "preflight.shape_needs_update"), "SHAPE_REQUIRES_UPDATE")
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
		if a := strings.TrimSpace(tm.Assignee); a != "" {
			if _, ok := validateAssignee(a); !ok {
				push(i18n.T(lang, "preflight.task_assignee_invalid"), "ASSIGNEE_INVALID")
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

// slugBrevityAdvisory was a 25-rune fixed gate before the 2026-04-28
// title-guide locale split. The threshold now comes from
// titleguide.SlugVerboseThreshold(body_locale) so a verbose-slug
// warning fires at a band proportional to the title length the locale
// allows. SLUG_VERBOSE was promoted from info → warn in the same pass
// because dogfood data showed agents (incl. this one) silently shipping
// slugs the info-tier severity made too easy to ignore.
func slugBrevityAdvisory(in artifactProposeInput, finalSlug string) ([]string, []string) {
	if strings.TrimSpace(in.Slug) != "" {
		return nil, nil
	}
	threshold := titleguide.SlugVerboseThreshold(in.BodyLocale)
	n := utf8.RuneCountInString(finalSlug)
	if n < threshold {
		return nil, nil
	}
	actions := []string{
		fmt.Sprintf("SLUG_VERBOSE: auto-generated slug %q is %d runes (locale=%s threshold=%d); pass an explicit shorter slug.", finalSlug, n, strings.TrimSpace(in.BodyLocale), threshold),
	}
	for _, candidate := range conciseSlugCandidates(in.Type, in.Title, finalSlug, threshold) {
		actions = append(actions, fmt.Sprintf("Candidate explicit slug: `%s`", candidate))
	}
	return []string{"SLUG_VERBOSE"}, actions
}

func conciseSlugCandidates(artType, title, finalSlug string, threshold int) []string {
	prefix := slugify(artType)
	if prefix == "" {
		prefix = "artifact"
	}
	titleSlug := slugify(title)
	parts := splitSlugParts(titleSlug)
	if len(parts) == 0 {
		return nil
	}

	var out []string
	seen := map[string]struct{}{}
	add := func(parts []string) {
		if len(parts) == 0 || len(out) >= 3 {
			return
		}
		candidate := prefix + "-" + strings.Join(parts, "-")
		candidate = trimSlugRunes(candidate, threshold)
		if candidate == "" || candidate == finalSlug {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	if len(parts) >= 3 {
		add(parts[:3])
	}
	if len(parts) >= 2 {
		add(parts[:2])
	}
	if len(parts) >= 4 {
		add([]string{parts[0], parts[1], parts[len(parts)-1]})
	}
	add(parts[:1])

	return out
}

func splitSlugParts(slug string) []string {
	raw := strings.Split(slug, "-")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func trimSlugRunes(slug string, maxRunes int) string {
	if utf8.RuneCountInString(slug) <= maxRunes {
		return strings.Trim(slug, "-")
	}
	runes := []rune(slug)
	return strings.Trim(string(runes[:maxRunes]), "-")
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

func areaUnknownChecklist(ctx context.Context, deps Deps, scope *auth.ProjectScope, lang, areaSlug string) []string {
	out := []string{
		fmt.Sprintf(i18n.T(lang, "preflight.area_not_found"), areaSlug, scope.ProjectSlug),
	}
	if deps.DB == nil {
		return out
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT area.slug
		FROM areas area
		JOIN projects p ON p.id = area.project_id
		WHERE p.slug = $1
		ORDER BY CASE WHEN area.parent_id IS NULL THEN 0 ELSE 1 END, area.slug
		LIMIT 24
	`, scope.ProjectSlug)
	if err != nil {
		return out
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return out
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) > 0 {
		out = append(out, "Valid area_slug examples: "+strings.Join(slugs, ", ")+"; full list via pindoc.area.list.")
	}
	return out
}

// defaultNextTools maps a stable fail code to the MCP tools an agent
// should call next to unblock itself. Returning nil means "no suggestion;
// agent should re-read the checklist" — schema-level failures mostly fall
// into that bucket because the fix is "fill in the missing field".
func defaultNextTools(code string) []NextToolHint {
	if isMissingH2Code(code) {
		return toolHints("pindoc.artifact.read", "pindoc.artifact.propose")
	}
	switch code {
	case "NO_SRCH", "RECEIPT_UNKNOWN", "RECEIPT_EXPIRED", "RECEIPT_WRONG_PROJECT", "RECEIPT_SUPERSEDED":
		return toolHints("pindoc.artifact.search", "pindoc.context.for_task")
	case "CONFLICT_EXACT_TITLE", "POSSIBLE_DUP", "TASK_SUPERSEDE_REQUIRED", "TASK_ACTIVE_CONTEXT_REQUIRED":
		return toolHints("pindoc.artifact.read", "pindoc.artifact.propose")
	case "VER_CONFLICT", "FIELD_VALUE_RESERVED":
		return toolHints("pindoc.artifact.revisions", "pindoc.artifact.diff")
	case "UPDATE_TARGET_NOT_FOUND", "SUPERSEDE_TARGET_NOT_FOUND", "REL_TARGET_NOT_FOUND", "SCOPE_DEFER_TARGET_NOT_FOUND":
		return toolHints("pindoc.artifact.search", "pindoc.area.list")
	case "AREA_UNKNOWN", "AREA_NOT_FOUND", "AREA_EMPTY", "DECISION_AREA_DEPRECATED":
		return toolHints("pindoc.area.list")
	case "TASK_NO_ACCEPTANCE", "DEC_NO_SECTIONS", "DBG_NO_REPRO", "DBG_NO_RESOLUTION":
		return toolHints("pindoc.harness.install")
	case "UPDATE_SUPERSEDE_EXCLUSIVE":
		return toolHints("pindoc.artifact.read")
	default:
		return nil
	}
}

func toolHints(names ...string) []NextToolHint {
	out := make([]NextToolHint, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, NextToolHint{Tool: name})
	}
	return out
}

func nextToolsForNotReady(code, artifactType string, failed []string) []NextToolHint {
	tools := defaultNextTools(code)
	if !needsTemplateSelfHealHint(failed) {
		return tools
	}
	slug := templateSlugForType(artifactType)
	if slug == "" {
		return tools
	}
	read := NextToolHint{
		Tool: "pindoc.artifact.read",
		Args: map[string]any{
			"id_or_slug": slug,
		},
		Reason: "Read the matching template before retrying artifact.propose.",
	}
	return prependToolHint(read, tools)
}

func prependToolHint(first NextToolHint, rest []NextToolHint) []NextToolHint {
	out := make([]NextToolHint, 0, len(rest)+1)
	out = append(out, first)
	for _, hint := range rest {
		if hint.Tool == first.Tool {
			if sameIDOrSlug(hint.Args, first.Args) || len(hint.Args) == 0 {
				continue
			}
		}
		out = append(out, hint)
	}
	return out
}

func sameIDOrSlug(a, b map[string]any) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	return fmt.Sprint(a["id_or_slug"]) == fmt.Sprint(b["id_or_slug"])
}

func suggestedActionsForNotReady(lang, artifactType string, failed []string, base []string) []string {
	out := append([]string{}, base...)
	if !needsTemplateSelfHealHint(failed) {
		return out
	}
	if slug := templateSlugForType(artifactType); slug != "" {
		out = append(out, fmt.Sprintf(i18n.T(lang, "suggested.read_template_self_heal"), slug))
	}
	return out
}

func expectedForNotReady(ctx context.Context, deps Deps, projectSlug, artifactType string, failed []string) *ExpectedShape {
	if !needsTemplateSelfHealHint(failed) {
		return nil
	}
	slots := requiredH2SlotsFor(ctx, deps, projectSlug, artifactType)
	if len(slots) == 0 {
		return nil
	}
	out := &ExpectedShape{
		ArtifactType: artifactType,
		TemplateSlug: templateSlugForType(artifactType),
		RequiredH2:   make([]ExpectedH2Slot, 0, len(slots)),
	}
	for _, slot := range slots {
		out.RequiredH2 = append(out.RequiredH2, ExpectedH2Slot{
			Label:   slot.Label,
			Aliases: aliasesWithoutLabel(slot.Label, slot.Aliases),
		})
	}
	return out
}

func aliasesWithoutLabel(label string, aliases []string) []string {
	var out []string
	seen := map[string]struct{}{strings.ToLower(strings.TrimSpace(label)): {}}
	for _, alias := range aliases {
		key := strings.ToLower(strings.TrimSpace(alias))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, alias)
	}
	return out
}

func needsTemplateSelfHealHint(failed []string) bool {
	for _, code := range failed {
		if isMissingH2Code(code) {
			return true
		}
		switch code {
		case "TASK_NO_ACCEPTANCE", "DEC_NO_SECTIONS", "DBG_NO_REPRO", "DBG_NO_RESOLUTION":
			return true
		}
	}
	return false
}

func isMissingH2Code(code string) bool {
	return strings.HasPrefix(code, "MISSING_H2:")
}

func templateSlugForType(artifactType string) string {
	t := strings.ToLower(strings.TrimSpace(artifactType))
	if t == "" {
		return ""
	}
	switch artifactType {
	case "Decision", "Task", "Analysis", "Debug":
		return "_template_" + t
	default:
		return ""
	}
}

// makeRelated builds a RelatedRef from the minimal fields most not_ready
// sites have on hand. Empty ID is fine — slug alone is stable for URL
// construction.
func makeRelated(deps Deps, scope *auth.ProjectScope, slug, id, artType, title, reason string) RelatedRef {
	return RelatedRef{
		ID:          id,
		Slug:        slug,
		Type:        artType,
		Title:       title,
		AgentRef:    "pindoc://" + slug,
		HumanURL:    HumanURL(scope.ProjectSlug, scope.ProjectLocale, slug),
		HumanURLAbs: AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, slug),
		Reason:      reason,
	}
}

// patchFieldsFor maps a stable fail code to the minimum set of input
// fields an agent should change to pass the retry. Empty set means
// "rework the whole submission" (schema problems, etc.). Mirrors the 3rd
// peer review's `patchable_fields[]` proposal.
func patchFieldsFor(code string) []string {
	switch code {
	case "NO_SRCH", "RECEIPT_UNKNOWN", "RECEIPT_EXPIRED", "RECEIPT_WRONG_PROJECT", "RECEIPT_SUPERSEDED":
		return []string{"basis.search_receipt"}
	case "NEED_VER", "FIELD_VALUE_RESERVED":
		return []string{"expected_version"}
	case "VER_CONFLICT":
		return []string{"expected_version", "body_markdown", "title"}
	case "SHAPE_INVALID", "SHAPE_REQUIRES_UPDATE", "SHAPE_NOT_IMPLEMENTED":
		return []string{"shape"}
	case "META_PATCH_HAS_BODY":
		return []string{"body_markdown", "body_patch", "shape"}
	case "META_PATCH_EMPTY":
		return []string{"tags", "completeness", "task_meta", "artifact_meta"}
	case "TASK_STATUS_VIA_TRANSITION_TOOL":
		return []string{"task_meta.status"}
	case "ACCEPT_TRANSITION_REQUIRED",
		"ACCEPT_TRANSITION_INDEX_REQUIRED",
		"ACCEPT_TRANSITION_INDEX_NEGATIVE",
		"ACCEPT_TRANSITION_INDEX_OUT_OF_RANGE",
		"ACCEPT_TRANSITION_STATE_INVALID",
		"ACCEPT_TRANSITION_REASON_REQUIRED",
		"ACCEPT_TRANSITION_NOOP":
		return []string{"acceptance_transition"}
	case "SCOPE_DEFER_REQUIRED",
		"SCOPE_DEFER_REASON_REQUIRED",
		"SCOPE_DEFER_TARGET_NOT_FOUND",
		"SCOPE_DEFER_SELF":
		return []string{"scope_defer"}
	case "MISSING_COMMIT_MSG":
		return []string{"commit_msg"}
	case "POSSIBLE_DUP":
		return []string{"update_of", "title"}
	case "TASK_SUPERSEDE_REQUIRED":
		return []string{"supersede_of", "unrelated_reason"}
	case "UNRELATED_REASON_TOO_SHORT":
		return []string{"unrelated_reason"}
	case "TASK_ACTIVE_CONTEXT_REQUIRED":
		return []string{"basis.search_receipt"}
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
	case "AREA_UNKNOWN", "AREA_NOT_FOUND", "AREA_EMPTY", "DECISION_AREA_DEPRECATED":
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
	case "TASK_META_WRONG_TYPE":
		return []string{"task_meta"}
	case "META_APPLIES_AREA_INVALID":
		return []string{"artifact_meta.applies_to_areas"}
	case "META_APPLIES_TYPE_INVALID":
		return []string{"artifact_meta.applies_to_types"}
	case "META_RULE_SEVERITY_INVALID":
		return []string{"artifact_meta.rule_severity"}
	case "TASK_STATUS_INVALID":
		return []string{"task_meta.status"}
	case "TASK_PRIORITY_INVALID":
		return []string{"task_meta.priority"}
	case "ASSIGNEE_INVALID":
		return []string{"task_meta.assignee"}
	case "TASK_DUE_AT_INVALID":
		return []string{"task_meta.due_at"}
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
func insertPins(ctx context.Context, tx pgx.Tx, projectID, artifactID string, pins []ArtifactPinInput, repoRoot string) (int, []string, error) {
	n := 0
	var warnings []string
	for _, p := range pins {
		kind := pinmodel.NormalizeKind(p.Kind, p.Path)
		repo := strings.TrimSpace(p.Repo)
		if repo == "" {
			repo = "origin"
		}
		repoID, repoMatched, err := pgit.ResolvePinRepoID(ctx, tx, projectID, p.RepoID, repo, p.Path, repoRoot)
		if err != nil {
			return n, warnings, fmt.Errorf("pin repo resolve: %w", err)
		}
		if !repoMatched && kind == "code" {
			warnings = append(warnings, "RECOMMEND_REPO_REGISTRATION:"+strings.TrimSpace(p.Path))
		}
		var repoIDArg any
		if repoID != "" {
			repoIDArg = repoID
		}
		// Non-code/doc/config/asset kinds don't use line ranges or commit_sha;
		// null them out so the row stays consistent with the kind semantics.
		var commit any = nullIfEmpty(p.CommitSHA)
		var linesStart any = nullIfZero(p.LinesStart)
		var linesEnd any = nullIfZero(p.LinesEnd)
		if kind != "code" && kind != "doc" && kind != "config" && kind != "asset" {
			commit, linesStart, linesEnd = nil, nil, nil
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_pins (artifact_id, kind, repo_id, repo, commit_sha, path, lines_start, lines_end)
			VALUES ($1, $2, $3::uuid, $4, $5, $6, $7, $8)
		`, artifactID, kind, repoIDArg, repo, commit, p.Path, linesStart, linesEnd,
		); err != nil {
			return n, warnings, fmt.Errorf("pin insert: %w", err)
		}
		n++
	}
	return n, warnings, nil
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
	} else if tm.assigneeSet {
		payload["assignee"] = nil
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

func hasExplicitMetadataUpdate(in artifactProposeInput) bool {
	return in.TaskMeta != nil ||
		in.ArtifactMeta != nil ||
		in.Tags != nil ||
		len(in.Pins) > 0 ||
		len(in.RelatesTo) > 0 ||
		strings.TrimSpace(in.Completeness) != ""
}

func applyCreateCommitMsgFallback(in *artifactProposeInput) []string {
	if strings.TrimSpace(in.CommitMsg) != "" {
		in.CommitMsg = strings.TrimSpace(in.CommitMsg)
		return nil
	}
	in.CommitMsg = fallbackCreateCommitMsg(in.Title)
	return nil
}

func fallbackCreateCommitMsg(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "(untitled)"
	}
	return fmt.Sprintf("[fallback_missing_commit_msg] create artifact: %s", title)
}

func dryRunProposeOutput(out artifactProposeOutput, keepTargetIdentity bool) artifactProposeOutput {
	out.DryRun = true
	out.Created = false
	out.PublishedAt = time.Time{}
	out.RevisionNumber = 0
	out.PinsStored = 0
	out.EdgesStored = 0
	out.Superseded = false
	if !keepTargetIdentity {
		out.ArtifactID = ""
		out.Slug = ""
		out.AgentRef = ""
		out.HumanURL = ""
		out.HumanURLAbs = ""
	}
	return out
}

func taskStatusFromJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if s, ok := m["status"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func shouldAutoClaimDone(artifactType string, taskMetaRaw []byte, body string) bool {
	if artifactType != "Task" {
		return false
	}
	status := taskStatusFromJSON(taskMetaRaw)
	if status != "" && status != "open" {
		return false
	}
	resolved, total := countAcceptanceResolution(body)
	return total > 0 && resolved == total
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
	AppliesToAreas    []string `json:"applies_to_areas,omitempty"`
	AppliesToTypes    []string `json:"applies_to_types,omitempty"`
	RuleSeverity      string   `json:"rule_severity,omitempty"`
	RuleExcerpt       string   `json:"rule_excerpt,omitempty"`
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
		out.AppliesToAreas = normalizeStringSlice(in.AppliesToAreas)
		out.AppliesToTypes = normalizeStringSlice(in.AppliesToTypes)
		out.RuleSeverity = strings.TrimSpace(in.RuleSeverity)
		out.RuleExcerpt = strings.TrimSpace(in.RuleExcerpt)
	}

	hasCodePin := false
	for _, p := range pins {
		kind := pinmodel.NormalizeKind(p.Kind, p.Path)
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
	if len(r.AppliesToAreas) > 0 {
		payload["applies_to_areas"] = r.AppliesToAreas
	}
	if len(r.AppliesToTypes) > 0 {
		payload["applies_to_types"] = r.AppliesToTypes
	}
	if r.RuleSeverity != "" {
		payload["rule_severity"] = r.RuleSeverity
	}
	if r.RuleExcerpt != "" {
		payload["rule_excerpt"] = r.RuleExcerpt
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(buf)
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func consentRequiredForUserChatWarning(isConvDerived bool, meta ResolvedArtifactMeta, body string) bool {
	return isConvDerived && meta.ConsentState == "" && hasPIISignal(body)
}

// applyTaskCreateDefaults normalizes Task create inputs so new Task rows
// always land with the lifecycle's baseline status. Assignee is deliberately
// not inferred from author_id: omitted means unassigned, explicit
// agent:<id>/user:<id>/@handle means owned.
// Update paths skip this helper because they should preserve the current
// task_meta unless the caller explicitly changes it.
func applyTaskCreateDefaults(in *artifactProposeInput) {
	if in == nil || in.Type != "Task" || strings.TrimSpace(in.UpdateOf) != "" {
		return
	}
	if in.TaskMeta == nil {
		in.TaskMeta = &TaskMetaInput{}
	}
	if strings.TrimSpace(in.TaskMeta.Status) == "" {
		in.TaskMeta.Status = "open"
	}
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
const unrelatedReasonMinRunes = 20

type semanticCandidate struct {
	ArtifactID string
	Slug       string
	Type       string
	Title      string
	Distance   float64
}

func receiptSnapshotsContainAny(snapshots []receipts.ArtifactRef, candidates []semanticCandidate) bool {
	if len(snapshots) == 0 || len(candidates) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(snapshots))
	for _, s := range snapshots {
		seen[s.ArtifactID] = struct{}{}
	}
	for _, c := range candidates {
		if _, ok := seen[c.ArtifactID]; ok {
			return true
		}
	}
	return false
}

func receiptExemptionLimit(deps Deps) int {
	if deps.ReceiptExemptionLimit < 0 {
		return 0
	}
	return deps.ReceiptExemptionLimit
}

func maybeExemptMissingReceipt(ctx context.Context, deps Deps, projectID, areaID, authorID string) (*ReceiptExemptionSignal, bool, error) {
	limit := receiptExemptionLimit(deps)
	if limit <= 0 {
		return nil, false, nil
	}
	var total, otherAuthors int
	if err := deps.DB.QueryRow(ctx, `
		SELECT
			count(*)::int,
			count(*) FILTER (WHERE author_id IS DISTINCT FROM $3)::int
		FROM artifacts
		WHERE project_id = $1::uuid
		  AND area_id = $2::uuid
		  AND status <> 'archived'
		  AND NOT starts_with(slug, '_template_')
	`, projectID, areaID, authorID).Scan(&total, &otherAuthors); err != nil {
		return nil, false, err
	}
	signal, ok := receiptExemptionFromStats(total, otherAuthors, limit)
	return signal, ok, nil
}

func receiptExemptionFromStats(totalArtifacts, otherAuthorArtifacts, limit int) (*ReceiptExemptionSignal, bool) {
	if limit <= 0 || totalArtifacts < 0 || otherAuthorArtifacts < 0 {
		return nil, false
	}
	if otherAuthorArtifacts > 0 || totalArtifacts >= limit {
		return nil, false
	}
	remaining := limit - totalArtifacts - 1
	if remaining < 0 {
		remaining = 0
	}
	return &ReceiptExemptionSignal{
		Reason:     "empty_area_first_proposes",
		NRemaining: remaining,
		Limit:      limit,
	}, true
}

func activeTasksInArea(ctx context.Context, deps Deps, projectID, areaID, excludeArtifactID string) ([]semanticCandidate, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT a.id::text, a.slug, a.type, a.title, 0::float8 AS distance
		FROM artifacts a
		WHERE a.project_id = $1::uuid
		  AND a.area_id = $2::uuid
		  AND ($3::text = '' OR a.id <> $3::uuid)
		  AND a.type = 'Task'
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
		  AND COALESCE(NULLIF(a.task_meta->>'status', ''), 'open') IN ('open', 'claimed_done')
		ORDER BY a.updated_at DESC
		LIMIT 20
	`, projectID, areaID, excludeArtifactID)
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
		out = append(out, c)
	}
	return out, rows.Err()
}

func filterTaskSemanticCandidates(ctx context.Context, deps Deps, projectID, areaID string, candidates []semanticCandidate) ([]semanticCandidate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(candidates))
	byID := make(map[string]semanticCandidate, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.ArtifactID)
		byID[c.ArtifactID] = c
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT a.id::text
		FROM artifacts a
		WHERE a.project_id = $1::uuid
		  AND a.area_id = $2::uuid
		  AND a.id = ANY($3::uuid[])
		  AND a.type = 'Task'
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
		  AND COALESCE(NULLIF(a.task_meta->>'status', ''), 'open') IN ('open', 'claimed_done')
	`, projectID, areaID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []semanticCandidate
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// createWarnings runs a best-effort advisory vector check after an accepted
// create and returns any soft warnings. Non-blocking — failure here never
// rejects the write. We report close neighbours, including non-Task
// matches that would have been hard conflicts before the Task-only
// supersede gate, to nudge "you might want to supersede this next time".
func createWarnings(ctx context.Context, deps Deps, projectID, title, body string) []string {
	if deps.Embedder == nil || deps.Embedder.Info().Name == "stub" {
		return nil
	}
	conflicts, err := findSemanticConflicts(ctx, deps, projectID, title, body)
	if err == nil && len(conflicts) > 0 {
		return []string{"RECOMMEND_READ_BEFORE_CREATE"}
	}
	cands, err := findSemanticAdvisories(ctx, deps, projectID, title, body)
	if err != nil || len(cands) == 0 {
		return nil
	}
	return []string{"RECOMMEND_READ_BEFORE_CREATE"}
}

// pinPathWarnings checks every repo-backed pin path against the configured
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
		kind := pinmodel.NormalizeKind(p.Kind, p.Path)
		if kind == "resource" || kind == "url" {
			// Resource/url kinds don't point at a local checkout path.
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

// titleQualityWarnings runs the locale-aware title evaluation defined in
// the titleguide package — length bounds + jargon-token detection. The
// `decision-title-heading-rule-preflight` decision used to live inline
// here as a fixed 15/80 band; 2026-04-28 dogfood split the language-
// neutral META rules from the per-locale DATA so adding a new locale
// (ja, etc.) is a JSON-shaped contribution rather than a Go rewrite.
//
// `bodyLocale` should be the artifact's body_locale; empty falls through
// to the embedded en baseline. `extraJargon` is the project-side override
// (instance settings → eventually project_settings).
func titleQualityWarnings(title, bodyLocale string, extraJargon []string) []string {
	findings := titleguide.EvaluateTitle(title, bodyLocale, titleguide.ProjectOverride{ExtraJargon: extraJargon})
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Code)
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

const (
	sectionDuplicatesEdgesWarning = "SECTION_DUPLICATES_EDGES"
	sectionDuplicatesEdgesAction  = "관계는 relates_to 필드로 제출하세요; 본문은 narrative 전용."
)

var edgeDuplicateSectionHeadings = map[string]struct{}{
	"연관":                  {},
	"역참조":                 {},
	"선후":                  {},
	"리소스 경로":              {},
	"dependencies":        {},
	"dependencies / 선후":   {},
	"related":             {},
	"related artifacts":   {},
	"relationships":       {},
	"references":          {},
	"backreferences":      {},
	"backlinks":           {},
	"resource paths":      {},
	"related resources":   {},
	"resource references": {},
}

func sectionDuplicatesEdgesWarnings(body string) []string {
	for _, line := range strings.Split(body, "\n") {
		after, ok := strings.CutPrefix(line, "## ")
		if !ok {
			continue
		}
		heading := normalizeEdgeDuplicateHeading(after)
		if _, dup := edgeDuplicateSectionHeadings[heading]; dup {
			return []string{sectionDuplicatesEdgesWarning}
		}
	}
	return nil
}

func normalizeEdgeDuplicateHeading(heading string) string {
	heading = strings.ToLower(strings.TrimSpace(strings.TrimRight(heading, "#")))
	heading = strings.Join(strings.Fields(heading), " ")
	return heading
}

func sectionDuplicatesEdgesSuggestedActions(warnings []string) []string {
	for _, w := range warnings {
		if w == sectionDuplicatesEdgesWarning {
			return []string{sectionDuplicatesEdgesAction}
		}
	}
	return nil
}

func decisionSubjectAreaWarnings(in artifactProposeInput) []string {
	if in.Type != "Decision" {
		return nil
	}
	switch strings.TrimSpace(in.AreaSlug) {
	case "misc", "_unsorted":
		return []string{"DECISION_AREA_MUST_BE_SUBJECT:docs/19-area-taxonomy.md"}
	default:
		return nil
	}
}

// countAcceptanceCheckboxes returns (resolved, total) for the body's
// 4-state acceptance checkboxes. Phase D widened the semantics from
// binary [ ]/[x] to 4-state: resolved now counts [x] (done), [~]
// (partial, recorded reason), and [-] (deferred, recorded reason).
// claimed_done gate: resolved == total means no unchecked [ ] remain;
// [~]/[-] count as judgment calls an agent recorded via shape=
// acceptance_transition and are no longer blocking.
func countAcceptanceCheckboxes(body string) (resolved, total int) {
	return countAcceptanceResolution(body)
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

type requiredH2Slot struct {
	Label   string
	Aliases []string
}

// requiredH2Warnings flags missing mandatory H2 sections per Type. Fuzzy
// match: a slot is satisfied if any of its aliases appear as an H2
// (case-insensitive). en/ko aliases tolerate the current bilingual
// template corpus; Phase 18 template locale-split will normalise them.
func requiredH2Warnings(body, artifactType string) []string {
	return requiredH2WarningsForSlots(body, defaultRequiredH2Slots(artifactType))
}

func requiredH2WarningsFor(ctx context.Context, deps Deps, projectSlug, body, artifactType string) []string {
	return requiredH2WarningsForSlots(body, requiredH2SlotsFor(ctx, deps, projectSlug, artifactType))
}

func requiredH2WarningsForSlots(body string, slots []requiredH2Slot) []string {
	missing := missingRequiredH2Slots(body, slots)
	out := make([]string, 0, len(missing))
	for _, slot := range missing {
		out = append(out, "MISSING_H2:"+slot.Label)
	}
	return out
}

func requiredH2SlotsFor(ctx context.Context, deps Deps, projectSlug, artifactType string) []requiredH2Slot {
	return requiredH2SlotsFromHints(artifactType, getValidatorHints(ctx, deps, projectSlug, artifactType))
}

func requiredH2SlotsFromHints(artifactType string, hints *validatorHints) []requiredH2Slot {
	if hints != nil && len(hints.RequiredH2) > 0 {
		out := make([]requiredH2Slot, 0, len(hints.RequiredH2))
		for _, label := range hints.RequiredH2 {
			label = strings.TrimSpace(label)
			if label == "" {
				continue
			}
			out = append(out, requiredH2SlotForLabel(artifactType, label))
		}
		return out
	}
	return defaultRequiredH2Slots(artifactType)
}

func missingRequiredH2Slots(body string, slots []requiredH2Slot) []requiredH2Slot {
	if len(slots) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(body, "\n") {
		after, ok := strings.CutPrefix(line, "## ")
		if !ok {
			continue
		}
		for _, key := range h2HeadingKeys(after) {
			seen[key] = true
		}
	}
	var out []requiredH2Slot
	for _, slot := range slots {
		matched := false
		for _, alt := range slot.Aliases {
			if seen[normalizeH2Label(alt)] {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, slot)
		}
	}
	return out
}

func h2HeadingKeys(heading string) []string {
	heading = strings.TrimSpace(strings.TrimRight(heading, "#"))
	parts := []string{heading}
	for _, sep := range []string{"/", "·", "—", "–", "-"} {
		if strings.Contains(heading, sep) {
			parts = append(parts, strings.Split(heading, sep)...)
		}
	}
	if before, rest, ok := strings.Cut(heading, "("); ok {
		parts = append(parts, before)
		if inside, _, ok := strings.Cut(rest, ")"); ok {
			parts = append(parts, inside)
		}
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		key := normalizeH2Label(part)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func normalizeH2Label(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimRight(s, "#")))
}

func requiredH2SlotForLabel(artifactType, label string) requiredH2Slot {
	slot := requiredH2Slot{Label: label, Aliases: []string{label}}
	for _, candidate := range defaultRequiredH2Slots(artifactType) {
		for _, alias := range candidate.Aliases {
			if normalizeH2Label(alias) == normalizeH2Label(label) {
				slot.Aliases = uniqueStrings(append(slot.Aliases, candidate.Aliases...))
				return slot
			}
		}
	}
	return slot
}

// defaultRequiredH2Slots returns canonical H2 slots per Type plus accepted
// aliases. The first label is what legacy warning codes surface when no
// project template supplies a more specific label.
func defaultRequiredH2Slots(t string) []requiredH2Slot {
	switch t {
	case "Decision":
		return []requiredH2Slot{
			{Label: "Context", Aliases: []string{"Context", "맥락", "컨텍스트"}},
			{Label: "Decision", Aliases: []string{"Decision", "결정"}},
			{Label: "Rationale", Aliases: []string{"Rationale", "근거"}},
			{Label: "Alternatives considered", Aliases: []string{"Alternatives considered", "Alternatives", "대안"}},
			{Label: "Consequences", Aliases: []string{"Consequences", "영향", "결과"}},
		}
	case "Analysis":
		return []requiredH2Slot{
			{Label: "TL;DR", Aliases: []string{"TL;DR", "TL", "요약"}},
		}
	case "Task":
		return []requiredH2Slot{
			{Label: "Purpose", Aliases: []string{"Purpose", "목적"}},
			{Label: "Scope", Aliases: []string{"Scope", "범위"}},
			{Label: "TODO", Aliases: []string{"TODO", "Acceptance criteria", "Acceptance", "완료 기준", "완료기준"}},
		}
	case "Debug":
		return []requiredH2Slot{
			{Label: "Symptom", Aliases: []string{"Symptom", "Symptoms", "증상"}},
			{Label: "Reproduction", Aliases: []string{"Reproduction", "Repro", "재현"}},
			{Label: "Root cause", Aliases: []string{"Root cause", "Cause", "원인"}},
			{Label: "Resolution", Aliases: []string{"Resolution", "해결"}},
		}
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
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
		WHERE a.project_id = $2
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
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
		WHERE a.project_id = $2
		  AND a.status <> 'archived'
		  AND a.status <> 'superseded'
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

// BodyPatchInput is the shape of the light-weight patch an agent can
// send in place of a full body_markdown on update_of. Three modes:
//
//	"section_replace"   — swap one `## heading` section's body with new text
//	"checkbox_toggle"   — flip one `- [ ]` / `- [x]` item to the target state
//	"append"            — tack text on the end of the current body
//
// applyBodyPatch reads the previous body (already fetched by handleUpdate)
// and writes the resulting body back into propose input so the rest of
// the update path — canonical_rewrite guard, embedding, revision insert —
// runs unchanged.
type BodyPatchInput struct {
	Mode           string `json:"mode" jsonschema:"section_replace | checkbox_toggle | append"`
	SectionHeading string `json:"section_heading,omitempty" jsonschema:"for section_replace: the H2 heading text (fuzzy match) whose body is replaced"`
	Replacement    string `json:"replacement,omitempty" jsonschema:"for section_replace: new body content for the target section (no H2 line — the heading itself is preserved)"`
	CheckboxIndex  *int   `json:"checkbox_index,omitempty" jsonschema:"for checkbox_toggle: 0-based index across the whole body, scanning - [ ] and - [x] in document order"`
	CheckboxState  *bool  `json:"checkbox_state,omitempty" jsonschema:"for checkbox_toggle: target state (true = checked, false = unchecked). Must be explicitly set"`
	AppendText     string `json:"append_text,omitempty" jsonschema:"for append: literal text appended after a blank line at the end of the body"`
}

// applyBodyPatch materialises prev + patch into the new body string. On
// success returns newBody + optional warnings (e.g. PATCH_NOOP) and nil
// stableCode. On failure returns empty body + nil warnings + a stable
// error code string the caller wraps into artifactProposeOutput.
func applyBodyPatch(prevBody string, patch *BodyPatchInput) (string, []string, string) {
	if patch == nil {
		return prevBody, nil, ""
	}
	switch strings.TrimSpace(patch.Mode) {
	case "section_replace":
		return applyBodyPatchSection(prevBody, patch)
	case "checkbox_toggle":
		return applyBodyPatchCheckbox(prevBody, patch)
	case "append":
		return applyBodyPatchAppend(prevBody, patch)
	default:
		return "", nil, "PATCH_MODE_INVALID"
	}
}

func applyBodyPatchSection(prev string, patch *BodyPatchInput) (string, []string, string) {
	heading := strings.TrimSpace(patch.SectionHeading)
	if heading == "" {
		return "", nil, "PATCH_HEADING_EMPTY"
	}
	// Reuse parseH2Sections's fuzzy matching — same H2 resolver the
	// canonical-rewrite guard uses, so `## 목적 / Purpose` and `## Purpose`
	// both point at the same slot.
	target := normaliseSectionKey(heading)
	lines := strings.Split(prev, "\n")
	start, end := -1, -1
	inFence := false
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			sec := normaliseSectionKey(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			if start == -1 && sectionsMatch(sec, target) {
				start = i
				continue
			}
			if start != -1 {
				end = i
				break
			}
		}
	}
	if start == -1 {
		return "", nil, "PATCH_SECTION_NOT_FOUND"
	}
	if end == -1 {
		end = len(lines)
	}
	// Preserve the heading line at `start`; replace lines (start+1 .. end-1).
	head := lines[:start+1]
	tail := lines[end:]
	replacement := strings.Split(strings.TrimRight(patch.Replacement, "\n"), "\n")
	// Blank separator between heading and replacement, same for tail.
	combined := append([]string{}, head...)
	combined = append(combined, "")
	combined = append(combined, replacement...)
	if len(tail) > 0 {
		combined = append(combined, "")
		combined = append(combined, tail...)
	}
	return strings.Join(combined, "\n"), nil, ""
}

func applyBodyPatchCheckbox(prev string, patch *BodyPatchInput) (string, []string, string) {
	if patch.CheckboxIndex == nil {
		return "", nil, "PATCH_CHECKBOX_INDEX_REQUIRED"
	}
	if patch.CheckboxState == nil {
		return "", nil, "PATCH_CHECKBOX_STATE_REQUIRED"
	}
	target := *patch.CheckboxIndex
	desired := *patch.CheckboxState
	if target < 0 {
		return "", nil, "PATCH_CHECKBOX_INDEX_NEGATIVE"
	}
	lines := strings.Split(prev, "\n")
	idx := 0
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		var currentState *bool
		var prefixLen int
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			checked := strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]")
			currentState = &checked
			prefixLen = len(line) - len(trimmed)
		}
		if currentState == nil {
			continue
		}
		if idx != target {
			idx++
			continue
		}
		// Located — same state means PATCH_NOOP warning, still accept.
		if *currentState == desired {
			return prev, []string{"PATCH_NOOP"}, ""
		}
		prefix := line[:prefixLen]
		rest := trimmed[len("- [ ]"):] // matches "- [x]" too, same length
		marker := "- [ ]"
		if desired {
			marker = "- [x]"
		}
		lines[i] = prefix + marker + rest
		return strings.Join(lines, "\n"), nil, ""
	}
	return "", nil, "PATCH_CHECKBOX_OUT_OF_RANGE"
}

func applyBodyPatchAppend(prev string, patch *BodyPatchInput) (string, []string, string) {
	text := strings.TrimSpace(patch.AppendText)
	if text == "" {
		return "", nil, "PATCH_APPEND_EMPTY"
	}
	joiner := "\n\n"
	if strings.HasSuffix(prev, "\n") {
		joiner = "\n"
	}
	return strings.TrimRight(prev, "\n") + joiner + patch.AppendText + "\n", nil, ""
}

// normaliseSectionKey / sectionsMatch lift the fuzzy heading logic out
// of parseH2Sections so section_replace can match "## 목적 / Purpose"
// against the agent's "Purpose" input without duplicating the table.
func normaliseSectionKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// strip markdown emphasis / heading-id suffix if present
	s = strings.TrimSuffix(s, " #")
	return s
}

// recordWarningEvent writes a best-effort events.artifact.warning_raised
// row whenever an accepted propose returns a non-empty warnings slice
// (Task propose-경로-warning-영속화). The Reader Trust Card queries these
// events per-artifact to surface "Uncertain rewrite" / "Consent pending"
// etc. so both the author reviewing the artifact and a future agent
// session can see advisories that previously lived only in the propose
// response. Insert failures are logged and swallowed — we never roll
// back a successful revision because the audit event couldn't land.
func recordWarningEvent(ctx context.Context, deps Deps, projectID, artifactID string, revision int, warnings []string, authorID string, canonicalFlag bool) {
	if len(warnings) == 0 {
		return
	}
	payload := map[string]any{
		"codes":           warnings,
		"revision_number": revision,
		"author_id":       authorID,
	}
	if canonicalFlag {
		payload["canonical_rewrite_without_evidence"] = true
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("warning event payload marshal failed",
				"artifact_id", artifactID, "err", err)
		}
		return
	}
	_, err = deps.DB.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.warning_raised', $2, $3::jsonb)
	`, projectID, artifactID, raw)
	if err != nil && deps.Logger != nil {
		deps.Logger.Warn("warning event insert failed — artifact saved without audit row",
			"artifact_id", artifactID, "err", err)
	}
}

// bodyContainsAnyKeyword returns true when at least one keyword substring
// appears in body (case-insensitive). Used by the per-type preflight
// guardrails so a Task can pass when the template's required_keywords
// or the hard-coded fallback keywords show up anywhere in the body.
func bodyContainsAnyKeyword(body string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lower := strings.ToLower(body)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// bodyContainsAllKeywords is the Decision-strength variant: every keyword
// must appear. Decisions traditionally demand both "decision" and
// "context" sections, so the AND semantics preserve that stricter bar
// while still being driven by template meta instead of hard-coded
// literals.
func bodyContainsAllKeywords(body string, keywords []string) bool {
	lower := strings.ToLower(body)
	for _, kw := range keywords {
		if !strings.Contains(lower, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}

// patchExplain maps a body_patch stable code to the one-line checklist
// entry surfaced in not_ready responses. Keeping the mapping inline
// (vs i18n.T) because these codes are deliberately low-traffic — they
// only fire when the agent mis-uses the API shape, not during normal
// propose flows.
func patchExplain(code string) string {
	switch code {
	case "PATCH_MODE_INVALID":
		return "body_patch.mode must be one of section_replace | checkbox_toggle | append."
	case "PATCH_HEADING_EMPTY":
		return "body_patch.mode=section_replace requires section_heading."
	case "PATCH_SECTION_NOT_FOUND":
		return "body_patch.section_heading did not match any `## heading` in the current body (fuzzy match included)."
	case "PATCH_CHECKBOX_INDEX_REQUIRED":
		return "body_patch.mode=checkbox_toggle requires checkbox_index (0-based across the whole body)."
	case "PATCH_CHECKBOX_STATE_REQUIRED":
		return "body_patch.mode=checkbox_toggle requires checkbox_state (true = checked, false = unchecked)."
	case "PATCH_CHECKBOX_INDEX_NEGATIVE":
		return "body_patch.checkbox_index must be ≥ 0."
	case "PATCH_CHECKBOX_OUT_OF_RANGE":
		return "body_patch.checkbox_index is past the last `- [ ]` / `- [x]` item in the body."
	case "PATCH_APPEND_EMPTY":
		return "body_patch.mode=append requires append_text with non-whitespace content."
	}
	return "body_patch failed with code " + code
}

func sectionsMatch(a, b string) bool {
	if a == b {
		return true
	}
	// split on "/" or "·" to accept mixed ko/en slots like "목적 / purpose".
	parts := strings.FieldsFunc(a, func(r rune) bool { return r == '/' || r == '·' })
	for _, p := range parts {
		if strings.TrimSpace(p) == b {
			return true
		}
	}
	parts = strings.FieldsFunc(b, func(r rune) bool { return r == '/' || r == '·' })
	for _, p := range parts {
		if strings.TrimSpace(p) == a {
			return true
		}
	}
	return false
}
