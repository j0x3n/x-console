import { useState } from "react";
import { Link } from "react-router";
import { Quote } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import {
  usePreferencesStore,
  type QuoteMode,
} from "../../stores/preferences-store";
import { toast } from "../../hooks/useToast";
import { QUOTE_TAG } from "../overview/quote";

const MODES: { value: QuoteMode; label: string; hint: string }[] = [
  { value: "off", label: "Don't show", hint: "" },
  {
    value: "fixed",
    label: "Always the same",
    hint: "The first pinned quote. Without a pinned one, the newest.",
  },
  {
    value: "refresh",
    label: "A new one each time",
    hint: "Picked at random each time you open the today page.",
  },
  {
    value: "daily",
    label: "A new one each day",
    hint: "Picked at random once a day.",
  },
];

/**
 * B89：便签视图的“每日一句”按钮。今日页问候语后面显示一条带“名言”标签的便签。
 * 设置存在个人偏好里，换浏览器也一样。
 */
export default function QuoteButton({ count }: { count: number }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const mode = usePreferencesStore((s) => s.quoteMode);
  const setMode = usePreferencesStore((s) => s.setQuoteMode);
  return (
    <>
      <button
        type="button"
        className={`xc-btn small notes-quote-button${mode !== "off" ? " on" : ""}`}
        title={t("Daily quote")}
        aria-label={t("Daily quote")}
        onClick={() => setOpen(true)}
      >
        <Quote size={13} /> {t("Daily quote")}
      </button>
      {open && (
        <Dialog open onClose={() => setOpen(false)} title={t("Daily quote")}>
          <p className="xc-muted notes-quote-help">
            {t("Shown after the greeting on the today page.")}{" "}
            {t("Memos with the tag")} “{QUOTE_TAG}”：{count}
          </p>
          <div className="notes-quote-modes" role="radiogroup">
            {MODES.map((m) => (
              <label key={m.value} className="xc-check">
                <input
                  type="radio"
                  name="quote-mode"
                  checked={mode === m.value}
                  onChange={() => {
                    setMode(m.value);
                    toast(t("Saved"));
                  }}
                />
                <span>
                  {t(m.label)}
                  {m.hint && (
                    <small className="xc-check-hint">{t(m.hint)}</small>
                  )}
                </span>
              </label>
            ))}
          </div>
          <div className="xc-dialog-actions">
            <Link
              className="xc-btn"
              to={`/notes?tag=${encodeURIComponent(QUOTE_TAG)}`}
              onClick={() => setOpen(false)}
            >
              {t("See the quotes")}
            </Link>
            <button
              type="button"
              className="xc-btn primary"
              onClick={() => setOpen(false)}
            >
              {t("Close")}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}
