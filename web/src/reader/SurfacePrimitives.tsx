import { useI18n } from "../i18n";

type SurfaceHeaderProps = {
  name: string;
  count: number;
  secondary?: {
    label: string;
    count: number;
  };
};

type EmptyStateProps = {
  message: string;
  action?: {
    label: string;
    href?: string;
    onClick?: () => void;
  };
};

type TFn = (key: string, ...args: Array<string | number>) => string;

export function SurfaceHeader({ name, count, secondary }: SurfaceHeaderProps) {
  const { t } = useI18n();
  const surfaceName = surfaceDisplayName(name, t);
  return (
    <header className="surface-header">
      <div className="surface-header__chip">{t("surface.header_label")}</div>
      <div className="surface-header__line">
        <h1 className="surface-header__name">{surfaceName}</h1>
        <span className="surface-header__count" aria-label={t("surface.count_label", surfaceName, count)}>
          <span className="surface-header__dot">·</span>
          <span className="surface-header__number">{count}</span>
          {secondary && (
            <>
              <span className="surface-header__slash">/</span>
              <span className="surface-header__secondary-label">{secondary.label}</span>
              <span className="surface-header__number">{secondary.count}</span>
            </>
          )}
        </span>
      </div>
    </header>
  );
}

export function surfaceDisplayName(name: string, t: TFn): string {
  const surfaceKey = normalizeSurfaceName(name);
  const key = `surface.name.${surfaceKey}`;
  const translated = t(key);
  if (translated !== key) return translated;
  return humanizeSurfaceName(surfaceKey);
}

function normalizeSurfaceName(name: string): string {
  const raw = safeDecodeURIComponent(name).trim().toLowerCase();
  return raw || "surface";
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function humanizeSurfaceName(value: string): string {
  const words = value
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  return words || "surface";
}

export function EmptyState({ message, action }: EmptyStateProps) {
  return (
    <div className="empty-state">
      <div className="empty-state__message">{message}</div>
      {action && action.href ? (
        <a className="empty-state__action" href={action.href}>
          {action.label}
        </a>
      ) : action?.onClick ? (
        <button type="button" className="empty-state__action" onClick={action.onClick}>
          {action.label}
        </button>
      ) : action ? (
        <span className="empty-state__action empty-state__action--static">
          {action.label}
        </span>
      ) : null}
    </div>
  );
}
