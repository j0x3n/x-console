import { useEffect, useState } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { usePutWatchersSettings, useWatchersSettings } from "./api";
import { parseHosts } from "./lib";

/** 网页盯梢的设置：每天的 AI 花费上限、点之前要批准的按钮字样。 */
export default function SettingsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useWatchersSettings(open);
  const save = usePutWatchersSettings();
  const [budget, setBudget] = useState("1");
  const [words, setWords] = useState("");

  useEffect(() => {
    if (!open || !settings.data) return;
    setBudget(String(settings.data.budgetUsd));
    setWords(settings.data.dangerWords.join("，"));
  }, [open, settings.data]);

  const value = Number(budget);
  const valid = budget.trim() !== "" && value >= 0 && value <= 100;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Watcher settings")}
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={!valid || save.isPending || !settings.data}
            onClick={() =>
              save.mutate(
                { budgetUsd: value, dangerWords: parseHosts(words) },
                {
                  onSuccess: () => {
                    toast(t("Saved"));
                    onClose();
                  },
                  onError: (e) =>
                    toast({ message: errorMessage(e), tone: "error" }),
                },
              )
            }
          >
            {t("Save")}
          </button>
        </>
      }
    >
      <div className="watchers-form single">
        <div className="xc-field">
          <label htmlFor="watchers-budget">{t("Daily AI limit (USD)")}</label>
          <input
            id="watchers-budget"
            className="xc-input"
            inputMode="decimal"
            value={budget}
            onChange={(e) => setBudget(e.target.value)}
          />
          <small>
            所有网页盯梢一天最多花这么多。到了上限只按记下的步骤检查，不再叫
            AI。填 0 就完全不用 AI。
          </small>
        </div>
        <div className="xc-field">
          <label htmlFor="watchers-words">
            {t("Buttons that need approval")}
          </label>
          <input
            id="watchers-words"
            className="xc-input"
            value={words}
            onChange={(e) => setWords(e.target.value)}
          />
          <small>AI 要点的按钮上有这些字时，先发通知问你。用逗号分开。</small>
        </div>
      </div>
    </Dialog>
  );
}
