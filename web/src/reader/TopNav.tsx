import { useEffect, useRef, useState } from "react";
import { NavLink } from "react-router";
import { Activity, AlignCenter, AlignJustify, CalendarDays, ChevronDown, CircleHelp, ExternalLink, FileText, Inbox, Maximize2, Menu, Moon, Search, Share2, Sun } from "lucide-react";
import type { ComponentType } from "react";
import { api, type ProjectListItem } from "../api/client";
import { useI18n, type Lang } from "../i18n";
import type { Project } from "../api/client";
import type { Theme } from "./theme";
import type { ReaderWidth } from "./readerWidth";
import { typeChipClass } from "./typeChip";
import { topLevelVisualAreaSlugs, visualDescription, visualLabel, visualLanguage } from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";
import { Tooltip } from "./Tooltip";

type Props = {
  project: Project;
  surface: SurfaceId;
  theme: Theme;
  onToggleTheme: () => void;
  onOpenPalette: () => void;
  onToggleMenu: () => void;
  inboxCount: number;
  readerWidth: ReaderWidth;
  onChangeReaderWidth: (next: ReaderWidth) => void;
};
type SurfaceId = "today" | "reader" | "inbox" | "graph" | "tasks";

// Width toggle ordering + icons. AlignCenter = narrow (tight column),
// AlignJustify = default (balanced), Maximize2 = wide (full-bleed).
// Kept in one array so TopNav renders a tight segmented control with
// stable aria labels.
const WIDTH_OPTIONS: Array<{ mode: ReaderWidth; icon: ComponentType<{ className?: string }>; labelKey: string }> = [
  { mode: "narrow", icon: AlignCenter, labelKey: "nav.width_narrow" },
  { mode: "default", icon: AlignJustify, labelKey: "nav.width_default" },
  { mode: "wide", icon: Maximize2, labelKey: "nav.width_wide" },
];

export function TopNav({
  project,
  surface,
  theme,
  onToggleTheme,
  onOpenPalette,
  onToggleMenu,
  inboxCount,
  readerWidth,
  onChangeReaderWidth,
}: Props) {
  const { t, lang, setLang } = useI18n();
  const nextLang: Lang = lang === "ko" ? "en" : "ko";
  const baseRoute = `/p/${project.slug}`;

  return (
    <div className="nav">
      <button className="nav__menu" aria-label="Open menu" onClick={onToggleMenu}>
        <Menu className="lucide" />
      </button>
      <NavLink to="/design" className="nav__brand">
        <svg width="20" height="20" viewBox="0 0 32 32" fill="none" style={{ color: "var(--fg-0)", flexShrink: 0 }}>
          <rect x="5" y="7.5" width="19" height="21" rx="2.5" stroke="currentColor" strokeWidth="1.5" />
          <line x1="9" y1="17" x2="20" y2="17" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" opacity="0.55" />
          <circle cx="24" cy="7.5" r="3.5" fill="currentColor" />
        </svg>
        <span className="word">pindoc</span>
      </NavLink>

      <ProjectSwitcher project={project} />

      <div className="nav__tabs">
        <NavLink to={`${baseRoute}/today`} className="nav__tab">
          <CalendarDays className="lucide" />
          <span className="label">{t("nav.today")}</span>
        </NavLink>
        <NavLink to={`${baseRoute}/wiki`} className="nav__tab">
          <FileText className="lucide" />
          <span className="label">{t("nav.wiki_reader")}</span>
        </NavLink>
        <NavLink to={`${baseRoute}/inbox`} className="nav__tab">
          <Inbox className="lucide" />
          <span className="label">{t("nav.inbox")}</span>
          {inboxCount > 0 && <span className="count">{inboxCount}</span>}
        </NavLink>
        <NavLink to={`${baseRoute}/graph`} className="nav__tab">
          <Share2 className="lucide" />
          <span className="label">{t("nav.graph")}</span>
        </NavLink>
        <NavLink to={`${baseRoute}/tasks`} className="nav__tab">
          <FileText className="lucide" />
          <span className="label">{t("nav.tasks")}</span>
        </NavLink>
      </div>

      <div className="nav__spacer" />

      <button className="nav__search" onClick={onOpenPalette}>
        <Search className="lucide" />
        <span>{t("nav.search_hint")}</span>
        <span className="kbd">⌘K</span>
      </button>

      <HelpPopover surface={surface} />

      <div className="nav__width" role="group" aria-label={t("nav.width_toggle")}>
        {WIDTH_OPTIONS.map(({ mode, icon: Icon, labelKey }) => {
          const active = readerWidth === mode;
          return (
            <Tooltip key={mode} content={t(labelKey)}>
              <button
                type="button"
                className={`nav__width-btn${active ? " is-active" : ""}`}
                onClick={() => onChangeReaderWidth(mode)}
                aria-pressed={active}
                aria-label={t(labelKey)}
              >
                <Icon className="lucide" />
              </button>
            </Tooltip>
          );
        })}
      </div>

      <button className="nav__lang" onClick={() => setLang(nextLang)} aria-label={t("lang.switch")}>
        {lang === "ko" ? "KO" : "EN"}
      </button>

      <Tooltip content={t("nav.telemetry")}>
        <NavLink
          to="/ops/telemetry"
          className="nav__theme"
          aria-label={t("nav.telemetry")}
        >
          <Activity className="lucide" />
        </NavLink>
      </Tooltip>

      <button className="nav__theme" onClick={onToggleTheme} aria-label={t("nav.theme_toggle")}>
        {theme === "dark" ? <Moon className="lucide" /> : <Sun className="lucide" />}
      </button>

      {/* Avatar placeholder — V1 self-host binds it to the GitHub OAuth
          user; M1 has no auth yet, so the tooltip spells that out rather
          than putting random project initials here. */}
      <Tooltip content={t("nav.user_placeholder")}>
        <div
          className="nav__user"
          style={{ opacity: 0.55 }}
        >
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 10 }}>—</span>
        </div>
      </Tooltip>
    </div>
  );
}

