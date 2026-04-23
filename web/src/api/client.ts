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
  // Phase 18 — default project's locale segment. Reader's LegacyRedirect
  // inserts it into /p/:slug/... URLs so bare /wiki/... shares still
  // resolve to a canonical /p/:slug/:locale/wiki/... URL. Empty falls
  // back to "en" in the UI helper.
  default_project_locale?: string;
  multi_project: boolean;
  version: string;
};

export type Project = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  color?: string;
  primary_language: string;
  // Phase 18 — canonical locale of this project row. Same slug may
  // live in multiple locales; (slug, locale) is the unique key.
  locale?: string;
  areas_count: number;
  artifacts_count: number;
  created_at: string;
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

export type ArtifactRef = {
  id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  completeness: string;
  status: string;
  review_state: string;
  author_id: string;
  published_at?: string;
  updated_at: string;
  task_meta?: TaskMeta;
  artifact_meta?: ArtifactMeta;
  author_user?: AuthorUserRef;
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
  author_id: string;
  author_version?: string;
  commit_msg?: string;
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

export type DiffResp = {
  artifact_id: string;
  slug: string;
  from: DiffRevMeta;
  to: DiffRevMeta;
  stats: DiffStats;
  section_deltas: SectionDelta[];
  unified_diff: string;
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

export const api = {
  // Instance-wide
  config: () => j<ServerConfig>("/api/config"),
  projectList: () => j<ProjectListResp>("/api/projects"),

  // Project-scoped
  project: (project: string) => j<Project>(p(project)),
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
