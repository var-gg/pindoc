import { useEffect, useRef, useState } from "react";
import { NavLink, useLocation } from "react-router";
import { Activity, AlignCenter, AlignJustify, CalendarDays, ChevronDown, CircleHelp, ExternalLink, FileText, Inbox, Languages, LogOut, Maximize2, Menu, Moon, Search, Share2, Sun, UserCircle } from "lucide-react";
import type { ComponentType } from "react";
import { api, type CurrentUserResp, type ProjectListItem } from "../api/client";
import { useI18n, type Lang } from "../i18n";
import type { Project } from "../api/client";
import { InviteButton } from "../project/InviteButton";
import type { Theme } from "./theme";
import type { ReaderWidth } from "./readerWidth";
import { typeChipClass } from "./typeChip";
import { topLevelVisualAreaSlugs, visualDescription, visualLabel, visualLanguage } from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";
import { Tooltip } from "./Tooltip";
import { canShowTelemetryNav, telemetryDebugEnabled } from "./opsAccess";
import { paletteOpenAfterProjectSwitcherToggle, projectSwitcherOpenAfterPaletteChange } from "./overlayStack";
import { isReaderDevSurfaceEnabled } from "../readerRoutes";
import { maskEmail } from "./profilePrivacy";

type Props = {
  project: Project;
  surface: SurfaceId;
  theme: Theme;
  onToggleTheme: () => void;
  onOpenPalette: () => void;
  onClosePalette: () => void;
  onToggleMenu: () => void;
  paletteOpen: boolean;
  inboxCount: number;
  readerWidth: ReaderWidth;
  onChangeReaderWidth: (next: ReaderWidth) => void;
  onOpenInvite?: () => void;
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
  onClosePalette,
  onToggleMenu,
  paletteOpen,
  inboxCount,
  readerWidth,
  onChangeReaderWidth,
  onOpenInvite,
}: Props) {
  const { t, lang, setLang } = useI18n();
  const location = useLocation();
  const nextLang: Lang = lang === "ko" ? "en" : "ko";
  const baseRoute = `/p/${project.slug}`;
  const canInvite = project.current_role === "owner" && Boolean(onOpenInvite);
  const showGraphSurface = isReaderDevSurfaceEnabled(location.search, import.meta.env.DEV);
  const [projectSwitcherOpen, setProjectSwitcherOpen] = useState(false);
  const [mobileSurfaceMenuOpen, setMobileSurfaceMenuOpen] = useState(false);
  const opsDebug = telemetryDebugEnabled(
    location.search,
    typeof window === "undefined" ? null : window.localStorage.getItem("pindoc.ops.debug"),
  );
  const showTelemetry = canShowTelemetryNav(project.current_role, opsDebug);

  useEffect(() => {
    setProjectSwitcherOpen((open) => projectSwitcherOpenAfterPaletteChange(open, paletteOpen));
  }, [paletteOpen]);

  useEffect(() => {
    setMobileSurfaceMenuOpen(false);
  }, [location.pathname, location.search]);

  function openPalette() {
    setProjectSwitcherOpen(false);
    onOpenPalette();
  }

  function setProjectSwitcher(next: boolean) {
    setProjectSwitcherOpen(next);
    if (!paletteOpenAfterProjectSwitcherToggle(next, paletteOpen)) {
      onClosePalette();
    }
  }

  function toggleMobileMenu() {
    setMobileSurfaceMenuOpen((open) => !open);
    onToggleMenu();
  }

  return (
    <>
      <div className="nav">
        <button
          className="nav__menu"
          aria-label={t("nav.mobile_menu")}
          aria-expanded={mobileSurfaceMenuOpen}
          onClick={toggleMobileMenu}
        >
          <Menu className="lucide" />
        </button>
      <NavLink to={`${baseRoute}/today`} className="nav__brand">
        <svg width="20" height="20" viewBox="0 0 32 32" fill="none" style={{ color: "var(--fg-0)", flexShrink: 0 }}>
          <rect x="5" y="7.5" width="19" height="21" rx="2.5" stroke="currentColor" strokeWidth="1.5" />
          <line x1="9" y1="17" x2="20" y2="17" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" opacity="0.55" />
          <circle cx="24" cy="7.5" r="3.5" fill="currentColor" />
        </svg>
        <span className="word">pindoc</span>
      </NavLink>

      <ProjectSwitcher
        project={project}
        open={projectSwitcherOpen}
        onOpenChange={setProjectSwitcher}
        showHiddenProjects={opsDebug}
      />

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
        {showGraphSurface && (
          <NavLink to={`${baseRoute}/graph${location.search || ""}`} className="nav__tab">
            <Share2 className="lucide" />
            <span className="label">{t("nav.graph")}</span>
          </NavLink>
        )}
        <NavLink to={`${baseRoute}/tasks`} className="nav__tab">
          <FileText className="lucide" />
          <span className="label">{t("nav.tasks")}</span>
        </NavLink>
      </div>

      <div className="nav__spacer" />

      <button className="nav__search" onClick={openPalette}>
        <Search className="lucide" />
        <span>{t("nav.search_hint")}</span>
        <span className="kbd">⌘K</span>
      </button>

      {canInvite && (
        <Tooltip content={t("invite.nav.tooltip")}>
          <InviteButton label={t("invite.nav.tooltip")} onClick={onOpenInvite!} />
        </Tooltip>
      )}

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

      {showTelemetry && (
        <Tooltip content={t("nav.telemetry")}>
          <NavLink
            to="/ops/telemetry"
            className="nav__theme"
            aria-label={t("nav.telemetry")}
          >
            <Activity className="lucide" />
          </NavLink>
        </Tooltip>
      )}

        <UserProfileMenu
          project={project}
          theme={theme}
          nextLang={nextLang}
          onToggleTheme={onToggleTheme}
          onChangeLang={setLang}
        />
      </div>
      {mobileSurfaceMenuOpen && (
        <nav className="nav-mobile-surfaces" aria-label={t("nav.mobile_surfaces")}>
          <NavLink to={`${baseRoute}/today`} className="nav-mobile-surfaces__item" onClick={() => setMobileSurfaceMenuOpen(false)}>
            <CalendarDays className="lucide" />
            <span>{t("nav.today")}</span>
          </NavLink>
          <NavLink to={`${baseRoute}/wiki`} className="nav-mobile-surfaces__item" onClick={() => setMobileSurfaceMenuOpen(false)}>
            <FileText className="lucide" />
            <span>{t("nav.wiki_reader")}</span>
          </NavLink>
          <NavLink to={`${baseRoute}/inbox`} className="nav-mobile-surfaces__item" onClick={() => setMobileSurfaceMenuOpen(false)}>
            <Inbox className="lucide" />
            <span>{t("nav.inbox")}</span>
            {inboxCount > 0 && <span className="count">{inboxCount}</span>}
          </NavLink>
          <NavLink to={`${baseRoute}/tasks`} className="nav-mobile-surfaces__item" onClick={() => setMobileSurfaceMenuOpen(false)}>
            <FileText className="lucide" />
            <span>{t("nav.tasks")}</span>
          </NavLink>
        </nav>
      )}
    </>
  );
}