function HelpPopover({ surface }: { surface: SurfaceId }) {
  const { t, lang } = useI18n();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    function onClickAway(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    window.addEventListener("mousedown", onClickAway);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onClickAway);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="nav-help" ref={ref}>
      <Tooltip content={t("help.open")}>
        <button
          type="button"
          className="nav__theme nav-help__trigger"
          onClick={() => setOpen((v) => !v)}
          aria-label={t("help.open")}
          aria-expanded={open}
          aria-haspopup="dialog"
        >
          <CircleHelp className="lucide" />
        </button>
      </Tooltip>
      {open && (
        <div className="nav-help__popover" role="dialog" aria-label={t("help.title")}>
          <div className="nav-help__surface">
            <div className="nav-help__eyebrow">{t("help.surface_label")}</div>
            <h2>{t(`help.surface.${surface}.title`)}</h2>
            <p>{t(`help.surface.${surface}.description`)}</p>
          </div>

          <section className="nav-help__section">
            <h3>{t("help.types_title")}</h3>
            <div className="nav-help__grid nav-help__grid--types">
              {Object.values(visualLanguage.types).map((entry) => (
                <div key={entry.canonical} className="nav-help__card">
                  <span className={typeChipClass(entry.canonical)}>{visualLabel(entry, lang)}</span>
                  <p>{visualDescription(entry, lang)}</p>
                </div>
              ))}
            </div>
          </section>

          <section className="nav-help__section">
            <h3>{t("help.areas_title")}</h3>
            <div className="nav-help__grid">
              {topLevelVisualAreaSlugs.map((slug) => {
                const entry = visualLanguage.areas[slug];
                const Icon = visualIconComponent(entry.icon);
                const style = { "--area-color": `var(${entry.color_token})` } as React.CSSProperties & Record<"--area-color", string>;
                return (
                  <div key={slug} className="nav-help__card nav-help__card--area" style={style}>
                    <span className="chip-area chip-area--visual">
                      <Icon className="lucide" />
                      {visualLabel(entry, lang)}
                    </span>
                    <p>{visualDescription(entry, lang)}</p>
                  </div>
                );
              })}
            </div>
          </section>

          <div className="nav-help__links" aria-label={t("help.docs_title")}>
            <a href="https://github.com/var-gg/pindoc/blob/main/docs/19-area-taxonomy.md" target="_blank" rel="noreferrer">
              <span>{t("help.docs_area_taxonomy")}</span>
              <ExternalLink className="lucide" aria-hidden="true" />
            </a>
            <a href="https://github.com/var-gg/pindoc/blob/main/docs/glossary.md" target="_blank" rel="noreferrer">
              <span>{t("help.docs_glossary")}</span>
              <ExternalLink className="lucide" aria-hidden="true" />
            </a>
          </div>
        </div>
      )}
    </div>
  );
}

