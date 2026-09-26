import { AlertCircle, Check, X } from "lucide-react";
import type { ToastNotification } from "../../types/domain";
import { useT } from "../../contexts/LanguageContext";

interface ToastProps {
  toast: ToastNotification;
  hideToast: () => void;
}

export default function Toast({ toast, hideToast }: ToastProps) {
  const t = useT();
  return (
    <div
      className={"toast" + (toast.closing ? " is-closing" : "")}
      role="status"
    >
      <span className="toast-check">
        {toast.tone === "error" ? (
          <AlertCircle size={14} />
        ) : (
          <Check size={14} />
        )}
      </span>
      <div className="toast-copy">
        <strong>{toast.message}</strong>
        {toast.subtitle && <small>{toast.subtitle}</small>}
      </div>
      {toast.onUndo && (
        <button
          className="toast-undo"
          onClick={() => {
            toast.onUndo?.();
            hideToast();
          }}
        >
          {t("Undo")}
        </button>
      )}
      <button
        className="toast-close"
        aria-label={t("Dismiss")}
        onClick={hideToast}
      >
        <X size={14} />
      </button>
    </div>
  );
}