function UserProfileMenu({
  project,
  theme,
  nextLang,
  onToggleTheme,
  onChangeLang,
}: {
  project: Project;
  theme: Theme;
  nextLang: Lang;
  onToggleTheme: () => void;
  onChangeLang: (next: Lang) => void;
}) {
  const { t, lang } = useI18n();
  const [open, setOpen] = useState(false);
  const [current, setCurrent] = useState<CurrentUserResp | null>(null);
  const [signingOut, setSigningOut] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    api.currentUser()
      .then((resp) => {
        if (!cancelled) setCurrent(resp);
      })
      .catch(() => {
        if (!cancelled) setCurrent(null);
      });
    return () => {
      cancelled = true;
    };
  }, [project.slug]);

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

  const user = current?.user;
  const authMode = current?.auth_mode ?? "unknown";
  const name = user?.display_name || t("profile.fallback_name");
  const email = user?.email ? maskEmail(user.email) : t("profile.fallback_email");
  const initials = profileInitials(user?.display_name || user?.email || project.slug);
  const canSignOut = authMode === "oauth_github";

  async function handleSignOut() {
    if (!canSignOut || signingOut) return;
    setSigningOut(true);
    try {
      await api.signOut();
      window.location.assign("/");
    } finally {
      setSigningOut(false);
    }
  }

  return (
    <div className="nav-profile" ref={ref}>
      <Tooltip content={t("profile.open")}>
        <button
          type="button"
          className="nav__user"
          onClick={() => setOpen((v) => !v)}
          aria-label={t("profile.open")}
          aria-expanded={open}
          aria-haspopup="dialog"
        >
          <span>{initials}</span>
        </button>
      </Tooltip>
      {open && (
        <div className="profile-menu" role="dialog" aria-label={t("profile.menu_label")}>
          <div className="profile-menu__head">
            <div className="profile-menu__avatar" aria-hidden>
              <UserCircle className="lucide" />
            </div>
            <div className="profile-menu__identity">
              <strong>{name}</strong>
              <span>{email}</span>
            </div>
          </div>
          <dl className="profile-menu__meta">
            <div>
              <dt>{t("profile.role")}</dt>
              <dd>{t(`members_panel.role_${project.current_role || "viewer"}`)}</dd>
            </div>
            <div>
              <dt>{t("profile.auth_mode")}</dt>
              <dd>{t(`profile.auth_mode.${authMode}`)}</dd>
            </div>
          </dl>
          <div className="profile-menu__actions">
            <button
              type="button"
              className="profile-menu__action"
              onClick={() => onChangeLang(nextLang)}
            >
              <Languages className="lucide" />
              <span>{t("lang.switch")}</span>
              <strong>{lang === "ko" ? "KO" : "EN"}</strong>
            </button>
            <button
              type="button"
              className="profile-menu__action"
              onClick={onToggleTheme}
            >
              {theme === "dark" ? <Moon className="lucide" /> : <Sun className="lucide" />}
              <span>{t("nav.theme_toggle")}</span>
              <strong>{theme === "dark" ? t("profile.theme_dark") : t("profile.theme_light")}</strong>
            </button>
          </div>
          {canSignOut ? (
            <button
              type="button"
              className="profile-menu__signout"
              onClick={handleSignOut}
              disabled={signingOut}
            >
              <LogOut className="lucide" />
              {signingOut ? t("profile.signing_out") : t("profile.sign_out")}
            </button>
          ) : (
            <p className="profile-menu__hint">{t("profile.local_signout_hint")}</p>
          )}
        </div>
      )}
    </div>
  );
}

