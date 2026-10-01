import { useState, type FormEvent } from "react";
import { WandSparkles } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import Markdown from "./Markdown";
import { polishText, type PolishScene } from "./polish";

/*
 * AI 润色（B40 笔记先有，B56 改成公共的）。可以不填要求直接润色，
 * 也可以写一句要求。结果先预览，点“替换”才改原文。
 */
export default function PolishDialog({
  text,
  scene,
  onApply,
  onClose,
}: {
  text: string;
  /** 场景决定提示词：笔记偏好读，卡片还要专业简洁 */
  scene: PolishScene;
  onApply: (text: string) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [prompt, setPrompt] = useState("");
  const [result, setResult] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const run = async (e?: FormEvent) => {
    e?.preventDefault();
    // 弹窗常从别的表单里打开，提交事件不能冒泡到外层表单
    e?.stopPropagation();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      setResult(await polishText(scene, text, prompt.trim() || undefined));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={t("AI polish")}
      description={t("Leave it empty to fix wording and layout automatically.")}
    >
      <form className="xc-polish" onSubmit={run}>
        <label className="xc-field">
          <span>{t("Polish instructions")}</span>
          <input
            className="xc-input"
            value={prompt}
            maxLength={500}
            placeholder={t("Optional. For example: turn it into a bullet list")}
            onChange={(e) => setPrompt(e.target.value)}
            autoFocus
          />
        </label>
        {result !== null && (
          <div className="xc-polish-result" aria-label={t("Polished text")}>
            <Markdown source={result} />
          </div>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <span className="xc-spacer" />
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          {result === null ? (
            <button className="xc-btn primary" disabled={busy}>
              <WandSparkles size={14} className={busy ? "xc-spin" : ""} />
              {busy ? t("Polishing…") : t("Start polishing")}
            </button>
          ) : (
            <>
              <button className="xc-btn" disabled={busy}>
                {busy ? t("Polishing…") : t("Polish again")}
              </button>
              <button
                type="button"
                className="xc-btn primary"
                disabled={busy || !result.trim()}
                onClick={() => onApply(result)}
              >
                {t("Replace text")}
              </button>
            </>
          )}
        </div>
      </form>
    </Dialog>
  );
}
