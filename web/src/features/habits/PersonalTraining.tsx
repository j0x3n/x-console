import { useState } from "react";
import { CalendarRange, Save } from "lucide-react";
import PageActions from "../../components/layout/PageActions";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useSavePlans, useWorkoutPlans } from "./api";
import { personalSession, personalWorkoutItems } from "./personal";
import {
  useLogPersonalWorkout,
  useSavePersonalDay,
  type PersonalDay,
  type PersonalLibrary,
  type PersonalProfile,
} from "./personalApi";
import { ExerciseDetail } from "./ExerciseLibrary";

export default function PersonalTraining({
  day,
  profile,
  library,
}: {
  day: PersonalDay;
  profile: PersonalProfile;
  library: PersonalLibrary;
}) {
  const t = useT();
  const [template, setTemplate] = useState("");
  const [duration, setDuration] = useState(
    String(day.workoutDurationMinutes ?? 45),
  );
  const [note, setNote] = useState(day.note);
  const session =
    library.sessions.find((s) => s.id === template) ??
    personalSession(day.date, profile, library);
  const saveDay = useSavePersonalDay(day.date);
  const log = useLogPersonalWorkout(day.date);
  const plans = useWorkoutPlans();
  const savePlans = useSavePlans();
  const completed = personalWorkoutItems(session, library, day);
  const submit = () =>
    log.mutate(
      { items: completed, durationMinutes: Number(duration), note },
      { onSuccess: () => toast(t("Workout logged")) },
    );
  const addPlan = () => {
    if (!plans.data) return;
    const weekday =
      ((new Date(`${day.date}T12:00:00Z`).getUTCDay() + 6) % 7) + 1;
    const items = session.items.map((item) => ({
      name: library.exercises.find((e) => e.id === item.exerciseId)!.name,
      exerciseId: item.exerciseId,
      sets: item.sets,
      prescription: item.prescription,
      note: item.optional ? t("Optional exercise") : "",
    }));
    const next = plans.data.map((p) => ({ ...p, items: [...p.items] }));
    const match = next.find((p) => p.weekday === weekday);
    if (match) {
      match.items.push(
        ...items.filter(
          (it) =>
            !match.items.some(
              (old) =>
                old.exerciseId === it.exerciseId &&
                old.prescription === it.prescription,
            ),
        ),
      );
      if (!match.title) match.title = session.name;
    } else next.push({ weekday, title: session.name, items });
    savePlans.mutate(next, {
      onSuccess: () => toast(t("Added to weekly plan")),
    });
  };
  return (
    <>
      <PageActions>
        <button
          className="xc-btn primary"
          title={t(
            day.workoutLogId
              ? "Update saved workout"
              : "Save completed workout",
          )}
          disabled={
            log.isPending ||
            saveDay.isPending ||
            !completed.length ||
            !duration ||
            Number(duration) < 0 ||
            Number(duration) > 1440
          }
          onClick={submit}
        >
          <Save size={15} />{" "}
          {t(
            day.workoutLogId
              ? "Update saved workout"
              : "Save completed workout",
          )}
        </button>
      </PageActions>
      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{session.name}</h2>
          <button
            className="xc-btn small"
            title={t("Add session to weekly plan")}
            disabled={!plans.data || savePlans.isPending}
            onClick={addPlan}
          >
            <CalendarRange size={14} /> {t("Add session to weekly plan")}
          </button>
        </div>
        <div className="habits-personal-form">
          <label className="xc-field">
            <span>{t("Training template")}</span>
            <select
              className="xc-select"
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
            >
              <option value="">{t("Scheduled session")}</option>
              {library.sessions.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <label className="xc-field">
            <span>{t("Duration (minutes)")}</span>
            <input
              className="xc-input"
              type="number"
              min="0"
              max="1440"
              value={duration}
              onChange={(e) => setDuration(e.target.value)}
            />
          </label>
        </div>
        <p className="xc-muted">
          {session.focus} · {session.place} · {session.time}
        </p>
        <p>
          {t(
            "Check the sets you completed, then save the workout. Saving again updates the same daily workout.",
          )}
        </p>
        {day.workoutLogId && (
          <span className="xc-badge ok">{t("Workout saved")}</span>
        )}
      </section>
      <div className="xc-stack">
        {session.items.map((item, i) => {
          const exercise = library.exercises.find(
            (e) => e.id === item.exerciseId,
          )!;
          return (
            <section
              className="xc-card habits-session-exercise"
              key={`${session.id}:${i}`}
            >
              <details>
                <summary>
                  <strong>
                    {i + 1}. {exercise.name}
                    {item.optional ? ` · ${t("Optional exercise")}` : ""}
                  </strong>
                  <small>
                    {item.sets} {t("Sets")} × {item.prescription} ·{" "}
                    {exercise.rest}
                  </small>
                </summary>
                <ExerciseDetail exercise={exercise} />
              </details>
              <div className="habits-set-row">
                {Array.from({ length: item.sets }, (_, n) => {
                  const key = `${session.id}:${i}:${n}`;
                  return (
                    <label className="xc-check" key={key}>
                      <input
                        type="checkbox"
                        checked={day.sets[key] ?? false}
                        disabled={saveDay.isPending}
                        aria-label={`${exercise.name} ${t("Workout set")} ${n + 1}`}
                        onChange={(e) =>
                          saveDay.mutate({ sets: { [key]: e.target.checked } })
                        }
                      />
                      <span>
                        {t("Workout set")} {n + 1}
                      </span>
                    </label>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>
      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Training and recovery notes")}</h2>
        </div>
        <div className="xc-field">
          <MarkdownEditor
            value={note}
            onChange={setNote}
            label={t("Training and recovery notes")}
            minRows={4}
          />
        </div>
        <button
          className="xc-btn small"
          disabled={saveDay.isPending}
          onClick={() =>
            saveDay.mutate({ note }, { onSuccess: () => toast(t("Saved")) })
          }
        >
          <Save size={14} /> {t("Save notes")}
        </button>
      </section>
    </>
  );
}
