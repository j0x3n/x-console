import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { CalendarPlus, Check, Plus, TriangleAlert } from "lucide-react";
import { isNotLive } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { SearchBox, Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useHabitList, useSavePlans, useWorkoutPlans, type Habit } from "./api";
import HabitDialog from "./HabitDialog";
import { exerciseToWorkout } from "./personal";
import { usePersonalLibrary, type LibraryExercise } from "./personalApi";
import {
  FITNESS_PROGRAMS,
  exerciseHabitInput,
  programHabitInput,
  sameName,
  type FitnessProgram,
  type ProgramKind,
} from "./recommend";
import { weekdayLabels } from "./workout";

type View = ProgramKind | "library";
const views: View[] = ["problem", "daily", "library"];

/**
 * 健身（用户 2026-10-05 要求）：按身体问题或每天的基础动作选一套方案，整套加入习惯；
 * 也能在动作库里挑单个动作加入习惯或每周计划。
 * 桌面上三栏放在一屏里，各栏自己滚动：方案、方案里的动作、动作说明。
 * 窄屏时一栏一栏往下排，动作说明用弹窗打开。
 * 地址参数：?view=problem|daily|library，?program=方案，?exercise=动作，?group=动作分类。
 */
export default function FitnessPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const library = usePersonalLibrary();
  const habits = useHabitList();
  const narrow = useNarrow();
  const [draft, setDraft] = useState<FitnessDraft | null>(null);
  const [editing, setEditing] = useState<Habit | null>(null);
  const [planning, setPlanning] = useState<LibraryExercise | null>(null);
  const [reading, setReading] = useState(false);
  const [search, setSearch] = useState("");

  const programId = params.get("program");
  const program =
    FITNESS_PROGRAMS.find((p) => p.id === programId) ?? FITNESS_PROGRAMS[0];
  const exerciseParam = params.get("exercise");
  const rawView = params.get("view");
  const view: View = views.includes(rawView as View)
    ? (rawView as View)
    : programId
      ? program.kind
      : exerciseParam
        ? "library"
        : "problem";
  const group = params.get("group") ?? "";
  const set = (next: Record<string, string | null>) => {
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(next)) {
      if (v === null) p.delete(k);
      else p.set(k, v);
    }
    setParams(p, { replace: true });
  };

  if (library.isPending) return <Loading />;
  if (library.isError)
    return isNotLive(library.error) ? (
      <NotLive name={t("Fitness")} />
    ) : (
      <ErrorState error={library.error} onRetry={() => library.refetch()} />
    );

  const exercises = library.data.exercises;
  const byId = new Map(exercises.map((e) => [e.id, e]));
  const active = (habits.data ?? []).filter((h) => !h.archived);
  const habitNamed = (name: string) =>
    active.find((h) => sameName(h.name, name));

  // 当前方案（身体问题、每天的基础）或动作库里筛出来的动作
  const programs = FITNESS_PROGRAMS.filter((p) => p.kind === view);
  const current =
    view === "library"
      ? null
      : (programs.find((p) => p.id === programId) ?? programs[0]);
  const listed =
    view === "library"
      ? exercises.filter(
          (e) =>
            (!group || e.group === group) &&
            `${e.name} ${e.en} ${e.target} ${e.equipment}`
              .toLowerCase()
              .includes(search.trim().toLowerCase()),
        )
      : (current?.items ?? [])
          .map((i) => byId.get(i.exerciseId))
          .filter((e): e is LibraryExercise => !!e);
  const exercise =
    listed.find((e) => e.id === exerciseParam) ?? listed[0] ?? null;
  const groups = [...new Set(exercises.map((e) => e.group))];

  const pick = (id: string) => {
    set({ exercise: id });
    if (narrow) setReading(true);
  };
  const addProgram = (p: FitnessProgram) => {
    const habit = habitNamed(p.habitName);
    if (habit) setEditing(habit);
    else setDraft({ input: programHabitInput(p) });
  };
  const addExercise = (e: LibraryExercise) => {
    const habit = habitNamed(e.name);
    if (habit) setEditing(habit);
    else setDraft({ input: exerciseHabitInput(e) });
  };

  const detail = exercise && (
    <ExerciseBrief
      exercise={exercise}
      dose={current?.items.find((i) => i.exerciseId === exercise.id)?.dose}
      added={!!habitNamed(exercise.name)}
      onAddHabit={() => addExercise(exercise)}
      onAddPlan={() => setPlanning(exercise)}
    />
  );

  return (
    <div className="habits-fit-page">
      <Toolbar
        start={
          <Segmented<View>
            label={t("Fitness")}
            value={view}
            onChange={(v) =>
              set({ view: v, program: null, exercise: null, group: null })
            }
            options={[
              { value: "problem", label: t("Body problems") },
              { value: "daily", label: t("Daily basics") },
              { value: "library", label: t("Exercise library") },
            ]}
          />
        }
        end={
          view === "library" && (
            <>
              <select
                className="xc-select habits-fit-group"
                aria-label={t("Exercise group")}
                value={group}
                onChange={(e) =>
                  set({ group: e.target.value || null, exercise: null })
                }
              >
                <option value="">
                  {t("All")} {exercises.length}
                </option>
                {groups.map((g) => (
                  <option key={g} value={g}>
                    {g} {exercises.filter((e) => e.group === g).length}
                  </option>
                ))}
              </select>
              <SearchBox
                value={search}
                onChange={setSearch}
                placeholder={t("Search exercises, muscles or equipment")}
              />
            </>
          )
        }
      />
      <div className={`habits-fit habits-fit-${view}`}>
        {current && (
          <nav className="habits-fit-col habits-fit-programs">
            {programs.map((p) => {
              const added = !!habitNamed(p.habitName);
              return (
                <button
                  key={p.id}
                  className={`habits-fit-program${p.id === current.id ? " active" : ""}`}
                  aria-pressed={p.id === current.id}
                  onClick={() =>
                    set({ view: p.kind, program: p.id, exercise: null })
                  }
                >
                  <span className="habits-fit-emoji" aria-hidden>
                    {p.icon}
                  </span>
                  <span className="habits-fit-program-text">
                    <strong>{p.name}</strong>
                    <small>
                      {p.items.length} {t("exercises")} · {p.minutes}{" "}
                      {t("minutes")}
                    </small>
                  </span>
                  {added && (
                    <Check
                      size={14}
                      className="habits-fit-added"
                      aria-label={t("In habits")}
                    />
                  )}
                </button>
              );
            })}
          </nav>
        )}
        <section className="habits-fit-col habits-fit-middle">
          {current && (
            <ProgramHead
              program={current}
              added={!!habitNamed(current.habitName)}
              onAdd={() => addProgram(current)}
            />
          )}
          {listed.length === 0 ? (
            <p className="xc-muted habits-fit-empty">
              {t("No matching exercises")}
            </p>
          ) : (
            <ol className="habits-fit-exercises">
              {listed.map((e, i) => (
                <li key={e.id}>
                  <button
                    className={`habits-fit-exercise${e.id === exercise?.id ? " active" : ""}`}
                    aria-pressed={e.id === exercise?.id}
                    onClick={() => pick(e.id)}
                  >
                    {current && (
                      <span className="habits-fit-step">{i + 1}</span>
                    )}
                    <img src={e.image} alt="" loading="lazy" />
                    <span className="habits-fit-exercise-text">
                      <strong>{e.name}</strong>
                      <small>
                        {current?.items.find((x) => x.exerciseId === e.id)
                          ?.dose ?? `${e.group} · ${e.dose}`}
                      </small>
                    </span>
                  </button>
                </li>
              ))}
            </ol>
          )}
        </section>
        {!narrow && (
          <aside className="habits-fit-col habits-fit-detail">{detail}</aside>
        )}
      </div>
      {narrow && (
        <Dialog
          open={reading && !!exercise}
          onClose={() => setReading(false)}
          title={exercise?.name ?? ""}
        >
          {detail}
        </Dialog>
      )}
      <HabitDialog
        open={draft !== null || editing !== null}
        habit={editing}
        initialInput={draft?.input}
        onClose={() => {
          setDraft(null);
          setEditing(null);
        }}
      />
      <PlanDialog exercise={planning} onClose={() => setPlanning(null)} />
    </div>
  );
}

