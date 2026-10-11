import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { usePutPlanSettings, type DayPlanSettings } from "./api";

/** 计划设置：自动出草稿、AI、工作时段、晚上提醒。 */
export default function PlanSettingsDialog({
  open,
  onClose,
  settings,
}: {
  open: boolean;
  onClose: () => void;
  settings?: DayPlanSettings;
}) {
  const t = useT();
  const save = usePutPlanSettings();
  const [form, setForm] = useState<DayPlanSettings>({
    auto: true,
    generateTime: "07:30",
    useAi: true,
    workStart: "09:00",
    workEnd: "18:00",
    review: true,
    reviewTime: "20:30",
  });
  const [error, setError] = useState("");
  useEffect(() => {
    if (open && settings) {
      setForm(settings);
      setError("");
    }
  }, [open, settings]);
  const set = (patch: Partial<DayPlanSettings>) =>
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
    <Dialog open={open} onClose={onClose} title={t("Plan settings")}>
      <form onSubmit={submit} className="plan-settings">
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.auto}
            onChange={(e) => set({ auto: e.target.checked })}
          />
          <span>{t("Make a draft every morning")}</span>
        </label>
        <label className="xc-field">
          <span>{t("Draft time")}</span>
          <input
            className="xc-input"
            type="time"
            value={form.generateTime}
            onChange={(e) => set({ generateTime: e.target.value })}
          />
        </label>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.useAi}
            onChange={(e) => set({ useAi: e.target.checked })}
          />
          <span>{t("Plan with AI")}</span>
        </label>
        <p className="xc-muted plan-hint">
          {t(
            "With AI on, the free time and the titles of due items are sent to your AI provider. With it off, a fixed rule plans the day and nothing is sent.",
          )}
        </p>
        <div className="plan-settings-row">
          <label className="xc-field">
            <span>{t("Plan work starts")}</span>
            <input
              className="xc-input"
              type="time"
              value={form.workStart}
              onChange={(e) => set({ workStart: e.target.value })}
            />
          </label>
          <label className="xc-field">
            <span>{t("Plan work ends")}</span>
            <input
              className="xc-input"
              type="time"
              value={form.workEnd}
              onChange={(e) => set({ workEnd: e.target.value })}
            />
          </label>
        </div>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.review}
            onChange={(e) => set({ review: e.target.checked })}
          />
          <span>{t("Ask in the evening what got done")}</span>
        </label>
        <label className="xc-field">
          <span>{t("Evening time")}</span>
          <input
            className="xc-input"
            type="time"
            value={form.reviewTime}
            onChange={(e) => set({ reviewTime: e.target.value })}
          />
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
