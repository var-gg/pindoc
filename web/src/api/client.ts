// Tiny fetch wrapper. Keeps response shapes in one place so components
// don't all re-derive types from JSON. Everything is read-only — writes go
// through the MCP path, not here.
//
// URL convention: every project-scoped endpoint lives under
// /api/p/{project}/… . Helpers here take the project slug as the first
// argument so callers can't forget. /api/config and /api/projects are
// unscoped (instance-wide).

export type ServerConfig = {
  default_project_slug: string;
  // Compatibility alias for the default project's primary_language.
  // Canonical Reader URLs no longer carry locale.
  default_project_locale?: string;
  multi_project: boolean;
  version: string;
  // providers + bind_addr replace the deprecated auth_mode enum
  // (Decision `decision-auth-model-loopback-and-providers`). Empty
  // providers list + loopback bind_addr matches the historical
  // single-user self-host path. TaskControls now keys "operator can
  // edit inline" off providers being empty (no IdP active means the
  // Reader user is the operator on their own box).
  providers: string[];
  bind_addr: string;
  // onboarding_required tells the SPA to redirect a fresh install to
  // the new-project wizard instead of the legacy "open default project"
  // landing. True when the instance has no projects other than the
  // seed `pindoc` row. Decision project-bootstrap-canonical-flow-
  // reader-ui-first-class.
  onboarding_required?: boolean;
  // identity_required surfaces the loopback identity gap so the
  // Reader can route fresh installs to /onboarding/identity before
  // any project / artifact UI loads. True only when the daemon could
  // not bind a user from settings, env, or the lone-row backfill.
  identity_required?: boolean;
};

// task: agent-era first-time identity flow. The Reader posts the
// form, gets the new user_id + the three MCP-connect copy targets
// (URL only / `.mcp.json` / agent prompt).
export type OnboardingIdentityInput = {
  display_name: string;
  email: string;
  github_handle?: string;
};

export type OnboardingMCPConnect = {
  url: string;
  mcp_json: string;
  agent_prompt: string;
};

export type OnboardingIdentityResp = {
  status: "ok";
  user_id: string;
  display_name: string;
  email: string;
  project: { slug: string; role: "owner"; url: string };
  mcp_connect: OnboardingMCPConnect;
};

export type OnboardingIdentityError = {
  error_code: string;
  message: string;
};

export type Project = {
  id: string;
  slug: string;
  org_slug?: string;
  organization_slug?: string;
  name: string;
  description?: string;
  color?: string;
  primary_language: string;
  sensitive_ops?: ProjectSensitiveOps;
  current_role?: "owner" | "editor" | "viewer";
  // Compatibility alias for primary_language. Locale is metadata, not a
  // route or identity key.
  locale?: string;
  areas_count: number;
  artifacts_count: number;
  created_at: string;
  capabilities?: {
    review_queue_supported?: boolean;
  };
};

export type ProjectSensitiveOps = "auto" | "confirm";

export type VisibilityTier = "public" | "org" | "private";

export type ProjectSettingsPatchInput = {
  sensitive_ops: ProjectSensitiveOps;
};

export type ProjectSettingsPatchResp = {
  status: "ok";
  sensitive_ops: ProjectSensitiveOps;
};

export type ProjectListItem = {
  id: string;
  slug: string;
  org_slug?: string;
  organization_slug?: string;
  name: string;
  description?: string;
  color?: string;
  primary_language: string;
  artifacts_count: number;
  created_at: string;
  reader_hidden?: boolean;
};

export type ProjectListResp = {
  projects: ProjectListItem[];
  default_project_slug: string;
  multi_project: boolean;
};

export type ProjectListOptions = {
  includeHidden?: boolean;
};

export type Area = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  parent_slug?: string;
  is_cross_cutting: boolean;
  artifact_count: number;
  children_slugs?: string[];
};

export type TaskMeta = {
  // Task lifecycle. `claimed_done` is the settled completion state and
  // requires 100% acceptance checkboxes.
  status?: "open" | "claimed_done" | "blocked" | "cancelled";
  priority?: "p0" | "p1" | "p2" | "p3";
  assignee?: string;
  due_at?: string;
  parent_slug?: string;
};

export type TaskFlowReadiness = "ready" | "blocked" | "blocked_status" | "done" | "other";

export type TaskFlowStage = "ready" | "blocked" | "done" | "other";

export type TaskFlowBlocker = {
  project_slug: string;
  artifact_id: string;
  slug: string;
  title: string;
  status: string;
  priority?: "p0" | "p1" | "p2" | "p3";
  assignee?: string;
  human_url_abs?: string;
};

export type TaskFlowRow = {
  project_slug: string;
  artifact_id: string;
  slug: string;
  title: string;
  area_slug: string;
  status: "open" | "claimed_done" | "blocked" | "cancelled" | "missing_status" | "other";
  raw_status?: string;
  priority?: "p0" | "p1" | "p2" | "p3";
  assignee?: string;
  due_at?: string;
  stage: TaskFlowStage;
  ordinal: number;
  readiness: TaskFlowReadiness;
  blockers?: TaskFlowBlocker[] | null;
  updated_at: string;
  agent_ref: string;
  human_url: string;
  human_url_abs?: string;
};

