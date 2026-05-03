import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { ChevronDown } from "lucide-react";
import { api, type Project, type ProjectListItem } from "../api/client";
import { useI18n } from "../i18n";
import { projectSurfacePath } from "../readerRoutes";
import {
  groupProjectSwitcherProjects,
  projectSwitcherActionItems,
  projectSwitcherKeyboardIndex,
  projectSwitcherOptionID,
  projectSwitcherProjectKey,
  sanitizeIDPart,
} from "./projectSwitcherModel";

export type ProjectSwitcherPlacement = "topnav" | "sidebar";

export function ProjectSwitcher({
  project,
  orgSlug,
  open,
  onOpenChange,
  showHiddenProjects,
  projectCreateAllowed,
  placement = "topnav",
}: {
  project: Project;
  orgSlug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  showHiddenProjects: boolean;
  projectCreateAllowed: boolean;
  placement?: ProjectSwitcherPlacement;
}) {
  const { t } = useI18n();
  const [projects, setProjects] = useState<ProjectListItem[] | null>(null);
  const [loadedHiddenProjects, setLoadedHiddenProjects] = useState<boolean | null>(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const ref = useRef<HTMLDivElement | null>(null);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const instanceID = sanitizeIDPart(useId());
  const buttonID = `${instanceID}-button`;
  const listboxID = `${instanceID}-listbox`;
  const groups = useMemo(
    () => groupProjectSwitcherProjects(projects ?? [], orgSlug),
    [projects, orgSlug],
  );
  const actionItems = useMemo(
    () => projectSwitcherActionItems(groups, orgSlug, projectCreateAllowed),
    [groups, orgSlug, projectCreateAllowed],
  );
  const activeItem = actionItems[activeIndex];
  const activeOptionID = open && activeItem ? projectSwitcherOptionID(instanceID, activeItem.key) : undefined;
  const actionIndexByKey = useMemo(
    () => new Map(actionItems.map((item, index) => [item.key, index])),
    [actionItems],
  );
  const showGroupHeadings = groups.length > 1;

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
      if (e.key === "Escape") {
        e.preventDefault();
        closeAndFocus();
      }
    }
    window.addEventListener("mousedown", onClickAway);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onClickAway);
      window.removeEventListener("keydown", onKey);
    };
  }, [open, onOpenChange]);

  useEffect(() => {
    if (activeIndex >= actionItems.length) {
      setActiveIndex(Math.max(0, actionItems.length - 1));
    }
  }, [activeIndex, actionItems.length]);

  function closeAndFocus() {
    onOpenChange(false);
    buttonRef.current?.focus();
  }

  function toggleOpen() {
    const next = !open;
    if (next) setActiveIndex(0);
    onOpenChange(next);
  }

  function navigateActive() {
    const item = actionItems[activeIndex];
    if (!item) return;
    window.location.assign(item.href);
  }

  function handleKeyDown(e: ReactKeyboardEvent) {
    if (e.key === "Escape" && open) {
      e.preventDefault();
      closeAndFocus();
      return;
    }
    if (e.key === "ArrowDown" || e.key === "ArrowUp" || e.key === "Home" || e.key === "End") {
      e.preventDefault();
      if (!open) {
        onOpenChange(true);
        setActiveIndex(e.key === "ArrowUp" ? Math.max(0, actionItems.length - 1) : 0);
        return;
      }
      setActiveIndex((current) => projectSwitcherKeyboardIndex(current, actionItems.length, e.key));
      return;
    }
    if (open && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      navigateActive();
    }
  }

  return (
    <div
      className={`nav__project-wrap nav__project-wrap--${placement}`}
      ref={ref}
    >
      <button
        id={buttonID}
        ref={buttonRef}
        type="button"
        className="nav__project"
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-controls={open ? listboxID : undefined}
        aria-activedescendant={activeOptionID}
        aria-label={t("nav.project_switcher_label")}
        onClick={toggleOpen}
        onKeyDown={handleKeyDown}
      >
        <span className="nav__project-slug">{project.slug}</span>
        <span className="nav__project-org">{orgSlug}</span>
        <ChevronDown className="lucide" />
      </button>
      {open && (
        <div
          id={listboxID}
          role="listbox"
          aria-labelledby={buttonID}
          className="project-menu"
          onKeyDown={handleKeyDown}
        >
          {projects === null && (
            <div className="project-menu__state">
              {t("wiki.loading")}
            </div>
          )}
          {projects !== null && groups.map((group) => (
            <div
              key={group.orgSlug}
              className="project-menu__group"
              role={showGroupHeadings ? "group" : undefined}
              aria-label={showGroupHeadings ? group.orgSlug : undefined}
            >
              {showGroupHeadings && (
                <div className="project-menu__group-heading">
                  {group.orgSlug}
                </div>
              )}
              {group.projects.map((p) => {
                const key = projectSwitcherProjectKey(p);
                const itemIndex = actionIndexByKey.get(key) ?? -1;
                const projectOrg = p.organization_slug || p.org_slug || orgSlug;
                return (
                  <a
                    key={p.id}
                    id={projectSwitcherOptionID(instanceID, key)}
                    href={projectSurfacePath(p.slug, "wiki", undefined, projectOrg)}
                    role="option"
                    aria-selected={p.slug === project.slug}
                    aria-label={`${p.name} ${p.slug} ${projectOrg}`}
                    className={`project-menu__option${p.slug === project.slug ? " is-selected" : ""}${itemIndex === activeIndex ? " is-active" : ""}`}
                    onMouseEnter={() => setActiveIndex(itemIndex)}
                  >
                    <span
                      aria-hidden
                      className="project-menu__color"
                      style={{ background: p.color || "var(--accent)" }}
                    />
                    <span className="project-menu__name">
                      {p.name}
                    </span>
                    <span className="project-menu__slug">
                      {p.slug}
                    </span>
                  </a>
                );
              })}
            </div>
          ))}
          {projects && projects.length === 0 && !projectCreateAllowed && (
            <div className="project-menu__state">
              {t("nav.no_other_projects")}
            </div>
          )}
          {projectCreateAllowed && (
            <a
              id={projectSwitcherOptionID(instanceID, "create-project")}
              href="/projects/new?welcome=1"
              role="option"
              aria-selected={false}
              className={`project-menu__option project-menu__option--create${activeItem?.kind === "create" ? " is-active" : ""}`}
              onMouseEnter={() => setActiveIndex(actionItems.length - 1)}
            >
              <span className="project-menu__create-title">
                {t("nav.new_project_hint_title")}
              </span>
              <span className="project-menu__create-body">
                {t("nav.new_project_hint_body")}
              </span>
            </a>
          )}
        </div>
      )}
    </div>
  );
}
