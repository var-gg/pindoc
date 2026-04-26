import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link } from "react-router";
import {
  ArrowDownLeft,
  ArrowUpRight,
  ChevronDown,
  ChevronRight,
  History as HistoryIcon,
} from "lucide-react";
import {
  api,
  type Artifact,
  type ArtifactMeta,
  type EdgeRef,
  type PinRef,
  type RevisionRow,
  type ServerConfig,
  type SourceSessionRef,
  type UserRef,
} from "../api/client";
import type { Aggregate } from "./useReaderData";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { localizedAreaName } from "./areaLocale";
import { TaskControls } from "./TaskControls";
import { Toc } from "./Toc";
import { headingsFromBody } from "./slug";
import { isStructureOverlapHeading, structureOverlapSectionsFromBody } from "./structureSections";
import { typeChipClass } from "./typeChip";
import { RevisionTypeBadge } from "./RevisionTypeBadge";
import {
  visualDescription,
  visualLabel,
  visualMetaEnum,
  visualPin,
  visualQuickAction,
  visualRelation,
  type VisualMetaEnumKey,
} from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";
import {
  MINI_GRAPH_CENTER,
  graphRadialPositions,
  graphRelationClass,
  graphRelationLabel,
  graphTypeClassSuffix,
} from "./graphSvg";

type Props = {
  projectSlug: string;
  detail: Artifact | null;
  emptyMessage?: string;
  // auth_mode lets TaskControls flip between inline-editable and
  // read-only without a second round-trip (Decision agent-only-write-
  // 분할). Undefined = config not yet loaded — treat as non-trusted and
  // stay read-only until we know.
  authMode?: ServerConfig["auth_mode"];
  // agents is the author_id aggregate across the current project's
  // artifact list. TaskControls surfaces it as the "assigned to an
  // agent" half of the assignee dropdown.
  agents?: Aggregate[];
  // users is the instance-wide users table projection. Combined with
  // agents to build the assignee dropdown — null when the /api/users
  // fetch failed (Reader still renders, TaskControls hides the users
  // section).
  users?: UserRef[] | null;
  // onArtifactUpdated is called after a successful task-meta write so
  // the Reader refetches the detail and the revision rail / TaskControls
  // reflect the new head.
  onArtifactUpdated?: () => void;
  // Wiki list inspector mode reuses Sidecar as a brief. In that mode the
  // full article is one explicit action away instead of implicit card
  // navigation.
  showOpenDetailAction?: boolean;
};

type CollapsibleKey = "provenance" | "policy" | "timeline" | "meta";
type QuickActionID = Parameters<typeof visualQuickAction>[0];

type CollapsedState = Record<CollapsibleKey, boolean>;

const SIDECAR_COLLAPSE_STORAGE_KEY = "pindoc.reader.sidecar.sections.v1";

const DEFAULT_COLLAPSED_STATE: CollapsedState = {
  provenance: true,
  policy: true,
  timeline: true,
  meta: true,
};

