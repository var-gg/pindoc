import type { CSSProperties, ReactNode } from "react";
import { useI18n } from "../i18n";
import { typeChipClass } from "./typeChip";
import {
  visualArea,
  visualDescription,
  visualLabel,
  visualType,
} from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";
import { BadgeWithExplain } from "./BadgeWithExplain";
import { Tooltip } from "./Tooltip";

type ArtifactTypeChipProps = {
  type: string;
  compact?: boolean;
};

type AreaChipProps = {
  areaSlug: string;
  label: string;
  onClick?: () => void;
};

type TypeCountChipProps = {
  type: string;
  count: number;
};

export function ArtifactTypeChip({ type, compact }: ArtifactTypeChipProps) {
  const { lang } = useI18n();
  const entry = visualType(type);
  const Icon = visualIconComponent(entry?.icon);
  const label = entry ? visualLabel(entry, lang) : type;
  const description = entry ? visualDescription(entry, lang) : type;
  return (
    <BadgeWithExplain
      label={compact ? type : label}
      description={description}
      className={`${typeChipClass(type)} type-chip--visual${compact ? " type-chip--compact" : ""}`}
    >
      <Icon className="lucide" aria-hidden="true" />
      <span>{compact ? type : label}</span>
    </BadgeWithExplain>
  );
}

export function VisualAreaChip({ areaSlug, label, onClick }: AreaChipProps) {
  const { lang } = useI18n();
  const entry = visualArea(areaSlug);
  const Icon = visualIconComponent(entry?.icon ?? "Folder");
  const visualLabelText = entry ? visualLabel(entry, lang) : label;
  const description = entry ? visualDescription(entry, lang) : visualLabelText;
  const style = entry
    ? ({ "--area-color": `var(${entry.color_token})` } as CSSProperties)
    : undefined;
  const className = `chip-area chip-area--visual${entry ? "" : " chip-area--custom"}`;
  const content: ReactNode = (
    <>
      <Icon className="lucide" aria-hidden="true" />
      <span>{visualLabelText}</span>
    </>
  );

  if (onClick) {
    return (
      <Tooltip content={description}>
        <button
          type="button"
          className={className}
          style={style}
          aria-label={description}
          onClick={onClick}
        >
          {content}
        </button>
      </Tooltip>
    );
  }

  return (
    <BadgeWithExplain label={visualLabelText} description={description} className={className} style={style}>
      {content}
    </BadgeWithExplain>
  );
}

export function TypeCountChip({ type, count }: TypeCountChipProps) {
  const { lang } = useI18n();
  const entry = visualType(type);
  const Icon = visualIconComponent(entry?.icon);
  const label = entry ? visualLabel(entry, lang) : type;
  const description = entry ? visualDescription(entry, lang) : label;
  return (
    <BadgeWithExplain
      label={`${label} ${count}`}
      description={description}
      className={`${typeChipClass(type)} type-chip--visual type-chip--count`}
    >
      <Icon className="lucide" aria-hidden="true" />
      <span>{shortTypeLabel(type)}</span>
      {count > 1 && <span className="type-chip__count">{count}</span>}
    </BadgeWithExplain>
  );
}

function shortTypeLabel(type: string): string {
  const entry = visualType(type);
  const canonical = entry?.canonical ?? type;
  switch (canonical) {
    case "APIEndpoint":
      return "API";
    case "DataModel":
      return "DM";
    case "TC":
      return "TC";
    default:
      return canonical.slice(0, 1).toUpperCase();
  }
}
