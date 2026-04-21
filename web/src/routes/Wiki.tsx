import { useEffect, useMemo, useState } from "react";
import { Link, NavLink, useParams } from "react-router";
import { api, type Area, type Artifact, type ArtifactRef, type Project } from "../api/client";
import { useI18n } from "../i18n";

type View = "reader" | "tasks" | "graph" | "inbox";

type LoadState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; project: Project; areas: Area[]; list: ArtifactRef[]; detail: Artifact | null };

export function WikiRoute({ view }: { view: View }) {
  const params = useParams<{ slug?: string }>();
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const [selectedArea, setSelectedArea] = useState<string | null>(null);
  const { t } = useI18n();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [project, areasResp, listResp] = await Promise.all([
          api.currentProject(),
          api.areas(),
          api.artifacts(),
        ]);
        let detail: Artifact | null = null;
        const typeFilter = view === "tasks" ? "Task" : null;
        const scoped = typeFilter
          ? listResp.artifacts.filter((a) => a.type === typeFilter)
          : listResp.artifacts;
        if (params.slug) {
          detail = await api.artifact(params.slug);
        } else if (scoped.length > 0 && view !== "graph" && view !== "inbox") {
          detail = await api.artifact(scoped[0]!.slug);
        }
        if (cancelled) return;
        setState({
          kind: "ready",
          project,
          areas: areasResp.areas,
          list: listResp.artifacts,
          detail,
        });
      } catch (err) {
        if (cancelled) return;
        setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [params.slug, view]);

  if (state.kind === "loading") {
    return <div className="wiki__state">{t("wiki.loading")}</div>;
  }
  if (state.kind === "error") {
    return (
      <div className="wiki__state wiki__state--error">
        <strong>{t("wiki.error_title")}</strong>
        <p>{state.message}</p>
        <p>
          {t("wiki.error_hint_prefix")} <code>{t("wiki.error_hint_cmd")}</code>{" "}
          {t("wiki.error_hint_suffix")}
        </p>
      </div>
    );
  }

  const { project, areas, list, detail } = state;
  const typeFilter = view === "tasks" ? "Task" : null;
  const scopedList = typeFilter ? list.filter((a) => a.type === typeFilter) : list;
  const filteredList = selectedArea
    ? scopedList.filter((a) => a.area_slug === selectedArea)
    : scopedList;

  return (
    <div className="wiki">
      <aside className="wiki__nav">
        <header className="wiki__project">
          <span className="wiki__dot" style={{ background: project.color || "var(--accent)" }} />
          <div>
            <div className="wiki__name">{project.name}</div>
            <div className="wiki__meta">
              {t("wiki.meta_artifacts", project.artifacts_count, project.primary_language)}
            </div>
          </div>
        </header>
        <nav className="wiki__surface-tabs">
          <NavLink to="/wiki" className="wiki__tab">{t("nav.wiki_reader")}</NavLink>
          <NavLink to="/tasks" className="wiki__tab">
            {t("nav.tasks")} <span className="wiki__tab-count">{list.filter((a) => a.type === "Task").length}</span>
          </NavLink>
          <NavLink to="/graph" className="wiki__tab">{t("nav.graph")}</NavLink>
          <NavLink to="/inbox" className="wiki__tab">{t("nav.inbox")}</NavLink>
        </nav>
        <section>
          <h3>{t("wiki.section_areas")}</h3>
          <ul className="wiki__areas">
            <li>
              <button
                type="button"
                className={`wiki__area-btn ${selectedArea === null ? "is-active" : ""}`}
                onClick={() => setSelectedArea(null)}
              >
                <span className="wiki__area-name">{t("wiki.area_all")}</span>
                <span className="wiki__area-count">{scopedList.length}</span>
              </button>
            </li>
            {areas.map((a) => {
              const areaCount = typeFilter
                ? list.filter((x) => x.area_slug === a.slug && x.type === typeFilter).length
                : a.artifact_count;
              return (
                <li key={a.id} className={a.is_cross_cutting ? "is-cross" : undefined}>
                  <button
                    type="button"
                    className={`wiki__area-btn ${selectedArea === a.slug ? "is-active" : ""}`}
                    onClick={() =>
                      setSelectedArea((prev) => (prev === a.slug ? null : a.slug))
                    }
                  >
                    <span className="wiki__area-name">{a.name}</span>
                    <span className="wiki__area-count">{areaCount}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
        {view !== "graph" && view !== "inbox" && (
          <section>
            <h3>
              {view === "tasks" ? t("nav.tasks") : t("wiki.section_artifacts")}
            </h3>
            <ul className="wiki__list">
              {filteredList.length === 0 && (
                <li className="wiki__empty">
                  {view === "tasks" ? t("wiki.empty_tasks") : t("wiki.empty_list")}
                </li>
              )}
              {filteredList.map((a) => (
                <li key={a.id} className={detail?.id === a.id ? "is-active" : undefined}>
                  <Link to={`${view === "tasks" ? "/tasks" : "/wiki"}/${a.slug}`}>
                    <span className="wiki__chip">{a.type}</span>
                    <span className="wiki__title">{a.title}</span>
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        )}
      </aside>
      <main className="wiki__main">
        {view === "graph" && <GraphStub />}
        {view === "inbox" && <InboxStub />}
        {(view === "reader" || view === "tasks") &&
          (detail ? (
            <ArtifactView detail={detail} />
          ) : (
            <div className="wiki__empty">
              {view === "tasks" ? t("wiki.empty_tasks_detail") : t("wiki.empty_detail")}
            </div>
          ))}
      </main>
    </div>
  );
}

function ArtifactView({ detail }: { detail: Artifact }) {
  const { t } = useI18n();
  const publishedAt = useMemo(
    () => (detail.published_at ? new Date(detail.published_at).toLocaleString() : "—"),
    [detail.published_at],
  );
  return (
    <article className="reader">
      <header>
        <div className="reader__crumb">
          {detail.area_slug} · <span className="reader__type">{detail.type}</span> ·{" "}
          <span className="reader__slug">{detail.slug}</span>
        </div>
        <h1>{detail.title}</h1>
        <div className="reader__meta">
          <span className={`reader__badge reader__badge--${detail.status}`}>{detail.status}</span>
          <span>{t("wiki.written_by", detail.author_id)}</span>
          <span>{t("wiki.published", publishedAt)}</span>
        </div>
      </header>
      <pre className="reader__body">{detail.body_markdown}</pre>
    </article>
  );
}

function GraphStub() {
  const { t } = useI18n();
  return (
    <div className="wiki__stub">
      <h1>{t("nav.graph")}</h1>
      <p>{t("wiki.stub_graph")}</p>
      <p>
        <Link to="/ui/reader">{t("wiki.stub_graph_preview")}</Link>
      </p>
    </div>
  );
}

function InboxStub() {
  const { t } = useI18n();
  return (
    <div className="wiki__stub">
      <h1>{t("nav.inbox")}</h1>
      <p>{t("wiki.stub_inbox")}</p>
    </div>
  );
}
