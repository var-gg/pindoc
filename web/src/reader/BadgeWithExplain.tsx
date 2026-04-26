import type { CSSProperties, ReactNode } from "react";
import { Tooltip } from "./Tooltip";

type Props = {
  label: ReactNode;
  description?: ReactNode;
  className: string;
  style?: CSSProperties;
  children?: ReactNode;
};

export function BadgeWithExplain({ label, description, className, style, children }: Props) {
  return (
    <Tooltip content={description}>
      <span
        className={className}
        style={style}
        aria-label={typeof description === "string" ? description : undefined}
        tabIndex={description ? 0 : undefined}
      >
        {children ?? label}
      </span>
    </Tooltip>
  );
}
