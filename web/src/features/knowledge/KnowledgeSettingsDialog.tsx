import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useClearKnowledge,
  usePutKnowledgeSettings,
  type KnowledgeSettings,
} from "./api";
import { SOURCE_LABELS } from "./format";

const ALL = ["notes", "readlater", "journal", "cards"];

/** 资料问答设置：开关、嵌入模型、要建索引的内容、清空索引。 */
export default function KnowledgeSettingsDialog({
  open,
  onClose,
  settings,
}: {
  open: boolean;
  onClose: () => void;
  settings?: KnowledgeSettings;
}) {
  const t = useT();
  const save = usePutKnowledgeSettings();
  const clear = useClearKnowledge();
  const [form, setForm] = useState<KnowledgeSettings>({
    enabled: false,
    model: "",
    sources: ALL,
  });
  const [error, setError] = useState("");
  useEffect(() => {
    if (open && settings) {
      setForm(settings);
      setError("");
    }
  }, [open, settings]);

  const toggle = (name: string, on: boolean) =>
    setForm((f) => ({
      ...f,
      sources: on
        ? [...f.sources.filter((s) => s !== name), name]
        : f.sources.filter((s) => s !== name),
    }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    try {
      await save.mutateAsync(form);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const wipe = async () => {
    const ok = await confirmAction({
      title: t("Ask clear title"),
      description: t("Ask clear body"),
      confirmLabel: t("Ask clear index"),
    });
    if (!ok) return;
    clear.mutate(undefined, {
      onSuccess: () => toast(t("Saved")),
      onError: (err) => setError(errorMessage(err)),
    });
  };

  return (
    <Dialog open={open} onClose={onClose} title={t("Ask settings")}>
      <form onSubmit={submit} className="ask-settings">
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
          />
          <span>{t("Ask enable")}</span>
        </label>
        <p className="xc-muted ask-hint">{t("Ask enable hint")}</p>
        <label className="xc-field">
          <span>{t("Ask model")}</span>
          <input
            className="xc-input"
            value={form.model}
            maxLength={100}
            placeholder={t("Ask model hint")}
            onChange={(e) => setForm({ ...form, model: e.target.value })}
          />
        </label>
        <fieldset className="ask-sources">
          <legend>{t("Ask sources to index")}</legend>
          {ALL.map((name) => (
            <label key={name} className="xc-check">
              <input
                type="checkbox"
                checked={form.sources.includes(name)}
                onChange={(e) => toggle(name, e.target.checked)}
              />
              <span>{t(SOURCE_LABELS[name])}</span>
            </label>
          ))}
        </fieldset>
        {error && (
          <p className="xc-error" role="alert">
            {error}
          </p>
        )}
        <div className="xc-dialog-actions">
          <button
            type="button"
            className="xc-btn danger ask-clear"
            onClick={wipe}
          >
            {t("Ask clear index")}
          </button>
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
