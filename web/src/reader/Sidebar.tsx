import { useState } from "react";
import { ChevronDown, ChevronRight, Folder, FolderOpen, FileText, Zap, Bug, Book, BookOpen, Hash, Check, Code, LayoutTemplate } from "lucide-react";
import type { ComponentType } from "react";
import type { Area } from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { localizedAreaName } from "./areaLocale";
import type { Aggregate } from "./useReaderData";

type Props = {
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
};

// AreaNode is the tree-enriched Area: same fields + resolved children.
// Cross-cutting areas stay outside the tree (they are category-orthogonal
// by design — see docs/04-data-model §Area).
type AreaNode = Area & { children: AreaNode[] };

const TYPE_ICONS: Record<string, ComponentType<{ className?: string }>> = {
  Decision: FileText,
  Analysis: FileText,
  Debug: Bug,
  Flow: Zap,
  Task: Check,
  TC: Check,
  Glossary: BookOpen,
  Feature: Zap,
  APIEndpoint: Code,
  Screen: Book,
  DataModel: Hash,
};

// buildAreaTree turns a flat list into a tree by parent_slug. Areas whose
// parent_slug is unknown (or empty) are roots. Sorts siblings by slug at
// every level so the tree is deterministic across renders.
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
    nodes.sort((a, b) => a.slug.localeCompare(b.slug));
    nodes.forEach((n) => sortTree(n.children));
  };
  sortTree(roots);
  return roots;
}

export function Sidebar({
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
}: Props) {
  const { t } = useI18n();

  const regular = areas.filter((a) => !a.is_cross_cutting);
  const crossCutting = areas.filter((a) => a.is_cross_cutting);
  const tree = buildAreaTree(regular);

  return (
    <aside className={`sidebar${open ? " open" : ""}`}>
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
        />
      ))}
      {crossCutting.length > 0 && (
        <div className="side-sub">
          {crossCutting.map((a) => (
            <button
              type="button"
              key={a.id}
              className={`side-item${selectedArea === a.slug ? " active" : ""}`}
              onClick={() => onSelectArea(selectedArea === a.slug ? null : a.slug)}
              data-testid={`area-${a.slug}`}
              title={a.description || undefined}
            >
              <Folder className="lucide" />
              <span>{localizedAreaName(t, a.slug, a.name)}</span>
              <span className="side-item__count">{a.artifact_count}</span>
            </button>
          ))}
        </div>
      )}

      {typeFilterLocked ? (
        <>
          <div className="side-section" style={{ marginTop: 12 }}>
            {t("sidebar.types")}
          </div>
          <div
            className="side-item"
            style={{ color: "var(--fg-3)", cursor: "default" }}
            title={t("sidebar.type_locked_hint")}
          >
            <Check className="lucide" />
            <span>Task</span>
            <span className="side-item__count">{t("sidebar.type_locked_badge")}</span>
          </div>
        </>
      ) : (
        types.length > 0 && (
          <>
            <div className="side-section" style={{ marginTop: 12 }}>
              {t("sidebar.types")}
            </div>
            {types.map(({ key, count }) => {
              const Icon = TYPE_ICONS[key] ?? FileText;
              return (
                <button
                  type="button"
                  key={key}
                  className={`side-item${selectedType === key ? " active" : ""}`}
                  onClick={() => onSelectType(selectedType === key ? null : key)}
                >
                  <Icon className="lucide" />
                  <span>{key}</span>
                  <span className="side-item__count">{count}</span>
                </button>
              );
            })}
          </>
        )
      )}

      <div className="side-section" style={{ marginTop: 12 }}>
        {t("sidebar.view")}
      </div>
      <button
        type="button"
        className={`side-item${showTemplates ? " active" : ""}`}
        onClick={onToggleTemplates}
        title={t("sidebar.templates_hint")}
      >
        <LayoutTemplate className="lucide" />
        <span>{t("sidebar.templates")}</span>
      </button>

      {agents.length > 0 && (
        <>
          <div className="side-section" style={{ marginTop: 12 }}>
            {t("sidebar.agents")}
          </div>
          {agents.map(({ key, count }) => {
            const av = agentAvatar(key);
            return (
              <div className="side-item" key={key} role="listitem">
                <span
                  className={av.className}
                  style={{ width: 14, height: 14, fontSize: 8 }}
                >
                  {av.initials}
                </span>
                <span>{key}</span>
                <span className="side-item__count">{count}</span>
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
}: {
  node: AreaNode;
  level: number;
  selectedArea: string | null;
  onSelectArea: (slug: string | null) => void;
  t: (key: string) => string;
}) {
  // Default to expanded: Pindoc trees are shallow (2-3 levels), so
  // collapse-by-default hides more than it helps on first render.
  const [expanded, setExpanded] = useState(true);
  const hasChildren = node.children.length > 0;
  const active = selectedArea === node.slug;
  const indent = { paddingLeft: 8 + level * 14 } as React.CSSProperties;

  return (
    <>
      <button
        type="button"
        className={`side-item${active ? " active" : ""}`}
        style={indent}
        onClick={() => onSelectArea(active ? null : node.slug)}
        title={node.description || undefined}
      >
        {hasChildren ? (
          <span
            role="button"
            tabIndex={0}
            onClick={(e) => {
              e.stopPropagation();
              setExpanded((v) => !v);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                e.stopPropagation();
                setExpanded((v) => !v);
              }
            }}
            className="side-item__toggle"
            aria-label={expanded ? "collapse" : "expand"}
            style={{ display: "inline-flex", alignItems: "center" }}
          >
            {expanded ? <ChevronDown className="lucide" /> : <ChevronRight className="lucide" />}
          </span>
        ) : (
          <span style={{ width: 14, display: "inline-block" }} />
        )}
        {active ? <FolderOpen className="lucide" /> : <Folder className="lucide" />}
        <span>{localizedAreaName(t, node.slug, node.name)}</span>
        <span className="side-item__count">{node.artifact_count}</span>
      </button>
      {hasChildren && expanded && node.children.map((child) => (
        <AreaTreeNode
          key={child.id}
          node={child}
          level={level + 1}
          selectedArea={selectedArea}
          onSelectArea={onSelectArea}
          t={t}
        />
      ))}
    </>
  );
}
