import { useState, type FormEvent } from "react";
import { WandSparkles } from "lucide-react";
import Markdown from "../../../components/markdown/Markdown";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { useNoteAiTools } from "../api";

/*
 * B40：AI 润色。可以不填要求直接润色，也可以写一句要求。
 * 结果先预览，点“替换”才改正文。
 */
export default function PolishDialog({
  body,
  onApply,
  onClose,
}: {
  body: string;
  onApply: (body: string) => void;
  onClose: () => void;
}) {
  const t = useT();
  const { polish } = useNoteAiTools();
  const [prompt, setPrompt] = useState("");
  const [result, setResult] = useState<string | null>(null);

  const run = (e?: FormEvent) => {
    e?.preventDefault();
    if (polish.isPending) return;
    polish.mutate(
      { body, prompt: prompt.trim() || undefined },
      { onSuccess: (out) => setResult(out.body) },
    );
  };

  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={t("AI polish")}
      description={t("Leave it empty to fix wording and layout automatically.")}
    >
      <form className="notes-polish" onSubmit={run}>
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
          <div className="notes-polish-result" aria-label={t("Polished text")}>
            <Markdown className="notes-preview" source={result} />
          </div>
        )}
        <div className="xc-dialog-actions">
          <span className="xc-spacer" />
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          {result === null ? (
            <button className="xc-btn primary" disabled={polish.isPending}>
              <WandSparkles
                size={14}
                className={polish.isPending ? "notes-spin" : ""}
              />
              {polish.isPending ? t("Polishing…") : t("Start polishing")}
            </button>
          ) : (
            <>
              <button className="xc-btn" disabled={polish.isPending}>
                {polish.isPending ? t("Polishing…") : t("Polish again")}
              </button>
              <button
                type="button"
                className="xc-btn primary"
                disabled={polish.isPending || !result.trim()}
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
