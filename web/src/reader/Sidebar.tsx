import { useEffect, useMemo, useState } from "react";
import { Check, ChevronDown, ChevronRight, FolderOpen, LayoutTemplate } from "lucide-react";
import type { Area, ArtifactReadState, ArtifactRef, Project } from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { compareAreas, isFixedTaxonomyArea, localizedAreaName } from "./areaLocale";
import type { Aggregate } from "./useReaderData";
import { visualArea, visualDescription, visualLabel, visualType } from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";
import { dismissTooltipsForModal, Tooltip } from "./Tooltip";
import { sidebarAgentRows } from "./readerInternalVisibility";
import { buildAreaUnreadOwnCounts, subtreeUnreadCount } from "./sidebarUnread";
import { ProjectSwitcher } from "./ProjectSwitcher";

type Props = {
  project: Project;
  projectSlug: string;
  orgSlug: string;
  artifacts: ArtifactRef[];
  areas: Area[];
  types: Aggregate[];
  agents: Aggregate[];
  selectedArea: string | null;
  onSelectArea: (slug: string | null) => void;
  selectedType: string | null;
  onSelectType: (t: string | null) => void;
  // typeFilterLocked hides the Type section when the current Surface
  // already pins Type to a single value (Tasks Surface = Task). The label
  // falls back to a muted "Type · Task" chip so users still see why the
  // filter is gone. Decision `decision-reader-ia-hierarchy` §Surface 집합.
  typeFilterLocked?: boolean;
  open: boolean;
  showTemplates: boolean;
  onToggleTemplates: () => void;
  showInternalAgents?: boolean;
  showProjectSwitcher: boolean;
  projectCreateAllowed: boolean;
  readStates?: ArtifactReadState[] | null;
};

// AreaNode is the tree-enriched Area: same fields + resolved children.
type AreaNode = Area & { children: AreaNode[] };

// buildAreaTree turns a flat list into a tree by parent_slug. Areas whose
// parent_slug is unknown (or empty) are roots. Top-level rows follow the
// canonical taxonomy order from docs/19-area-taxonomy.md; subareas stay
// alphabetical so project-specific additions remain predictable.
function buildAreaTree(areas: Area[]): AreaNode[] {
  const bySlug = new Map<string, AreaNode>();
  for (const a of areas) {
    bySlug.set(a.slug, { ...a, children: [] });
  }
  const roots: AreaNode[] = [];
  bySlug.forEach((node) => {
    const parent = node.parent_slug ? bySlug.get(node.parent_slug) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  });
  const sortTree = (nodes: AreaNode[]) => {
    nodes.sort(compareAreas);
    nodes.forEach((n) => sortTree(n.children));
  };
  sortTree(roots);
  return roots;
}

function subtreeArtifactCount(node: AreaNode): number {
  return node.artifact_count + node.children.reduce((sum, child) => sum + subtreeArtifactCount(child), 0);
}

function areaNodeTitle(node: AreaNode, subtreeCount: number, taxonomyHint: string): string | undefined {
  if (node.children.length === 0) return [taxonomyHint, node.description].filter(Boolean).join("\n") || undefined;
  const childCount = subtreeCount - node.artifact_count;
  const countSummary = node.artifact_count > 0
    ? `직접: ${node.artifact_count} / 자식: ${childCount} / 합계: ${subtreeCount}`
    : `직접: 0 / 자식: ${childCount}`;
  return [taxonomyHint, node.description, countSummary].filter(Boolean).join("\n") || undefined;
}

function containsSelected(node: AreaNode, selectedArea: string | null): boolean {
  if (!selectedArea) return false;
  if (node.slug === selectedArea) return true;
  return node.children.some((child) => containsSelected(child, selectedArea));
}

