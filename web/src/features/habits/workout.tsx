import { useEffect, useMemo, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate } from "../../lib/time";
import {
  useDeleteWorkoutLog,
  useLogWorkout,
  useSavePlans,
  useSaveWorkoutSettings,
  useWorkoutLogs,
  useWorkoutSettings,
  type WorkoutItem,
  type WorkoutPlan,
} from "./api";
import { describeItem, plansToSave, weekPlans, type DayPlan } from "./progress";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ExercisePicker } from "./ExerciseLibrary";
import { exerciseToWorkout } from "./personal";

const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

export const weekdayLabels = [
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
  "Sunday",
];

export function toItem(i: WorkoutItem) {
  return {
    name: i.name,
    exerciseId: i.exerciseId ?? undefined,
    prescription: i.prescription ?? undefined,
    sets: i.sets ?? undefined,
    reps: i.reps ?? undefined,
    weight: i.weight ?? undefined,
    note: i.note ?? undefined,
  };
}

export function WeekEditor({ plans }: { plans: WorkoutPlan[] }) {
  const t = useT();
  const save = useSavePlans();
  const initial = useMemo(
    () => weekPlans(plans.map((p) => ({ ...p, items: p.items.map(toItem) }))),
    [plans],
  );
  const [week, setWeek] = useState<DayPlan[]>(initial);
  const [pickingDay, setPickingDay] = useState<number | null>(null);
  useEffect(() => setWeek(initial), [initial]);

  const setDay = (i: number, patch: Partial<DayPlan>) =>
    setWeek(week.map((d, j) => (j === i ? { ...d, ...patch } : d)));
  const setItem = (day: number, idx: number, patch: Record<string, unknown>) =>
    setDay(day, {
      items: week[day].items.map((it, j) =>
        j === idx ? { ...it, ...patch } : it,
      ),
    });
  const num = (v: string) => (v === "" ? undefined : Math.max(0, Number(v)));

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Weekly plan")}</h2>
        <button
          className="xc-btn small primary"
          disabled={save.isPending}
          onClick={() =>
            save.mutate(
              plansToSave(week).map((d) => ({
                weekday: d.weekday,
                title: d.title,
                items: d.items,
              })),
              { onSuccess: () => toast(t("Saved")), onError },
            )
          }
        >
          {t("Save plan")}
        </button>
      </div>
      <div className="habits-week">
        {week.map((d, i) => (
          <div className="habits-week-day" key={d.weekday}>
            <div className="habits-week-head">
              <strong>{t(weekdayLabels[i])}</strong>
              <input
                className="xc-input"
                value={d.title}
                placeholder={t("For example legs, or rest")}
                onChange={(e) => setDay(i, { title: e.target.value })}
              />
            </div>
            {d.items.length > 0 && (
              <div
                className="habits-item-row habits-item-head"
                aria-hidden="true"
              >
                <span>{t("Exercise")}</span>
                <span>{t("Sets")}</span>
                <span>{t("Reps")}</span>
                <span>kg</span>
                <span />
              </div>
            )}
            {d.items.map((it, j) => (
              <div key={j}>
                <div className="habits-item-row">
                  <input
                    className="xc-input"
                    value={it.name}
                    placeholder={t("Exercise")}
                    onChange={(e) => setItem(i, j, { name: e.target.value })}
                  />
                  <input
                    className="xc-input"
                    type="number"
                    min="0"
                    value={it.sets ?? ""}
                    placeholder={t("Sets")}
                    aria-label={t("Sets")}
                    onChange={(e) =>
                      setItem(i, j, { sets: num(e.target.value) })
                    }
                  />
                  <input
                    className="xc-input"
                    type="number"
                    min="0"
                    value={it.reps ?? ""}
                    placeholder={t("Reps")}
                    aria-label={t("Reps")}
                    onChange={(e) =>
                      setItem(i, j, { reps: num(e.target.value) })
                    }
                  />
                  <input
                    className="xc-input"
                    type="number"
                    min="0"
                    step="any"
                    value={it.weight ?? ""}
                    placeholder="kg"
                    aria-label={t("Weight (kg)")}
                    onChange={(e) =>
                      setItem(i, j, { weight: num(e.target.value) })
                    }
                  />
                  <button
                    className="xc-btn ghost small"
                    aria-label={t("Delete")}
                    onClick={() =>
                      setDay(i, { items: d.items.filter((_, k) => k !== j) })
                    }
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
                {it.prescription !== undefined && (
                  <input
                    className="xc-input habits-prescription"
                    aria-label={t("Training prescription")}
                    value={it.prescription}
                    onChange={(e) =>
                      setItem(i, j, { prescription: e.target.value })
                    }
                  />
                )}
              </div>
            ))}
            <button
              className="xc-btn ghost small habits-add-item"
              onClick={() => setDay(i, { items: [...d.items, { name: "" }] })}
            >
              <Plus size={13} /> {t("Add exercise")}
            </button>
            <button
              className="xc-btn ghost small"
              onClick={() => setPickingDay(i)}
            >
              <Plus size={13} /> {t("Choose from exercise library")}
            </button>
          </div>
        ))}
      </div>
      <ExercisePicker
        open={pickingDay !== null}
        onClose={() => setPickingDay(null)}
        onSelect={(e) => {
          if (pickingDay !== null)
            setDay(pickingDay, {
              items: [...week[pickingDay].items, exerciseToWorkout(e)],
            });
        }}
      />
    </div>
  );
}

