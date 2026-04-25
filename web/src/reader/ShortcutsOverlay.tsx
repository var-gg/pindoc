import type { ComponentType, ReactNode } from "react";
import { Link } from "react-router";
import {
  ArrowLeftRight,
  ArrowUpDown,
  BadgeInfo,
  CircleHelp,
  CornerDownLeft,
  FileText,
  GitBranch,
  Keyboard,
  ListFilter,
  MousePointer2,
  Search,
  ShieldCheck,
  Tags,
  X,
} from "lucide-react";
import type { Artifact } from "../api/client";
import { useI18n } from "../i18n";
import type { BadgeFilter } from "./badgeFilters";
import { typeChipClass } from "./typeChip";

type ReaderView = "reader" | "inbox" | "graph" | "tasks";
type Icon = ComponentType<{ className?: string }>;
type TFn = (key: string, ...args: Array<string | number>) => string;

type ShortcutRow = {
  keys: string[];
  label: string;
  hint?: string;
  icon: Icon;
};

type ShortcutGroup = {
  title: string;
  rows: ShortcutRow[];
};

type SymbolRow = {
  icon: Icon;
  sample: ReactNode;
  label: string;
  hint: string;
};

type Props = {
  open: boolean;
  view: ReaderView;
  projectSlug: string;
  projectLocale: string;
  detail: Artifact | null;
  selectedArea: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
  areaNameBySlug: ReadonlyMap<string, string>;
  onClose: () => void;
};