export type TaskFlowResp = {
  mode: "derived";
  project_scope: "current" | "list" | "visible";
  project_slugs: string[];
  actor_scope: "all_visible" | "assignee" | "agent" | "user" | "requester" | "team";
  actor_ids?: string[];
  include_unassigned?: boolean;
  flow_scope: "active" | "all" | "ready" | "blocked";
  items: TaskFlowRow[];
  count: number;
  truncated?: boolean;
  notice: string;
};

export type TaskFlowParams = {
  project_scope?: "current" | "list" | "visible" | "caller_visible";
  project_slugs?: string[];
  actor_scope?: TaskFlowResp["actor_scope"];
  actor_id?: string;
  actor_ids?: string[];
  include_unassigned?: boolean;
  flow_scope?: TaskFlowResp["flow_scope"];
  area_slug?: string;
  priority?: "p0" | "p1" | "p2" | "p3";
  status?: "pending" | "all" | "open" | "missing_status" | "missing" | "claimed_done" | "blocked" | "cancelled";
  limit?: number;
};

// ArtifactMeta carries the epistemic axes persisted via migration 0012
// (artifact_meta JSONB). Every field is optional — the server omits axes
// the resolver didn't set, and legacy rows arrive as an empty object.
// Union-typed string fields mirror the server enums so Reader code can
// switch/narrow without re-parsing strings at every call site.
export type ArtifactMeta = {
  source_type?: "code" | "artifact" | "user_chat" | "external" | "mixed";
  consent_state?: "not_needed" | "requested" | "granted" | "denied";
  confidence?: "low" | "medium" | "high";
  audience?: "owner_only" | "approvers" | "project_readers";
  next_context_policy?: "default" | "opt_in" | "excluded";
  verification_state?: "verified" | "partially_verified" | "unverified";
  applies_to_areas?: string[];
  applies_to_types?: string[];
  rule_severity?: "binding" | "guidance" | "reference";
  rule_excerpt?: string;
};

// SourceSessionRef is the pass-through of the JSONB column by the same name.
// agent_id is server-issued and trusted; reported_author_id is the client
// string; source_session is free-form. All optional depending on whether
// the latest propose supplied basis.source_session.
export type SourceSessionRef = {
  agent_id?: string;
  reported_author_id?: string;
  source_session?: string;
};

// PinRef mirrors artifact_pins rows for the Reader Sidecar. repo defaults
// to "origin" in the DB, commit_sha + line range only meaningful on
// repo_id is the canonical project_repos coordinate when available.
export type PinRef = {
  kind: "code" | "doc" | "config" | "asset" | "resource" | "url";
  repo_id?: string;
  repo?: string;
  commit_sha?: string;
  path: string;
  lines_start?: number;
  lines_end?: number;
};

export type AssetProjection = {
  alt_text?: string;
  caption?: string;
  ocr_text?: string;
  layout_summary?: string;
};

export type AssetRef = {
  id: string;
  asset_ref: string;
  role: "inline_image" | "attachment" | "evidence" | "generated_output";
  mime_type: string;
  size_bytes: number;
  original_filename?: string;
  blob_url: string;
  is_image: boolean;
  projection: AssetProjection;
  display_order: number;
  created_by?: string;
  created_at?: string;
};

export type GitPreviewEnvelope = {
  available: boolean;
  reason?: string;
  fallback_url?: string;
};

export type GitRepoSummary = {
  id: string;
  name: string;
  default_branch: string;
  git_remote_url?: string;
};

export type GitReposResp = {
  project_slug: string;
  repos: GitRepoSummary[];
};

export type GitCommitInfo = {
  sha: string;
  author: string;
  author_email?: string;
  author_time: string;
  summary: string;
};

export type GitChangedFile = {
  path: string;
  status: string;
  additions?: number;
  deletions?: number;
  binary?: boolean;
};

export type GitChangedFilesResp = {
  git_preview: GitPreviewEnvelope;
  repo_id: string;
  commit: string;
  commit_info?: GitCommitInfo;
  files?: GitChangedFile[];
};

export type GitCommitResp = {
  git_preview: GitPreviewEnvelope;
  repo_id: string;
  commit: string;
  commit_info?: GitCommitInfo;
};

export type GitBlob = {
  path: string;
  size: number;
  text?: string;
  binary?: boolean;
};

export type GitBlobResp = {
  git_preview: GitPreviewEnvelope;
  repo_id: string;
  commit: string;
  blob?: GitBlob;
};

export type GitDiffResp = {
  git_preview: GitPreviewEnvelope;
  repo_id: string;
  commit: string;
  path?: string;
  diff?: string;
};

export type GitCommitReference = {
  artifact_id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  kind: PinRef["kind"];
  path: string;
  lines_start?: number;
  lines_end?: number;
  human_url: string;
  human_url_abs?: string;
};

export type GitCommitReferencesResp = {
  project_slug: string;
  commit: string;
  references: GitCommitReference[];
};

export type EdgeRef = {
  artifact_id: string;
  slug: string;
  type: string;
  title: string;
  relation: string;
  // body_locale of the target artifact (BCP47). Empty string when the
  // target predates body_locale rollout. Reader uses this to label
  // translation_of edges with a language chip without a second fetch.
  body_locale?: string;
};

