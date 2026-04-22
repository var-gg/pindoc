import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import type { Artifact, ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { CmdK } from "./CmdK";
import { ReaderSurface } from "./ReaderSurface";
import { Sidebar } from "./Sidebar";
import { Sidecar } from "./Sidecar";
import { TopNav } from "./TopNav";
import { initTheme, setTheme, type Theme } from "./theme";
import { useReaderData } from "./useReaderData";
import "../styles/reader.css";

export type ReaderView = "reader" | "inbox" | "graph" | "tasks";

type Props = {
  view: ReaderView;
};

export function ReaderShell({ view }: Props) {
  const { project = "", slug } = useParams<{ project: string; slug?: string }>();
  const { t } = useI18n();
  const navigate = useNavigate();
  const [showTemplates, setShowTemplates] = useState(false);
  const state = useReaderData(project, slug, showTemplates);
  const [theme, setThemeState] = useState<Theme>(() => initTheme());
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [selectedArea, setSelectedArea] = useState<string | null>(null);
  const [selectedType, setSelectedTypeState] = useState<string | null>(
    view === "tasks" ? "Task" : null,
  );

  // Keep Tasks route's type filter sticky. Switching to /wiki clears it.
  useEffect(() => {
    setSelectedTypeState(view === "tasks" ? "Task" : null);
  }, [view]);

  const baseRoute = `/p/${project}/${view === "tasks" ? "tasks" : "wiki"}`;

  // When the user filters via the sidebar we drop the currently-selected
  // artifact so the filter effect is visible (otherwise the reader body
  // would paper over the list). Filter-while-detail-shown is a future
  // UX improvement that needs in-place scroll to next matching artifact.
  function handleSelectArea(next: string | null) {
    setSelectedArea(next);
    if (slug) navigate(baseRoute);
  }
  function handleSelectType(next: string | null) {
    setSelectedTypeState((prev) => (prev === next ? null : next));
    if (slug) navigate(baseRoute);
  }

  // ⌘K listener — global-level so palette opens from any surface.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPaletteOpen(true);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const toggleTheme = () => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    setThemeState(next);
  };

  // Hooks must stay at top level (rules-of-hooks). Filter against a
  // possibly-empty list when we haven't loaded yet so we never call
  // this hook conditionally.
  const baseList =
    state.kind === "ready" ? state.data.artifacts : ([] as ArtifactRef[]);
  const filteredArtifacts = useMemo(
    () =>
      baseList.filter((a) => {
        if (selectedArea && a.area_slug !== selectedArea) return false;
        if (selectedType && a.type !== selectedType) return false;
        return true;
      }),
    [baseList, selectedArea, selectedType],
  );

  if (state.kind === "loading") {
    return <div className="reader-state">{t("wiki.loading")}</div>;
  }
  if (state.kind === "error") {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_title")}</strong>
        <p>{state.message}</p>
        <p>
          {t("wiki.error_hint_prefix")} <code>{t("wiki.error_hint_cmd")}</code>{" "}
          {t("wiki.error_hint_suffix")}
        </p>
      </div>
    );
  }

  const { project: projectData, areas, detail, types, agents } = state.data;

  return (
    <div className="app-shell">
      <TopNav
        project={projectData}
        theme={theme}
        onToggleTheme={toggleTheme}
        onOpenPalette={() => setPaletteOpen(true)}
        onToggleMenu={() => setMenuOpen((v) => !v)}
        inboxCount={0}
      />
      <div className="main">
        <Sidebar
          areas={areas}
          types={types}
          agents={agents}
          selectedArea={selectedArea}
          onSelectArea={handleSelectArea}
          selectedType={selectedType}
          onSelectType={handleSelectType}
          open={menuOpen}
          showTemplates={showTemplates}
          onToggleTemplates={() => setShowTemplates((v) => !v)}
        />
        <Body
          view={view}
          projectSlug={project}
          detail={detail}
          list={filteredArtifacts}
          currentSlug={slug}
          selectedType={selectedType}
        />
        <Sidecar projectSlug={project} detail={view === "reader" || view === "tasks" ? detail : null} />
      </div>
      <CmdK projectSlug={project} open={paletteOpen} onClose={() => setPaletteOpen(false)} />
    </div>
  );
}

function Body({
  view,
  projectSlug,
  detail,
  list,
  currentSlug,
  selectedType,
}: {
  view: ReaderView;
  projectSlug: string;
  detail: Artifact | null;
  list: ArtifactRef[];
  currentSlug: string | undefined;
  selectedType: string | null;
}) {
  const { t } = useI18n();

  if (view === "graph") {
    return (
      <main className="content">
        <div className="surface-stub">
          <h1>{t("nav.graph")}</h1>
          <p>{t("wiki.stub_graph")}</p>
          <p>
            <Link to="/ui/reader">{t("wiki.stub_graph_preview")}</Link>
          </p>
        </div>
      </main>
    );
  }
  if (view === "inbox") {
    return (
      <main className="content">
        <div className="surface-stub">
          <h1>{t("nav.inbox")}</h1>
          <p>{t("wiki.stub_inbox")}</p>
        </div>
      </main>
    );
  }

  // When an artifact is selected (detail != null) we render the Reader
  // surface to match the handoff prototype. Otherwise fall back to a
  // compact list so the user can pick one — Claude Design's original
  // flow expects ⌘K navigation here, but a visible list keeps the
  // empty-first-render less mysterious.
  if (detail) {
    return (
      <ReaderSurface
        detail={detail}
        emptyMessage={view === "tasks" ? t("wiki.empty_tasks_detail") : t("wiki.empty_detail")}
      />
    );
  }

  const empty =
    view === "tasks"
      ? t("wiki.empty_tasks")
      : selectedType
      ? t("reader.empty_filtered", selectedType)
      : t("wiki.empty_list");

  return (
    <main className="content">
      <div className="reader-article">
        <div className="side-section" style={{ padding: "0 0 12px" }}>
          {view === "tasks" ? t("nav.tasks") : t("wiki.section_artifacts")} · {list.length}
        </div>
        {list.length === 0 ? (
          <div style={{ color: "var(--fg-3)", fontSize: 13 }}>{empty}</div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.map((a) => {
              const linkBase = `/p/${projectSlug}/${view === "tasks" ? "tasks" : "wiki"}`;
              const isActive = currentSlug === a.slug;
              return (
                <Link
                  key={a.id}
                  to={`${linkBase}/${a.slug}`}
                  className={`backlink${isActive ? " is-active" : ""}`}
                  style={isActive ? {
                    borderColor: "var(--accent)",
                    background: "color-mix(in oklch, var(--accent) 10%, transparent)",
                  } : undefined}
                >
                  <div className="backlink__head">
                    <span className="chip">{a.type}</span>
                    <span>{a.title}</span>
                  </div>
                  <div className="backlink__excerpt">
                    {a.area_slug} · {a.author_id} · {new Date(a.updated_at).toLocaleDateString()}
                  </div>
                </Link>
              );
            })}
          </div>
        )}
      </div>
    </main>
  );
}
