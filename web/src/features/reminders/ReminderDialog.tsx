import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateReminder,
  useUpdateReminder,
  type Reminder,
} from "./api";
import {
  buildRRule,
  defaultStart,
  describeRule,
  detectPreset,
  fromLocalInput,
  isValidRule,
  repeatPresets,
  toLocalInput,
  type RepeatPreset,
} from "./rrule";

const presetLabels: Record<RepeatPreset, string> = {
  none: "Does not repeat",
  daily: "Every day",
  weekdays: "Weekdays",
  weekly: "Every week",
  monthly: "Every month",
  yearly: "Every year",
  custom: "Custom RRULE",
};

interface Props {
  open: boolean;
  onClose: () => void;
  /** 编辑时传入；新建时为空。 */
  reminder?: Reminder | null;
}

export default function ReminderDialog({ open, onClose, reminder }: Props) {
  const t = useT();
  const language = useLanguage();
  const create = useCreateReminder();
  const update = useUpdateReminder();
  const [title, setTitle] = useState("");
  const [when, setWhen] = useState("");
  const [preset, setPreset] = useState<RepeatPreset>("none");
  const [custom, setCustom] = useState("");
  const [link, setLink] = useState("");
  const [body, setBody] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    if (reminder) {
      setTitle(reminder.title);
      setWhen(toLocalInput(new Date(reminder.dtstart)));
      const p = detectPreset(reminder.rrule);
      setPreset(p);
      setCustom(p === "custom" ? reminder.rrule : "");
      setLink(reminder.link);
      setBody(reminder.body);
      setEnabled(reminder.enabled);
    } else {
      setTitle("");
      setWhen(toLocalInput(defaultStart(new Date())));
      setPreset("none");
      setCustom("");
      setLink("");
      setBody("");
      setEnabled(true);
    }
  }, [open, reminder]);

  const start = fromLocalInput(when);
  const rule = buildRRule(preset, custom);
  const pending = create.isPending || update.isPending;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!title.trim()) return setError(t("Title is required"));
    if (!start) return setError(t("Pick a valid time"));
    if (preset === "custom" && (!rule || !isValidRule(rule)))
      return setError(t("The RRULE looks wrong, for example FREQ=WEEKLY;BYDAY=MO"));
    const at = start.toISOString();
    try {
      if (reminder) {
        await update.mutateAsync({
          id: reminder.id,
          body: { title: title.trim(), at, rrule: rule, link: link.trim(), body, enabled },
        });
      } else {
        await create.mutateAsync({ title: title.trim(), at, rrule: rule, link: link.trim(), body });
      }
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={reminder ? t("Edit reminder") : t("New reminder")}
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Title")}</span>
          <input
            className="xc-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="例如 给妈妈打电话"
            maxLength={200}
            autoFocus
          />
        </label>
        <div className="reminders-form-row">
          <label className="xc-field">
            <span>{t("Time")}</span>
            <input
              className="xc-input"
              type="datetime-local"
              value={when}
              onChange={(e) => setWhen(e.target.value)}
              required
            />
          </label>
          <label className="xc-field">
            <span>{t("Repeat")}</span>
            <select
              className="xc-select"
              value={preset}
              onChange={(e) => setPreset(e.target.value as RepeatPreset)}
            >
              {repeatPresets.map((p) => (
                <option key={p} value={p}>
                  {t(presetLabels[p])}
                </option>
              ))}
            </select>
          </label>
        </div>
        {preset === "custom" && (
          <label className="xc-field">
            <span>RRULE</span>
            <input
              className="xc-input xc-mono"
              value={custom}
              onChange={(e) => setCustom(e.target.value)}
              placeholder="FREQ=WEEKLY;BYDAY=MO,WE"
            />
          </label>
        )}
        {preset !== "none" && start && isValidRule(rule) && (
          <p className="xc-muted reminders-hint">
            {describeRule(rule, start, language)} · {t("time stays at")}{" "}
            {when.slice(11)}
          </p>
        )}
        <label className="xc-field">
          <span>{t("Link")}</span>
          <input
            className="xc-input"
            value={link}
            onChange={(e) => setLink(e.target.value)}
            placeholder="/projects 或 https://..."
          />
        </label>
        <label className="xc-field">
          <span>{t("Note")}</span>
          <textarea
            className="xc-textarea"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={3}
          />
        </label>
        {reminder && (
          <label className="reminders-check">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            {t("Enabled")}
          </label>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={pending}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