// AuthorUserRef is the thin projection of the `users` row an artifact
// was authored by (migration 0014 / Decision author-identity-dual).
// Optional because older artifacts and MCP sessions launched without
// PINDOC_USER_NAME leave author_user_id NULL.
export type AuthorUserRef = {
  id: string;
  display_name: string;
  github_handle?: string;
};

// UserRef mirrors the /api/users row shape. TaskControls uses this to
// build the assignee datalist — display_name is the primary label,
// github_handle gets the @ prefix when present so an agent or human can
// type either form.
export type UserRef = {
  id: string;
  display_name: string;
  email?: string;
  github_handle?: string;
  source: "harness_install" | "pindoc_admin" | "github_oauth";
};

export type CurrentUser = UserRef & {
  auth_mode: "trusted_local" | "oauth_github" | "unknown";
};

export type CurrentUserResp = {
  status: "ok" | "informational" | "not_authenticated";
  auth_mode: CurrentUser["auth_mode"];
  user?: CurrentUser;
};

export type ArtifactRef = {
  id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  body_locale?: string;
  visibility: VisibilityTier;
  completeness: string;
  status: string;
  review_state: string;
  author_id: string;
  published_at?: string;
  updated_at: string;
  task_meta?: TaskMeta;
  artifact_meta?: ArtifactMeta;
  author_user?: AuthorUserRef;
  recent_warnings?: RecentWarning[];
};

// RecentWarning mirrors one events.artifact.warning_raised row for the
// Reader Trust Card (Task propose-경로-warning-영속화). The server emits
// up to 5 rows per artifact detail, newest first.
export type RecentWarning = {
  codes: string[];
  revision_number: number;
  author_id?: string;
  canonical_rewrite_without_evidence?: boolean;
  created_at: string;
};

export type Artifact = ArtifactRef & {
  body_markdown: string;
  tags: string[];
  author_version?: string;
  superseded_by?: string;
  created_at: string;
  // revision_number is the current head revision. TaskControls passes
  // this back as expected_version on POST /task-meta so the UI inherits
  // the same optimistic-lock contract every MCP write uses.
  revision_number?: number;
  relates_to?: EdgeRef[];
  related_by?: EdgeRef[];
  pins?: PinRef[];
  assets?: AssetRef[];
  source_session_ref?: SourceSessionRef;
  recent_warnings?: RecentWarning[];
  can_edit_visibility?: boolean;
};

export type SearchHit = {
  artifact_id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  heading?: string;
  snippet: string;
  distance: number;
  updated_at?: string;
  status?: string;
  completeness?: string;
  task_status?: string;
  task_priority?: string;
};

export type RevisionRow = {
  revision_number: number;
  title: string;
  body_hash: string;
  author_id: string;
  author_user?: AuthorUserRef;
  author_version?: string;
  commit_msg?: string;
  completeness: string;
  revision_shape?: RevisionShape;
  revision_type?: RevisionType;
  bulk_op_id?: string;
  created_at: string;
};

export type RevisionsResp = {
  artifact_id: string;
  slug: string;
  title: string;
  revisions: RevisionRow[];
};

export type DiffRevMeta = {
  revision_number: number;
  title: string;
  body_hash?: string;
  author_id: string;
  author_user?: AuthorUserRef;
  author_version?: string;
  commit_msg?: string;
  revision_shape?: RevisionShape;
  revision_type?: RevisionType;
  created_at: string;
};

export type DiffStats = {
  lines_added: number;
  lines_removed: number;
  bytes_added: number;
  bytes_removed: number;
};

export type SectionDelta = {
  heading: string;
  change: "unchanged" | "modified" | "added" | "removed";
  excerpt_before?: string;
  excerpt_after?: string;
  lines_added: number;
  lines_removed: number;
};

export type RevisionShape =
  | "body_patch"
  | "meta_patch"
  | "acceptance_transition"
  | "scope_defer";

export type RevisionType =
  | "text_edit"
  | "acceptance_toggle"
  | "meta_change"
  | "system_auto"
  | "mixed";

export type MetaDeltaEntry = {
  key: string;
  before: unknown;
  after: unknown;
};

export type AcceptanceItem = {
  index: number;
  state: "[ ]" | "[x]" | "[~]" | "[-]";
  text: string;
  changed?: boolean;
  from_state?: "[ ]" | "[x]" | "[~]" | "[-]";
  to_state?: "[ ]" | "[x]" | "[~]" | "[-]";
  reason?: string;
};

export type AcceptanceChecklist = {
  items: AcceptanceItem[];
  has_change: boolean;
  changed_index?: number;
  reason?: string;
};

export type DiffResp = {
  artifact_id: string;
  slug: string;
  from: DiffRevMeta;
  to: DiffRevMeta;
  stats: DiffStats;
  meta_delta: MetaDeltaEntry[];
  acceptance_checklist?: AcceptanceChecklist;
  revision_type?: RevisionType;
  section_deltas: SectionDelta[];
  unified_diff: string;
};

export type ChangeGroupImportance = {
  score: number;
  level: "low" | "medium" | "high";
  reasons?: string[];
};

export type ChangeGroupTypeCount = {
  type: string;
  count: number;
};