export function Sidebar({
  project,
  projectSlug,
  orgSlug,
  artifacts,
  areas,
  types,
  agents,
  selectedArea,
  onSelectArea,
  selectedType,
  onSelectType,
  typeFilterLocked = false,
  open,
  showTemplates,
  onToggleTemplates,
  showInternalAgents = false,
  showProjectSwitcher,
  projectCreateAllowed,
  readStates = null,
}: Props) {
  const { t, lang } = useI18n();
  const [projectSwitcherOpen, setProjectSwitcherOpen] = useState(false);

  const regular = areas.filter((a) => !a.is_cross_cutting);
  const crossCutting = areas.filter((a) => a.is_cross_cutting);
  const tree = buildAreaTree(regular);
  const crossCuttingTree = buildAreaTree(crossCutting);
  const visibleAgents = sidebarAgentRows(agents, showInternalAgents);
  const unreadOwnCounts = useMemo(
    () => readStates ? buildAreaUnreadOwnCounts(artifacts, readStates) : new Map<string, number>(),
    [artifacts, readStates],
  );

  return (
    <aside className={`sidebar${open ? " open" : ""}`}>
      <div className="sidebar-scope" aria-label={t("sidebar.scope_label")}>
        <span className="sidebar-scope__label">{t("sidebar.scope_label")}</span>
        <span className="sidebar-scope__path">/{orgSlug}/p/{projectSlug}</span>
      </div>
      {showProjectSwitcher ? (
        <ProjectSwitcher
          project={project}
          orgSlug={orgSlug}
          open={projectSwitcherOpen}
          onOpenChange={(next) => {
            if (next) dismissTooltipsForModal();
            setProjectSwitcherOpen(next);
          }}
          showHiddenProjects={showInternalAgents}
          projectCreateAllowed={projectCreateAllowed}
          placement="sidebar"
        />
      ) : projectCreateAllowed ? (
        <a className="sidebar-project-create" href="/projects/new?welcome=1">
          <span className="sidebar-project-create__title">{t("nav.new_project_hint_title")}</span>
          <span className="sidebar-project-create__body">{t("nav.new_project_hint_body")}</span>
        </a>
      ) : null}
      <div className="side-section">{t("wiki.section_areas")}</div>
      <button
        type="button"
        className={`side-item${selectedArea === null ? " active" : ""}`}
        onClick={() => onSelectArea(null)}
      >
        <FolderOpen className="lucide" />
        <span>{t("wiki.area_all")}</span>
      </button>
      {tree.map((node) => (
        <AreaTreeNode
          key={node.id}
          node={node}
          level={0}
          selectedArea={selectedArea}
          onSelectArea={onSelectArea}
          t={t}
          lang={lang}
          unreadOwnCounts={unreadOwnCounts}
        />
      ))}
      {crossCuttingTree.length > 0 && (
        <>
          <div className="side-section" style={{ marginTop: 12 }}>
            {localizedAreaName(t, "cross-cutting", "Cross-cutting")}
          </div>
          {crossCuttingTree.map((node) => (
            <AreaTreeNode
              key={node.id}
              node={node}
              level={0}
              selectedArea={selectedArea}
              onSelectArea={onSelectArea}
              t={t}
              lang={lang}
              unreadOwnCounts={unreadOwnCounts}
            />
          ))}
        </>
      )}

      {typeFilterLocked ? (
        <>
          <div className="side-section" style={{ marginTop: 12 }}>
            {t("sidebar.types")}
          </div>
          <Tooltip content={t("sidebar.type_locked_hint")}>
            <div
              className="side-item"
              style={{ color: "var(--fg-3)", cursor: "default" }}
            >
              <Check className="lucide" />
              <span>{t("nav.tasks")}</span>
              <span className="side-item__count">{t("sidebar.type_locked_badge")}</span>
            </div>
          </Tooltip>
        </>
      ) : (
        types.length > 0 && (
          <>
            <div className="side-section" style={{ marginTop: 12 }}>
              {t("sidebar.types")}
            </div>
            {types.map(({ key, count }) => {
              const typeVisual = visualType(key);
              const Icon = visualIconComponent(typeVisual?.icon);
              const label = typeVisual ? visualLabel(typeVisual, lang) : key;
              const title = typeVisual ? visualDescription(typeVisual, lang) : t("sidebar.type_fixed_hint");
              return (
                <Tooltip key={key} content={title}>
                  <button
                    type="button"
                    className={`side-item${selectedType === key ? " active" : ""}`}
                    onClick={() => onSelectType(selectedType === key ? null : key)}
                  >
                    <Icon className="lucide" />
                    <span className="side-item__label">{label}</span>
                    <span className="side-item__count">{count}</span>
                  </button>
                </Tooltip>
              );
            })}
          </>
        )
      )}

      <div className="side-section" style={{ marginTop: 12 }}>
        {t("sidebar.view")}
      </div>
      <Tooltip content={t("sidebar.templates_hint")}>
        <button
          type="button"
          className={`side-item${showTemplates ? " active" : ""}`}
          onClick={onToggleTemplates}
        >
          <LayoutTemplate className="lucide" />
          <span>{t("sidebar.templates")}</span>
        </button>
      </Tooltip>

      {visibleAgents.length > 0 && (
        <>
          <div className="side-section" style={{ marginTop: 12 }}>
            {t("sidebar.agents")}
          </div>
          {visibleAgents.map((row) => {
            const av = agentAvatar(row.avatarKey);
            const label = row.labelKey ? t(row.labelKey) : row.key;
            return (
              <div className="side-item" key={row.key} role="listitem">
                <span
                  className={av.className}
                  style={{ width: 14, height: 14, fontSize: 8 }}
                >
                  {av.initials}
                </span>
                <span className="side-item__label">{label}</span>
                <span className="side-item__count">{row.count}</span>
              </div>
            );
          })}
        </>
      )}
    </aside>
  );
}

