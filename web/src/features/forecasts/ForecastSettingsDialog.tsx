import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { usePutForecastSettings, type ForecastSettings } from "./api";

/** 预测设置：到期当天提醒填结果，以及提醒时间。 */
export default function ForecastSettingsDialog({
  open,
  onClose,
  settings,
}: {
  open: boolean;
  onClose: () => void;
  settings?: ForecastSettings;
}) {
  const t = useT();
  const save = usePutForecastSettings();
  const [form, setForm] = useState<ForecastSettings>({
    remind: true,
    remindTime: "09:00",
  });
  const [error, setError] = useState("");
  useEffect(() => {
    if (open && settings) {
      setForm(settings);
      setError("");
    }
  }, [open, settings]);

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
    <Dialog open={open} onClose={onClose} title={t("Forecast settings")}>
      <form onSubmit={submit} className="fc-settings">
        <label className="xc-check">
          <input
            type="checkbox"
            checked={form.remind}
            onChange={(e) => setForm({ ...form, remind: e.target.checked })}
          />
          <span>{t("Forecast remind")}</span>
        </label>
        <label className="xc-field">
          <span>{t("Forecast remind time")}</span>
          <input
            className="xc-input"
            type="time"
            value={form.remindTime}
            onChange={(e) => setForm({ ...form, remindTime: e.target.value })}
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
