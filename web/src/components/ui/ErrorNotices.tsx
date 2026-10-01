import { useState } from "react";
import { AlertCircle, X } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { copyText, useErrorStore, type ErrorNotice } from "../../lib/errors";

/*
 * 报错提示（B41）：不自动消失，最多显示 3 条，新的在上面。
 * 正文默认两行，点开最多 12 行；“复制”复制完整文本。
 */
const VISIBLE = 3;

export function CopyButton({ text, label }: { text: string; label?: string }) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="error-notice-copy"
      onClick={async () => {
        if (await copyText(text)) {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 2000);
        }
      }}
    >
      {copied ? t("Copied") : t(label ?? "Copy")}
    </button>
  );
}

function Notice({ notice }: { notice: ErrorNotice }) {
  const t = useT();
  const dismiss = useErrorStore((s) => s.dismiss);
  const [open, setOpen] = useState(false);
  return (
    <div className="error-notice" role="alert">
      <span className="error-notice-icon">
        <AlertCircle size={16} />
      </span>
      <div className="error-notice-body">
        <div className="error-notice-head">
          <strong>{t(notice.title)}</strong>
          {notice.count > 1 && (
            <span className="error-notice-count">×{notice.count}</span>
          )}
        </div>
        <button
          type="button"
          className={"error-notice-text" + (open ? " is-open" : "")}
          aria-expanded={open}
          title={open ? t("Show less") : t("Show more")}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? notice.detail : notice.message}
        </button>
      </div>
      <div className="error-notice-actions">
        <CopyButton text={notice.detail} />
        <button
          type="button"
          className="error-notice-close"
          aria-label={t("Dismiss")}
          onClick={() => dismiss(notice.id)}
        >
          <X size={14} />
        </button>
      </div>
    </div>
  );
}

export default function ErrorNotices() {
  const t = useT();
  const notices = useErrorStore((s) => s.notices);
  const dismissAll = useErrorStore((s) => s.dismissAll);
  const [showAll, setShowAll] = useState(false);
  if (notices.length === 0) return null;
  const shown = showAll ? notices : notices.slice(0, VISIBLE);
  const hidden = notices.length - shown.length;
  return (
    <div className="error-notices" aria-live="assertive">
      {shown.map((n) => (
        <Notice key={n.id} notice={n} />
      ))}
      {(hidden > 0 || notices.length > 1) && (
        <div className="error-notices-more">
          {hidden > 0 ? (
            <button type="button" onClick={() => setShowAll(true)}>
              {t("{n} more errors").replace("{n}", String(hidden))}
            </button>
          ) : (
            <span />
          )}
          <button
            type="button"
            onClick={() => {
              dismissAll();
              setShowAll(false);
            }}
          >
            {t("Dismiss all")}
          </button>
        </div>
      )}
    </div>
  );
}
