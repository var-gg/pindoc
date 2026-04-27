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

export function SurfaceHeader({ name, count, secondary }: SurfaceHeaderProps) {
  const surfaceName = name.trim().toLowerCase();
  return (
    <header className="surface-header">
      <div className="surface-header__chip">surface</div>
      <div className="surface-header__line">
        <h1 className="surface-header__name">{surfaceName}</h1>
        <span className="surface-header__count" aria-label={`${surfaceName} count ${count}`}>
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