interface FitnessDraft {
  input: ReturnType<typeof programHabitInput>;
}

function ProgramHead({
  program: p,
  added,
  onAdd,
}: {
  program: FitnessProgram;
  added: boolean;
  onAdd: () => void;
}) {
  const t = useT();
  return (
    <div className="habits-fit-head">
      <div className="habits-fit-head-text">
        <h2>
          <span aria-hidden>{p.icon}</span> {p.name}
        </h2>
        <p>{p.summary}</p>
        <small className="xc-muted">
          {t("About")} {p.minutes} {t("minutes")} · {t("Default reminder")}{" "}
          {p.remindAt}
        </small>
        {p.caution && (
          <p className="habits-fit-caution">
            <TriangleAlert size={13} /> {p.caution}
          </p>
        )}
      </div>
      {added ? (
        <button className="xc-btn small ghost" onClick={onAdd}>
          <Check size={14} /> {t("In habits")}
        </button>
      ) : (
        <button className="xc-btn small primary" onClick={onAdd}>
          <Plus size={14} /> {t("Add as habit")}
        </button>
      )}
    </div>
  );
}

/** 一屏里的动作说明：图、量、步骤，其他内容收起来。 */
function ExerciseBrief({
  exercise: e,
  dose,
  added,
  onAddHabit,
  onAddPlan,
}: {
  exercise: LibraryExercise;
  /** 方案里的量，没有时用动作库的参考量 */
  dose?: string;
  added: boolean;
  onAddHabit: () => void;
  onAddPlan: () => void;
}) {
  const t = useT();
  return (
    <div className="habits-fit-brief">
      <div className="habits-fit-brief-head">
        <h3>{e.name}</h3>
        <small className="xc-muted">{e.en}</small>
      </div>
      <a href={e.image} target="_blank" rel="noreferrer">
        <img src={e.image} alt={e.name} className="habits-fit-image" />
      </a>
      <div className="habits-fit-tags">
        <span className="xc-badge accent">{dose ?? e.dose}</span>
        <span className="xc-badge">
          {t("Rest between sets")} {e.rest}
        </span>
        <span className="xc-badge">{e.equipment}</span>
      </div>
      <p className="xc-muted habits-fit-target">{e.target}</p>
      <ol className="habits-fit-steps">
        {e.steps.map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ol>
      <details>
        <summary>{t("Common mistakes")}</summary>
        <ul>
          {e.errors.map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ul>
      </details>
      <details>
        <summary>{t("Exercise setup")}</summary>
        <ul>
          {e.setup.map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ul>
      </details>
      <details>
        <summary>
          {t("Easier variation")} · {t("When to progress")}
        </summary>
        <p>{e.easier}</p>
        <p>{e.progress}</p>
      </details>
      <div className="habits-fit-brief-actions">
        <button
          className={`xc-btn small${added ? " ghost" : ""}`}
          onClick={onAddHabit}
        >
          {added ? <Check size={14} /> : <Plus size={14} />}{" "}
          {added ? t("In habits") : t("Add this exercise as a habit")}
        </button>
        <button className="xc-btn small ghost" onClick={onAddPlan}>
          <CalendarPlus size={14} /> {t("Add exercise to weekly plan")}
        </button>
      </div>
    </div>
  );
}

/** 把一个动作加到每周计划的某一天（B97 原来的功能）。 */
function PlanDialog({
  exercise,
  onClose,
}: {
  exercise: LibraryExercise | null;
  onClose: () => void;
}) {
  const t = useT();
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
        onClose();
      },
    });
  };
  return (
    <Dialog
      open={!!exercise}
      onClose={onClose}
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
            <button className="xc-btn ghost" onClick={onClose}>
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
  );
}

/** 窄屏（手机、竖着的平板）时动作说明改用弹窗。 */
function useNarrow() {
  const query = "(max-width: 900px)";
  const [narrow, setNarrow] = useState(
    () => typeof window !== "undefined" && window.matchMedia?.(query).matches,
  );
  useEffect(() => {
    const m = window.matchMedia?.(query);
    if (!m) return;
    const on = () => setNarrow(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return !!narrow;
}
