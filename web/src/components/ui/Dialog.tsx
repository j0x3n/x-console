import { useEffect, type ReactNode } from "react";

interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  wide?: boolean;
  children: ReactNode;
  footer?: ReactNode;
}

/** 通用弹窗。点遮罩或按 Esc 关闭。 */
export default function Dialog({
  open,
  onClose,
  title,
  description,
  wide,
  children,
  footer,
}: DialogProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div
      className="modal-backdrop"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <div
        className={`xc-dialog${wide ? " wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <h2>{title}</h2>
        {description && <p>{description}</p>}
        {children}
        {footer && <div className="xc-dialog-actions">{footer}</div>}
      </div>
    </div>
  );
}
