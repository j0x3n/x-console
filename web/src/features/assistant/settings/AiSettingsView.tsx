import { formatRate } from "../usage";
import { Link } from "react-router";
import { useEffect, useState, type FormEvent } from "react";
import {
  AlertTriangle,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  Zap,
} from "lucide-react";
import { errorMessage } from "../../../api/client";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import Dialog from "../../../components/ui/Dialog";
import MoreMenu from "../../../components/ui/MoreMenu";
import { Segmented } from "../../../components/ui/Toolbar";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import {
  useAiModels,
  useAiUsage,
  useModelSettings,
  useProviderMutations,
  useSaveModelSettings,
  useSetModelSpec,
  type AiModel,
  type AiModelSettings,
  type AiProvider,
  type ModelRef,
  type ReasoningEffort,
} from "../api";
import ModelPicker from "./ModelPicker";
import NotesAiCard from "./NotesAiCard";
import MemoryCard from "./MemoryCard";
import ProviderDialog from "./ProviderDialog";
import {
  canUseTools,
  currentMonth,
  findModel,
  formatCost,
  formatTokens,
} from "./models";

const EFFORTS: { value: ReasoningEffort; label: string }[] = [
  { value: "off", label: "Off" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

/** 设置 → AI（B32）：供应商、两个模型、思考程度、用量、笔记。 */
export default function AiSettingsView({
  providers,
}: {
  providers: AiProvider[];
}) {
  const models = useAiModels();
  const settings = useModelSettings();
  return (
    <div className="settings-grid">
      <ProvidersCard providers={providers} />
      {settings.data && (
        <ModelsCard
          initial={settings.data}
          providers={providers}
          models={models.data ?? []}
        />
      )}
      <UsageCard />
      <MemoryCard />
      <NotesAiCard />
    </div>
  );
}

function ProvidersCard({ providers }: { providers: AiProvider[] }) {
  const t = useT();
  const language = useLanguage();
  const ops = useProviderMutations();
  const [editing, setEditing] = useState<AiProvider | "new" | null>(null);

  const test = (p: AiProvider) =>
    ops.test.mutate(p.id, {
      onSuccess: (r) =>
        toast({
          message: `${p.name}: ${r.message}`,
          tone: r.ok ? "ok" : "error",
        }),
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  const refresh = (p: AiProvider) =>
    ops.refresh.mutate(p.id, {
      onSuccess: (list) => toast(`${p.name}: ${list.length} ${t("models")}`),
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  const remove = async (p: AiProvider) =>
    (await confirmAction({
      title: `${t("Delete provider")}“${p.name}”？`,
      description: t("Models that use it are cleared from the settings."),
    })) &&
    ops.remove.mutate(p.id, {
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });

  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Providers")}</h2>
        <button className="xc-btn small" onClick={() => setEditing("new")}>
          <Plus size={14} /> {t("Add provider")}
        </button>
      </div>
      {providers.length === 0 ? (
        <p className="xc-muted">
          {t(
            "Add an OpenAI compatible provider, such as OpenAI, DeepSeek, OpenRouter or a local Ollama.",
          )}
        </p>
      ) : (
        <ul className="ai-providers">
          {providers.map((p) => (
            <li key={p.id}>
              <div className="ai-provider-main">
                <strong>{p.name}</strong>
                <span className="xc-mono xc-muted">{p.baseUrl}</span>
                <span className="ai-provider-meta">
                  {p.modelCount} {t("models")}
                  {p.modelsRefreshedAt &&
                    ` · ${t("Refreshed")} ${relativeTime(p.modelsRefreshedAt, language)}`}
                  {!p.hasApiKey && ` · ${t("No key")}`}
                  {p.apiStyle === "responses" && " · Responses"}
                </span>
                {p.lastError && (
                  <span className="xc-error-text ai-provider-error">
                    <AlertTriangle size={12} /> {p.lastError}
                  </span>
                )}
              </div>
              <button
                className="xc-btn small"
                disabled={ops.test.isPending}
                onClick={() => test(p)}
              >
                <Zap size={13} /> {t("Test")}
              </button>
              <MoreMenu
                label={`${t("More")}: ${p.name}`}
                title={p.name}
                items={[
                  {
                    key: "refresh",
                    label: t("Refresh model list"),
                    icon: <RefreshCw size={14} />,
                    onSelect: () => refresh(p),
                  },
                  {
                    key: "edit",
                    label: t("Edit"),
                    icon: <Pencil size={14} />,
                    onSelect: () => setEditing(p),
                  },
                  {
                    key: "delete",
                    label: t("Delete"),
                    icon: <Trash2 size={14} />,
                    danger: true,
                    onSelect: () => void remove(p),
                  },
                ]}
              />
            </li>
          ))}
        </ul>
      )}
      <ProviderDialog
        open={editing !== null}
        onClose={() => setEditing(null)}
        provider={editing === "new" || editing === null ? undefined : editing}
      />
    </section>
  );
}

function ModelsCard({
  initial,
  providers,
  models,
}: {
  initial: AiModelSettings;
  providers: AiProvider[];
  models: AiModel[];
}) {
  const t = useT();
  const save = useSaveModelSettings();
  const [fast, setFast] = useState<ModelRef | undefined>(initial.fast);
  const [agent, setAgent] = useState<ModelRef | undefined>(initial.agent);
  const [effort, setEffort] = useState<ReasoningEffort>(
    initial.reasoningEffort,
  );
  const [fastEffort, setFastEffort] = useState<ReasoningEffort>(
    initial.fastReasoningEffort,
  );
  const [confirmAll, setConfirmAll] = useState(initial.confirmAllWrites);
  const [spec, setSpec] = useState<AiModel | null>(null);
  useEffect(() => {
    setFast(initial.fast);
    setAgent(initial.agent);
    setEffort(initial.reasoningEffort);
    setFastEffort(initial.fastReasoningEffort);
    setConfirmAll(initial.confirmAllWrites);
  }, [initial]);

  const agentModel = findModel(models, agent);
  const noReasoning = agentModel?.reasoning === false;
  // 快速模型不选时用 Agent 模型。
  const fastModel = fast ? findModel(models, fast) : agentModel;
  const fastNoReasoning = fastModel?.reasoning === false;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        fast: fast ?? null,
        agent: agent ?? null,
        reasoningEffort: noReasoning ? "off" : effort,
        fastReasoningEffort: fastNoReasoning ? "off" : fastEffort,
        confirmAllWrites: confirmAll,
      },
      {
        onSuccess: () => toast(t("Saved")),
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  return (
    <form className="xc-card" onSubmit={submit}>
      <div className="xc-card-head">
        <h2>{t("Models")}</h2>
      </div>
      {initial.legacyAnthropic && (
        <p className="ai-notice">
          <AlertTriangle size={14} />
          {t(
            "AI now uses OpenAI compatible APIs. The old Anthropic settings are no longer used. Add a provider and choose models again.",
          )}
        </p>
      )}
      <div className="xc-field">
        <span>{t("Agent model")}</span>
        <ModelPicker
          label={t("Agent model")}
          value={agent}
          onChange={setAgent}
          models={models}
          providers={providers}
          disabled={(m) => (canUseTools(m) ? null : t("No tool calls"))}
          onFillSpec={setSpec}
        />
        <small>
          {t(
            "Used by the AI panel, automations and the server Agent tab. It must support tool calls.",
          )}
        </small>
      </div>
      <div className="xc-field">
        <span>{t("Reasoning effort")}</span>
        <div className={noReasoning ? "ai-disabled" : undefined}>
          <Segmented
            label={t("Reasoning effort")}
            value={noReasoning ? "off" : effort}
            onChange={(v) => !noReasoning && setEffort(v)}
            options={EFFORTS.map((o) => ({
              value: o.value,
              label: t(o.label),
            }))}
          />
        </div>
        {noReasoning && (
          <small>{t("This model does not support reasoning.")}</small>
        )}
        {initial.reasoningUnsupported && !noReasoning && (
          <small className="ai-warn">
            {t(
              "This API rejected the reasoning effort, so it is sent without it.",
            )}
          </small>
        )}
      </div>
      <div className="xc-field">
        <span>{t("Fast model")}</span>
        <ModelPicker
          label={t("Fast model")}
          value={fast}
          onChange={setFast}
          models={models}
          providers={providers}
          allowNone={t("Same as the Agent model")}
          onFillSpec={setSpec}
        />
        <small>
          {t("Used for note titles and tags, and to polish the daily brief.")}
        </small>
      </div>
      <div className="xc-field">
        <span>{t("Fast model reasoning effort")}</span>
        <div className={fastNoReasoning ? "ai-disabled" : undefined}>
          <Segmented
            label={t("Fast model reasoning effort")}
            value={fastNoReasoning ? "off" : fastEffort}
            onChange={(v) => !fastNoReasoning && setFastEffort(v)}
            options={EFFORTS.map((o) => ({
              value: o.value,
              label: t(o.label),
            }))}
          />
        </div>
        {fastNoReasoning ? (
          <small>{t("This model does not support reasoning.")}</small>
        ) : (
          <small>
            {t("Off is quickest. Turn it up for polishing long notes.")}
          </small>
        )}
      </div>
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
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
      <SpecDialog model={spec} onClose={() => setSpec(null)} />
    </form>
  );
}

/** 规格未知的模型，手动填上下文长度和能力。 */
function SpecDialog({
  model,
  onClose,
}: {
  model: AiModel | null;
  onClose: () => void;
}) {
  const t = useT();
  const setSpec = useSetModelSpec();
  const [context, setContext] = useState("");
  const [tools, setTools] = useState(true);
  const [reasoning, setReasoning] = useState(false);
  useEffect(() => {
    setContext(model?.contextWindow ? String(model.contextWindow) : "");
    setTools(model?.toolCall ?? true);
    setReasoning(model?.reasoning ?? false);
  }, [model]);
  const n = Number(context);
  return (
    <Dialog
      open={!!model}
      onClose={onClose}
      title={model?.id ?? ""}
      description={t(
        "models.dev has no spec for this model. Fill it in yourself.",
      )}
    >
      <label className="xc-field">
        <span>{t("Context length")}</span>
        <input
          className="xc-input"
          inputMode="numeric"
          value={context}
          onChange={(e) => setContext(e.target.value.replace(/\D/g, ""))}
          placeholder="128000"
        />
      </label>
      <label className="xc-check">
        <input
          type="checkbox"
          checked={tools}
          onChange={(e) => setTools(e.target.checked)}
        />
        <span>{t("Supports tool calls")}</span>
      </label>
      <label className="xc-check">
        <input
          type="checkbox"
          checked={reasoning}
          onChange={(e) => setReasoning(e.target.checked)}
        />
        <span>{t("Supports reasoning")}</span>
      </label>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={setSpec.isPending || !model}
          onClick={() =>
            model &&
            setSpec.mutate(
              {
                providerId: model.providerId,
                modelId: model.id,
                contextWindow: n > 0 ? n : undefined,
                toolCall: tools,
                reasoning,
              },
              {
                onSuccess: () => {
                  toast(t("Saved"));
                  onClose();
                },
                onError: (err) =>
                  toast({ message: errorMessage(err), tone: "error" }),
              },
            )
          }
        >
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

function UsageCard() {
  const t = useT();
  const month = currentMonth();
  const usage = useAiUsage(month);
  const data = usage.data;
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Usage this month")}</h2>
        {data?.cost !== undefined && (
          <span className="xc-badge">{formatCost(data.cost)}</span>
        )}
      </div>
      {usage.isError ? (
        <p className="xc-muted">{t("Usage is not available yet.")}</p>
      ) : !data ? (
        <p className="xc-muted">{t("Loading")}…</p>
      ) : data.calls === 0 ? (
        <p className="xc-muted">{t("No calls this month.")}</p>
      ) : (
        <>
          <dl className="ai-usage-sum">
            <div>
              <dt>{t("Calls")}</dt>
              <dd>{data.calls}</dd>
            </div>
            <div>
              <dt>{t("Input tokens")}</dt>
              <dd>{formatTokens(data.inputTokens)}</dd>
            </div>
            <div>
              <dt>{t("Output tokens")}</dt>
              <dd>{formatTokens(data.outputTokens)}</dd>
            </div>
            <div>
              <dt>{t("Cache hit rate")}</dt>
              <dd>{formatRate(data.cacheHitRate)}</dd>
            </div>
          </dl>
          <table className="ai-usage-table">
            <thead>
              <tr>
                <th>{t("Model")}</th>
                <th>{t("Calls")}</th>
                <th>Token</th>
                <th>{t("Cost")}</th>
              </tr>
            </thead>
            <tbody>
              {data.byModel.map((m) => (
                <tr key={`${m.providerId}-${m.model}`}>
                  <td>
                    <span className="xc-mono">{m.model}</span>
                    <small className="xc-muted">{m.providerName}</small>
                  </td>
                  <td>{m.calls}</td>
                  <td>
                    {formatTokens(m.inputTokens)} /{" "}
                    {formatTokens(m.outputTokens)}
                  </td>
                  <td>{formatCost(m.cost) || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <small className="xc-muted">
            {t("Estimated from models.dev prices. Your bill may differ.")}
          </small>
          <Link className="xc-btn small ai-usage-link" to="/settings/ai-usage">
            {t("See usage details")}
          </Link>
        </>
      )}
    </section>
  );
}
