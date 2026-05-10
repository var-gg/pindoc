import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router";
import {
  ArrowDownLeft,
  ArrowUpRight,
  ChevronDown,
  ChevronRight,
  History as HistoryIcon,
} from "lucide-react";
import { GitBlobPreview, GitDiff, PinKindBadge, formatShortSha } from "../git/GitPreview";
import { gitCommitPath } from "../git/routes";
import {
  api,
  type Artifact,
  type ArtifactMeta,
  type EdgeRef,
  type PinRef,
  type RevisionRow,
  type SourceSessionRef,
} from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { authorAvatarKey, authorDisplayLabel } from "./authorDisplay";
import { localizedAreaName } from "./areaLocale";
import { Toc } from "./Toc";
import { headingsFromBody } from "./slug";
import { isStructureOverlapHeading, structureOverlapSectionsFromBody } from "./structureSections";
import { typeChipClass } from "./typeChip";
import { RevisionTypeBadge } from "./RevisionTypeBadge";
import { BadgeWithExplain } from "./BadgeWithExplain";
import { Tooltip } from "./Tooltip";
import {
  visualDescription,
  visualLabel,
  visualMetaEnum,
  visualPin,
  visualQuickAction,
  visualRelation,
  visualType,
  type VisualMetaEnumKey,
} from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";
import { splitEvidenceEdges } from "./sidecarEvidence";
import { taskAssigneeLabel } from "./assigneeDisplay";
import {
  MINI_GRAPH_CENTER,
  graphRadialPositions,
  graphRelationClass,
  graphRelationLabel,
  graphTypeClassSuffix,
  type StartFocusReason,
} from "./graphSvg";
import { isGeneratedAgentId } from "./readerInternalVisibility";
import { projectSurfacePath } from "../readerRoutes";
import { formatDate, formatDateTime } from "../utils/formatDateTime";

