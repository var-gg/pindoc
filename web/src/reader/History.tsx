import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { Bot, ChevronDown, ChevronRight } from "lucide-react";
import {
  api,
  type RevisionRow,
  type RevisionType,
  type RevisionsResp,
} from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { RevisionTypeBadge } from "./RevisionTypeBadge";

type Load =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; data: RevisionsResp };

type TimelineEntry =
  | { kind: "single"; revision: RevisionRow }
  | {
      kind: "rollup";
      key: string;
      revisions: RevisionRow[];
      revisionType: RevisionType;
      authorId: string;
      bulkOpId?: string;
    };

const revisionTypes: RevisionType[] = [
  "text_edit",
  "acceptance_toggle",
  "meta_change",
  "system_auto",
  "mixed",
];

const rollupWindowMs = 30 * 60 * 1000;

export function History() {
  const { project = "", slug = "" } = useParams<{ project: string; slug: string }>();
  const { t } = useI18n();
  const [state, setState] = useState<Load>({ kind: "loading" });
  const [enabledTypes, setEnabledTypes] = useState<Record<RevisionType, boolean>>({
    text_edit: true,
    acceptance_toggle: true,
    meta_change: true,
    system_auto: false,
    mixed: true,
  });
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await api.revisions(project, slug);
        if (!cancelled) setState({ kind: "ready", data });
      } catch (err) {
        if (!cancelled) setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [project, slug]);

  const entries = useMemo(() => {
    if (state.kind !== "ready") return [] as TimelineEntry[];
    const visible = state.data.revisions.filter((r) => enabledTypes[revisionTypeOf(r)]);
    return rollupRevisions(visible);
  }, [state, enabledTypes]);

  if (state.kind === "loading") {
    return <div className="reader-state">{t("wiki.loading")}</div>;
  }
  if (state.kind === "error") {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_title")}</strong>
        <p>{state.message}</p>
      </div>
    );
  }

  const { data } = state;
  return (
    <main className="content">
      <article className="reader-article">
        <div className="crumbs">
          <Link to={`/p/${project}/wiki/${slug}`}>{data.title}</Link>
          <ChevronRight className="lucide" />
          <span className="current">{t("history.title")}</span>
        </div>
        <h1 className="art-title">{t("history.title")}</h1>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16, flexWrap: "wrap", marginBottom: 24 }}>
          <div style={{ color: "var(--fg-3)", fontFamily: "var(--font-mono)", fontSize: 12 }}>
            {t("history.count", data.revisions.length)}
          </div>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }} aria-label={t("history.type_filters")}>
            {revisionTypes.map((rt) => (
              <button
                key={rt}
                type="button"
                className={`chip chip--${enabledTypes[rt] ? revisionTypeFilterClass(rt) : "archived"}`}
                aria-pressed={enabledTypes[rt]}
                onClick={() => setEnabledTypes((prev) => ({ ...prev, [rt]: !prev[rt] }))}
                style={{
                  cursor: "pointer",
                  opacity: enabledTypes[rt] ? 1 : 0.48,
                }}
              >
                {t(`revision_type.${rt}`)}
              </button>
            ))}
          </div>
        </div>

        <ol style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {entries.map((entry) => (
            <TimelineNode
              key={entry.kind === "single" ? `rev-${entry.revision.revision_number}` : entry.key}
              entry={entry}
              project={project}
              slug={slug}
              allRevisions={data.revisions}
              expanded={Boolean(expanded[entry.kind === "rollup" ? entry.key : ""])}
              onToggle={() => {
                if (entry.kind !== "rollup") return;
                setExpanded((prev) => ({ ...prev, [entry.key]: !prev[entry.key] }));
              }}
            />
          ))}
        </ol>
      </article>
    </main>
  );
}

