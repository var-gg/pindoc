import { type ReactNode, useEffect, useId, useRef } from "react";

type ConfirmDialogCheckbox = {
  checked: boolean;
  label: string;
  onChange: (checked: boolean) => void;
};

type ConfirmDialogProps = {
  open: boolean;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  confirmBusy?: boolean;
  checkbox?: ConfirmDialogCheckbox;
};

export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
  confirmBusy = false,
  checkbox,
}: ConfirmDialogProps) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement | null>(null);
  const cancelRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !confirmBusy) onCancel();
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      previous?.focus();
    };
  }, [confirmBusy, onCancel, open]);

  if (!open) return null;

  return (
    <div
      className="confirm-dialog"
      role="presentation"
      onMouseDown={(e) => {
        if (confirmBusy) return;
        if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
          onCancel();
        }
      }}
    >
      <div
        className="confirm-dialog__panel"
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <header className="confirm-dialog__header">
          <h2 id={titleId}>{title}</h2>
        </header>
        <div className="confirm-dialog__body">
          {typeof body === "string" ? <p>{body}</p> : body}
          {checkbox && (
            <label className="confirm-dialog__check">
              <input
                type="checkbox"
                checked={checkbox.checked}
                onChange={(e) => checkbox.onChange(e.currentTarget.checked)}
              />
              <span>{checkbox.label}</span>
            </label>
          )}
        </div>
        <footer className="confirm-dialog__actions">
          <button
            type="button"
            className="ops__secondary"
            ref={cancelRef}
            disabled={confirmBusy}
            onClick={onCancel}
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            className="ops__danger"
            disabled={confirmBusy}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </footer>
      </div>
    </div>
  );
}