export function Sidecar({
  projectSlug,
  detail,
  emptyMessage,
  authMode,
  agents,
  users,
  onArtifactUpdated,
  showOpenDetailAction,
}: Props) {
  const { t } = useI18n();
  const [collapsed, toggleSection] = useSidecarCollapseState();
  // TOC feeds off body_markdown; Markdown.tsx independently derives the
  // same slugs via the same uniqueSlug ledger so `<h2 id>` matches
  // `href="#..."` without a DOM round-trip. Computed here (not ReaderSurface)
  // so Sidecar owns the TOC lifecycle alongside its other metadata rails.
  const headings = useMemo(
    () => (detail ? headingsFromBody(detail.body_markdown).filter((h) => !isStructureOverlapHeading(h.text)) : []),
    [detail],
  );
  const bodyOverlapSections = useMemo(
    () => (detail ? structureOverlapSectionsFromBody(detail.body_markdown).filter((section) => section.body.trim() !== "") : []),
    [detail],
  );

  if (!detail) {
    return (
      <aside className="sidecar sidecar--empty">
        <div className="sidecar__head">
          <h3>{t("sidecar.this_artifact")}</h3>
        </div>
        <div className="graph-wrap">
          <div className="mini-graph mini-graph--empty">
            <span className="mini-graph__empty">{emptyMessage ?? t("sidecar.no_selection")}</span>
          </div>
        </div>
      </aside>
    );
  }

  const av = agentAvatar(detail.author_id);
  const publishedAt = detail.published_at
    ? new Date(detail.published_at).toLocaleString()
    : "—";
  const areaLabel = localizedAreaName(t, detail.area_slug, detail.area_slug);
  const artifactHref = `/p/${projectSlug}/wiki/${detail.slug}`;

  // Graph edges aren't derived yet (Phase 3+ pipeline populates these via
  // artifact.superseded_by + future artifact_edges). Show placeholder
  // states so the visual treatment is faithful and the data gap is
  // honest.
  const hasSupersedes = Boolean(detail.superseded_by && detail.superseded_by !== "");

  return (
    <aside id="sidecar-live-data" className="sidecar sidecar--detail">
      <IdentityStrip detail={detail} />

      <QuickActions detail={detail} artifactHref={artifactHref} />
      {showOpenDetailAction && (
        <Link to={artifactHref} className="sidecar-open-detail">
          <span>{t("reader.inspector_open_detail")}</span>
          <ArrowUpRight className="lucide" aria-hidden="true" />
        </Link>
      )}

      {headings.length >= 2 && <Toc headings={headings} />}

      {detail.type === "Task" && <TaskInspectorSummary detail={detail} areaLabel={areaLabel} />}

      {detail.type === "Task" && (
        <TaskControls
          projectSlug={projectSlug}
          detail={detail}
          authMode={authMode}
          agents={agents ?? []}
          users={users ?? []}
          onUpdated={() => onArtifactUpdated?.()}
        />
      )}

      <div className="graph-wrap">
        <MiniGraph detail={detail} projectSlug={projectSlug} />
      </div>

      <SidecarStaticSection title={t("sidecar.relations")}>
        {bodyOverlapSections.length > 0 && (
          <BodyOverlapLink count={bodyOverlapSections.length} />
        )}
        <ConnectedArtifacts
          projectSlug={projectSlug}
          relates={detail.relates_to ?? []}
          relatedBy={detail.related_by ?? []}
          hasSupersedes={hasSupersedes}
          supersededBy={detail.superseded_by ?? ""}
        />
      </SidecarStaticSection>

      <SidecarCollapsibleSection
        id="provenance"
        title={t("sidecar.provenance")}
        collapsed={collapsed.provenance}
        onToggle={toggleSection}
      >
        <ProvenanceBlock
          pins={detail.pins}
          sourceSession={detail.source_session_ref}
          updatedAt={detail.updated_at}
        />
      </SidecarCollapsibleSection>

      <SidecarCollapsibleSection
        id="policy"
        title={t("sidecar.policy")}
        collapsed={collapsed.policy}
        onToggle={toggleSection}
      >
        <PolicyBlock meta={detail.artifact_meta} />
      </SidecarCollapsibleSection>

      <SidecarCollapsibleSection
        id="timeline"
        title={t("sidecar.timeline")}
        collapsed={collapsed.timeline}
        onToggle={toggleSection}
      >
        <RecentChanges projectSlug={projectSlug} slug={detail.slug} />
      </SidecarCollapsibleSection>

      <SidecarCollapsibleSection
        id="meta"
        title={t("sidecar.meta")}
        collapsed={collapsed.meta}
        onToggle={toggleSection}
      >
        <MetaBlock
          avClassName={av.className}
          avInitials={av.initials}
          authorID={detail.author_id}
          authorVersion={detail.author_version}
          areaLabel={areaLabel}
          status={detail.status}
          completeness={detail.completeness}
          publishedAt={publishedAt}
          slug={detail.slug}
        />
      </SidecarCollapsibleSection>
    </aside>
  );
}

