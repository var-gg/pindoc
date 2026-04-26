import { useEffect, useState } from "react";
import { ChevronDown, ChevronRight, Folder, FolderOpen, FileText, Zap, Bug, Book, BookOpen, Hash, Check, Code, LayoutTemplate, Lock } from "lucide-react";
import type { ComponentType } from "react";
import type { Area } from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { compareAreas, isFixedTaxonomyArea, localizedAreaName } from "./areaLocale";
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
  const crossCuttingTree = buildAreaTree(crossCutting);

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
            />
          ))}
        </>
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
                  title={t("sidebar.type_fixed_hint")}
                >
                  <Icon className="lucide" />
                  <span className="side-item__label">{key}</span>
                  <Lock
                    className="side-item__taxonomy side-item__taxonomy--fixed"
                    aria-label={t("sidebar.type_fixed_hint")}
                  />
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
  const selectedInside = containsSelected(node, selectedArea);
  // Default closed keeps the eight-domain taxonomy scannable; selected
  // descendants auto-open so URL-restored filters remain visible.
  const [expanded, setExpanded] = useState(() => selectedInside);
  const hasChildren = node.children.length > 0;
  const active = selectedArea === node.slug;
  const subtreeCount = subtreeArtifactCount(node);
  const empty = subtreeCount === 0;
  const indent = { paddingLeft: 8 + level * 14 } as React.CSSProperties;
  const fixed = isFixedTaxonomyArea(node.slug);
  const taxonomyHint = fixed ? t("sidebar.area_fixed_hint") : t("sidebar.area_user_promoted_hint");

  useEffect(() => {
    if (selectedInside) setExpanded(true);
  }, [selectedInside]);

  return (
    <>
      <button
        type="button"
        className={`side-item${active ? " active" : ""}${empty ? " empty" : ""}`}
        style={indent}
        onClick={() => onSelectArea(active ? null : node.slug)}
        title={areaNodeTitle(node, subtreeCount, taxonomyHint)}
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
        <span className="side-item__label">{localizedAreaName(t, node.slug, node.name)}</span>
        {fixed ? (
          <Lock
            className="side-item__taxonomy side-item__taxonomy--fixed"
            aria-label={taxonomyHint}
          />
        ) : (
          <span
            className="side-item__taxonomy side-item__taxonomy--promoted"
            aria-label={taxonomyHint}
          />
        )}
        <span className="side-item__count">{subtreeCount}</span>
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