export type ChangeGroupArtifactRef = {
  id: string;
  slug: string;
  title: string;
  type: string;
  area_slug: string;
};

export type ChangeGroup = {
  group_id: string;
  group_kind: "human_trigger" | "auto_sync" | "maintenance" | "system";
  grouping_key: { kind: string; value: string; confidence: "low" | "medium" | "high" };
  commit_summary: string;
  revision_count: number;
  artifact_count: number;
  type_counts?: ChangeGroupTypeCount[];
  first_artifact?: ChangeGroupArtifactRef;
  artifacts?: ChangeGroupArtifactRef[];
  areas: string[];
  authors: string[];
  time_start: string;
  time_end: string;
  importance: ChangeGroupImportance;
  verification_state: string;
};

export type TodaySummary = {
  headline: string;
  bullets: string[];
  source: "llm" | "rule_based";
  ai_hint?: string;
  created_at: string;
};

export type TodayBaseline = {
  revision_watermark: number;
  last_seen_at?: string;
  defaulted_to_days?: number;
  // Set when the watermark-since query was empty and the server fell back.
  // "recent_7d" — last-7-day window. "importance_top" — no time filter,
  // importance ordering. Empty/undefined means primary tier had results.
  fallback_used?: "recent_7d" | "importance_top";
};

export type TodayResp = {
  project_slug: string;
  groups: ChangeGroup[];
  summary: TodaySummary;
  baseline: TodayBaseline;
  max_revision_id: number;
};

export type ReadState = "unseen" | "glanced" | "read" | "deeply_read";

export type ArtifactReadState = {
  artifact_id: string;
  user_key?: string;
  read_state: ReadState;
  completion_pct: number;
  last_seen_at?: string;
  event_count: number;
};

export type ReadStatesResp = {
  project_slug: string;
  user_key: string;
  states: ArtifactReadState[];
};

export type ReadMarkResp = {
  project_slug: string;
  user_key: string;
  revision_watermark: number;
};

export type InboxResp = {
  project_slug: string;
  count: number;
  items: ArtifactRef[];
};

export type InboxReviewResp = {
  status: "accepted";
  artifact_id: string;
  slug: string;
  review_state: "approved" | "rejected";
  row_status: "published" | "archived";
};

export type ReadEventInput = {
  artifact_id: string;
  artifact_slug?: string;
  started_at: string;
  ended_at: string;
  active_seconds: number;
  scroll_max_pct: number;
  idle_seconds: number;
  locale?: string;
};

export type ReadEventResp = {
  id: string;
  artifact_id: string;
  active_seconds: number;
  scroll_max_pct: number;
};

const base = "";

async function j<T>(path: string): Promise<T> {
  const res = await fetch(base + path, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
  }
  return res.json() as Promise<T>;
}

async function gitJ<T extends { git_preview?: GitPreviewEnvelope }>(path: string): Promise<T> {
  const res = await fetch(base + path, { headers: { Accept: "application/json" } });
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }
  if (!res.ok) {
    if (parsed && typeof parsed === "object" && "git_preview" in parsed) {
      return parsed as T;
    }
    throw new Error(`${res.status} ${res.statusText}: ${text.slice(0, 200)}`);
  }
  return parsed as T;
}

function p(project: string): string {
  return `/api/p/${encodeURIComponent(project)}`;
}

function gitQuery(params: Record<string, string | undefined>): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value) qs.set(key, value);
  }
  return qs.toString();
}

function taskFlowQuery(params: TaskFlowParams = {}): string {
  const qs = new URLSearchParams();
  if (params.project_scope) qs.set("project_scope", params.project_scope);
  for (const slug of params.project_slugs ?? []) qs.append("project_slugs", slug);
  if (params.actor_scope) qs.set("actor_scope", params.actor_scope);
  if (params.actor_id) qs.set("actor_id", params.actor_id);
  for (const id of params.actor_ids ?? []) qs.append("actor_ids", id);
  if (params.include_unassigned !== undefined) qs.set("include_unassigned", String(params.include_unassigned));
  if (params.flow_scope) qs.set("flow_scope", params.flow_scope);
  if (params.area_slug) qs.set("area_slug", params.area_slug);
  if (params.priority) qs.set("priority", params.priority);
  if (params.status) qs.set("status", params.status);
  if (params.limit) qs.set("limit", String(params.limit));
  return qs.toString();
}

export function projectListPath(options: ProjectListOptions = {}): string {
  if (!options.includeHidden) return "/api/projects";
  const qs = new URLSearchParams();
  qs.set("include_hidden", "true");
  return `/api/projects?${qs.toString()}`;
}

// TaskMetaPatchInput is the write surface the Reader's TaskControls
// ships to POST /api/p/{project}/artifacts/{idOrSlug}/task-meta. Fields
// map 1:1 onto the server taskMetaPatchRequest — status / assignee /
// priority / due_at / parent_slug are the operational-metadata axes the
// Decision permits. `null` is never emitted by the current UI (clearing a field needs a
// separate design pass); every `undefined` field is omitted from the wire
// payload.
export type TaskMetaPatchInput = {
  expected_version: number;
  commit_msg: string;
  author_id: string;
  author_version?: string;
  status?: "open" | "claimed_done" | "blocked" | "cancelled";
  assignee?: string;
  priority?: "p0" | "p1" | "p2" | "p3";
  due_at?: string;
  parent_slug?: string;
};

