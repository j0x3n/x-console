import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useProjects } from "../projects/api";
import { usePutInboxSettings, type InboxSettings } from "./api";

/** 收集箱设置：AI 建议、每晚提醒、转写模型、卡片默认项目。 */
export default function InboxSettingsDialog({
  open,
  onClose,
  settings,
}: {
  open: boolean;
  onClose: () => void;
  settings?: InboxSettings;
}) {
  const t = useT();
  const projects = useProjects();
  const save = usePutInboxSettings();
  const [form, setForm] = useState<InboxSettings>({
    suggest: true,
    reviewReminder: true,
    reviewTime: "21:00",
    transcribeModel: "",
    language: "",
    cardProjectId: 0,
  });
  const [error, setError] = useState("");
  useEffect(() => {
    if (open && settings) {
      setForm(settings);
      setError("");
    }
  }, [open, settings]);
  const set = (patch: Partial<InboxSettings>) =>
    setForm((f) => ({ ...f, ...patch }));

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

  return (
    <Dialog open={open} onClose={onClose} title={t("Inbox settings")}>
      <form onSubmit={submit} className="inbox-settings">
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.suggest}
            onChange={(e) => set({ suggest: e.target.checked })}
          />
          <span>{t("Suggest where each note belongs with AI")}</span>
        </label>
        <p className="xc-muted inbox-hint">
          {t(
            "The text of each note is sent to your AI provider. Recordings are sent to it too, to turn them into text, and are not kept. Turn this off to only collect.",
          )}
        </p>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.reviewReminder}
            onChange={(e) => set({ reviewReminder: e.target.checked })}
          />
          <span>{t("Remind me every evening when notes are waiting")}</span>
        </label>
        <label className="xc-field">
          <span>{t("Inbox reminder time")}</span>
          <input
            className="xc-input"
            type="time"
            value={form.reviewTime}
            onChange={(e) => set({ reviewTime: e.target.value })}
          />
        </label>
        <label className="xc-field">
          <span>{t("Speech to text model")}</span>
          <input
            className="xc-input"
            value={form.transcribeModel}
            onChange={(e) => set({ transcribeModel: e.target.value })}
            placeholder="whisper-1"
          />
        </label>
        <label className="xc-field">
          <span>{t("Speech language")}</span>
          <input
            className="xc-input"
            value={form.language}
            maxLength={3}
            onChange={(e) => set({ language: e.target.value })}
            placeholder="zh"
          />
        </label>
        <label className="xc-field">
          <span>{t("Default project for cards")}</span>
          <select
            className="xc-select"
            value={form.cardProjectId}
            onChange={(e) => set({ cardProjectId: Number(e.target.value) })}
          >
            <option value={0}>{t("Not set")}</option>
            {projects.data?.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        {error && (
          <p className="xc-error" role="alert">
            {error}
          </p>
        )}
        <div className="xc-dialog-actions">
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