function AreaTreeNode({
  node,
  level,
  selectedArea,
  onSelectArea,
  t,
  lang,
  unreadOwnCounts,
}: {
  node: AreaNode;
  level: number;
  selectedArea: string | null;
  onSelectArea: (slug: string | null) => void;
  t: (key: string, ...args: Array<string | number>) => string;
  lang: string;
  unreadOwnCounts: ReadonlyMap<string, number>;
}) {
  const selectedInside = containsSelected(node, selectedArea);
  // Default closed keeps the eight-domain taxonomy scannable; selected
  // descendants auto-open so URL-restored filters remain visible.
  const [expanded, setExpanded] = useState(() => selectedInside);
  const hasChildren = node.children.length > 0;
  const active = selectedArea === node.slug;
  const subtreeCount = subtreeArtifactCount(node);
  const unreadCount = subtreeUnreadCount(node, unreadOwnCounts);
  const empty = subtreeCount === 0;
  const indent = { paddingLeft: 8 + level * 14 } as React.CSSProperties;
  const fixed = isFixedTaxonomyArea(node.slug);
  const taxonomyHint = fixed ? t("sidebar.area_fixed_hint") : t("sidebar.area_user_promoted_hint");
  const areaVisual = visualArea(node.slug);
  const AreaIcon = visualIconComponent(areaVisual?.icon ?? (active ? "FolderOpen" : "Folder"));
  const areaLabel = areaVisual ? visualLabel(areaVisual, lang) : localizedAreaName(t, node.slug, node.name);
  const visualHint = areaVisual ? visualDescription(areaVisual, lang) : "";
  const areaTitle = areaNodeTitle(node, subtreeCount, [taxonomyHint, visualHint].filter(Boolean).join("\n"));
  const style = {
    ...indent,
    ...(areaVisual ? { "--area-color": `var(${areaVisual.color_token})` } : {}),
  } as React.CSSProperties & Record<"--area-color", string | undefined>;
  const toggleLabel = expanded ? t("sidebar.collapse_area", areaLabel) : t("sidebar.expand_area", areaLabel);

  useEffect(() => {
    if (selectedInside) setExpanded(true);
  }, [selectedInside]);

  return (
    <>
      <div className="side-item-row side-item-row--area" style={style}>
        {hasChildren ? (
          <button
            type="button"
            className="side-item__toggle"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-label={toggleLabel}
            title={toggleLabel}
          >
            {expanded ? <ChevronDown className="lucide" /> : <ChevronRight className="lucide" />}
          </button>
        ) : (
          <span className="side-item__toggle-placeholder" aria-hidden="true" />
        )}
        <Tooltip content={areaTitle}>
          <button
            type="button"
            className={`side-item side-item--area${level > 0 ? " side-item--area-child" : ""}${active ? " active" : ""}${empty ? " empty" : ""}`}
            onClick={() => onSelectArea(active ? null : node.slug)}
          >
            <AreaIcon className="lucide side-item__area-icon" />
            <span className="side-item__label">{areaLabel}</span>
            {!fixed && (
              <span
                className="side-item__taxonomy side-item__taxonomy--promoted"
                aria-label={taxonomyHint}
              />
            )}
            <span className="side-item__count">{subtreeCount}</span>
            {unreadCount > 0 && (
              <span className="side-item__unread">{t("sidebar.unread_count", unreadCount)}</span>
            )}
          </button>
        </Tooltip>
      </div>
      {hasChildren && expanded && node.children.map((child) => (
        <AreaTreeNode
          key={child.id}
          node={child}
          level={level + 1}
          selectedArea={selectedArea}
          onSelectArea={onSelectArea}
          t={t}
          lang={lang}
          unreadOwnCounts={unreadOwnCounts}
        />
      ))}
    </>
  );
}
