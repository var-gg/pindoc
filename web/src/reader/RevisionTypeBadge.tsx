import type { CSSProperties } from "react";
import type { RevisionType } from "../api/client";
import { useI18n } from "../i18n";
import { Tooltip } from "./Tooltip";

type Props = {
  revisionType?: RevisionType;
  compact?: boolean;
  style?: CSSProperties;
};

export function RevisionTypeBadge({ revisionType, compact = false, style }: Props) {
  const { t } = useI18n();
  if (!revisionType) return null;
  return (
    <Tooltip content={t(`revision_type.${revisionType}`)}>
      <span
        className={`chip chip--${revisionTypeChipClass(revisionType)}`}
        style={{
          height: compact ? 18 : 20,
          paddingInline: compact ? 6 : 8,
          fontSize: compact ? 10 : 11,
          ...style,
        }}
      >
        {t(`revision_type.${revisionType}`)}
      </span>
    </Tooltip>
  );
}

function revisionTypeChipClass(revisionType: RevisionType): string {
  switch (revisionType) {
    case "text_edit":
      return "live";
    case "acceptance_toggle":
      return "draft";
    case "meta_change":
      return "area";
    case "system_auto":
      return "archived";
    case "mixed":
      return "stale";
  }
}