export type TaskMetaPatchResp = {
  artifact_id: string;
  slug: string;
  revision_number: number;
};

export type TaskAssignInput = {
  assignee: string;
  reason?: string;
  author_id?: string;
  author_version?: string;
};

export type TaskAssignResp = {
  status: "accepted";
  artifact_id: string;
  slug: string;
  revision_number: number;
  new_assignee: string;
};

export type TaskMetaPatchError = {
  error_code: string;
  message: string;
  failed?: string[];
};

export type ArtifactVisibilityPatchInput = {
  visibility: VisibilityTier;
};

export type ArtifactVisibilityPatchResp = {
  status: "ok";
  artifact_id: string;
  slug: string;
  visibility: VisibilityTier;
};

export type ArtifactVisibilityPatchError = {
  error_code: string;
  message: string;
};

// Project bootstrap (Decision project-bootstrap-canonical-flow-reader-ui-
// first-class). primary_language is required and immutable post-create —
// the form makes the user pick deliberately; defaulting to a guessed
// language would silently break agents that hit the artifact later.
export type CreateProjectInput = {
  slug: string;
  name: string;
  primary_language: "en" | "ko" | "ja";
  description?: string;
  color?: string;
  git_remote_url?: string;
};

export type CreateProjectResp = {
  project_id: string;
  slug: string;
  name: string;
  primary_language: string;
  url: string;
  default_area: string;
  areas_created: number;
  templates_created: number;
};

export type InviteRole = "editor" | "viewer";
export type InviteExpiryPolicy = "1d" | "7d" | "30d" | "permanent";
export type InviteExtendTo = "+7d" | "+30d" | "permanent";

export type InviteIssueInput = {
  role: InviteRole;
  expires_in_hours?: number;
  expires_policy?: InviteExpiryPolicy;
};

export type InviteIssueResp = {
  invite_url: string;
  expires_at: string | null;
};

export type InviteJoinInfo = {
  project_slug: string;
  project_name: string;
  role: InviteRole;
  expires_at: string | null;
};

export type InviteError = {
  error_code: string;
  message: string;
};

// ProjectCreateError mirrors the REST envelope from
// internal/pindoc/httpapi/projects.go. error_code values: SLUG_INVALID /
// SLUG_RESERVED / SLUG_TAKEN / NAME_REQUIRED / LANG_REQUIRED /
// LANG_INVALID / BAD_JSON / INTERNAL_ERROR. The form maps each to an
// i18n key for inline display.
export type ProjectCreateError = {
  error_code: string;
  message: string;
};

// task-providers-admin-ui — instance-level identity provider
// registry. The admin UI mutates `instance_providers` rows at
// runtime; the daemon swaps OAuth credentials in-process so changes
// take effect on the next request.
export type InstanceProvider = {
  id: string;
  provider_name: string;
  display_name: string;
  client_id: string;
  has_client_secret: boolean;
  config?: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  created_by_user_id?: string;
};

export type InstanceProvidersResp = {
  providers: InstanceProvider[];
};

export type InstanceProviderUpsertInput = {
  provider_name: string;
  display_name?: string;
  client_id: string;
  client_secret?: string;
  config?: Record<string, unknown>;
  enabled?: boolean;
};

export type InstanceProviderOpResp = {
  status: "upserted" | "deleted";
  provider?: InstanceProvider;
};

export type InstanceProviderError = {
  error_code: string;
  message: string;
};

// Phase D — permission management plane shapes. These mirror
// internal/pindoc/httpapi/members.go envelopes 1:1. Errors come back
// as the same { error_code, message } shape every other write
// endpoint uses; the helper below throws a typed Error so the panel
// can map error_code → user-facing copy.
export type MemberRow = {
  user_id: string;
  display_name?: string;
  github_handle?: string;
  role: "owner" | "editor" | "viewer";
  invited_by_id?: string;
  joined_at: string;
  is_self?: boolean;
};

export type MembersListResp = {
  project_slug: string;
  viewer_role: "owner" | "editor" | "viewer";
  viewer_id?: string;
  members: MemberRow[];
};

export type ActiveInviteRow = {
  token_hash: string;
  role: "editor" | "viewer";
  issued_by_id?: string;
  issued_at: string;
  expires_at: string | null;
};

export type InvitesListResp = {
  project_slug: string;
  invites: ActiveInviteRow[];
};

export type MembersOpResp = {
  status: "removed" | "revoked" | "extended";
};

export type MembersOpError = {
  error_code: string;
  message: string;
};

