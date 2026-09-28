import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  isNotLive,
  useAiSettings,
  useSaveAiSettings,
  type AiSettings,
} from "./api";
import "./i18n";
import "./assistant.css";
import { confirmAction } from "../../components/ui/ConfirmDialog";

export const MODELS = [
  { id: "claude-opus-5-5", label: "Claude Opus 5.5（默认，最强）" },
  { id: "claude-sonnet-5", label: "Claude Sonnet 5（更快、更便宜）" },
  { id: "claude-haiku-4-5-20251001", label: "Claude Haiku 4.5（最快）" },
];

/** 设置 → AI 助手：API Key、模型、写操作是否都要确认。 */
export default function AssistantSettingsTab() {
  const settings = useAiSettings();
  if (settings.isPending) return <Loading />;
  if (settings.isError)
    return isNotLive(settings.error) ? (
      <EmptyState title="AI 助手还没上线" />
    ) : (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  return (
    <div className="settings-grid">
      <SettingsForm initial={settings.data} />
    </div>
  );
}

function SettingsForm({ initial }: { initial: AiSettings }) {
  const t = useT();
  const save = useSaveAiSettings();
  const [apiKey, setApiKey] = useState("");
  const [model, setModel] = useState(initial.model);
  const [confirmAll, setConfirmAll] = useState(initial.confirmAllWrites);
  useEffect(() => {
    setModel(initial.model);
    setConfirmAll(initial.confirmAllWrites);
  }, [initial]);
  const custom = !MODELS.some((m) => m.id === model);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        apiKey: apiKey.trim() || undefined,
        model: model.trim(),
        confirmAllWrites: confirmAll,
      },
      {
        onSuccess: () => {
          setApiKey("");
          toast(t("Saved"));
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };
  const clearKey = async () =>
    (await confirmAction({
      title: t("Remove the API key?"),
      description: t("AI stops working until you add a key again."),
      confirmLabel: t("Remove"),
    })) &&
    save.mutate(
      { apiKey: "" },
      {
        onSuccess: () => toast(t("API key removed")),
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );

  return (
    <form className="xc-card" onSubmit={submit}>
      <div className="xc-card-head">
        <h2>{t("AI assistant")}</h2>
        <span className={`xc-badge ${initial.hasApiKey ? "ok" : ""}`}>
          {initial.hasApiKey ? t("Key saved") : t("No key yet")}
        </span>
      </div>
      <label className="xc-field">
        <span>Anthropic API Key</span>
        <input
          className="xc-input"
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={
            initial.hasApiKey ? t("leave empty to keep") : "sk-ant-..."
          }
          autoComplete="new-password"
        />
        <small>
          在 console.anthropic.com 创建。加密保存在服务器上，不会再显示。
        </small>
      </label>
      <label className="xc-field">
        <span>{t("Model")}</span>
        <select
          className="xc-select"
          value={custom ? "custom" : model}
          onChange={(e) =>
            setModel(e.target.value === "custom" ? "" : e.target.value)
          }
        >
          {MODELS.map((m) => (
            <option key={m.id} value={m.id}>
              {m.label}
            </option>
          ))}
          <option value="custom">{t("Other model")}</option>
        </select>
      </label>
      {custom && (
        <label className="xc-field">
          <span>{t("Model ID")}</span>
          <input
            className="xc-input xc-mono"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="claude-..."
            required
          />
        </label>
      )}
      <label className="xc-check">
        <input
          type="checkbox"
          checked={confirmAll}
          onChange={(e) => setConfirmAll(e.target.checked)}
        />
        <span>{t("Confirm every write")}</span>
      </label>
      <small className="xc-check-hint">
        关着时，新建和修改直接执行，删除和高危操作先问你。打开后所有改动都先问你。
      </small>
      <div className="ai-form-actions">
        {initial.hasApiKey && (
          <button
            type="button"
            className="xc-btn ghost"
            onClick={clearKey}
            disabled={save.isPending}
          >
            {t("Remove API key")}
          </button>
        )}
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
    </form>
  );
}
