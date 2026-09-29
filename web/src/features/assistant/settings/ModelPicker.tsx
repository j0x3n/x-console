import { useState } from "react";
import { Brain, ChevronDown, ImageIcon, Wrench } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { SearchBox } from "../../../components/ui/Toolbar";
import { useT } from "../../../contexts/LanguageContext";
import type { AiModel, AiProvider, ModelRef } from "../api";
import {
  filterModels,
  findModel,
  formatContext,
  priceText,
  sameModel,
} from "./models";

/** 模型规格的小图标：工具调用、思考、图片输入。 */
export function ModelBadges({ model }: { model: AiModel }) {
  const t = useT();
  if (model.specSource === "unknown")
    return <span className="ai-model-unknown">{t("Spec unknown")}</span>;
  return (
    <span className="ai-model-badges">
      {model.contextWindow ? (
        <span title={t("Context length")}>
          {formatContext(model.contextWindow)}
        </span>
      ) : null}
      {model.toolCall && (
        <span title={t("Tool calls")} aria-label={t("Tool calls")}>
          <Wrench size={12} />
        </span>
      )}
      {model.reasoning && (
        <span title={t("Reasoning")} aria-label={t("Reasoning")}>
          <Brain size={12} />
        </span>
      )}
      {model.imageInput && (
        <span title={t("Image input")} aria-label={t("Image input")}>
          <ImageIcon size={12} />
        </span>
      )}
    </span>
  );
}

/**
 * 选模型：按钮显示当前的模型，点开是可以搜索的列表，按供应商分组。
 * disabled 返回原因时这一项标灰，比如 Agent 模型要支持工具调用。
 */
export default function ModelPicker({
  label,
  value,
  onChange,
  models,
  providers,
  disabled,
  allowNone,
  onFillSpec,
}: {
  label: string;
  value: ModelRef | undefined;
  onChange: (value: ModelRef | undefined) => void;
  models: AiModel[];
  providers: AiProvider[];
  disabled?: (model: AiModel) => string | null;
  /** 可以不选，比如快速模型不选时用 Agent 模型 */
  allowNone?: string;
  /** 规格未知的模型点“填写规格” */
  onFillSpec?: (model: AiModel) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const current = findModel(models, value);
  const providerName = (id: number) =>
    providers.find((p) => p.id === id)?.name ?? `#${id}`;
  const shown = filterModels(models, q);
  const groups = providers
    .map((p) => ({
      provider: p,
      models: shown.filter((m) => m.providerId === p.id),
    }))
    .filter((g) => g.models.length > 0);

  return (
    <>
      <button
        type="button"
        className="xc-select ai-model-button"
        aria-label={label}
        onClick={() => setOpen(true)}
      >
        {value ? (
          <>
            <span className="xc-mono">{value.model}</span>
            <span className="ai-model-provider">
              {providerName(value.providerId)}
            </span>
            {current && <ModelBadges model={current} />}
          </>
        ) : (
          <span className="xc-muted">{allowNone ?? t("Choose a model")}</span>
        )}
        <ChevronDown size={14} className="ai-model-caret" />
      </button>
      <Dialog open={open} onClose={() => setOpen(false)} title={label} wide>
        <SearchBox
          className="ai-model-search"
          value={q}
          onChange={setQ}
          placeholder={t("Search models")}
          clearLabel={t("Clear")}
        />
        <div className="ai-model-list" role="listbox" aria-label={label}>
          {allowNone && (
            <button
              type="button"
              role="option"
              aria-selected={!value}
              className={`ai-model-row${!value ? " on" : ""}`}
              onClick={() => {
                onChange(undefined);
                setOpen(false);
              }}
            >
              <span className="xc-muted">{allowNone}</span>
            </button>
          )}
          {groups.map((g) => (
            <section key={g.provider.id}>
              <h3>{g.provider.name}</h3>
              {g.models.map((m) => {
                const why = disabled?.(m) ?? null;
                const on = sameModel(value, m);
                return (
                  <div
                    key={m.id}
                    className={`ai-model-row${on ? " on" : ""}${why ? " off" : ""}`}
                  >
                    <button
                      type="button"
                      role="option"
                      aria-selected={on}
                      aria-disabled={!!why}
                      title={why ?? m.name ?? m.id}
                      onClick={() => {
                        if (why) return;
                        onChange({ providerId: m.providerId, model: m.id });
                        setOpen(false);
                      }}
                    >
                      <span className="ai-model-id xc-mono">{m.id}</span>
                      <ModelBadges model={m} />
                      <span className="ai-model-price">
                        {why ?? priceText(m)}
                      </span>
                    </button>
                    {m.specSource === "unknown" && onFillSpec && (
                      <button
                        type="button"
                        className="xc-btn ghost small"
                        onClick={() => onFillSpec(m)}
                      >
                        {t("Fill in spec")}
                      </button>
                    )}
                  </div>
                );
              })}
            </section>
          ))}
          {groups.length === 0 && (
            <p className="xc-muted ai-model-empty">
              {models.length === 0
                ? t("No models yet. Add a provider and refresh its models.")
                : t("No matching models")}
            </p>
          )}
        </div>
        <p className="xc-muted ai-model-note">
          {t("Prices are US dollars per million tokens, from models.dev.")}
        </p>
      </Dialog>
    </>
  );
}