export const api = {
  // Instance-wide
  config: () => j<ServerConfig>("/api/config"),
  projectList: (options?: ProjectListOptions) => j<ProjectListResp>(projectListPath(options)),
  users: () => j<{ users: UserRef[] }>("/api/users"),
  currentUser: () => j<CurrentUserResp>("/api/user/current"),
  signOut: async (): Promise<void> => {
    const res = await fetch("/auth/logout", {
      method: "POST",
      headers: { Accept: "application/json" },
    });
    if (!res.ok && res.status !== 404) {
      const body = await res.text().catch(() => "");
      throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
    }
  },

  // Project bootstrap (Decision project-bootstrap-canonical-flow-reader-
  // ui-first-class). The Reader's "+ New project" page calls this; the
  // pindoc-admin CLI and pindoc.project.create MCP tool share the same
  // backend function (projects.CreateProject in Go). On 4xx the server
  // returns { error_code, message } — this throws a typed Error so the
  // page can map error_code → i18n key for inline form errors.
  createProject: async (input: CreateProjectInput): Promise<CreateProjectResp> => {
    const res = await fetch("/api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      let parsed: ProjectCreateError | null = null;
      try {
        parsed = (await res.json()) as ProjectCreateError;
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<ProjectCreateError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as CreateProjectResp;
  },

  // Assignee semantic shortcut. The browser cannot speak stdio MCP
  // directly, so the HTTP bridge records an ops telemetry row as
  // tool_name=pindoc.task.assign and writes the same assignee-only
  // meta_patch revision the MCP tool would produce.
  taskAssign: async (
    project: string,
    idOrSlug: string,
    input: TaskAssignInput,
  ): Promise<TaskAssignResp> => {
    const res = await fetch(
      `${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}/task-assign`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      },
    );
    if (!res.ok) {
      let parsed: TaskMetaPatchError | null = null;
      try {
        parsed = (await res.json()) as TaskMetaPatchError;
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<TaskMetaPatchError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
        err.failed = parsed.failed;
      }
      throw err;
    }
    return (await res.json()) as TaskAssignResp;
  },

  // Operational-metadata write — the generic UI-side POST. Throws a
  // TaskMetaPatchError-shaped Error with error_code preserved so callers
  // can surface "status via transition tool" / "version conflict" etc.
  // as first-class UX instead of a generic network error.
  taskMetaPatch: async (
    project: string,
    idOrSlug: string,
    input: TaskMetaPatchInput,
  ): Promise<TaskMetaPatchResp> => {
    const res = await fetch(
      `${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}/task-meta`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      },
    );
    if (!res.ok) {
      let parsed: TaskMetaPatchError | null = null;
      try {
        parsed = (await res.json()) as TaskMetaPatchError;
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<TaskMetaPatchError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
        err.failed = parsed.failed;
      }
      throw err;
    }
    return (await res.json()) as TaskMetaPatchResp;
  },
  artifactVisibilityPatch: async (
    project: string,
    idOrSlug: string,
    input: ArtifactVisibilityPatchInput,
  ): Promise<ArtifactVisibilityPatchResp> => {
    const res = await fetch(
      `${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      },
    );
    if (!res.ok) {
      let parsed: ArtifactVisibilityPatchError | null = null;
      try {
        parsed = (await res.json()) as ArtifactVisibilityPatchError;
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<ArtifactVisibilityPatchError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as ArtifactVisibilityPatchResp;
  },

  // Project-scoped
  project: (project: string) => j<Project>(p(project)),
  projectSettingsPatch: async (
    project: string,
    input: ProjectSettingsPatchInput,
  ): Promise<ProjectSettingsPatchResp> => {
    const res = await fetch(`${p(project)}/settings`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      let parsed: { error_code?: string; message?: string } | null = null;
      try {
        parsed = (await res.json()) as { error_code?: string; message?: string };
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & { error_code?: string };
      if (parsed?.error_code) err.error_code = parsed.error_code;
      throw err;
    }
    return (await res.json()) as ProjectSettingsPatchResp;
  },
  issueInvite: async (
    project: string,
    input: InviteIssueInput,
  ): Promise<InviteIssueResp> => {
    const res = await fetch(`${p(project)}/invite`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      let parsed: InviteError | null = null;
      try {
        parsed = (await res.json()) as InviteError;
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<InviteError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as InviteIssueResp;
  },
  inviteInfo: async (invite: string): Promise<InviteJoinInfo> => {
    const res = await fetch(`/join?invite=${encodeURIComponent(invite)}`, {
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      let parsed: InviteError | null = null;
      try {
        parsed = (await res.json()) as InviteError;
      } catch {
        // fall through to generic
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<InviteError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as InviteJoinInfo;
  },
  areas: (project: string, params?: { includeTemplates?: boolean }) => {
    const qs = new URLSearchParams();
    if (params?.includeTemplates) qs.set("include_templates", "true");
    const q = qs.toString();
    return j<{ project_slug: string; areas: Area[] }>(
      `${p(project)}/areas${q ? `?${q}` : ""}`,
    );
  },
  artifacts: (project: string, params?: { area?: string; type?: string; includeTemplates?: boolean }) => {
    const qs = new URLSearchParams();
    if (params?.area) qs.set("area", params.area);
    if (params?.type) qs.set("type", params.type);
    if (params?.includeTemplates) qs.set("include_templates", "true");
    const q = qs.toString();
    return j<{ project_slug: string; artifacts: ArtifactRef[] }>(
      `${p(project)}/artifacts${q ? `?${q}` : ""}`,
    );
  },
  taskFlow: (project: string, params?: TaskFlowParams) => {
    const q = taskFlowQuery(params);
    return j<TaskFlowResp>(`${p(project)}/task-flow${q ? `?${q}` : ""}`);
  },
  artifact: (project: string, idOrSlug: string) =>
    j<Artifact>(`${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}`),
  inbox: (project: string) => j<InboxResp>(`${p(project)}/inbox`),
  inboxReview: async (
    project: string,
    idOrSlug: string,
    decision: "approve" | "reject",
  ): Promise<InboxReviewResp> => {
    const res = await fetch(
      `${p(project)}/inbox/${encodeURIComponent(idOrSlug)}/review`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          decision,
          reviewer_id: "reader",
          commit_msg: `Reader Inbox ${decision}`,
        }),
      },
    );
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
    }
    return (await res.json()) as InboxReviewResp;
  },
  changeGroups: (project: string, params?: { limit?: number; area?: string; kind?: string; locale?: string }) => {
    const qs = new URLSearchParams();
    if (params?.limit) qs.set("limit", String(params.limit));
    if (params?.area) qs.set("area", params.area);
    if (params?.kind) qs.set("kind", params.kind);
    if (params?.locale) qs.set("locale", params.locale);
    const q = qs.toString();
    return j<TodayResp>(`${p(project)}/change-groups${q ? `?${q}` : ""}`);
  },
  readStates: (project: string) =>
    j<ReadStatesResp>(`${p(project)}/read-states`),
  artifactReadState: (project: string, idOrSlug: string) =>
    j<ArtifactReadState>(`${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}/read-state`),
  readMark: async (project: string, revisionWatermark?: number): Promise<ReadMarkResp> => {
    const res = await fetch(`${p(project)}/read-mark`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ revision_watermark: revisionWatermark ?? 0 }),
    });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
    }
    return (await res.json()) as ReadMarkResp;
  },
  readEvent: async (
    project: string,
    input: ReadEventInput,
    opts?: { keepalive?: boolean },
  ) => {
    const res = await fetch(`${p(project)}/read-events`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      keepalive: opts?.keepalive,
    });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
    }
    return res.json() as Promise<ReadEventResp>;
  },
  exportProjectUrl: (project: string, params?: { area?: string; includeRevisions?: boolean; format?: "zip" | "tar" }) => {
    const qs = new URLSearchParams();
    if (params?.area) qs.set("area", params.area);
    if (params?.includeRevisions) qs.set("include_revisions", "true");
    if (params?.format) qs.set("format", params.format);
    const q = qs.toString();
    return `${p(project)}/export${q ? `?${q}` : ""}`;
  },
  search: (project: string, q: string) =>
    j<{ query: string; project_slug: string; hits: SearchHit[]; notice?: string }>(
      `${p(project)}/search?q=${encodeURIComponent(q)}`,
    ),
  revisions: (project: string, idOrSlug: string) =>
    j<RevisionsResp>(`${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}/revisions`),
  diff: (project: string, idOrSlug: string, from?: number, to?: number) => {
    const qs = new URLSearchParams();
    if (from) qs.set("from", String(from));
    if (to) qs.set("to", String(to));
    const q = qs.toString();
    return j<DiffResp>(
      `${p(project)}/artifacts/${encodeURIComponent(idOrSlug)}/diff${q ? `?${q}` : ""}`,
    );
  },
  gitRepos: (project: string) =>
    j<GitReposResp>(`${p(project)}/git/repos`),
  gitCommit: (project: string, repoID: string, commit: string) =>
    gitJ<GitCommitResp>(
      `${p(project)}/git/commit?${gitQuery({ repo_id: repoID, commit })}`,
    ),
  gitChangedFiles: (project: string, repoID: string, commit: string) =>
    gitJ<GitChangedFilesResp>(
      `${p(project)}/git/changed-files?${gitQuery({ repo_id: repoID, commit })}`,
    ),
  gitBlob: (project: string, repoID: string, commit: string, path: string) =>
    gitJ<GitBlobResp>(
      `${p(project)}/git/blob?${gitQuery({ repo_id: repoID, commit, path })}`,
    ),
  gitDiff: (project: string, repoID: string, commit: string, path: string) =>
    gitJ<GitDiffResp>(
      `${p(project)}/git/diff?${gitQuery({ repo_id: repoID, commit, path })}`,
    ),
  gitCommitReferences: (project: string, sha: string) =>
    j<GitCommitReferencesResp>(
      `${p(project)}/git/commits/${encodeURIComponent(sha)}/referencing-artifacts`,
    ),

  // Phase D — permission management plane. Members list is open to
  // anyone in the project; invites list and the two DELETEs are
  // owner-only at the server. The fetch helpers below all parse the
  // standard error envelope so the panel can show a friendly message
  // (LAST_OWNER, PROJECT_OWNER_REQUIRED, etc.) instead of a raw status.
  members: (project: string) =>
    j<MembersListResp>(`${p(project)}/members`),
  removeMember: async (project: string, userId: string): Promise<MembersOpResp> => {
    const res = await fetch(`${p(project)}/members/${encodeURIComponent(userId)}`, {
      method: "DELETE",
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      let parsed: MembersOpError | null = null;
      try {
        parsed = (await res.json()) as MembersOpError;
      } catch {
        // fall through
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<MembersOpError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as MembersOpResp;
  },
  activeInvites: (project: string) =>
    j<InvitesListResp>(`${p(project)}/invites`),
  revokeInvite: async (
    project: string,
    tokenHash: string,
  ): Promise<MembersOpResp> => {
    const res = await fetch(
      `${p(project)}/invites/${encodeURIComponent(tokenHash)}`,
      { method: "DELETE", headers: { Accept: "application/json" } },
    );
    if (!res.ok) {
      let parsed: MembersOpError | null = null;
      try {
        parsed = (await res.json()) as MembersOpError;
      } catch {
        // fall through
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<MembersOpError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as MembersOpResp;
  },
  extendInvite: async (
    project: string,
    tokenHash: string,
    extendTo: InviteExtendTo,
  ): Promise<MembersOpResp> => {
    const res = await fetch(
      `${p(project)}/invites/${encodeURIComponent(tokenHash)}/extend`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ extend_to: extendTo }),
      },
    );
    if (!res.ok) {
      let parsed: MembersOpError | null = null;
      try {
        parsed = (await res.json()) as MembersOpError;
      } catch {
        // fall through
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<MembersOpError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as MembersOpResp;
  },

  // First-time identity flow — fresh installs land on /onboarding/
  // identity, fill the form, and the BE binds settings + creates
  // the loopback principal's users.id row in one call. Loopback
  // only; non-loopback callers see INSTANCE_OWNER_REQUIRED.
  setupIdentity: async (
    input: OnboardingIdentityInput,
  ): Promise<OnboardingIdentityResp> => {
    const res = await fetch("/api/onboarding/identity", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      let parsed: OnboardingIdentityError | null = null;
      try {
        parsed = (await res.json()) as OnboardingIdentityError;
      } catch {
        // fall through
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<OnboardingIdentityError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as OnboardingIdentityResp;
  },

  // task-providers-admin-ui — instance-level identity provider
  // registry. Loopback principal only at the BE; non-loopback
  // callers see INSTANCE_OWNER_REQUIRED, the ProvidersPanel maps
  // each error_code to a friendly explanation.
  instanceProviders: () => j<InstanceProvidersResp>("/api/instance/providers"),
  upsertInstanceProvider: async (
    input: InstanceProviderUpsertInput,
  ): Promise<InstanceProviderOpResp> => {
    const res = await fetch("/api/instance/providers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      let parsed: InstanceProviderError | null = null;
      try {
        parsed = (await res.json()) as InstanceProviderError;
      } catch {
        // fall through
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<InstanceProviderError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as InstanceProviderOpResp;
  },
  deleteInstanceProvider: async (
    idOrName: string,
  ): Promise<InstanceProviderOpResp> => {
    const res = await fetch(
      `/api/instance/providers/${encodeURIComponent(idOrName)}`,
      { method: "DELETE", headers: { Accept: "application/json" } },
    );
    if (!res.ok) {
      let parsed: InstanceProviderError | null = null;
      try {
        parsed = (await res.json()) as InstanceProviderError;
      } catch {
        // fall through
      }
      const err = new Error(
        parsed?.message ?? `${res.status} ${res.statusText}`,
      ) as Error & Partial<InstanceProviderError>;
      if (parsed) {
        err.error_code = parsed.error_code;
        err.message = parsed.message;
      }
      throw err;
    }
    return (await res.json()) as InstanceProviderOpResp;
  },

  // Ops — instance-wide MCP tool-call telemetry. Reads from the async
  // mcp_tool_calls pipeline (Phase J). window = "1h" | "6h" | "24h" |
  // "7d" | "30d", default 24h. project filters by project_slug; omit
  // for an instance-wide view.
  telemetry: (params?: { window?: TelemetryWindow; project?: string; recentLimit?: number }) => {
    const qs = new URLSearchParams();
    if (params?.window) qs.set("window", params.window);
    if (params?.project) qs.set("project", params.project);
    if (params?.recentLimit) qs.set("recent_limit", String(params.recentLimit));
    const q = qs.toString();
    return j<TelemetryResponse>(`/api/ops/telemetry${q ? `?${q}` : ""}`);
  },
};

export type TelemetryWindow = "1h" | "6h" | "24h" | "7d" | "30d";

export type TelemetryTotals = {
  calls: number;
  errors: number;
  total_input_tokens: number;
  total_output_tokens: number;
  unique_agents: number;
};

export type TelemetryToolRow = {
  tool_name: string;
  calls: number;
  errors: number;
  error_rate: number;
  avg_duration_ms: number;
  p50_duration_ms: number;
  p95_duration_ms: number;
  total_input_tokens: number;
  total_output_tokens: number;
  avg_input_tokens: number;
  avg_output_tokens: number;
  avg_input_bytes: number;
  avg_output_bytes: number;
  last_call_at: string;
};

export type TelemetryRecentCall = {
  started_at: string;
  duration_ms: number;
  tool_name: string;
  agent_id?: string;
  project_slug?: string;
  input_bytes: number;
  output_bytes: number;
  input_tokens_est: number;
  output_tokens_est: number;
  error_code?: string;
  toolset_version?: string;
};

export type TelemetryResponse = {
  window_hours: number;
  project_slug?: string;
  totals: TelemetryTotals;
  tools: TelemetryToolRow[];
  recent: TelemetryRecentCall[];
};
