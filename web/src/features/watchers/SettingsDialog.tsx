import { useEffect, useState } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { Segmented } from "../../components/ui/Toolbar";
import { useAiModels, useAiProviders } from "../assistant/api";
import ModelPicker from "../assistant/settings/ModelPicker";
import {
  usePutWatchersSettings,
  useWatchersSettings,
  type WatcherEffort,
} from "./api";
import { parseHosts, parseModelRef } from "./lib";

const EFFORTS: { value: WatcherEffort; label: string }[] = [
  { value: "", label: "Follow" },
  { value: "off", label: "Off" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

/** 网站自动化的设置：每天的 AI 花费上限、点之前要批准的按钮字样、用哪个模型（B212）。 */
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
  const [ids, setIds] = useState<Record<string, string>>({});
  const models = useAiModels(open);
  const providers = useAiProviders(open);
  const [agentModel, setAgentModel] = useState("");
  const [agentEffort, setAgentEffort] = useState<WatcherEffort>("");
  const [judgeModel, setJudgeModel] = useState("");
  const [judgeEffort, setJudgeEffort] = useState<WatcherEffort>("");

  useEffect(() => {
    if (!open || !settings.data) return;
    setBudget(String(settings.data.budgetUsd));
    setWords(settings.data.dangerWords.join("，"));
    setIds(settings.data.xQueryIds ?? {});
    setAgentModel(settings.data.agentModel ?? "");
    setAgentEffort(settings.data.agentEffort ?? "");
    setJudgeModel(settings.data.judgeModel ?? "");
    setJudgeEffort(settings.data.judgeEffort ?? "");
  }, [open, settings.data]);

  const efforts = EFFORTS.map((o) => ({ value: o.value, label: t(o.label) }));
  const modelField = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    none: string,
    needsVision: boolean,
  ) => (
    <div className="xc-field">
      <span>{label}</span>
      <ModelPicker
        label={label}
        value={parseModelRef(value)}
        onChange={(v) => onChange(v ? `${v.providerId}:${v.model}` : "")}
        models={models.data ?? []}
        providers={providers.data ?? []}
        allowNone={none}
        disabled={(m) =>
          providers.data?.find((p) => p.id === m.providerId)?.apiStyle === "jev"
            ? t("Jev only classifies. It cannot chat.")
            : needsVision && m.toolCall === false
              ? t("No tool calls")
              : needsVision && m.imageInput === false
                ? t("No image input")
                : null
        }
      />
    </div>
  );

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
                {
                  budgetUsd: value,
                  dangerWords: parseHosts(words),
                  xQueryIds: ids,
                  agentModel,
                  agentEffort,
                  judgeModel,
                  judgeEffort,
                },
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
            所有网站自动化任务一天最多花这么多。到了上限只按记下的步骤检查，不再叫
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
        {modelField(
          t("Model for working the page"),
          agentModel,
          setAgentModel,
          t("Same as the Agent model"),
          true,
        )}
        <div className="xc-field">
          <span>{t("Reasoning effort")}</span>
          <Segmented
            label={t("Reasoning effort")}
            value={agentEffort}
            onChange={setAgentEffort}
            options={efforts}
          />
          <small>
            AI 摸索、每次都让 AI 看、读图形验证码用。要能看图和调工具。
          </small>
        </div>
        {modelField(
          t("Model for judging changes"),
          judgeModel,
          setJudgeModel,
          t("Same as the fast model"),
          false,
        )}
        <div className="xc-field">
          <span>{t("Reasoning effort")}</span>
          <Segmented
            label={t("Reasoning effort")}
            value={judgeEffort}
            onChange={setJudgeEffort}
            options={efforts}
          />
          <small>
            判断变化重不重要、筛选 X 推文用。“跟随”是用 设置 → AI 里的选择。
          </small>
        </div>
        <details className="watchers-ids">
          <summary>{t("X query ids")}</summary>
          <p className="xc-muted watchers-small">
            X 改版后读不到推文、报错说 queryId
            不对时，从浏览器开发者工具里找新的编号填进来。
          </p>
          {Object.keys(ids)
            .sort()
            .map((k) => (
              <div className="xc-field" key={k}>
                <label htmlFor={`watchers-id-${k}`}>{k}</label>
                <input
                  id={`watchers-id-${k}`}
                  className="xc-input"
                  value={ids[k]}
                  onChange={(e) => setIds({ ...ids, [k]: e.target.value })}
                />
              </div>
            ))}
        </details>
      </div>
    </Dialog>
  );
}