export function LogDialog({
  open,
  onClose,
  planId,
  items,
}: {
  open: boolean;
  onClose: () => void;
  planId?: number;
  items: DayPlan["items"];
}) {
  const t = useT();
  const log = useLogWorkout();
  const [duration, setDuration] = useState("45");
  const [note, setNote] = useState("");
  const [done, setDone] = useState<boolean[]>([]);
  const [entries, setEntries] = useState<DayPlan["items"]>([]);
  const [picking, setPicking] = useState(false);
  useEffect(() => {
    if (!open) return;
    setDuration("45");
    setNote("");
    setDone(items.map(() => true));
    setEntries(items);
  }, [open, items]);
  const submit = () =>
    log.mutate(
      {
        planId,
        durationMinutes: Math.max(0, Number(duration) || 0),
        note,
        items: entries.filter((_, i) => done[i]),
      },
      {
        onSuccess: () => {
          toast(t("Workout logged"));
          onClose();
        },
        onError,
      },
    );
  return (
    <Dialog open={open} onClose={onClose} title={t("Log workout")}>
      {entries.length > 0 && (
        <div className="habits-log-items">
          {entries.map((it, i) => (
            <label key={i} className="habits-check">
              <input
                type="checkbox"
                checked={done[i] ?? true}
                onChange={(e) =>
                  setDone(done.map((v, j) => (j === i ? e.target.checked : v)))
                }
              />
              {describeItem(it)}
            </label>
          ))}
        </div>
      )}
      <button className="xc-btn small" onClick={() => setPicking(true)}>
        <Plus size={14} /> {t("Choose from exercise library")}
      </button>
      <ExercisePicker
        open={picking}
        onClose={() => setPicking(false)}
        onSelect={(e) => {
          setEntries([...entries, exerciseToWorkout(e)]);
          setDone([...done, true]);
        }}
      />
      <label className="xc-field">
        <span>{t("Duration (minutes)")}</span>
        <input
          className="xc-input"
          type="number"
          min="0"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
        />
      </label>
      <label className="xc-field">
        <span>{t("Workout note")}</span>
        <textarea
          className="xc-textarea"
          rows={2}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </label>
      <div className="xc-dialog-actions">
        <button className="xc-btn ghost" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          className="xc-btn primary"
          disabled={log.isPending}
          onClick={submit}
        >
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

export function RecentLogs() {
  const t = useT();
  const language = useLanguage();
  const logs = useWorkoutLogs(30);
  const remove = useDeleteWorkoutLog();
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Last 30 days")}</h2>
        {logs.data && (
          <span className="xc-muted">
            {logs.data.length} {t("workouts")}
          </span>
        )}
      </div>
      {logs.isPending ? (
        <Loading />
      ) : logs.isError ? (
        <ErrorState error={logs.error} onRetry={() => logs.refetch()} />
      ) : logs.data.length === 0 ? (
        <p className="xc-muted habits-plan-empty">
          {t("No workouts logged yet")}
        </p>
      ) : (
        <div className="habits-logs">
          {logs.data.map((l) => (
            <div className="habits-log" key={l.id}>
              <div>
                <strong>
                  {formatDate(new Date(`${l.date}T00:00:00`), language)}
                </strong>
                <span className="xc-muted">
                  {" "}
                  · {l.durationMinutes} {t("min")}
                </span>
                {l.items.length > 0 && (
                  <div className="xc-muted habits-log-detail">
                    {l.items.map((i) => describeItem(toItem(i))).join("，")}
                  </div>
                )}
                {l.note && <div className="habits-log-detail">{l.note}</div>}
              </div>
              <button
                className="xc-btn ghost small"
                aria-label={t("Delete")}
                disabled={remove.isPending}
                onClick={async () =>
                  (await confirmAction({ title: t("Delete this workout?") })) &&
                  remove.mutate(l.id, { onError })
                }
              >
                <Trash2 size={13} />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function NoticeSettings() {
  const t = useT();
  const settings = useWorkoutSettings();
  const save = useSaveWorkoutSettings();
  const [enabled, setEnabled] = useState(true);
  const [time, setTime] = useState("08:00");
  useEffect(() => {
    if (!settings.data) return;
    setEnabled(settings.data.notifyEnabled);
    setTime(settings.data.notifyTime);
  }, [settings.data]);
  return (
    <div className="xc-card habits-notice">
      <div className="xc-card-head">
        <h2>{t("Workout reminder")}</h2>
      </div>
      <div className="xc-row habits-notice-row">
        <label className="habits-check">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          {t("On days with a plan, send it at")}
        </label>
        <input
          className="xc-input habits-time"
          type="time"
          value={time}
          onChange={(e) => setTime(e.target.value)}
        />
        <span className="xc-spacer" />
        <button
          className="xc-btn small primary"
          disabled={save.isPending || !settings.data}
          onClick={() =>
            save.mutate(
              { notifyEnabled: enabled, notifyTime: time },
              { onSuccess: () => toast(t("Saved")), onError },
            )
          }
        >
          {t("Save")}
        </button>
      </div>
    </div>
  );
}
