import type * as Model from "../../types/domain";
import React from "react";
import AgentBadge from "./AgentBadge";
import { Check, X } from "lucide-react";

interface ToastProps {
  toast: Model.ToastNotification;
  hideToast: () => void;
  onUndoDecision: (key: string) => void;
  language: Model.Language;
}

export default function Toast({
  toast,
  hideToast,
  onUndoDecision,
  language,
}: ToastProps) {
  return (
    <div
      className={"toast" + (toast.closing ? " is-closing" : "")}
      role="status"
      key={toast.id}
    >
      {toast.agent ? (
        <AgentBadge name={toast.agent} size="small" />
      ) : (
        <span className="toast-check">
          <Check size={14} />
        </span>
      )}
      <div className="toast-copy">
        <strong>{toast.message}</strong>
        {toast.subtitle && <small>{toast.subtitle}</small>}
      </div>
      {(toast.undoKey || toast.onUndo) && (
        <button
          className="toast-undo"
          onClick={() => {
            if (toast.onUndo) {
              toast.onUndo();
              hideToast();
            } else if (toast.undoKey) onUndoDecision(toast.undoKey);
          }}
        >
          {language === "zh" ? "撤销" : "Undo"}
        </button>
      )}
      <button
        className="toast-close"
        aria-label={language === "zh" ? "关闭通知" : "Dismiss"}
        onClick={hideToast}
      >
        <X size={14} />
      </button>
    </div>
  );
}
