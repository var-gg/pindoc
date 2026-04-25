import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { ChevronRight } from "lucide-react";
import { api, type AcceptanceChecklist, type DiffResp, type MetaDeltaEntry } from "../api/client";
import { useI18n } from "../i18n";
import { RevisionTypeBadge } from "./RevisionTypeBadge";

type Load =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; data: DiffResp };

export function Diff() {
  const { project = "", slug = "" } = useParams<{ project: string; slug: string }>();
  const [search] = useSearchParams();
  const fromRev = Number(search.get("from")) || undefined;
  const toRev = Number(search.get("to")) || undefined;
  const { t } = useI18n();
  const [state, setState] = useState<Load>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await api.diff(project, slug, fromRev, toRev);
        if (!cancelled) setState({ kind: "ready", data });
      } catch (err) {
        if (!cancelled) setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [project, slug, fromRev, toRev]);

  if (state.kind === "loading") return <div className="reader-state">{t("wiki.loading")}</div>;
  if (state.kind === "error") {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_title")}</strong>
        <p>{state.message}</p>
      </div>
    );
  }
  const { data } = state;
  const metaDelta = data.meta_delta ?? [];
  const revisionType = data.revision_type ?? data.to.revision_type;
  const acceptanceChecklist = data.acceptance_checklist;
  const showAcceptanceChecklist = Boolean(
    acceptanceChecklist?.has_change &&
    acceptanceChecklist.items.length > 0 &&
    (revisionType === "acceptance_toggle" || revisionType === "mixed"),
  );

  return (
    <main className="content">
      <article className="reader-article" style={{ maxWidth: 980 }}>
        <div className="crumbs">
          <Link to={`/p/${project}/wiki/${slug}`}>{data.to.title}</Link>
          <ChevronRight className="lucide" />
          <Link to={`/p/${project}/wiki/${slug}/history`}>{t("history.title")}</Link>
          <ChevronRight className="lucide" />
          <span className="current">
            rev {data.from.revision_number} → rev {data.to.revision_number}
          </span>
        </div>
        <h1 className="art-title">{t("diff.title", data.from.revision_number, data.to.revision_number)}</h1>

        <div style={{ display: "flex", gap: 16, flexWrap: "wrap", color: "var(--fg-3)", fontFamily: "var(--font-mono)", fontSize: 12, marginBottom: 32, paddingBottom: 20, borderBottom: "1px solid var(--border)" }}>
          <span>{data.from.author_id} → {data.to.author_id}</span>
          <span>·</span>
          <RevisionTypeBadge revisionType={revisionType} />
          {revisionType && <span>·</span>}
          <span style={{ color: "var(--live)" }}>+{data.stats.lines_added}</span>
          <span style={{ color: "var(--stale)" }}>−{data.stats.lines_removed}</span>
          <span>lines</span>
          {data.to.commit_msg && (
            <>
              <span>·</span>
              <span style={{ color: "var(--fg-1)" }}>{data.to.commit_msg}</span>
            </>
          )}
        </div>

        <h2 style={{ fontSize: 18, margin: "0 0 12px", color: "var(--fg-0)" }}>{t("diff.section_summary")}</h2>
        {metaDelta.length > 0 && <MetaDeltaCard rows={metaDelta} />}
        {showAcceptanceChecklist && (
          <AcceptanceChecklistCard checklist={acceptanceChecklist!} />
        )}
        <ul style={{ listStyle: "none", margin: "0 0 32px", padding: 0 }}>
          {data.section_deltas.map((sd, idx) => (
            <li key={idx} style={{
              display: "grid",
              gridTemplateColumns: "100px 1fr auto",
              gap: 12,
              padding: "8px 0",
              borderBottom: "1px solid var(--border)",
              fontSize: 13,
            }}>
              <span className={`chip chip--${changeChipClass(sd.change)}`} style={{ justifySelf: "start" }}>
                {t(`diff.change.${sd.change}`)}
              </span>
              <span style={{ color: "var(--fg-1)" }}>{sd.heading || t("diff.preamble")}</span>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)" }}>
                {sd.lines_added > 0 && <span style={{ color: "var(--live)" }}>+{sd.lines_added}</span>}
                {sd.lines_added > 0 && sd.lines_removed > 0 && " "}
                {sd.lines_removed > 0 && <span style={{ color: "var(--stale)" }}>−{sd.lines_removed}</span>}
              </span>
            </li>
          ))}
        </ul>

        <h2 style={{ fontSize: 18, margin: "0 0 12px", color: "var(--fg-0)" }}>{t("diff.unified")}</h2>
        <UnifiedBlock src={data.unified_diff} />
      </article>
    </main>
  );
}