function TimelineNode({
  entry,
  project,
  slug,
  allRevisions,
  expanded,
  onToggle,
}: {
  entry: TimelineEntry;
  project: string;
  slug: string;
  allRevisions: RevisionRow[];
  expanded: boolean;
  onToggle: () => void;
}) {
  const { t } = useI18n();
  if (entry.kind === "single") {
    return (
      <RevisionListItem
        revision={entry.revision}
        project={project}
        slug={slug}
        previous={previousRevision(allRevisions, entry.revision)}
      />
    );
  }

  const newest = entry.revisions[0]!;
  const oldest = entry.revisions[entry.revisions.length - 1]!;
  const av = agentAvatar(entry.authorId);
  const label = entry.bulkOpId
    ? t("history.rollup_bulk", entry.revisions.length)
    : t("history.rollup_acceptance", entry.revisions.length);
  return (
    <li style={{
      padding: "14px 0",
      borderBottom: "1px solid var(--border)",
    }}>
      <div style={{
        display: "grid",
        gridTemplateColumns: "68px 1fr auto",
        gap: 12,
        alignItems: "center",
      }}>
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--fg-3)" }}>
          rev {newest.revision_number}-{oldest.revision_number}
        </span>
        <div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", color: "var(--fg-0)", marginBottom: 5 }}>
            <RevisionTypeBadge revisionType={entry.revisionType} compact />
            <span>{label}</span>
            {entry.bulkOpId && <span className="chip chip--area">bulk:{entry.bulkOpId.slice(0, 8)}</span>}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--fg-3)" }}>
            <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>{av.initials}</span>
            <span>{entry.authorId}</span>
            <span>·</span>
            <span>{formatTimeRange(oldest.created_at, newest.created_at)}</span>
          </div>
        </div>
        <button type="button" className="chip" onClick={onToggle} style={{ cursor: "pointer" }}>
          {expanded ? <ChevronDown className="lucide" style={{ width: 12, height: 12 }} /> : <ChevronRight className="lucide" style={{ width: 12, height: 12 }} />}
          {expanded ? t("history.collapse") : t("history.expand")}
        </button>
      </div>
      {expanded && (
        <ol style={{ listStyle: "none", margin: "10px 0 0 68px", padding: 0, borderLeft: "1px solid var(--border)" }}>
          {entry.revisions.map((r) => (
            <RevisionListItem
              key={r.revision_number}
              revision={r}
              project={project}
              slug={slug}
              previous={previousRevision(allRevisions, r)}
              nested
            />
          ))}
        </ol>
      )}
    </li>
  );
}

function RevisionListItem({
  revision,
  project,
  slug,
  previous,
  nested = false,
}: {
  revision: RevisionRow;
  project: string;
  slug: string;
  previous?: RevisionRow;
  nested?: boolean;
}) {
  const { t } = useI18n();
  const av = agentAvatar(revision.author_id);
  const diffHref = previous
    ? `/p/${project}/wiki/${slug}/diff?from=${previous.revision_number}&to=${revision.revision_number}`
    : null;
  const revisionType = revisionTypeOf(revision);
  const system = revisionType === "system_auto";
  return (
    <li style={{
      display: "grid",
      gridTemplateColumns: nested ? "58px 1fr auto" : "68px 1fr auto",
      gap: 12,
      alignItems: "baseline",
      padding: nested ? "10px 0 10px 12px" : "16px 0",
      borderBottom: nested ? "0" : "1px solid var(--border)",
      opacity: system ? 0.58 : 1,
      filter: system ? "grayscale(0.25)" : undefined,
    }}>
      <span style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--fg-3)" }}>
        rev {revision.revision_number}
      </span>
      <div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, color: system ? "var(--fg-2)" : "var(--fg-0)", marginBottom: 4 }}>
          {system && <Bot className="lucide" style={{ width: 13, height: 13 }} />}
          <span>{revision.commit_msg || t("history.no_commit_msg")}</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--fg-3)", flexWrap: "wrap" }}>
          <RevisionTypeBadge revisionType={revisionType} compact />
          <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>{av.initials}</span>
          <span>{revision.author_id}{revision.author_version ? `@${revision.author_version}` : ""}</span>
          {revision.bulk_op_id && <span className="chip chip--area">bulk:{revision.bulk_op_id.slice(0, 8)}</span>}
          <span>·</span>
          <span>{new Date(revision.created_at).toLocaleString()}</span>
        </div>
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        {diffHref && (
          <Link to={diffHref} className="chip" style={{ textDecoration: "none" }}>
            {t("history.diff_prev")}
          </Link>
        )}
      </div>
    </li>
  );
}