function BodyOverlapLink({ count }: { count: number }) {
  const { t } = useI18n();
  return (
    <div className="sidecar-body-overlap">
      <span>{t("sidecar.body_overlap", count)}</span>
      <a href="#reader-original-structure">{t("sidecar.body_overlap_open")}</a>
    </div>
  );
}

function useSidecarCollapseState(): [CollapsedState, (key: CollapsibleKey) => void] {
  const [collapsed, setCollapsed] = useState<CollapsedState>(() => {
    if (typeof window === "undefined") return DEFAULT_COLLAPSED_STATE;
    try {
      const raw = window.localStorage.getItem(SIDECAR_COLLAPSE_STORAGE_KEY);
      if (!raw) return DEFAULT_COLLAPSED_STATE;
      const parsed = JSON.parse(raw) as Partial<CollapsedState>;
      return { ...DEFAULT_COLLAPSED_STATE, ...parsed };
    } catch {
      return DEFAULT_COLLAPSED_STATE;
    }
  });

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(SIDECAR_COLLAPSE_STORAGE_KEY, JSON.stringify(collapsed));
  }, [collapsed]);

  const toggle = (key: CollapsibleKey) => {
    setCollapsed((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  return [collapsed, toggle];
}

function IdentityStrip({ detail }: { detail: Artifact }) {
  const lifecycle = detail.type === "Task" && detail.task_meta?.status
    ? detail.task_meta.status
    : `${detail.status} · ${detail.completeness}`;
  return (
    <div className="sidecar-identity">
      <span className={typeChipClass(detail.type)}>{detail.type}</span>
      <span className="sidecar-identity__slug" title={detail.slug}>
        {detail.slug}
      </span>
      <span className="sidecar-identity__status">{lifecycle}</span>
    </div>
  );
}

function QuickActions({ detail, artifactHref }: { detail: Artifact; artifactHref: string }) {
  const { t, lang } = useI18n();
  const agentRef = `pindoc://${detail.slug}`;
  const absoluteHref = typeof window === "undefined"
    ? artifactHref
    : `${window.location.origin}${artifactHref}`;
  const actions: Array<
    | { id: QuickActionID; kind: "button"; onClick: () => void }
    | { id: QuickActionID; kind: "link"; to: string }
  > = [
    { id: "verify_request", kind: "button", onClick: () => copyToClipboard(`Verify ${agentRef}`) },
    { id: "update_request", kind: "button", onClick: () => copyToClipboard(`Update ${agentRef}`) },
    { id: "copy_link", kind: "button", onClick: () => copyToClipboard(absoluteHref) },
    { id: "copy_agent_ref", kind: "button", onClick: () => copyToClipboard(agentRef) },
    { id: "history", kind: "link", to: `${artifactHref}/history` },
  ];
  return (
    <div className="sidecar-actions" aria-label={t("sidecar.quick_actions")}>
      {actions.map((action) => {
        const entry = visualQuickAction(action.id);
        const Icon = visualIconComponent(entry.icon);
        const label = visualLabel(entry, lang);
        const description = visualDescription(entry, lang);
        if (action.kind === "link") {
          return (
            <Link
              key={action.id}
              className="sidecar-action"
              title={description}
              aria-label={label}
              to={action.to}
            >
              <Icon className="lucide" />
            </Link>
          );
        }
        return (
          <button
            key={action.id}
            type="button"
            className="sidecar-action"
            title={description}
            aria-label={label}
            onClick={action.onClick}
          >
            <Icon className="lucide" />
          </button>
        );
      })}
    </div>
  );
}

function TaskInspectorSummary({ detail, areaLabel }: { detail: Artifact; areaLabel: string }) {
  const { t } = useI18n();
  const stats = acceptanceStats(detail.body_markdown);
  const percent = stats.total > 0 ? Math.round((stats.resolved / stats.total) * 100) : 0;
  const edges = (detail.relates_to?.length ?? 0) + (detail.related_by?.length ?? 0);
  const updatedAt = detail.updated_at ? new Date(detail.updated_at).toLocaleString() : "—";
  return (
    <div className="task-inspector-summary">
      <div className="task-inspector-summary__grid">
        <span>{t("task_inspector.status")}</span>
        <strong>{detail.task_meta?.status ?? t("tasks.col_no_status")}</strong>
        <span>{t("task_inspector.area")}</span>
        <strong>{areaLabel}</strong>
        <span>{t("task_inspector.due_at")}</span>
        <strong>{detail.task_meta?.due_at ? new Date(detail.task_meta.due_at).toLocaleString() : "—"}</strong>
        <span>{t("task_inspector.tags")}</span>
        <strong>{detail.tags.length > 0 ? detail.tags.join(", ") : "—"}</strong>
        <span>{t("task_inspector.edges")}</span>
        <strong>{edges}</strong>
        <span>{t("task_inspector.last_revision")}</span>
        <strong>{updatedAt} · {detail.author_id}</strong>
      </div>
      <div className="task-inspector-meter" aria-label={t("task_inspector.acceptance_ratio")}>
        <div className="task-inspector-meter__head">
          <span>{t("task_inspector.acceptance_ratio")}</span>
          <strong>{stats.resolved}/{stats.total} · {percent}%</strong>
        </div>
        <div className="task-inspector-meter__track">
          <span style={{ width: `${percent}%` }} />
        </div>
      </div>
    </div>
  );
}

function acceptanceStats(body: string): { resolved: number; total: number } {
  let resolved = 0;
  let total = 0;
  for (const line of body.split("\n")) {
    const match = /^\s*[-*+]\s+\[([ xX~-])\]\s+/.exec(line.trimEnd());
    if (!match) continue;
    total++;
    if (match[1] !== " ") resolved++;
  }
  return { resolved, total };
}

function copyToClipboard(text: string) {
  if (typeof navigator === "undefined" || !navigator.clipboard) return;
  void navigator.clipboard.writeText(text);
}

function SidecarStaticSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="sidecar-section">
      <div className="sidecar-section__head sidecar-section__head--static">
        <span>{title}</span>
      </div>
      <div className="sidecar-section__body">{children}</div>
    </section>
  );
}

