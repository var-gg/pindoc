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
  // auth_mode mirrors Capabilities.AuthMode. TaskControls flips between
  // inline-editable and read-only off this value. M1 always returns
  // "trusted_local"; V1.5+ adds "project_token" / "oauth" where the
  // Reader must fall back to read-only + chat-shortcut UX
  // (Decision agent-only-write-분할, Alternative C).
  auth_mode?: "trusted_local" | "project_token" | "oauth";
  // onboarding_required tells the SPA to redirect a fresh install to
  // the new-project wizard instead of the legacy "open default project"
  // landing. True when the instance has no projects other than the
  // seed `pindoc` row. Decision project-bootstrap-canonical-flow-
  // reader-ui-first-class.
  onboarding_required?: boolean;
};

export type Project = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  color?: string;
  primary_language: string;
  sensitive_ops?: "auto" | "confirm";
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

export type ProjectListItem = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  color?: string;
  primary_language: string;
  artifacts_count: number;
  created_at: string;
};

export type ProjectListResp = {
  projects: ProjectListItem[];
  default_project_slug: string;
  multi_project: boolean;
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
  // Task lifecycle v2 (migration 0013). `verified` is reserved for
  // pindoc.artifact.verify — direct transition via artifact.propose is
  // rejected server-side (VER_VIA_VERIFY_TOOL_ONLY). `claimed_done`
  // requires 100% acceptance checkboxes.
  status?: "open" | "claimed_done" | "verified" | "blocked" | "cancelled";
  priority?: "p0" | "p1" | "p2" | "p3";
  assignee?: string;
  due_at?: string;
  parent_slug?: string;
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
// kind="code".
export type PinRef = {
  kind: "code" | "resource" | "url";
  repo?: string;
  commit_sha?: string;
  path: string;
  lines_start?: number;
  lines_end?: number;
};

export type EdgeRef = {
  artifact_id: string;
  slug: string;
  type: string;
  title: string;
  relation: string;
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
  github_handle?: string;
  source: "harness_install" | "pindoc_admin" | "github_oauth";
};

export type ArtifactRef = {
  id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  body_locale?: string;
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
  source_session_ref?: SourceSessionRef;
  recent_warnings?: RecentWarning[];
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
};

export type RevisionRow = {
  revision_number: number;
  title: string;
  body_hash: string;
  author_id: string;
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

export type TodayResp = {
  project_slug: string;
  groups: ChangeGroup[];
  summary: TodaySummary;
  baseline: {
    revision_watermark: number;
    last_seen_at?: string;
    defaulted_to_days?: number;
  };
  max_revision_id: number;
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

function p(project: string): string {
  return `/api/p/${encodeURIComponent(project)}`;
}

// TaskMetaPatchInput is the write surface the Reader's TaskControls
// ships to POST /api/p/{project}/artifacts/{idOrSlug}/task-meta. Fields
// map 1:1 onto the server taskMetaPatchRequest — status / assignee /
// priority / due_at / parent_slug are the operational-metadata axes the
// Decision permits. `verified` remains verify-tool only server-side.
// `null` is never emitted by the current UI (clearing a field needs a
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
  owner_id?: string;
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

export type InviteIssueInput = {
  role: InviteRole;
  expires_in_hours: number;
};

export type InviteIssueResp = {
  invite_url: string;
  expires_at: string;
};

export type InviteJoinInfo = {
  project_slug: string;
  project_name: string;
  role: InviteRole;
  expires_at: string;
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

export const api = {
  // Instance-wide
  config: () => j<ServerConfig>("/api/config"),
  projectList: () => j<ProjectListResp>("/api/projects"),
  users: () => j<{ users: UserRef[] }>("/api/users"),

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

  // Project-scoped
  project: (project: string) => j<Project>(p(project)),
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
