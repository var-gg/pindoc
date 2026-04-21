import { NavLink } from "react-router";
import { ChevronDown, FileText, Inbox, Menu, Moon, Search, Share2, Sun } from "lucide-react";
import { useI18n, type Lang } from "../i18n";
import type { Project } from "../api/client";
import type { Theme } from "./theme";

type Props = {
  project: Project;
  theme: Theme;
  onToggleTheme: () => void;
  onOpenPalette: () => void;
  onToggleMenu: () => void;
  inboxCount: number;
};

export function TopNav({
  project,
  theme,
  onToggleTheme,
  onOpenPalette,
  onToggleMenu,
  inboxCount,
}: Props) {
  const { t, lang, setLang } = useI18n();
  const nextLang: Lang = lang === "ko" ? "en" : "ko";

  return (
    <div className="nav">
      <button className="nav__menu" aria-label="Open menu" onClick={onToggleMenu}>
        <Menu className="lucide" />
      </button>
      <NavLink to="/" className="nav__brand">
        <svg width="20" height="20" viewBox="0 0 32 32" fill="none" style={{ color: "var(--fg-0)", flexShrink: 0 }}>
          <rect x="5" y="7.5" width="19" height="21" rx="2.5" stroke="currentColor" strokeWidth="1.5" />
          <line x1="9" y1="17" x2="20" y2="17" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" opacity="0.55" />
          <circle cx="24" cy="7.5" r="3.5" fill="currentColor" />
        </svg>
        <span className="word">pindoc</span>
      </NavLink>

      <button
        className="nav__project"
        title={t("nav.project_switcher_soon")}
        aria-label={t("nav.project_switcher_soon")}
      >
        <span>{project.slug}</span>
        <ChevronDown className="lucide" />
      </button>

      <div className="nav__tabs">
        <NavLink to="/wiki" className="nav__tab">
          <FileText className="lucide" />
          <span className="label">{t("nav.wiki_reader")}</span>
        </NavLink>
        <NavLink to="/inbox" className="nav__tab">
          <Inbox className="lucide" />
          <span className="label">{t("nav.inbox")}</span>
          {inboxCount > 0 && <span className="count">{inboxCount}</span>}
        </NavLink>
        <NavLink to="/graph" className="nav__tab">
          <Share2 className="lucide" />
          <span className="label">{t("nav.graph")}</span>
        </NavLink>
        <NavLink to="/tasks" className="nav__tab">
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

      <button className="nav__lang" onClick={() => setLang(nextLang)} aria-label={t("lang.switch")}>
        {lang === "ko" ? "KO" : "EN"}
      </button>

      <button className="nav__theme" onClick={onToggleTheme} aria-label={t("nav.theme_toggle")}>
        {theme === "dark" ? <Moon className="lucide" /> : <Sun className="lucide" />}
      </button>

      {/* Avatar placeholder — V1 self-host binds it to the GitHub OAuth
          user; M1 has no auth yet, so the tooltip spells that out rather
          than putting random project initials here. */}
      <div
        className="nav__user"
        title={t("nav.user_placeholder")}
        style={{ opacity: 0.55 }}
      >
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 10 }}>—</span>
      </div>
    </div>
  );
}
