import { fail, json, noContent, route } from "../router";
import { at, date, day, rand } from "./util";

interface Habit {
  id: number;
  name: string;
  icon: string;
  kind: "count" | "workout";
  color: string;
  unit: string;
  dailyTarget: number;
  remindMode: "none" | "interval" | "times";
  remindIntervalMinutes: number;
  remindWindow: string;
  remindTimes: string[];
  haEntityId: string;
  archived: boolean;
  sortOrder: number;
  createdAt: string;
}
interface Log {
  id: number;
  habitId: number;
  at: string;
  amount: number;
  source: string;
  note: string;
}

const base = {
  kind: "count" as "count" | "workout",
  remindMode: "none" as const,
  remindIntervalMinutes: 0,
  remindWindow: "",
  remindTimes: [],
  haEntityId: "",
  archived: false,
};
const habits: Habit[] = [
  {
    ...base,
    id: 1,
    name: "喝水",
    icon: "💧",
    color: "info",
    unit: "杯",
    dailyTarget: 8,
    remindMode: "interval",
    remindIntervalMinutes: 90,
    remindWindow: "09:00-21:00",
    sortOrder: 1,
    createdAt: at(-60 * 24 * 90),
  },
  {
    ...base,
    id: 2,
    name: "跑步",
    icon: "🏃",
    color: "ok",
    unit: "公里",
    dailyTarget: 3,
    sortOrder: 2,
    createdAt: at(-60 * 24 * 60),
  },
  {
    ...base,
    id: 3,
    name: "读书",
    icon: "📖",
    color: "accent",
    unit: "页",
    dailyTarget: 20,
    remindMode: "times",
    remindTimes: ["21:30"],
    sortOrder: 3,
    createdAt: at(-60 * 24 * 45),
  },
  {
    ...base,
    id: 4,
    name: "冥想",
    icon: "🧘",
    color: "warn",
    unit: "分钟",
    dailyTarget: 10,
    sortOrder: 4,
    createdAt: at(-60 * 24 * 30),
  },
  {
    ...base,
    id: 5,
    name: "早睡",
    icon: "🌙",
    color: "danger",
    unit: "次",
    dailyTarget: 1,
    sortOrder: 5,
    createdAt: at(-60 * 24 * 20),
  },
  {
    ...base,
    id: 6,
    name: "健身",
    icon: "💪",
    kind: "workout",
    color: "ok",
    unit: "次",
    dailyTarget: 1,
    sortOrder: 6,
    createdAt: at(-60 * 24 * 90),
  },
];
let nextLog = 1;
const logs: Log[] = [];
const r = rand(7);
for (let d = -120; d <= 0; d++) {
  for (const h of habits) {
    if (h.kind === "workout") continue;
    if (
      d < -Math.round((Date.now() - new Date(h.createdAt).getTime()) / 86400000)
    )
      continue;
    const p = r();
    if (d === 0) continue;
    if (p < 0.25) continue;
    const amount =
      p > 0.45 ? h.dailyTarget : Math.max(1, Math.round(h.dailyTarget * p));
    logs.push({
      id: nextLog++,
      habitId: h.id,
      at: day(d, "20:00"),
      amount,
      source: "manual",
      note: "",
    });
  }
}
// 今天：喝水 5 杯，跑步完成，读书 12 页
for (let i = 0; i < 5; i++)
  logs.push({
    id: nextLog++,
    habitId: 1,
    at: day(0, `${9 + i * 2}:10`),
    amount: 1,
    source: i === 2 ? "ha" : "manual",
    note: "",
  });
logs.push({
  id: nextLog++,
  habitId: 2,
  at: day(0, "07:30"),
  amount: 3.2,
  source: "manual",
  note: "河边",
});
logs.push({
  id: nextLog++,
  habitId: 3,
  at: day(0, "12:30"),
  amount: 12,
  source: "manual",
  note: "",
});

const dayOf = (iso: string) => {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};
const total = (hid: number, dstr: string) =>
  logs
    .filter((l) => l.habitId === hid && dayOf(l.at) === dstr)
    .reduce((s, l) => s + l.amount, 0);
function streak(h: Habit) {
  let n = 0;
  for (let d = total(h.id, date(0)) >= h.dailyTarget ? 0 : -1; ; d--) {
    if (total(h.id, date(d)) >= h.dailyTarget) n++;
    else break;
  }
  return n;
}

const plans = [
  {
    id: 1,
    weekday: 1,
    title: "胸和三头",
    items: [
      { name: "卧推", sets: 4, reps: 10, weight: 60 },
      { name: "双杠臂屈伸", sets: 3, reps: 12 },
    ],
  },
  {
    id: 2,
    weekday: 3,
    title: "背和二头",
    items: [
      { name: "引体向上", sets: 4, reps: 8 },
      { name: "杠铃划船", sets: 4, reps: 10, weight: 50 },
    ],
  },
  {
    id: 3,
    weekday: 5,
    title: "腿",
    items: [
      { name: "深蹲", sets: 5, reps: 8, weight: 80 },
      { name: "腿举", sets: 3, reps: 12, weight: 120 },
    ],
  },
];
const workoutLogs: {
  id: number;
  date: string;
  planId?: number;
  items: (typeof plans)[number]["items"];
  durationMinutes: number;
  note: string;
  createdAt: string;
}[] = [];
// 最近 8 周，大约一周三次，按计划练
{
  const wr = rand(11);
  for (let d = -2; d > -56; d--) {
    const wd = new Date(day(d)).getDay() || 7;
    const plan = plans.find((p) => p.weekday === wd);
    if (!plan || wr() < 0.2) continue;
    workoutLogs.push({
      id: workoutLogs.length + 1,
      date: date(d),
      planId: plan.id,
      items: plan.items,
      durationMinutes: 40 + Math.round(wr() * 20),
      note: d === -2 ? "深蹲加了 5 公斤" : "",
      createdAt: day(d, "19:30"),
    });
  }
  for (const w of workoutLogs)
    logs.push({
      id: nextLog++,
      habitId: 6,
      at: w.createdAt,
      amount: 1,
      source: "workout",
      note: "",
    });
}
const workoutSettings = { notifyEnabled: true, notifyTime: "18:30" };

