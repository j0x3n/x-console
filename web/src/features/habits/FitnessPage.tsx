import { useState } from "react";
import { Plus } from "lucide-react";
import PageActions from "../../components/layout/PageActions";
import Dialog from "../../components/ui/Dialog";
import { toast } from "../../hooks/useToast";
import { useT } from "../../contexts/LanguageContext";
import { useSavePlans, useWorkoutPlans } from "./api";
import { ExerciseLibrary, ExercisePicker } from "./ExerciseLibrary";
import { exerciseToWorkout } from "./personal";
import type { LibraryExercise } from "./personalApi";
import { weekdayLabels } from "./workout";

export default function FitnessPage() {
  const t = useT();
  const [picking, setPicking] = useState(false);
  const [exercise, setExercise] = useState<LibraryExercise | null>(null);
  const [weekday, setWeekday] = useState(1);
  const plans = useWorkoutPlans();
  const save = useSavePlans();
  const add = () => {
    if (!exercise || !plans.data) return;
    const next = plans.data.map((p) => ({ ...p, items: [...p.items] }));
    const match = next.find((p) => p.weekday === weekday);
    if (match) match.items.push(exerciseToWorkout(exercise));
    else
      next.push({ weekday, title: "", items: [exerciseToWorkout(exercise)] });
    save.mutate(next, {
      onSuccess: () => {
        toast(t("Added to weekly plan"));
        setExercise(null);
      },
    });
  };
  return (
    <>
      <PageActions>
        <button
          className="xc-btn primary"
          title={t("Add exercise to weekly plan")}
          onClick={() => setPicking(true)}
        >
          <Plus size={15} /> {t("Add exercise to weekly plan")}
        </button>
      </PageActions>
      <ExerciseLibrary />
      <ExercisePicker
        open={picking}
        onClose={() => setPicking(false)}
        onSelect={setExercise}
      />
      <Dialog
        open={!!exercise}
        onClose={() => setExercise(null)}
        title={t("Add exercise to weekly plan")}
      >
        {exercise && (
          <>
            <p>
              {exercise.name} · {exercise.dose}
            </p>
            <label className="xc-field">
              <span>{t("Weekday")}</span>
              <select
                className="xc-select"
                value={weekday}
                onChange={(e) => setWeekday(Number(e.target.value))}
              >
                {weekdayLabels.map((d, i) => (
                  <option key={d} value={i + 1}>
                    {t(d)}
                  </option>
                ))}
              </select>
            </label>
            <div className="xc-dialog-actions">
              <button
                className="xc-btn ghost"
                onClick={() => setExercise(null)}
              >
                {t("Cancel")}
              </button>
              <button
                className="xc-btn primary"
                disabled={!plans.data || save.isPending}
                onClick={add}
              >
                {t("Add")}
              </button>
            </div>
          </>
        )}
      </Dialog>
    </>
  );
}
