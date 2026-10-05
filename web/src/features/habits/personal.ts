import type { WorkoutItem } from "./api";
import type {
  LibraryExercise,
  LibrarySession,
  PersonalDay,
  PersonalLibrary,
  PersonalProfile,
} from "./personalApi";

/** 个人计划的栏目。切换在左栏二级菜单里（2026-10-05），页面里不再放页签。 */
export const planSections = [
  ["recommend", "Recommended habits"],
  ["overview", "Plan overview"],
  ["training", "Guided training"],
  ["daily", "Daily routines"],
  ["food", "Diet plan"],
  ["english", "English learning"],
  ["records", "Personal records"],
  ["settings", "Plan settings"],
  ["reference", "Original and reference"],
] as const;

export function dateKey(now = new Date(), timezone?: string) {
  if (timezone) {
    const parts = Object.fromEntries(
      new Intl.DateTimeFormat("en", {
        timeZone: timezone,
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      })
        .formatToParts(now)
        .map(({ type, value }) => [type, value]),
    );
    return `${parts.year}-${parts.month}-${parts.day}`;
  }
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
}
export function validPersonalDate(value: string) {
  return (
    /^\d{4}-\d{2}-\d{2}$/.test(value) &&
    !Number.isNaN(Date.parse(value)) &&
    new Date(value).toISOString().slice(0, 10) === value
  );
}
const dateNumber = (date: string) => Date.parse(`${date}T12:00:00Z`);
export function personalWeek(date: string, start: string) {
  return Math.max(
    1,
    Math.floor((dateNumber(date) - dateNumber(start)) / 604800000) + 1,
  );
}
export function personalWeekDates(date: string) {
  const day = new Date(dateNumber(date));
  day.setUTCDate(day.getUTCDate() - ((day.getUTCDay() + 6) % 7));
  return Array.from({ length: 7 }, (_, i) =>
    new Date(day.getTime() + i * 86400000).toISOString().slice(0, 10),
  );
}
export function personalSessionId(
  date: string,
  profile: Pick<PersonalProfile, "phase" | "start">,
) {
  const weekday = (new Date(dateNumber(date)).getUTCDay() + 6) % 7;
  if (profile.phase === 3)
    return ["L3", "U3", "rest", "H3", "V3", "cardio", "rest"][weekday];
  if (profile.phase === 2)
    return ["A2", "cardio", "B2", "rest", "C2", "cardio", "rest"][weekday];
  const odd = personalWeek(date, profile.start) % 2 === 1;
  return [
    odd ? "A1" : "B1",
    "cardio",
    odd ? "B1" : "A1",
    "rest",
    odd ? "A1" : "B1",
    "cardio",
    "rest",
  ][weekday];
}
export function personalSession(
  date: string,
  profile: PersonalProfile,
  library: PersonalLibrary,
): LibrarySession {
  const id = personalSessionId(date, profile);
  if (id === "rest")
    return {
      id,
      name: "恢复日",
      focus: "轻松散步、体态和恢复",
      place: "按需要选择",
      time: "约 10–20 分钟",
      items: [
        {
          exerciseId: "walk",
          sets: 1,
          prescription: "轻松走动，不追强度",
          optional: true,
        },
        {
          exerciseId: "birddog",
          sets: 1,
          prescription: "每侧 5 次 × 5–10 秒",
          optional: true,
        },
        {
          exerciseId: "wallslide",
          sets: 1,
          prescription: "8–10 次",
          optional: true,
        },
        {
          exerciseId: "chin",
          sets: 1,
          prescription: "5–10 次 × 3–5 秒",
          optional: true,
        },
      ],
    };
  if (id === "cardio") {
    const r = library.runLevels[profile.runLevel];
    return {
      id,
      name: profile.runLevel ? "有氧日：走跑交替" : "有氧日：低冲击活动",
      focus: "心肺和低冲击活动",
      place: "户外或跑步机",
      time: profile.runLevel
        ? `约 ${r.durationMinutes + 15} 分钟`
        : "25–45 分钟",
      items: [
        {
          exerciseId: "walk",
          sets: 1,
          prescription: "热身 5–8 分钟",
          optional: false,
        },
        {
          exerciseId: profile.runLevel ? "run" : "elliptical",
          sets: 1,
          prescription: profile.runLevel
            ? `${r.name} × ${r.rounds} 组`
            : "步行或椭圆机 15–25 分钟",
          optional: false,
        },
        {
          exerciseId: "calf",
          sets: 1,
          prescription: "舒适范围 8–10 次",
          optional: true,
        },
        {
          exerciseId: "hipstretch",
          sets: 1,
          prescription: "每侧 20–30 秒",
          optional: true,
        },
      ],
    };
  }
  const session = library.sessions.find((s) => s.id === id)!;
  return {
    ...session,
    items: session.items.map((item) => ({
      ...item,
      sets:
        profile.phase === 1 && personalWeek(date, profile.start) <= 2
          ? Math.min(2, item.sets)
          : item.sets,
    })),
  };
}
export function exerciseToWorkout(exercise: LibraryExercise): WorkoutItem {
  const match = /^(\d+)\s*组\s*[×x]?\s*(.*)$/.exec(exercise.dose);
  return {
    name: exercise.name,
    exerciseId: exercise.id,
    sets: match ? Number(match[1]) : undefined,
    prescription: match ? match[2] : exercise.dose,
  };
}
export function personalWorkoutItems(
  session: LibrarySession,
  library: PersonalLibrary,
  day: PersonalDay,
): WorkoutItem[] {
  return session.items.flatMap((item, i) => {
    const sets = Array.from(
      { length: item.sets },
      (_, n) => day.sets[`${session.id}:${i}:${n}`],
    ).filter(Boolean).length;
    const exercise = library.exercises.find((e) => e.id === item.exerciseId)!;
    return sets
      ? [
          {
            name: exercise.name,
            exerciseId: exercise.id,
            sets,
            prescription: item.prescription,
          },
        ]
      : [];
  });
}
export function averageWeight(days: PersonalDay[], date: string) {
  const end = dateNumber(date),
    start = end - 6 * 86400000;
  const values = days
    .filter(
      (d) =>
        dateNumber(d.date) >= start &&
        dateNumber(d.date) <= end &&
        d.weight !== "" &&
        Number(d.weight) > 0,
    )
    .map((d) => Number(d.weight));
  return values.length
    ? values.reduce((a, b) => a + b, 0) / values.length
    : null;
}