export function ShortcutsOverlay({
  open,
  view,
  projectSlug,
  projectLocale,
  detail,
  selectedArea,
  selectedType,
  badgeFilters,
  areaNameBySlug,
  onClose,
}: Props) {
  const { t } = useI18n();
  if (!open) return null;

  const surfaceTitle = surfaceLabel(view, t);
  const shortcutGroups = buildShortcutGroups(view, Boolean(detail), badgeFilters.length > 0 || Boolean(selectedArea || selectedType), t);
  const symbols = buildSymbols({
    view,
    detail,
    selectedArea,
    selectedType,
    badgeFilters,
    areaNameBySlug,
    t,
  });
  const legendHref = `/p/${projectSlug}/${projectLocale || "ko"}/wiki/visual-language-reference`;

  return (
    <div
      className="shortcuts-overlay"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <section className="shortcuts-panel" role="dialog" aria-modal="true" aria-labelledby="shortcuts-title">
        <header className="shortcuts-panel__head">
          <div className="shortcuts-panel__title">
            <CircleHelp className="lucide" />
            <div>
              <h2 id="shortcuts-title">{t("shortcuts.title")}</h2>
              <p>{t("shortcuts.surface", surfaceTitle)}</p>
            </div>
          </div>
          <button type="button" className="shortcuts-panel__close" onClick={onClose} aria-label={t("shortcuts.close")}>
            <X className="lucide" />
          </button>
        </header>

        <div className="shortcuts-panel__grid">
          <div className="shortcuts-section">
            <div className="shortcuts-section__head">
              <Keyboard className="lucide" />
              <span>{t("shortcuts.keyboard")}</span>
            </div>
            <div className="shortcuts-group-list">
              {shortcutGroups.map((group) => (
                <section key={group.title} className="shortcuts-group" aria-label={group.title}>
                  <h3>{group.title}</h3>
                  <div className="shortcut-row-list">
                    {group.rows.map((row) => (
                      <ShortcutItem key={`${group.title}-${row.keys.join("+")}-${row.label}`} row={row} />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          </div>

          <div className="shortcuts-section">
            <div className="shortcuts-section__head">
              <BadgeInfo className="lucide" />
              <span>{t("shortcuts.symbols")}</span>
            </div>
            <div className="symbol-row-list">
              {symbols.map((row) => (
                <SymbolItem key={row.label} row={row} />
              ))}
            </div>
          </div>
        </div>

        <footer className="shortcuts-panel__foot">
          <Link to={legendHref} onClick={onClose}>
            {t("shortcuts.legend_link")}
          </Link>
          <span>
            <span className="kbd">esc</span> {t("cmdk.close")}
          </span>
        </footer>
      </section>
    </div>
  );
}

function ShortcutItem({ row }: { row: ShortcutRow }) {
  const Icon = row.icon;
  return (
    <div className="shortcut-row">
      <div className="shortcut-row__keys">
        {row.keys.map((key) => (
          <span key={key} className="kbd">{key}</span>
        ))}
      </div>
      <Icon className="lucide" />
      <div className="shortcut-row__body">
        <strong>{row.label}</strong>
        {row.hint && <span>{row.hint}</span>}
      </div>
    </div>
  );
}

function SymbolItem({ row }: { row: SymbolRow }) {
  const Icon = row.icon;
  return (
    <div className="symbol-row">
      <div className="symbol-row__sample">{row.sample}</div>
      <Icon className="lucide" />
      <div className="symbol-row__body">
        <strong>{row.label}</strong>
        <span>{row.hint}</span>
      </div>
    </div>
  );
}

function buildShortcutGroups(
  view: ReaderView,
  hasDetail: boolean,
  hasFilters: boolean,
  t: TFn,
): ShortcutGroup[] {
  const groups: ShortcutGroup[] = [
    {
      title: t("shortcuts.group_global"),
      rows: [
        {
          keys: ["?"],
          label: t("shortcuts.toggle.label"),
          hint: t("shortcuts.toggle.hint"),
          icon: CircleHelp,
        },
        {
          keys: ["⌘K"],
          label: t("shortcuts.search.label"),
          hint: t("shortcuts.search.hint"),
          icon: Search,
        },
        {
          keys: ["esc"],
          label: t("shortcuts.escape.label"),
          hint: t("shortcuts.escape.hint"),
          icon: X,
        },
      ],
    },
  ];

  if (view === "reader") {
    groups.push({
      title: t("shortcuts.group_wiki"),
      rows: hasDetail
        ? [
            {
              keys: ["[", "]"],
              label: t("shortcuts.wiki_siblings.label"),
              hint: t("shortcuts.wiki_siblings.hint"),
              icon: ArrowLeftRight,
            },
          ]
        : [
            {
              keys: ["⌘K"],
              label: t("shortcuts.wiki_list.label"),
              hint: t("shortcuts.wiki_list.hint"),
              icon: FileText,
            },
          ],
    });
  } else if (view === "tasks") {
    groups.push({
      title: t("shortcuts.group_tasks"),
      rows: [
        {
          keys: ["↑", "↓"],
          label: t("shortcuts.tasks_select.label"),
          hint: t("shortcuts.tasks_select.hint"),
          icon: ArrowUpDown,
        },
        {
          keys: ["↵"],
          label: t("shortcuts.tasks_open.label"),
          hint: t("shortcuts.tasks_open.hint"),
          icon: CornerDownLeft,
        },
        {
          keys: ["space"],
          label: t("shortcuts.tasks_focus.label"),
          hint: t("shortcuts.tasks_focus.hint"),
          icon: MousePointer2,
        },
        ...(hasFilters
          ? [{
              keys: ["esc"],
              label: t("shortcuts.tasks_clear_filters.label"),
              hint: t("shortcuts.tasks_clear_filters.hint"),
              icon: ListFilter,
            }]
          : []),
      ],
    });
  } else if (view === "graph") {
    groups.push({
      title: t("shortcuts.group_graph"),
      rows: [
        {
          keys: ["⌘K"],
          label: t("shortcuts.graph_search.label"),
          hint: t("shortcuts.graph_search.hint"),
          icon: GitBranch,
        },
      ],
    });
  } else {
    groups.push({
      title: t("shortcuts.group_inbox"),
      rows: [
        {
          keys: ["⌘K"],
          label: t("shortcuts.inbox_search.label"),
          hint: t("shortcuts.inbox_search.hint"),
          icon: Search,
        },
      ],
    });
  }

  return groups;
}

function buildSymbols({
  view,
  detail,
  selectedArea,
  selectedType,
  badgeFilters,
  areaNameBySlug,
  t,
}: {
  view: ReaderView;
  detail: Artifact | null;
  selectedArea: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
  areaNameBySlug: ReadonlyMap<string, string>;
  t: TFn;
}): SymbolRow[] {
  const rows: SymbolRow[] = [];
  const type = detail?.type ?? selectedType ?? (view === "tasks" ? "Task" : "Artifact");
  const areaSlug = detail?.area_slug ?? selectedArea;
  const areaLabel = areaSlug ? areaNameBySlug.get(areaSlug) ?? areaSlug : t("wiki.area_all");

  rows.push({
    icon: Tags,
    sample: <span className={typeChipClass(type)}>{type}</span>,
    label: t("shortcuts.symbol_type.label"),
    hint: t("shortcuts.symbol_type.hint"),
  });
  rows.push({
    icon: ListFilter,
    sample: <span className="chip-area">{areaLabel}</span>,
    label: t("shortcuts.symbol_area.label"),
    hint: t("shortcuts.symbol_area.hint"),
  });

  if (detail?.artifact_meta || detail?.recent_warnings?.length) {
    rows.push({
      icon: ShieldCheck,
      sample: <span className="chip chip--trust chip--trust-neutral">{t("shortcuts.symbol_trust.sample")}</span>,
      label: t("shortcuts.symbol_trust.label"),
      hint: t("shortcuts.symbol_trust.hint"),
    });
  }

  if (view === "tasks" || detail?.type === "Task") {
    rows.push({
      icon: Keyboard,
      sample: <span className="status-pill status-pill--todo"><span className="p-dot" />open</span>,
      label: t("shortcuts.symbol_status.label"),
      hint: t("shortcuts.symbol_status.hint"),
    });
    rows.push({
      icon: BadgeInfo,
      sample: <span className="prio prio--p1"><span className="dot" />P1</span>,
      label: t("shortcuts.symbol_priority.label"),
      hint: t("shortcuts.symbol_priority.hint"),
    });
  }

  if ((detail?.relates_to?.length ?? 0) + (detail?.related_by?.length ?? 0) > 0) {
    rows.push({
      icon: GitBranch,
      sample: <span className="shortcut-symbol-icon"><GitBranch className="lucide" /></span>,
      label: t("shortcuts.symbol_relation.label"),
      hint: t("shortcuts.symbol_relation.hint"),
    });
  }

  if (badgeFilters.length > 0) {
    const first = badgeFilters[0];
    rows.push({
      icon: ListFilter,
      sample: (
        <span className="task-filter-chip">
          <span className="task-filter-chip__key">{first.key}</span>
          {first.label}
        </span>
      ),
      label: t("shortcuts.symbol_filter.label"),
      hint: t("shortcuts.symbol_filter.hint"),
    });
  }

  return rows;
}

function surfaceLabel(view: ReaderView, t: TFn): string {
  switch (view) {
    case "reader":
      return t("nav.wiki_reader");
    case "tasks":
      return t("nav.tasks");
    case "graph":
      return t("nav.graph");
    case "inbox":
      return t("nav.inbox");
  }
}