function profileInitials(seed: string): string {
  const trimmed = seed.trim();
  if (!trimmed) return "?";
  const parts = trimmed.split(/\s+/).filter(Boolean);
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[1][0]}`.toUpperCase();
  }
  return trimmed.slice(0, 2).toUpperCase();
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
function ProjectSwitcher({
  project,
  open,
  onOpenChange,
  showHiddenProjects,
}: {
  project: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  showHiddenProjects: boolean;
}) {
  const { t } = useI18n();
  const [projects, setProjects] = useState<ProjectListItem[] | null>(null);
  const [loadedHiddenProjects, setLoadedHiddenProjects] = useState<boolean | null>(null);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open || loadedHiddenProjects === showHiddenProjects) return;
    let cancelled = false;
    setProjects(null);
    (async () => {
      try {
        const resp = await api.projectList({ includeHidden: showHiddenProjects });
        if (!cancelled) {
          setProjects(resp.projects);
          setLoadedHiddenProjects(showHiddenProjects);
        }
      } catch {
        if (!cancelled) {
          setProjects([]);
          setLoadedHiddenProjects(showHiddenProjects);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, loadedHiddenProjects, showHiddenProjects]);

  useEffect(() => {
    if (!open) return;
    function onClickAway(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onOpenChange(false);
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onOpenChange(false);
    }
    window.addEventListener("mousedown", onClickAway);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onClickAway);
      window.removeEventListener("keydown", onKey);
    };
  }, [open, onOpenChange]);

  return (
    <div className="nav__project-wrap" ref={ref} style={{ position: "relative" }}>
      <button
        type="button"
        className="nav__project"
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() => onOpenChange(!open)}
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
            maxHeight: "min(70vh, 420px)",
            overflowY: "auto",
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
