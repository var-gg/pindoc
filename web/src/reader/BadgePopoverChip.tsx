import { useEffect, useId, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router";
import { BookOpen, ListFilter } from "lucide-react";
import { useI18n } from "../i18n";
import { Tooltip } from "./Tooltip";

type Props = {
  label: ReactNode;
  description: string;
  className: string;
  style?: CSSProperties;
  onApply?: () => void;
  applyLabel?: string;
  legendHref?: string;
};

export function BadgePopoverChip({
  label,
  description,
  className,
  style,
  onApply,
  applyLabel,
  legendHref,
}: Props) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const id = useId();

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      setOpen(false);
    }
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [open]);

  return (
    <span className="badge-chip-wrap">
      <Tooltip content={description}>
        <button
          type="button"
          className={`${className} badge-popover-chip`}
          style={style}
          aria-expanded={open}
          aria-controls={open ? id : undefined}
          onClick={() => setOpen((v) => !v)}
        >
          {label}
        </button>
      </Tooltip>
      {open && (
        <div id={id} className="badge-popover" role="dialog" aria-label={t("reader.badge_popover_label")}>
          <div className="badge-popover__title">{label}</div>
          <p>{description}</p>
          <div className="badge-popover__actions">
            {onApply && (
              <button
                type="button"
                className="badge-popover__action"
                onClick={() => {
                  onApply();
                  setOpen(false);
                }}
              >
                <ListFilter className="lucide" />
                {applyLabel ?? t("reader.badge_filter_apply")}
              </button>
            )}
            {legendHref && (
              <Link to={legendHref} className="badge-popover__action">
                <BookOpen className="lucide" />
                {t("reader.badge_filter_legend")}
              </Link>
            )}
          </div>
        </div>
      )}
    </span>
  );
}