export function register() {
  route("GET", "/habits", ({ query }) =>
    json(
      habits.filter((h) => h.archived === (query.get("archived") === "true")),
    ),
  );
  route("POST", "/habits", ({ body }) => {
    const h: Habit = {
      ...base,
      id: habits.length + 1,
      icon: "",
      color: "",
      unit: "次",
      dailyTarget: 1,
      sortOrder: habits.length + 1,
      createdAt: at(0),
      ...body,
    };
    habits.push(h);
    return json(h, 201);
  });
  route("GET", "/habits/today", () =>
    json(
      habits
        .filter((h) => !h.archived)
        .map((h) => {
          const today = logs.filter(
            (l) => l.habitId === h.id && dayOf(l.at) === date(0),
          );
          const done = today.reduce((s, l) => s + l.amount, 0);
          return {
            habit: h,
            done,
            streak: streak(h),
            reached: done >= h.dailyTarget,
            logs: today,
          };
        }),
    ),
  );
  route("DELETE", "/habits/logs/:id", ({ params }) => {
    const i = logs.findIndex((l) => l.id === Number(params.id));
    if (i >= 0) logs.splice(i, 1);
    return noContent();
  });
  route("GET", "/habits/:id", ({ params }) => {
    const h = habits.find((x) => x.id === Number(params.id));
    return h ? json(h) : fail(404, "not_found", "资源不存在");
  });
  route("PATCH", "/habits/:id", ({ params, body }) => {
    const h = habits.find((x) => x.id === Number(params.id));
    if (!h) return fail(404, "not_found", "资源不存在");
    Object.assign(h, body);
    return json(h);
  });
  route("DELETE", "/habits/:id", ({ params }) => {
    const i = habits.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) habits.splice(i, 1);
    return noContent();
  });
  route("POST", "/habits/:id/checkin", ({ params, body }) => {
    const h = habits.find((x) => x.id === Number(params.id));
    if (!h) return fail(404, "not_found", "资源不存在");
    const l = {
      id: nextLog++,
      habitId: h.id,
      at: at(0),
      amount: body?.amount ?? 1,
      source: "manual",
      note: body?.note ?? "",
    };
    logs.push(l);
    return json(l, 201);
  });
  route("GET", "/habits/:id/stats", ({ params, query }) => {
    const h = habits.find((x) => x.id === Number(params.id));
    if (!h) return fail(404, "not_found", "资源不存在");
    const n = Number(query.get("days") ?? 30);
    const days = Array.from({ length: n }, (_, i) => {
      const d = date(i - n + 1);
      const amount = total(h.id, d);
      return { date: d, amount, reached: amount >= h.dailyTarget };
    });
    let best = 0;
    let run = 0;
    for (const d of days) {
      run = d.reached ? run + 1 : 0;
      best = Math.max(best, run);
    }
    return json({
      habitId: h.id,
      streak: streak(h),
      bestStreak: best,
      reachedDays: days.filter((d) => d.reached).length,
      total: days.reduce((s, d) => s + d.amount, 0),
      days,
    });
  });
  route("GET", "/workouts/plans", () => json(plans));
  route("PUT", "/workouts/plans", ({ body }) => {
    plans.splice(0, plans.length, ...body);
    return json(plans);
  });
  route("GET", "/workouts/logs", ({ query }) => {
    const days = Number(query.get("days") ?? 30);
    const from = date(-days + 1);
    return json(workoutLogs.filter((l) => l.date >= from));
  });
  route("POST", "/workouts/logs", ({ body }) => {
    const l = {
      id: workoutLogs.length + 1,
      date: body.date ?? date(0),
      planId: body.planId,
      items: body.items ?? [],
      durationMinutes: body.durationMinutes ?? 0,
      note: body.note ?? "",
      createdAt: at(0),
    };
    workoutLogs.unshift(l);
    // 记一次训练，健身类的习惯各打卡一次
    for (const h of habits.filter((x) => x.kind === "workout" && !x.archived))
      logs.push({
        id: nextLog++,
        habitId: h.id,
        at: at(0),
        amount: 1,
        source: "workout",
        note: "",
      });
    return json(l, 201);
  });
  route("DELETE", "/workouts/logs/:id", ({ params }) => {
    const i = workoutLogs.findIndex((l) => l.id === Number(params.id));
    if (i >= 0) workoutLogs.splice(i, 1);
    return noContent();
  });
  route("GET", "/workouts/settings", () => json(workoutSettings));
  route("PUT", "/workouts/settings", ({ body }) =>
    json(Object.assign(workoutSettings, body)),
  );
}
