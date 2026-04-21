import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { ChevronRight } from "lucide-react";
import { api, type RevisionsResp } from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";

type Load =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; data: RevisionsResp };

export function History() {
  const { slug = "" } = useParams<{ slug: string }>();
  const { t } = useI18n();
  const [state, setState] = useState<Load>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await api.revisions(slug);
        if (!cancelled) setState({ kind: "ready", data });
      } catch (err) {
        if (!cancelled) setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [slug]);

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
          <Link to={`/wiki/${slug}`}>{data.title}</Link>
          <ChevronRight className="lucide" />
          <span className="current">{t("history.title")}</span>
        </div>
        <h1 className="art-title">{t("history.title")}</h1>
        <div style={{ color: "var(--fg-3)", marginBottom: 24, fontFamily: "var(--font-mono)", fontSize: 12 }}>
          {t("history.count", data.revisions.length)}
        </div>

        <ol style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {data.revisions.map((r, i) => {
            const av = agentAvatar(r.author_id);
            const previous = data.revisions[i + 1];
            const diffHref = previous
              ? `/wiki/${slug}/diff?from=${previous.revision_number}&to=${r.revision_number}`
              : null;
            return (
              <li key={r.revision_number} style={{
                display: "grid",
                gridTemplateColumns: "68px 1fr auto",
                gap: 12,
                alignItems: "baseline",
                padding: "16px 0",
                borderBottom: "1px solid var(--border)",
              }}>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--fg-3)" }}>
                  rev {r.revision_number}
                </span>
                <div>
                  <div style={{ color: "var(--fg-0)", marginBottom: 4 }}>
                    {r.commit_msg || t("history.no_commit_msg")}
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--fg-3)" }}>
                    <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>{av.initials}</span>
                    <span>{r.author_id}{r.author_version ? `@${r.author_version}` : ""}</span>
                    <span>·</span>
                    <span>{new Date(r.created_at).toLocaleString()}</span>
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
          })}
        </ol>
      </article>
    </main>
  );
}
