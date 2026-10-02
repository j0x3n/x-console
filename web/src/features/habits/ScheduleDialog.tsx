import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useSaveSchedule, useSchedule, type Schedule } from "./api";
import { crossesMidnight } from "./presence";

const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/**
 * 作息（B83）：工作日、起床和睡觉时间、工作时间、多久不操作算离开。
 * 时区用浏览器当前时区，保存时一起存。
 */
export default function ScheduleDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const schedule = useSchedule();
  const save = useSaveSchedule();
  const [form, setForm] = useState<Schedule | null>(null);
  const [useWork, setUseWork] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open || !schedule.data) return;
    setForm(schedule.data);
    setUseWork(!!(schedule.data.workStart && schedule.data.workEnd));
    setError("");
  }, [open, schedule.data]);
  const set = (patch: Partial<Schedule>) =>
    setForm((f) => (f ? { ...f, ...patch } : f));
  const toggleDay = (d: number) =>
    form &&
    set({
      workDays: form.workDays.includes(d)
        ? form.workDays.filter((x) => x !== d)
        : [...form.workDays, d].sort(),
    });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    if (useWork && !(form.workStart && form.workEnd))
      return setError(t("Fill in both ends of the work hours"));
    const body: Schedule = {
      ...form,
      workStart: useWork ? form.workStart : undefined,
      workEnd: useWork ? form.workEnd : undefined,
      timezone:
        Intl.DateTimeFormat().resolvedOptions().timeZone || form.timezone,
    };
    save.mutate(body, {
      onSuccess: () => {
        toast(t("Saved"));
        onClose();
      },
      onError: (err) => setError(errorMessage(err)),
    });
  };
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Daily schedule")}
      description="提醒按这里的作息判断：醒着的时候、工作时间。"
    >
      {!form ? (
        schedule.isError ? (
          <p className="xc-error-text">{errorMessage(schedule.error)}</p>
        ) : (
          <Loading />
        )
      ) : (
        <form className="xc-stack habits-schedule" onSubmit={submit}>
          <div className="xc-field">
            <span>{t("Work days")}</span>
            <div
              className="habits-days"
              role="group"
              aria-label={t("Work days")}
            >
              {DAYS.map((d, i) => (
                <button
                  key={d}
                  type="button"
                  className={form.workDays.includes(i + 1) ? "on" : ""}
                  aria-pressed={form.workDays.includes(i + 1)}
                  onClick={() => toggleDay(i + 1)}
                >
                  {t(d)}
                </button>
              ))}
            </div>
          </div>
          <div className="habits-form-row">
            <label className="xc-field">
              <span>{t("Get up at")}</span>
              <input
                className="xc-input"
                type="time"
                value={form.wakeTime}
                required
                onChange={(e) => set({ wakeTime: e.target.value })}
              />
            </label>
            <label className="xc-field">
              <span>{t("Go to sleep at")}</span>
              <input
                className="xc-input"
                type="time"
                value={form.sleepTime}
                required
                onChange={(e) => set({ sleepTime: e.target.value })}
              />
            </label>
          </div>
          {crossesMidnight(form.wakeTime, form.sleepTime) && (
            <small className="xc-muted">
              {t("{time} is early the next morning.").replace(
                "{time}",
                form.sleepTime,
              )}
            </small>
          )}
          <label className="xc-check">
            <input
              type="checkbox"
              checked={useWork}
              onChange={(e) => {
                setUseWork(e.target.checked);
                if (e.target.checked && !form.workStart)
                  set({ workStart: "14:00", workEnd: "23:00" });
              }}
            />
            {t("Set work hours (work days only)")}
          </label>
          {useWork && (
            <div className="habits-form-row">
              <label className="xc-field">
                <span>{t("Work starts")}</span>
                <input
                  className="xc-input"
                  type="time"
                  value={form.workStart ?? ""}
                  onChange={(e) => set({ workStart: e.target.value })}
                />
              </label>
              <label className="xc-field">
                <span>{t("Work ends")}</span>
                <input
                  className="xc-input"
                  type="time"
                  value={form.workEnd ?? ""}
                  onChange={(e) => set({ workEnd: e.target.value })}
                />
              </label>
            </div>
          )}
          <label className="xc-field">
            <span>{t("Away after no input for (minutes)")}</span>
            <input
              className="xc-input"
              type="number"
              min={1}
              max={120}
              value={form.idleMinutes}
              onChange={(e) =>
                set({ idleMinutes: Math.max(1, Number(e.target.value) || 1) })
              }
            />
            <small>
              {t("Used by reminders that wait for you to be at the computer.")}
            </small>
          </label>
          <small className="xc-muted">
            {t("Time zone")}：{Intl.DateTimeFormat().resolvedOptions().timeZone}
          </small>
          {error && <p className="xc-error-text">{error}</p>}
          <div className="xc-dialog-actions">
            <button type="button" className="xc-btn ghost" onClick={onClose}>
              {t("Cancel")}
            </button>
            <button className="xc-btn primary" disabled={save.isPending}>
              {t("Save")}
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
