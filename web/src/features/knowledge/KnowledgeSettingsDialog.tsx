import { FolderOpen, Plus, X } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useAiProviders } from "../assistant/api";
import FolderPicker from "../music/FolderPicker";
import {
  useClearKnowledge,
  usePutKnowledgeSettings,
  type KnowledgeSettings,
} from "./api";
import { SOURCE_LABELS } from "./format";

const ALL = ["notes", "readlater", "journal", "cards"];

/** 资料问答设置：开关、嵌入用的供应商和模型、要建索引的内容、清空索引。 */
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
  const providers = useAiProviders(open);
  const providerList = providers.data ?? [];
  const [form, setForm] = useState<KnowledgeSettings>({
    enabled: false,
    model: "",
    sources: ALL,
  });
  const [error, setError] = useState("");
  const providerId = form.providerId ?? 0;
  const [picking, setPicking] = useState(false);
  const folders = form.driveFolders ?? [];
  const setFolders = (next: typeof folders) =>
    setForm((f) => ({ ...f, driveFolders: next }));
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
          <span>{t("Ask embed provider")}</span>
          <select
            className="xc-select"
            value={providerId}
            onChange={(e) =>
              setForm({
                ...form,
                providerId: Number(e.target.value) || undefined,
              })
            }
          >
            <option value={0}>{t("Ask embed provider fast")}</option>
            {providerList.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
            {providerId > 0 &&
              !providers.isPending &&
              !providerList.some((p) => p.id === providerId) && (
                <option value={providerId}>
                  {t("Ask embed provider gone")}
                </option>
              )}
          </select>
          <small>{t("Ask embed provider hint")}</small>
        </label>
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
        <fieldset className="ask-sources">
          <legend>{t("Ask drive folders")}</legend>
          {folders.length > 0 && (
            <ul className="ask-folder-list">
              {folders.map((f) => (
                <li key={f.id}>
                  <FolderOpen size={14} />
                  <span
                    className={
                      f.path ? "ask-folder-path" : "ask-folder-path xc-muted"
                    }
                    title={f.path}
                  >
                    {f.path || t("Ask drive folder gone")}
                  </span>
                  <button
                    type="button"
                    className="xc-btn small ghost"
                    aria-label={`${t("Ask remove drive folder")} ${f.path ?? f.id}`}
                    onClick={() =>
                      setFolders(folders.filter((x) => x.id !== f.id))
                    }
                  >
                    <X size={13} />
                  </button>
                </li>
              ))}
            </ul>
          )}
          <button
            type="button"
            className="xc-btn small ask-add-folder"
            disabled={folders.length >= 20}
            onClick={() => setPicking(true)}
          >
            <Plus size={14} /> {t("Ask add drive folder")}
          </button>
          <small className="xc-muted">{t("Ask drive folders hint")}</small>
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
      {picking && (
        <FolderPicker
          title={t("Ask choose drive folder")}
          description={t("Ask choose drive folder hint")}
          exclude={folders.map((f) => f.id)}
          onClose={() => setPicking(false)}
          onPick={(id) => {
            setPicking(false);
            setFolders([...folders, { id }]);
          }}
        />
      )}
    </Dialog>
  );
}
