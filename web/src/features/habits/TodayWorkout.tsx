import { useState } from "react";
import { Link } from "react-router";
import { ChevronRight, Dumbbell, ShieldAlert } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { useSchedule } from "./api";
import { ExerciseDetail } from "./ExerciseLibrary";
import { dateKey, personalSession } from "./personal";
import {
  usePersonalLibrary,
  usePersonalProfile,
  type LibraryExercise,
} from "./personalApi";

/**
 * B203：习惯“今天”页顶部的“今天练什么”。按个人计划的阶段和星期取今天的课
 * （力量、有氧走跑、恢复），列出动作和量，点动作看图解和要领，“开始跟练”进跟练清单。
 * 计划是按腰突写的，所以卡片底部固定提醒什么时候要停。
 */
export default function TodayWorkout() {
  const t = useT();
  const library = usePersonalLibrary();
  const profile = usePersonalProfile();
  const schedule = useSchedule();
  const [open, setOpen] = useState<LibraryExercise | null>(null);
  if (!library.data || !profile.data || !schedule.data) return null;
  const date = dateKey(new Date(), schedule.data.timezone);
  const session = personalSession(date, profile.data, library.data);
  const rows = session.items.flatMap((item) => {
    const exercise = library.data.exercises.find(
      (e) => e.id === item.exerciseId,
    );
    return exercise ? [{ item, exercise }] : [];
  });
  return (
    <section className="xc-card habits-today-workout">
      <div className="xc-card-head">
        <h2>
          <Dumbbell size={15} /> {t("Today's training")} · {session.name}
        </h2>
        <Link
          className="xc-btn small"
          to={`/habits/plan?section=training&date=${date}`}
        >
          {t("Start guided training")} <ChevronRight size={14} />
        </Link>
      </div>
      <p className="xc-muted habits-today-workout-meta">
        {[t("From personal plan"), session.focus, session.place, session.time]
          .filter(Boolean)
          .join(" · ")}
      </p>
      <ul className="habits-today-workout-list">
        {rows.map(({ item, exercise }, i) => (
          <li key={`${exercise.id}-${i}`}>
            <button
              type="button"
              className="habits-today-workout-row"
              onClick={() => setOpen(exercise)}
            >
              <span className="habits-today-workout-name">
                {exercise.name}
                {item.optional && (
                  <small className="xc-muted">
                    {" "}
                    · {t("Optional exercise")}
                  </small>
                )}
              </span>
              <span className="xc-muted">
                {item.sets > 1
                  ? `${item.sets} ${t("Training sets")} · ${item.prescription}`
                  : item.prescription}
              </span>
            </button>
          </li>
        ))}
      </ul>
      <p className="habits-today-workout-safety">
        <ShieldAlert size={13} /> {t("Back safety note")}
      </p>
      <Dialog
        open={open !== null}
        onClose={() => setOpen(null)}
        title={open?.name ?? ""}
      >
        {open && <ExerciseDetail exercise={open} />}
      </Dialog>
    </section>
  );
}
