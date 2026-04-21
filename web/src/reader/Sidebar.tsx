import { Folder, FolderOpen, FileText, Zap, Bug, Book, BookOpen, Hash, Check, Code } from "lucide-react";
import type { ComponentType } from "react";
import type { Area } from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import type { Aggregate } from "./useReaderData";

type Props = {
  areas: Area[];
  types: Aggregate[];
  agents: Aggregate[];
  selectedArea: string | null;
  onSelectArea: (slug: string | null) => void;
  selectedType: string | null;
  onSelectType: (t: string | null) => void;
  open: boolean;
};
// Sidebar click behavior: selecting an area or type clears any open
// artifact so the filter effect is immediately visible in the list.
// See ReaderShell.handleSelectArea / handleSelectType.

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

export function Sidebar({
  areas,
  types,
  agents,
  selectedArea,
  onSelectArea,
  selectedType,
  onSelectType,
  open,
}: Props) {
  const { t } = useI18n();

  const regular = areas.filter((a) => !a.is_cross_cutting);
  const crossCutting = areas.filter((a) => a.is_cross_cutting);

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
      {regular.map((a) => (
        <button
          type="button"
          key={a.id}
          className={`side-item${selectedArea === a.slug ? " active" : ""}`}
          onClick={() => onSelectArea(selectedArea === a.slug ? null : a.slug)}
        >
          {selectedArea === a.slug ? <FolderOpen className="lucide" /> : <Folder className="lucide" />}
          <span>{a.name}</span>
          <span className="side-item__count">{a.artifact_count}</span>
        </button>
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
            >
              <Folder className="lucide" />
              <span>{a.name}</span>
              <span className="side-item__count">{a.artifact_count}</span>
            </button>
          ))}
        </div>
      )}

      {types.length > 0 && (
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
      )}

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
