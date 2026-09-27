import { useEffect, useState } from "react";
import { ApiError, errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useNoteToReminder } from "../api";
import { defaultReminderTime } from "../logic";

const REPEATS = [
  { value: "", label: "Once" },
  { value: "FREQ=DAILY", label: "Every day" },
  { value: "FREQ=WEEKLY", label: "Every week" },
  { value: "FREQ=MONTHLY", label: "Every month" },
];

/** 用笔记建提醒。提醒模块没启用时显示提示。 */
export default function ToReminderDialog({
  open,
  onClose,
  noteId,
}: {
  open: boolean;
  onClose: () => void;
  noteId: number;
}) {
  const t = useT();
  const toReminder = useNoteToReminder();
  const [at, setAt] = useState(defaultReminderTime);
  const [rrule, setRrule] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setAt(defaultReminderTime());
    setRrule("");
    setError("");
  }, [open]);

  return (
    <Dialog open={open} onClose={onClose} title={t("Remind me")}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const when = new Date(at);
          if (Number.isNaN(when.getTime())) return;
          toReminder.mutate(
            { id: noteId, at: when.toISOString(), rrule: rrule || undefined },
            {
              onSuccess: () => {
                toast(t("Reminder created"));
                onClose();
              },
              onError: (err) =>
                setError(
                  err instanceof ApiError && err.code === "feature_unavailable"
                    ? t("Reminders are not available yet.")
                    : errorMessage(err),
                ),
            },
          );
        }}
      >
        <label className="xc-field">
          <span>{t("When")}</span>
          <input
            className="xc-input"
            type="datetime-local"
            value={at}
            onChange={(e) => setAt(e.target.value)}
            required
          />
        </label>
        <label className="xc-field">
          <span>{t("Repeat")}</span>
          <select
            className="xc-select"
            value={rrule}
            onChange={(e) => setRrule(e.target.value)}
          >
            {REPEATS.map((r) => (
              <option key={r.value} value={r.value}>
                {t(r.label)}
              </option>
            ))}
          </select>
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={!at || toReminder.isPending}
          >
            {t("Create reminder")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