type Props = {
  projectSlug: string;
  orgSlug: string;
  detail: Artifact | null;
  emptyMessage?: string;
  // focusReason explains why the graph surface picked this artifact as
  // the current focus. Rendered as a chip just below IdentityStrip
  // when present (graph mode only — Wiki/Tasks/Today leave it null).
  focusReason?: StartFocusReason | null;
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
  orgSlug,
  detail,
  emptyMessage,
  focusReason,
}: Props) {
  const { t, lang } = useI18n();
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

  const av = agentAvatar(authorAvatarKey(detail));
  const authorLabel = authorDisplayLabel(detail, t("reader.byline_unknown"));
  const publishedAt = detail.published_at
    ? formatDateTime(detail.published_at, lang)
    : "—";
  const areaLabel = localizedAreaName(t, detail.area_slug, detail.area_slug);
  const artifactHref = projectSurfacePath(projectSlug, "wiki", detail.slug, orgSlug);

  // Supersede is still a dedicated head field; typed artifact_edges come
  // through relates_to / related_by and are split below by relation role.
  const hasSupersedes = Boolean(detail.superseded_by && detail.superseded_by !== "");
  const {
    regularRelates,
    regularRelatedBy,
    evidenceRelates,
    evidenceRelatedBy,
  } = splitEvidenceEdges(detail.relates_to ?? [], detail.related_by ?? []);
  const hasEvidence = evidenceRelates.length > 0 || evidenceRelatedBy.length > 0;

  return (
    <aside id="sidecar-live-data" className="sidecar sidecar--detail">
      <IdentityStrip detail={detail} />
      {focusReason && <FocusReasonChip reason={focusReason} />}

      <QuickActions detail={detail} artifactHref={artifactHref} />

      {headings.length >= 2 && <Toc headings={headings} />}

      {detail.type === "Task" && <TaskInspectorSummary detail={detail} areaLabel={areaLabel} />}

      <div className="graph-wrap">
        <MiniGraph detail={detail} projectSlug={projectSlug} orgSlug={orgSlug} />
      </div>

      <SidecarStaticSection heading={t("sidecar.relations")}>
        {bodyOverlapSections.length > 0 && (
          <BodyOverlapLink count={bodyOverlapSections.length} />
        )}
          <ConnectedArtifacts
            projectSlug={projectSlug}
            orgSlug={orgSlug}
            relates={regularRelates}
          relatedBy={regularRelatedBy}
          hasSupersedes={hasSupersedes}
          supersededBy={detail.superseded_by ?? ""}
        />
      </SidecarStaticSection>

      {hasEvidence && (
        <SidecarStaticSection heading={t("sidecar.evidence")}>
          <ConnectedArtifacts
            projectSlug={projectSlug}
            orgSlug={orgSlug}
            relates={evidenceRelates}
            relatedBy={evidenceRelatedBy}
            hasSupersedes={false}
            supersededBy=""
          />
        </SidecarStaticSection>
      )}

      <SidecarStaticSection heading={t("sidecar.references")}>
        <PinReferencesPanel projectSlug={projectSlug} pins={detail.pins ?? []} />
      </SidecarStaticSection>

      <SidecarCollapsibleSection
        id="provenance"
        heading={t("sidecar.provenance")}
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
        heading={t("sidecar.policy")}
        collapsed={collapsed.policy}
        onToggle={toggleSection}
      >
        <PolicyBlock meta={detail.artifact_meta} />
      </SidecarCollapsibleSection>

      <SidecarCollapsibleSection
        id="timeline"
        heading={t("sidecar.timeline")}
        collapsed={collapsed.timeline}
        onToggle={toggleSection}
      >
        <RecentChanges projectSlug={projectSlug} orgSlug={orgSlug} slug={detail.slug} />
      </SidecarCollapsibleSection>

      <SidecarCollapsibleSection
        id="meta"
        heading={t("sidecar.meta")}
        collapsed={collapsed.meta}
        onToggle={toggleSection}
      >
        <MetaBlock
          avClassName={av.className}
          avInitials={av.initials}
          authorLabel={authorLabel}
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

function FocusReasonChip({ reason }: { reason: StartFocusReason }) {
  const { t, lang } = useI18n();
  let label: string;
  switch (reason.kind) {
    case "last_focused":
      label = t("graph.focus_reason_last_focused");
      break;
    case "recent_meaningful":
      label = `${t("graph.focus_reason_recent_meaningful")} · ${formatDate(reason.updated_at, lang)}`;
      break;
    case "most_connected":
      label = t("graph.focus_reason_most_connected", reason.degree);
      break;
    case "fallback":
      label = t("graph.focus_reason_fallback");
      break;
  }
  return (
    <div className="sidecar-focus-reason" aria-label={t("graph.focus_reason_label")}>
      <span className="sidecar-focus-reason__key">{t("graph.focus_reason_label")}</span>
      <span className="sidecar-focus-reason__value">{label}</span>
    </div>
  );
}

function IdentityStrip({ detail }: { detail: Artifact }) {
  const { t, lang } = useI18n();
  const typeVisual = visualType(detail.type);
  const typeLabel = typeVisual ? visualLabel(typeVisual, lang) : detail.type;
  const title = detail.title.trim();
  const hasTitle = title.length > 0;
  const displayTitle = hasTitle ? title : detail.slug;
  const lifecycle = detail.type === "Task" && detail.task_meta?.status
    ? taskStatusLabel(detail.task_meta.status, t)
    : `${artifactStatusLabel(detail.status, t)} · ${artifactCompletenessLabel(detail.completeness, t)}`;
  return (
    <div className="sidecar-identity">
      <span className={typeChipClass(detail.type)}>{typeLabel}</span>
      <Tooltip content={displayTitle}>
        <span className={`sidecar-identity__title${hasTitle ? "" : " sidecar-identity__title--fallback"}`}>
          {displayTitle}
        </span>
      </Tooltip>
      {hasTitle && (
        <Tooltip content={detail.slug}>
          <span className="sidecar-identity__slug">
            <span className="sidecar-identity__slug-key">{t("sidecar.prov_id")}</span>
            <span className="sidecar-identity__slug-value">{detail.slug}</span>
          </span>
        </Tooltip>
      )}
      <span className="sidecar-identity__status">{lifecycle}</span>
    </div>
  );
}

function artifactStatusLabel(value: string, t: (key: string, ...args: Array<string | number>) => string): string {
  return enumLabel(`artifact.status.${value}`, value, t);
}

function artifactCompletenessLabel(value: string, t: (key: string, ...args: Array<string | number>) => string): string {
  return enumLabel(`artifact.completeness.${value}`, value, t);
}

function taskStatusLabel(value: string, t: (key: string, ...args: Array<string | number>) => string): string {
  switch (value) {
    case "open":
      return t("tasks.col_open");
    case "claimed_done":
      return t("tasks.col_claimed_done");
    case "blocked":
      return t("tasks.col_blocked");
    case "cancelled":
      return t("tasks.col_cancelled");
    case "missing_status":
      return t("tasks.col_no_status");
    default:
      return value;
  }
}

function enumLabel(key: string, value: string, t: (key: string, ...args: Array<string | number>) => string): string {
  const label = t(key);
  return label === key ? value : label;
}

function QuickActions({ detail, artifactHref }: { detail: Artifact; artifactHref: string }) {
  const { t, lang } = useI18n();
  const agentRef = `pindoc://${detail.slug}`;
  const absoluteHref = typeof window === "undefined"
    ? artifactHref
    : `${window.location.origin}${artifactHref}`;
  const [toast, setToast] = useState<string | null>(null);
  const toastTimerRef = useRef<number | null>(null);
  useEffect(() => {
    return () => {
      if (toastTimerRef.current !== null) window.clearTimeout(toastTimerRef.current);
    };
  }, []);
  const flashToast = (msg: string) => {
    setToast(msg);
    if (toastTimerRef.current !== null) window.clearTimeout(toastTimerRef.current);
    toastTimerRef.current = window.setTimeout(() => setToast(null), 1800);
  };
  const copyWithToast = async (text: string, successLabel: string) => {
    if (typeof navigator === "undefined" || !navigator.clipboard) {
      flashToast(t("sidecar.copy_unsupported"));
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      flashToast(t("sidecar.copied_label", successLabel));
    } catch {
      flashToast(t("sidecar.copy_failed"));
    }
  };
  const actions: Array<
    | { id: QuickActionID; kind: "button"; onClick: () => void }
    | { id: QuickActionID; kind: "link"; to: string }
  > = [
    { id: "verify_request", kind: "button", onClick: () => copyWithToast(`Verify ${agentRef}`, visualLabel(visualQuickAction("verify_request"), lang)) },
    { id: "update_request", kind: "button", onClick: () => copyWithToast(`Update ${agentRef}`, visualLabel(visualQuickAction("update_request"), lang)) },
    { id: "copy_markdown", kind: "button", onClick: () => copyWithToast(detail.body_markdown, visualLabel(visualQuickAction("copy_markdown"), lang)) },
    { id: "copy_link", kind: "button", onClick: () => copyWithToast(absoluteHref, visualLabel(visualQuickAction("copy_link"), lang)) },
    { id: "copy_agent_ref", kind: "button", onClick: () => copyWithToast(agentRef, visualLabel(visualQuickAction("copy_agent_ref"), lang)) },
    { id: "history", kind: "link", to: `${artifactHref}/history` },
  ];
  return (
    <div className="sidecar-actions-wrap">
      <div className="sidecar-actions" aria-label={t("sidecar.quick_actions")}>
        {actions.map((action) => {
          const entry = visualQuickAction(action.id);
          const Icon = visualIconComponent(entry.icon);
          const label = visualLabel(entry, lang);
          const description = visualDescription(entry, lang);
          if (action.kind === "link") {
            return (
              <Tooltip key={action.id} content={description}>
                <Link
                  className="sidecar-action"
                  aria-label={label}
                  to={action.to}
                >
                  <Icon className="lucide" />
                </Link>
              </Tooltip>
            );
          }
          return (
            <Tooltip key={action.id} content={description}>
              <button
                type="button"
                className="sidecar-action"
                aria-label={label}
                onClick={action.onClick}
              >
                <Icon className="lucide" />
              </button>
            </Tooltip>
          );
        })}
      </div>
      <div
        className={`sidecar-toast${toast ? " is-visible" : ""}`}
        role="status"
        aria-live="polite"
      >
        {toast ?? ""}
      </div>
    </div>
  );
}

function TaskInspectorSummary({ detail, areaLabel }: { detail: Artifact; areaLabel: string }) {
  const { t, lang } = useI18n();
  const stats = acceptanceStats(detail.body_markdown);
  const percent = stats.total > 0 ? Math.round((stats.resolved / stats.total) * 100) : 0;
  const edges = (detail.relates_to?.length ?? 0) + (detail.related_by?.length ?? 0);
  const updatedAt = detail.updated_at ? formatDateTime(detail.updated_at, lang) : "—";
  const author = authorDisplayLabel(detail, t("reader.byline_unknown"));
  const status = detail.task_meta?.status ?? "missing_status";
  const priority = taskPriority(detail.task_meta?.priority);
  const assignee = taskAssigneeDisplay(detail.task_meta?.assignee, t);
  const assigneeAvatar = assignee.avatarKey ? agentAvatar(assignee.avatarKey) : null;
  return (
    <div className="task-inspector-summary">
      <div className="task-inspector-summary__head">
        <span>{t("task_inspector.heading")}</span>
      </div>
      <div className="task-inspector-summary__grid">
        <span>{t("task_inspector.status")}</span>
        <strong className="task-inspector-summary__value">
          <span className={`task-state-chip task-state-chip--${taskStateTone(status)}`}>
            {taskStatusLabel(status, t)}
          </span>
        </strong>
        <span>{t("task_inspector.priority")}</span>
        <strong className="task-inspector-summary__value">
          {priority ? (
            <span className={`prio prio--${priority}`}>
              <span className="dot" />
              {priority}
            </span>
          ) : "—"}
        </strong>
        <span>{t("task_inspector.assignee")}</span>
        <strong className="task-inspector-summary__value task-inspector-assignee">
          {assigneeAvatar && (
            <span className={assigneeAvatar.className}>
              {assigneeAvatar.initials}
            </span>
          )}
          <span>{assignee.label}</span>
        </strong>
        <span>{t("task_inspector.area")}</span>
        <strong>{areaLabel}</strong>
        <span>{t("task_inspector.due_at")}</span>
        <strong>{detail.task_meta?.due_at ? formatDateTime(detail.task_meta.due_at, lang) : "—"}</strong>
        <span>{t("task_inspector.tags")}</span>
        <strong>{detail.tags.length > 0 ? detail.tags.join(", ") : "—"}</strong>
        <span>{t("task_inspector.edges")}</span>
        <strong>{edges}</strong>
        <span>{t("task_inspector.last_revision")}</span>
        <strong>{updatedAt} · {author}</strong>
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

function taskPriority(value: string | undefined): "p0" | "p1" | "p2" | "p3" | "" {
  const normalized = value?.trim().toLowerCase();
  return normalized === "p0" || normalized === "p1" || normalized === "p2" || normalized === "p3"
    ? normalized
    : "";
}

function taskStateTone(value: string): "open" | "claimed-done" | "blocked" | "cancelled" | "missing" {
  switch (value) {
    case "open":
      return "open";
    case "claimed_done":
      return "claimed-done";
    case "blocked":
      return "blocked";
    case "cancelled":
      return "cancelled";
    default:
      return "missing";
  }
}

function taskAssigneeDisplay(
  value: string | undefined,
  t: (key: string, ...args: Array<string | number>) => string,
): { label: string; avatarKey: string } {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) return { label: t("tasks.assignee_unassigned"), avatarKey: "" };
  if (trimmed.startsWith("agent:")) {
    const rawAgent = trimmed.slice("agent:".length);
    if (isGeneratedAgentId(rawAgent)) {
      return { label: t("sidebar.agent_generated"), avatarKey: "system" };
    }
    return { label: trimmed, avatarKey: rawAgent };
  }
  if (trimmed.startsWith("@")) return { label: trimmed, avatarKey: trimmed.slice(1) };
  return { label: taskAssigneeLabel(trimmed, t), avatarKey: trimmed };
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


function SidecarStaticSection({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <section className="sidecar-section">
      <div className="sidecar-section__head sidecar-section__head--static">
        <span>{heading}</span>
      </div>
      <div className="sidecar-section__body">{children}</div>
    </section>
  );
}

function SidecarCollapsibleSection({
  id,
  heading,
  collapsed,
  onToggle,
  children,
}: {
  id: CollapsibleKey;
  heading: string;
  collapsed: boolean;
  onToggle: (key: CollapsibleKey) => void;
  children: ReactNode;
}) {
  const { t } = useI18n();
  return (
    <section className="sidecar-section">
      <Tooltip content={collapsed ? t("sidecar.expand_section") : t("sidecar.collapse_section")}>
        <button
          type="button"
          className="sidecar-section__head"
          aria-expanded={!collapsed}
          onClick={() => onToggle(id)}
        >
          {collapsed ? <ChevronRight className="lucide" /> : <ChevronDown className="lucide" />}
          <span>{heading}</span>
        </button>
      </Tooltip>
      {!collapsed && <div className="sidecar-section__body">{children}</div>}
    </section>
  );
}

function RecentChanges({ projectSlug, orgSlug, slug }: { projectSlug: string; orgSlug: string; slug: string }) {
  const { t, lang } = useI18n();
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
          to={`${projectSurfacePath(projectSlug, "wiki", slug, orgSlug)}/history`}
          className="sidecar-timeline-head__link"
        >
          <HistoryIcon className="lucide" style={{ width: 11, height: 11 }} />
          {shown.length < revs.length ? t("history.count", revs.length) : ""}
        </Link>
      </div>
      {shown.map((r) => {
        const av = agentAvatar(authorAvatarKey(r));
        return (
          <div
            key={r.revision_number}
            style={{ display: "flex", alignItems: "center", gap: 6, padding: "4px 0", fontSize: 12, color: "var(--fg-1)", minWidth: 0 }}
          >
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)", flexShrink: 0 }}>
              rev {r.revision_number}
            </span>
            <RevisionTypeBadge revisionType={r.revision_type} compact />
            <span className={av.className} style={{ width: 12, height: 12, fontSize: 7, flexShrink: 0 }}>
              {av.initials}
            </span>
            <span style={{ color: "var(--fg-2)", fontSize: 11.5, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", flex: 1, minWidth: 0 }}>
              {r.commit_msg || t("history.no_commit_msg")}
            </span>
            <time
              dateTime={r.created_at}
              title={formatDateTime(r.created_at, lang)}
              style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-4)", flexShrink: 0 }}
            >
              {formatDate(r.created_at, lang)}
            </time>
          </div>
        );
      })}
      {remainder > 0 && (
        <Link
          to={`${projectSurfacePath(projectSlug, "wiki", slug, orgSlug)}/history`}
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
  orgSlug,
}: {
  detail: Artifact;
  projectSlug: string;
  orgSlug: string;
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
      <Tooltip content={detail.title}>
        <Link
          to={projectSurfacePath(projectSlug, "wiki", detail.slug, orgSlug)}
          className={`mini-graph__node mini-graph__node--center mini-graph__node--${graphTypeClassSuffix(detail.type)}`}
          style={{ left: MINI_GRAPH_CENTER.x, top: MINI_GRAPH_CENTER.y }}
        >
          <span className="mini-graph__node-type">{detail.type}</span>
          <span className="mini-graph__node-label">{detail.title}</span>
        </Link>
      </Tooltip>
      {visible.map(({ edge }, i) => (
        <Tooltip key={`${edge.artifact_id}-${edge.relation}-${i}`} content={`${edge.title} (${edge.type})`}>
          <Link
            to={projectSurfacePath(projectSlug, "wiki", edge.slug, orgSlug)}
            className={`mini-graph__node mini-graph__node--${graphTypeClassSuffix(edge.type)}`}
            style={{ left: positions[i].x, top: positions[i].y }}
          >
            <span className="mini-graph__node-type">{edge.type}</span>
            <span className="mini-graph__node-label">{edge.title}</span>
          </Link>
        </Tooltip>
      ))}
      {hidden > 0 && (
        <Link to={projectSurfacePath(projectSlug, "graph", undefined, orgSlug)} className="mini-graph__more">
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
  orgSlug,
  relates,
  relatedBy,
  hasSupersedes,
  supersededBy,
}: {
  projectSlug: string;
  orgSlug: string;
  relates: EdgeRef[];
  relatedBy: EdgeRef[];
  hasSupersedes: boolean;
  supersededBy: string;
}) {
  const { t, lang } = useI18n();
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
        {rows.map(({ edge, direction }) => {
          const relEntry = visualRelation(edge.relation);
          const relLabel = relEntry ? visualLabel(relEntry, lang) : edge.relation;
          const relDesc = relEntry ? visualDescription(relEntry, lang) : "";
          const arrow = direction === "out" ? "→" : "←";
          const tip = relDesc
            ? `${arrow} ${relLabel} · ${edge.title}\n${relDesc}`
            : `${arrow} ${relLabel} · ${edge.title}`;
          return (
            <li key={`${direction}-${edge.relation}-${edge.artifact_id}`}>
              <Tooltip content={tip}>
                <Link
                  to={projectSurfacePath(projectSlug, "wiki", edge.slug, orgSlug)}
                  className="relation-card"
                  aria-label={`${arrow} ${relLabel}: ${edge.title}`}
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
              </Tooltip>
            </li>
          );
        })}
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
    <BadgeWithExplain label={label} description={description} className="relation-card__relation" style={style}>
      <Icon className="lucide" />
    </BadgeWithExplain>
  );
}

type PinGroup = {
  key: string;
  label: string;
  pins: PinRef[];
};

function PinReferencesPanel({
  projectSlug,
  pins,
}: {
  projectSlug: string;
  pins: PinRef[];
}) {
  const { t, lang } = useI18n();
  const visiblePins = pins.filter((pin) => Boolean(pin.path));
  const [activeKey, setActiveKey] = useState("");
  const [showDiff, setShowDiff] = useState(false);
  const groups = useMemo<PinGroup[]>(() => {
    const byCommit = new Map<string, PinRef[]>();
    for (const pin of visiblePins) {
      const key = pin.commit_sha || pin.path || pin.kind;
      if (!byCommit.has(key)) byCommit.set(key, []);
      byCommit.get(key)!.push(pin);
    }
    return Array.from(byCommit.entries()).map(([key, items]) => ({
      key,
      label: items[0]?.commit_sha ? formatShortSha(items[0].commit_sha) : t("sidecar.references_external"),
      pins: items,
    }));
  }, [visiblePins, t]);
  const activePin = useMemo(() => {
    if (visiblePins.length === 0) return undefined;
    return visiblePins.find((pin) => pinKey(pin) === activeKey) ?? visiblePins[0];
  }, [visiblePins, activeKey]);

  useEffect(() => {
    if (!activePin) {
      setActiveKey("");
      return;
    }
    const key = pinKey(activePin);
    if (key !== activeKey) setActiveKey(key);
  }, [activePin, activeKey]);

  useEffect(() => {
    setShowDiff(false);
  }, [activeKey]);

  if (visiblePins.length === 0) {
    return <div className="sidecar-empty-line">{t("sidecar.no_references")}</div>;
  }

  const canOpenCommit = Boolean(activePin?.repo_id && activePin?.commit_sha);
  const canDiff = Boolean(
    activePin?.repo_id
      && activePin.commit_sha
      && activePin.path
      && activePin.kind !== "url"
      && activePin.kind !== "resource"
      && activePin.kind !== "asset",
  );

  return (
    <div className="pin-references">
      <div className="pin-references__groups">
        {groups.map((group) => (
          <section key={group.key} className="pin-reference-group">
            <div className="pin-reference-group__head">
              <span>{group.label}</span>
              <span>{group.pins.length}</span>
            </div>
            <div className="pin-reference-group__items">
              {group.pins.map((pin) => {
                const key = pinKey(pin);
                return (
                  <button
                    key={key}
                    type="button"
                    className={`pin-reference-item${key === pinKey(activePin) ? " is-active" : ""}`}
                    onClick={() => setActiveKey(key)}
                  >
                    <PinKindBadge pin={pin} />
                    <span className="pin-reference-item__body">
                      <span className="pin-reference-item__path">{pin.path}</span>
                      <span className="pin-reference-item__meta">
                        {pinLabelForSidecar(pin, lang)}
                        {pin.lines_start
                          ? ` · ${pin.lines_start}${pin.lines_end && pin.lines_end !== pin.lines_start ? `-${pin.lines_end}` : ""}`
                          : ""}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          </section>
        ))}
      </div>

      {activePin && (
        <div className="pin-reference-preview">
          <div className="pin-reference-preview__head">
            <span className="pin-reference-preview__title">
              {activePin.path}
            </span>
            <span className="pin-reference-preview__actions">
              {canDiff && (
                <button type="button" onClick={() => setShowDiff((v) => !v)}>
                  {showDiff ? t("git.preview") : t("git.diff")}
                </button>
              )}
              {canOpenCommit && activePin.repo_id && activePin.commit_sha && (
                <Link to={gitCommitPath(projectSlug, activePin.repo_id, activePin.commit_sha)}>
                  {t("sidecar.open_full_reference")}
                </Link>
              )}
            </span>
          </div>
          {showDiff && canDiff && activePin.repo_id && activePin.commit_sha ? (
            <GitDiff
              project={projectSlug}
              repoID={activePin.repo_id}
              commit={activePin.commit_sha}
              path={activePin.path}
            />
          ) : (
            <GitBlobPreview project={projectSlug} pin={activePin} />
          )}
        </div>
      )}
    </div>
  );
}

function pinKey(pin: PinRef | undefined): string {
  if (!pin) return "";
  return [
    pin.kind,
    pin.repo_id ?? "",
    pin.commit_sha ?? "",
    pin.path,
    pin.lines_start ?? "",
    pin.lines_end ?? "",
  ].join("|");
}

function pinLabelForSidecar(pin: PinRef, locale: string): string {
  const entry = visualPin(pin.kind);
  const kind = entry ? visualLabel(entry, locale) : pin.kind;
  const commit = pin.commit_sha ? formatShortSha(pin.commit_sha) : "";
  return commit ? `${kind} · ${commit}` : kind;
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
        <BadgeWithExplain label={valueLabel} description={description} className="policy-enum-chip" style={style}>
          <Icon className="lucide" />
          {valueLabel}
        </BadgeWithExplain>
      </span>
    </div>
  );
}

function MetaBlock({
  avClassName,
  avInitials,
  authorLabel,
  areaLabel,
  status,
  completeness,
  publishedAt,
  slug,
}: {
  avClassName: string;
  avInitials: string;
  authorLabel: string;
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
          {authorLabel}
        </span>
      </div>
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_area")}</span>
        <span className="v">{areaLabel}</span>
      </div>
      <div className="provenance__row">
        <span className="k">{t("sidecar.prov_status")}</span>
        <span className="v">
          {artifactStatusLabel(status, t)} · {artifactCompletenessLabel(completeness, t)}
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
              >
                <BadgeWithExplain label={label} description={description} className="pin-kind-chip" style={style}>
                  <PinIcon className="lucide" />
                  {label}
                </BadgeWithExplain>
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
  const reported = session.reported_author_id && !isGeneratedAgentId(session.reported_author_id)
    ? session.reported_author_id
    : "";
  const generatedHidden = session.agent_id && isGeneratedAgentId(session.agent_id);
  const agent = reported || (generatedHidden ? "" : session.agent_id);
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Session: </span>
      {agent ? (
        <span style={{ fontFamily: "var(--font-mono)" }}>{agent}</span>
      ) : (
        <span style={{ color: "var(--fg-3)" }}>local session</span>
      )}
      {session.source_session ? (
        <Tooltip content={session.source_session}>
          <span
            style={{ color: "var(--fg-3)", fontFamily: "var(--font-mono)", marginLeft: 6 }}
          >
            · {session.source_session.slice(0, 10)}…
          </span>
        </Tooltip>
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
    <div className="provenance__row">
      <span className="k">context</span>
      <span className="v">
        <BadgeWithExplain label={label} description={why} className="policy-enum-chip" style={style}>
          <Icon className="lucide" />
          {label}
        </BadgeWithExplain>
      </span>
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