function rollupRevisions(revisions: RevisionRow[]): TimelineEntry[] {
  const out: TimelineEntry[] = [];
  for (let i = 0; i < revisions.length;) {
    const current = revisions[i]!;
    const bulkGroup = collectBulkGroup(revisions, i);
    if (bulkGroup.length > 1) {
      out.push({
        kind: "rollup",
        key: `bulk-${current.bulk_op_id}-${current.revision_number}`,
        revisions: bulkGroup,
        revisionType: revisionTypeOf(current),
        authorId: current.author_id,
        bulkOpId: current.bulk_op_id,
      });
      i += bulkGroup.length;
      continue;
    }

    const acceptanceGroup = collectAcceptanceGroup(revisions, i);
    if (acceptanceGroup.length > 1) {
      out.push({
        kind: "rollup",
        key: `acceptance-${current.revision_number}-${acceptanceGroup[acceptanceGroup.length - 1]!.revision_number}`,
        revisions: acceptanceGroup,
        revisionType: "acceptance_toggle",
        authorId: current.author_id,
      });
      i += acceptanceGroup.length;
      continue;
    }

    out.push({ kind: "single", revision: current });
    i++;
  }
  return out;
}

function collectBulkGroup(revisions: RevisionRow[], start: number): RevisionRow[] {
  const first = revisions[start]!;
  if (!first.bulk_op_id) return [first];
  const group = [first];
  for (let i = start + 1; i < revisions.length; i++) {
    const next = revisions[i]!;
    if (next.bulk_op_id !== first.bulk_op_id) break;
    group.push(next);
  }
  return group;
}

function collectAcceptanceGroup(revisions: RevisionRow[], start: number): RevisionRow[] {
  const first = revisions[start]!;
  if (revisionTypeOf(first) !== "acceptance_toggle") return [first];
  const group = [first];
  for (let i = start + 1; i < revisions.length; i++) {
    const next = revisions[i]!;
    const prev = group[group.length - 1]!;
    if (revisionTypeOf(next) !== "acceptance_toggle") break;
    if (next.author_id !== first.author_id) break;
    if (Math.abs(new Date(prev.created_at).getTime() - new Date(next.created_at).getTime()) > rollupWindowMs) break;
    group.push(next);
  }
  return group;
}

function revisionTypeOf(revision: RevisionRow): RevisionType {
  return revision.revision_type ?? "text_edit";
}

function previousRevision(revisions: RevisionRow[], revision: RevisionRow): RevisionRow | undefined {
  return revisions.find((candidate) => candidate.revision_number < revision.revision_number);
}

function revisionTypeFilterClass(revisionType: RevisionType): string {
  switch (revisionType) {
    case "text_edit":
      return "live";
    case "acceptance_toggle":
      return "draft";
    case "meta_change":
      return "area";
    case "system_auto":
      return "archived";
    case "mixed":
      return "stale";
  }
}

function formatTimeRange(start: string, end: string): string {
  const a = new Date(start);
  const b = new Date(end);
  const minutes = Math.max(0, Math.round((b.getTime() - a.getTime()) / 60000));
  if (minutes <= 0) return b.toLocaleString();
  return `${a.toLocaleTimeString()}-${b.toLocaleTimeString()} · ${minutes}m`;
}