function AcceptanceChecklistCard({ checklist }: { checklist: AcceptanceChecklist }) {
  const { t } = useI18n();
  const done = checklist.items.filter((item) => item.state === "[x]").length;
  return (
    <section style={{
      border: "1px solid var(--border)",
      borderRadius: "var(--r-2)",
      background: "var(--bg-1)",
      marginBottom: 18,
      overflow: "hidden",
    }}>
      <div style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 12,
        padding: "10px 12px",
        borderBottom: "1px solid var(--border)",
      }}>
        <span style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-0)" }}>{t("diff.acceptance")}</span>
        <span className="chip chip--area">{t("diff.acceptance_count", done, checklist.items.length)}</span>
      </div>
      <div style={{ display: "grid" }}>
        {checklist.items.map((item) => (
          <div
            key={item.index}
            style={{
              display: "grid",
              gridTemplateColumns: "24px 86px 1fr",
              gap: 10,
              alignItems: "start",
              padding: "9px 12px",
              borderBottom: "1px solid var(--border)",
              background: item.changed ? "color-mix(in oklch, var(--accent) 9%, transparent)" : undefined,
            }}
          >
            <span style={{ fontFamily: "var(--font-mono)", color: item.changed ? "var(--accent)" : "var(--fg-4)" }}>
              {item.changed ? "←" : item.index + 1}
            </span>
            <span className={`chip chip--${acceptanceChipClass(item.state)}`} style={{ justifySelf: "start" }}>
              {t(`diff.acceptance.state.${acceptanceStateKey(item.state)}`)}
            </span>
            <div style={{ minWidth: 0 }}>
              <div style={{ color: "var(--fg-1)", wordBreak: "break-word" }}>{item.text}</div>
              {item.changed && (
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 6, fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)" }}>
                  {item.from_state && item.to_state && (
                    <span>{item.from_state} → {item.to_state}</span>
                  )}
                  {item.reason && <span>{item.reason}</span>}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function MetaDeltaCard({ rows }: { rows: MetaDeltaEntry[] }) {
  const { t } = useI18n();
  return (
    <section style={{
      border: "1px solid var(--border)",
      borderRadius: "var(--r-2)",
      background: "var(--bg-1)",
      marginBottom: 18,
      overflow: "hidden",
    }}>
      <div style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 12,
        padding: "10px 12px",
        borderBottom: "1px solid var(--border)",
      }}>
        <span style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-0)" }}>{t("diff.meta")}</span>
        <span className="chip chip--area">{t("diff.meta_count", rows.length)}</span>
      </div>
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12.5 }}>
          <thead>
            <tr style={{ color: "var(--fg-3)", fontFamily: "var(--font-mono)", textAlign: "left" }}>
              <th style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", fontWeight: 500 }}>{t("diff.meta_key")}</th>
              <th style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", fontWeight: 500 }}>{t("diff.meta_before")}</th>
              <th style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", fontWeight: 500 }}>{t("diff.meta_after")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.key}>
                <td style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", fontFamily: "var(--font-mono)", color: "var(--fg-1)", whiteSpace: "nowrap" }}>{row.key}</td>
                <td style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", color: "var(--fg-2)" }}>{formatMetaValue(row.before)}</td>
                <td style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", color: "var(--fg-0)" }}>{formatMetaValue(row.after)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function formatMetaValue(value: unknown): string {
  if (value == null || value === "") return "∅";
  if (Array.isArray(value)) {
    if (value.length === 0) return "∅";
    return value.map((v) => formatMetaValue(v)).join(", ");
  }
  if (typeof value === "object") {
    const json = JSON.stringify(value);
    return json === "{}" ? "∅" : json;
  }
  return String(value);
}

function acceptanceStateKey(state: AcceptanceChecklist["items"][number]["state"]): string {
  switch (state) {
    case "[x]":
      return "done";
    case "[~]":
      return "partial";
    case "[-]":
      return "deferred";
    default:
      return "open";
  }
}

function acceptanceChipClass(state: AcceptanceChecklist["items"][number]["state"]): string {
  switch (state) {
    case "[x]":
      return "live";
    case "[~]":
      return "draft";
    case "[-]":
      return "stale";
    default:
      return "archived";
  }
}

function changeChipClass(change: string): string {
  switch (change) {
    case "added":     return "live";
    case "removed":   return "stale";
    case "modified":  return "draft";
    default:          return "archived";
  }
}

function UnifiedBlock({ src }: { src: string }) {
  const lines = src.split("\n");
  return (
    <div style={{
      background: "var(--bg-0)",
      border: "1px solid var(--border)",
      borderRadius: "var(--r-2)",
      padding: "12px 16px",
      overflowX: "auto",
      fontFamily: "var(--font-mono)",
      fontSize: 12.5,
      lineHeight: 1.6,
    }}>
      {lines.map((line, i) => <DiffLine key={i} line={line} />)}
    </div>
  );
}

function DiffLine({ line }: { line: string }) {
  let bg: string | undefined;
  let color = "var(--fg-2)";
  if (line.startsWith("+") && !line.startsWith("+++")) {
    bg = "var(--diff-add-bg, color-mix(in oklch, var(--live) 12%, transparent))";
    color = "var(--live)";
  } else if (line.startsWith("-") && !line.startsWith("---")) {
    bg = "var(--diff-del-bg, color-mix(in oklch, var(--stale) 12%, transparent))";
    color = "var(--stale)";
  } else if (line.startsWith("@@")) {
    color = "var(--accent)";
  } else if (line.startsWith("+++") || line.startsWith("---")) {
    color = "var(--fg-3)";
  }
  return (
    <div style={{ background: bg, color, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
      {line || "\u00A0"}
    </div>
  );
}