// ProjectSwitcher toggles a dropdown listing every project in this instance.
// Switching navigates to /p/<slug>/wiki of the chosen project. Creation of
// new projects is intentionally not available here: per architecture principle
// 1 (agent-only write surface), the user asks the agent which calls
// pindoc.project.create. The "+ new project" row surfaces that hint.
function ProjectSwitcher({ project }: { project: Project }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [projects, setProjects] = useState<ProjectListItem[] | null>(null);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open || projects) return;
    let cancelled = false;
    (async () => {
      try {
        const resp = await api.projectList();
        if (!cancelled) setProjects(resp.projects);
      } catch {
        if (!cancelled) setProjects([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, projects]);

  useEffect(() => {
    if (!open) return;
    function onClickAway(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    window.addEventListener("mousedown", onClickAway);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onClickAway);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="nav__project-wrap" ref={ref} style={{ position: "relative" }}>
      <button
        type="button"
        className="nav__project"
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() => setOpen((v) => !v)}
      >
        <span>{project.slug}</span>
        <ChevronDown className="lucide" />
      </button>
      {open && (
        <div
          role="listbox"
          className="project-menu"
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            left: 0,
            minWidth: 240,
            background: "var(--bg-1)",
            border: "1px solid var(--border)",
            borderRadius: "var(--r-2)",
            boxShadow: "0 8px 24px rgba(0,0,0,0.25)",
            zIndex: 40,
            padding: 4,
          }}
        >
          {projects === null && (
            <div style={{ padding: "8px 10px", fontSize: 12, color: "var(--fg-3)" }}>
              {t("wiki.loading")}
            </div>
          )}
          {projects?.map((p) => (
            <a
              key={p.id}
              href={`/p/${p.slug}/wiki`}
              role="option"
              aria-selected={p.slug === project.slug}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "8px 10px",
                borderRadius: "var(--r-1)",
                textDecoration: "none",
                color: "var(--fg-0)",
                background:
                  p.slug === project.slug
                    ? "color-mix(in oklch, var(--accent) 10%, transparent)"
                    : "transparent",
              }}
            >
              <span
                aria-hidden
                style={{
                  width: 10,
                  height: 10,
                  borderRadius: 2,
                  background: p.color || "var(--accent)",
                  flexShrink: 0,
                }}
              />
              <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {p.name}
              </span>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)" }}>
                {p.slug}
              </span>
            </a>
          ))}
          {projects && projects.length === 0 && (
            <div style={{ padding: "8px 10px", fontSize: 12, color: "var(--fg-3)" }}>
              {t("nav.no_other_projects")}
            </div>
          )}
          <div
            style={{
              borderTop: "1px solid var(--border)",
              marginTop: 4,
              padding: "8px 10px",
              fontSize: 11,
              color: "var(--fg-3)",
              lineHeight: 1.45,
            }}
          >
            <div style={{ color: "var(--fg-2)", marginBottom: 2 }}>
              {t("nav.new_project_hint_title")}
            </div>
            {t("nav.new_project_hint_body")}
          </div>
        </div>
      )}
    </div>
  );
}