function SidecarCollapsibleSection({
  id,
  title,
  collapsed,
  onToggle,
  children,
}: {
  id: CollapsibleKey;
  title: string;
  collapsed: boolean;
  onToggle: (key: CollapsibleKey) => void;
  children: ReactNode;
}) {
  const { t } = useI18n();
  return (
    <section className="sidecar-section">
      <button
        type="button"
        className="sidecar-section__head"
        aria-expanded={!collapsed}
        title={collapsed ? t("sidecar.expand_section") : t("sidecar.collapse_section")}
        onClick={() => onToggle(id)}
      >
        {collapsed ? <ChevronRight className="lucide" /> : <ChevronDown className="lucide" />}
        <span>{title}</span>
      </button>
      {!collapsed && <div className="sidecar-section__body">{children}</div>}
    </section>
  );
}

function RecentChanges({ projectSlug, slug }: { projectSlug: string; slug: string }) {
  const { t } = useI18n();
  const [revs, setRevs] = useState<RevisionRow[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await api.revisions(projectSlug, slug);
        if (!cancelled) setRevs(resp.revisions);
      } catch {
        if (!cancelled) setRevs([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectSlug, slug]);

  if (!revs || revs.length === 0) return null;
  const shown = revs.slice(0, 3);
  const remainder = revs.length - shown.length;

  return (
    <div className="sidecar-stack">
      <div className="sidecar-timeline-head">
        <span>{t("history.recent_changes")}</span>
        <Link
          to={`/p/${projectSlug}/wiki/${slug}/history`}
          className="sidecar-timeline-head__link"
        >
          <HistoryIcon className="lucide" style={{ width: 11, height: 11 }} />
          {shown.length < revs.length ? t("history.count", revs.length) : ""}
        </Link>
      </div>
      {shown.map((r) => {
        const av = agentAvatar(r.author_id);
        return (
          <div key={r.revision_number} style={{ padding: "4px 0", fontSize: 12 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--fg-1)" }}>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)" }}>
                rev {r.revision_number}
              </span>
              <RevisionTypeBadge revisionType={r.revision_type} compact />
              <span className={av.className} style={{ width: 12, height: 12, fontSize: 7 }}>
                {av.initials}
              </span>
              <span style={{ color: "var(--fg-2)", fontSize: 11.5, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {r.commit_msg || t("history.no_commit_msg")}
              </span>
            </div>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-4)", marginLeft: 36 }}>
              {new Date(r.created_at).toLocaleString()}
            </div>
          </div>
        );
      })}
      {remainder > 0 && (
        <Link
          to={`/p/${projectSlug}/wiki/${slug}/history`}
          style={{ fontSize: 11, color: "var(--fg-3)", fontFamily: "var(--font-mono)", textDecoration: "none" }}
        >
          {t("history.more_revisions", remainder)}
        </Link>
      )}
    </div>
  );
}

const MINI_GRAPH_MAX_NEIGHBORS = 8;

type MiniGraphEdge = {
  edge: EdgeRef;
  direction: "out" | "in";
};

function MiniGraph({
  detail,
  projectSlug,
}: {
  detail: Artifact;
  projectSlug: string;
}) {
  const { t, lang } = useI18n();
  const allEdges: MiniGraphEdge[] = [
    ...(detail.relates_to ?? []).map((edge) => ({ edge, direction: "out" as const })),
    ...(detail.related_by ?? []).map((edge) => ({ edge, direction: "in" as const })),
  ];
  const visible = allEdges.slice(0, MINI_GRAPH_MAX_NEIGHBORS);
  const hidden = Math.max(0, allEdges.length - visible.length);
  const positions = graphRadialPositions(visible.length);

  if (visible.length === 0) {
    return (
      <div className="mini-graph mini-graph--empty">
        <span className="mini-graph__empty">{t("sidecar.no_relations")}</span>
      </div>
    );
  }

  return (
    <div className="mini-graph mini-graph--live" aria-label={t("sidecar.relations")}>
      <svg className="mini-graph__edges" viewBox="0 0 280 200" aria-hidden="true">
        <defs>
          <marker
            id="mini-graph-arrow"
            markerHeight="7"
            markerWidth="7"
            orient="auto"
            refX="6"
            refY="3.5"
          >
            <path d="M0,0 L7,3.5 L0,7 Z" className="mini-graph__arrow" />
          </marker>
        </defs>
        {visible.map(({ edge, direction }, i) => {
          const p = positions[i];
          const start = direction === "out" ? MINI_GRAPH_CENTER : p;
          const end = direction === "out" ? p : MINI_GRAPH_CENTER;
          const labelX = (start.x + end.x) / 2;
          const labelY = (start.y + end.y) / 2 - 4;
          return (
            <g key={`${direction}-${edge.artifact_id}-${edge.relation}-${i}`}>
              <line
                x1={start.x}
                y1={start.y}
                x2={end.x}
                y2={end.y}
                className={`mini-graph__edge mini-graph__edge--${graphRelationClass(edge.relation)}`}
                markerEnd="url(#mini-graph-arrow)"
              />
              <text x={labelX} y={labelY} className="mini-graph__edge-label">
                {direction === "out" ? "→" : "←"} {graphRelationLabel(edge.relation, lang)}
              </text>
            </g>
          );
        })}
      </svg>
      <Link
        to={`/p/${projectSlug}/wiki/${detail.slug}`}
        className={`mini-graph__node mini-graph__node--center mini-graph__node--${graphTypeClassSuffix(detail.type)}`}
        style={{ left: MINI_GRAPH_CENTER.x, top: MINI_GRAPH_CENTER.y }}
        title={detail.title}
      >
        <span className="mini-graph__node-type">{detail.type}</span>
        <span className="mini-graph__node-label">{detail.title}</span>
      </Link>
      {visible.map(({ edge }, i) => (
        <Link
          key={`${edge.artifact_id}-${edge.relation}-${i}`}
          to={`/p/${projectSlug}/wiki/${edge.slug}`}
          className={`mini-graph__node mini-graph__node--${graphTypeClassSuffix(edge.type)}`}
          style={{ left: positions[i].x, top: positions[i].y }}
          title={`${edge.title} (${edge.type})`}
        >
          <span className="mini-graph__node-type">{edge.type}</span>
          <span className="mini-graph__node-label">{edge.title}</span>
        </Link>
      ))}
      {hidden > 0 && (
        <Link to={`/p/${projectSlug}/graph`} className="mini-graph__more">
          {t("sidecar.more_relations", hidden)}
        </Link>
      )}
    </div>
  );
}

// ConnectedArtifacts renders typed edges (relates_to / related_by) as
// clickable cards. This is the "Task hub → Wiki spoke" navigation layer
// — a user viewing a Task sees every Decision / Analysis / Debug it
// points to (or is pointed to by) one click away. Supersede relationship
// still uses the dedicated field since it has product meaning beyond a
// plain edge.
function ConnectedArtifacts({
  projectSlug,
  relates,
  relatedBy,
  hasSupersedes,
  supersededBy,
}: {
  projectSlug: string;
  relates: EdgeRef[];
  relatedBy: EdgeRef[];
  hasSupersedes: boolean;
  supersededBy: string;
}) {
  const { t } = useI18n();
  const nothing = !hasSupersedes && relates.length === 0 && relatedBy.length === 0;

  if (nothing) {
    return (
      <div className="relations relations--merged">
        <div className="relations__empty">{t("sidecar.no_relations")}</div>
      </div>
    );
  }

  const rows = [
    ...relates.map((edge) => ({ edge, direction: "out" as const })),
    ...relatedBy.map((edge) => ({ edge, direction: "in" as const })),
  ];

  return (
    <div className="relations relations--merged">
      {hasSupersedes && (
        <div className="relation relation--supersedes">
          <span className="relation__dir" aria-hidden="true">
            <ArrowUpRight className="lucide" />
          </span>
          <span className="relation__label">{t("sidecar.rel_supersedes")}</span>
          <span className="relation__target">
            {supersededBy}
          </span>
        </div>
      )}
      <ul className="relation-list">
        {rows.map(({ edge, direction }) => (
          <li key={`${direction}-${edge.relation}-${edge.artifact_id}`}>
            <Link
              to={`/p/${projectSlug}/wiki/${edge.slug}`}
              className="relation-card"
              title={edge.title}
            >
              <span className="relation-card__dir" aria-hidden="true">
                {direction === "out" ? (
                  <ArrowUpRight className="lucide" />
                ) : (
                  <ArrowDownLeft className="lucide" />
                )}
              </span>
              <RelationIcon relation={edge.relation} />
              <span className="relation-card__body">
                <span className="relation-card__title">{edge.title}</span>
                <span className="relation-card__type">{edge.type}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

function RelationIcon({ relation }: { relation: string }) {
  const { lang } = useI18n();
  const entry = visualRelation(relation);
  const Icon = visualIconComponent(entry?.icon);
  const label = entry ? visualLabel(entry, lang) : relation;
  const description = entry ? visualDescription(entry, lang) : relation;
  const style = entry
    ? { "--relation-color": `var(${entry.color_token})` } as React.CSSProperties & Record<"--relation-color", string>
    : undefined;
  return (
    <span className="relation-card__relation" title={description} aria-label={label} style={style}>
      <Icon className="lucide" />
    </span>
  );
}

// ProvenanceBlock renders the epistemic + evidence data the Trust Card
// alludes to: pins list grouped by kind, source_session_ref summary, and
// stale signal. Policy fields are separated into their own collapsed
// section so the sidecar hierarchy stays scannable.
function ProvenanceBlock({
  pins,
  sourceSession,
  updatedAt,
}: {
  pins?: PinRef[];
  sourceSession?: SourceSessionRef;
  updatedAt: string;
}) {
  const { t } = useI18n();
  const hasPins = (pins?.length ?? 0) > 0;
  const hasSource = Boolean(sourceSession && (sourceSession.agent_id || sourceSession.source_session));
  const stale = staleFromAge(updatedAt);
  if (!hasPins && !hasSource && !stale) {
    return <div className="sidecar-empty-line">{t("sidecar.no_provenance")}</div>;
  }

  return (
    <div className="sidecar-stack">
      {hasPins && <PinsList pins={pins!} />}
      {hasSource && <SourceSessionLine session={sourceSession!} />}
      {stale && <StaleLine reason={stale.reason} days={stale.days} />}
    </div>
  );
}

function PolicyBlock({ meta }: { meta?: ArtifactMeta }) {
  const { t } = useI18n();
  if (!meta || Object.keys(meta).length === 0) {
    return <div className="sidecar-empty-line">{t("sidecar.no_policy")}</div>;
  }
  return (
    <div className="sidecar-stack">
      <NextContextLine meta={meta} />
      <PolicyRow enumKey="source_type" label="source" value={meta.source_type} />
      <PolicyRow enumKey="consent_state" label="consent" value={meta.consent_state} />
      <PolicyRow enumKey="confidence" label="confidence" value={meta.confidence} />
      <PolicyRow enumKey="audience" label="audience" value={meta.audience} />
      <PolicyRow enumKey="verification_state" label="verification" value={meta.verification_state} />
    </div>
  );
}

function PolicyRow({ enumKey, label, value }: { enumKey: VisualMetaEnumKey; label: string; value?: string }) {
  const { lang } = useI18n();
  if (!value) return null;
  const entry = visualMetaEnum(enumKey, value);
  const Icon = visualIconComponent(entry?.icon);
  const valueLabel = entry ? visualLabel(entry, lang) : value;
  const description = entry ? visualDescription(entry, lang) : value;
  const style = entry
    ? { "--meta-color": `var(${entry.color_token})` } as React.CSSProperties & Record<"--meta-color", string>
    : undefined;
  return (
    <div className="provenance__row">
      <span className="k">{label}</span>
      <span className="v">
        <span className="policy-enum-chip" title={description} style={style}>
          <Icon className="lucide" />
          {valueLabel}
        </span>
      </span>
    </div>
  );
}

function MetaBlock({
  avClassName,
  avInitials,
  authorID,
  authorVersion,
  areaLabel,
  status,
  completeness,
  publishedAt,
  slug,
}: {
  avClassName: string;
  avInitials: string;
  authorID: string;
  authorVersion?: string;
  areaLabel: string;
  status: string;
  completeness: string;
  publishedAt: string;
  slug: string;
}) {
  const { t } = useI18n();
  return (
    <div className="sidecar-stack">
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_author")}</span>
        <span className="v sidecar-meta-author">
          <span className={avClassName}>
            {avInitials}
          </span>
          {authorID}
          {authorVersion ? `@${authorVersion}` : ""}
        </span>
      </div>
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_area")}</span>
        <span className="v">{areaLabel}</span>
      </div>
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_status")}</span>
        <span className="v">
          {status} · {completeness}
        </span>
      </div>
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_published")}</span>
        <span className="v">{publishedAt}</span>
      </div>
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_id")}</span>
        <span className="v">{slug}</span>
      </div>
    </div>
  );
}

function PinsList({ pins }: { pins: PinRef[] }) {
  const { lang } = useI18n();
  const groups = new Map<string, PinRef[]>();
  for (const p of pins) {
    const key = p.kind || "code";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(p);
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <div style={{ fontSize: 11, color: "var(--fg-2)" }}>Pins · {pins.length}</div>
      <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 3 }}>
        {Array.from(groups.entries()).flatMap(([kind, items]) =>
          items.map((p, idx) => {
            const entry = visualPin(kind);
            const PinIcon = visualIconComponent(entry?.icon);
            const label = entry ? visualLabel(entry, lang) : kind;
            const description = entry ? visualDescription(entry, lang) : `${kind} pin`;
            const style = entry
              ? { "--pin-color": `var(${entry.color_token})` } as React.CSSProperties & Record<"--pin-color", string>
              : undefined;
            return (
              <li
                key={`${kind}-${idx}-${p.path}`}
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 11,
                  color: "var(--fg-1)",
                  display: "flex",
                  gap: 6,
                  alignItems: "baseline",
                }}
                title={description}
              >
                <span className="pin-kind-chip" style={style}>
                  <PinIcon className="lucide" />
                  {label}
                </span>
                <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {p.path}
                  {p.lines_start
                    ? `:${p.lines_start}${p.lines_end && p.lines_end !== p.lines_start ? `-${p.lines_end}` : ""}`
                    : ""}
                </span>
              </li>
            );
          }),
        )}
      </ul>
    </div>
  );
}

function SourceSessionLine({ session }: { session: SourceSessionRef }) {
  const agent = session.agent_id || session.reported_author_id;
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Session: </span>
      {agent ? (
        <span style={{ fontFamily: "var(--font-mono)" }}>{agent}</span>
      ) : (
        <span style={{ color: "var(--fg-3)" }}>ephemeral — not recorded</span>
      )}
      {session.source_session ? (
        <span
          style={{ color: "var(--fg-3)", fontFamily: "var(--font-mono)", marginLeft: 6 }}
          title={session.source_session}
        >
          · {session.source_session.slice(0, 10)}…
        </span>
      ) : null}
    </div>
  );
}

function NextContextLine({ meta }: { meta: ArtifactMeta }) {
  const { lang } = useI18n();
  const policy = meta.next_context_policy ?? "default";
  const entry = visualMetaEnum("next_context_policy", policy);
  const Icon = visualIconComponent(entry?.icon);
  const label = entry ? visualLabel(entry, lang) : policy;
  const why = entry ? visualDescription(entry, lang) : "";
  const style = entry
    ? { "--meta-color": `var(${entry.color_token})` } as React.CSSProperties & Record<"--meta-color", string>
    : undefined;
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Context: </span>
      <span className="policy-enum-chip" style={style}>
        <Icon className="lucide" />
        {label}
      </span>
      {why && <span style={{ color: "var(--fg-3)", marginLeft: 6 }}>· {why}</span>}
    </div>
  );
}

function StaleLine({ reason, days }: { reason: string; days: number }) {
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Stale signal: </span>
      <span>{reason}</span>
      <span style={{ color: "var(--fg-3)", marginLeft: 6 }}>({days}d)</span>
    </div>
  );
}

// staleFromAge mirrors the server's Phase 11c age heuristic so the
// Sidecar can surface the signal without a second HTTP round-trip.
// Threshold is 60 days — matches staleAgeThreshold in
// internal/pindoc/mcp/tools/context_for_task.go. When the server eventually
// emits a richer stale_reason enum (pin_changed, ia_migration, etc.) this
// helper becomes a renderer and the enum wins.
function staleFromAge(updatedAt: string): { reason: string; days: number } | null {
  const t = new Date(updatedAt).getTime();
  if (!Number.isFinite(t)) return null;
  const days = Math.floor((Date.now() - t) / (1000 * 60 * 60 * 24));
  if (days <= 60) return null;
  return { reason: `not updated in ${days} days`, days };
}
