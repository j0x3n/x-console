/*
 * 习惯页用到的纯计算：进度环、热力图分级、柱状图比例、提醒说明、训练计划整理。
 */

/** 完成比例，0 到 1。 */
export function ratio(done: number, target: number): number {
  if (!(target > 0)) return done > 0 ? 1 : 0;
  return Math.min(1, Math.max(0, done / target));
}

/** 圆环的周长和 stroke-dashoffset。 */
export function ringGeometry(value: number, radius: number) {
  const circumference = 2 * Math.PI * radius;
  return {
    circumference,
    offset: circumference * (1 - Math.min(1, Math.max(0, value))),
  };
}

/** 数量显示：最多两位小数，去掉多余的 0。 */
export function formatAmount(value: number): string {
  return String(Math.round(value * 100) / 100);
}

/** 热力图分级：0 没打卡，1-3 部分完成，4 达标。 */
export function heatLevel(amount: number, target: number): 0 | 1 | 2 | 3 | 4 {
  if (amount <= 0) return 0;
  const r = ratio(amount, target);
  if (r >= 1) return 4;
  return Math.max(1, Math.min(3, Math.ceil(r * 3))) as 1 | 2 | 3;
}

/** 柱状图的纵轴上限：取最大值和目标里更大的一个，至少为 1。 */
export function barMax(amounts: number[], target: number): number {
  return Math.max(1, target, ...amounts);
}

/** "09:00-21:00" 拆成开始和结束。空字符串表示全天。 */
export function splitWindow(window: string): { start: string; end: string } {
  const [start, end] = window.split("-");
  if (!start || !end) return { start: "", end: "" };
  return { start: start.trim(), end: end.trim() };
}

export function joinWindow(start: string, end: string): string {
  return start && end ? `${start}-${end}` : "";
}

const clock = /^([01]\d|2[0-3]):[0-5]\d$/;

/** "8:00, 20:00" 这样的输入整理成排好序的 ["08:00","20:00"]。有错时返回 null。 */
export function parseTimes(input: string): string[] | null {
  const out = new Set<string>();
  for (const raw of input.split(/[,，\s]+/)) {
    if (!raw) continue;
    const padded = raw.length === 4 ? `0${raw}` : raw;
    if (!clock.test(padded)) return null;
    out.add(padded);
  }
  return [...out].sort();
}

/** 提醒方式的简短说明。 */
export function remindSummary(
  habit: {
    remindMode: "none" | "interval" | "times";
    remindIntervalMinutes: number;
    remindWindow: string;
    remindTimes: string[];
    remindWhen?: string[];
  },
  language: "zh" | "en" = "zh",
): string {
  const zh = language === "zh";
  switch (habit.remindMode) {
    case "interval": {
      const when = habit.remindWhen ?? ["window"];
      // B83：在用电脑时按连续使用的时间算
      const every = when.includes("active")
        ? zh
          ? `连续用电脑 ${habit.remindIntervalMinutes} 分钟`
          : `After ${habit.remindIntervalMinutes} min at the computer`
        : zh
          ? `每 ${habit.remindIntervalMinutes} 分钟`
          : `Every ${habit.remindIntervalMinutes} min`;
      const labels: Record<string, [string, string]> = {
        awake: ["醒着时", "while awake"],
        work: ["工作时间", "during work hours"],
      };
      const extra = when
        .filter((w) => labels[w])
        .map((w) => labels[w][zh ? 0 : 1]);
      if (when.includes("window") && habit.remindWindow)
        extra.push(habit.remindWindow);
      return [every, ...extra].join(" · ");
    }
    case "times":
      return habit.remindTimes.join(zh ? "、" : ", ");
    default:
      return "";
  }
}

/** ISO 星期：周一是 1，周日是 7。 */
export function isoWeekday(date: Date): number {
  return date.getDay() === 0 ? 7 : date.getDay();
}

export interface PlanItem {
  name: string;
  sets?: number;
  reps?: number;
  weight?: number;
  note?: string;
}

/** 一个动作的简短说明，例如“深蹲 5×5 60kg”。 */
export function describeItem(item: PlanItem): string {
  let s = item.name;
  if (item.sets && item.reps) s += ` ${item.sets}×${item.reps}`;
  else if (item.sets) s += ` ${item.sets} 组`;
  if (item.weight) s += ` ${formatAmount(item.weight)}kg`;
  return s;
}

export interface DayPlan {
  weekday: number;
  title: string;
  items: PlanItem[];
}

/** 服务端的计划按星期合并成 7 天（同一天多条时合并），方便编辑。 */
export function weekPlans(
  plans: { weekday: number; title: string; items: PlanItem[] }[],
): DayPlan[] {
  const week: DayPlan[] = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({
    weekday,
    title: "",
    items: [],
  }));
  for (const p of plans) {
    const day = week[p.weekday - 1];
    if (!day) continue;
    day.title = [day.title, p.title].filter(Boolean).join(" / ");
    day.items = [...day.items, ...p.items];
  }
  return week;
}

/** 保存时去掉空的一天和没填名字的动作。 */
export function plansToSave(week: DayPlan[]): DayPlan[] {
  return week
    .map((d) => ({
      ...d,
      title: d.title.trim(),
      items: d.items
        .map((i) => ({ ...i, name: i.name.trim() }))
        .filter((i) => i.name),
    }))
    .filter((d) => d.title || d.items.length > 0);
}

/**
 * 今天全部习惯的进度（今日页用）。没达标的按完成比例算，
 * 8 杯水喝了 1 杯算 1/8，不再只数达标的个数。
 */
export function todayProgress(
  list: { done: number; reached: boolean; habit: { dailyTarget: number } }[],
): { total: number; reached: number; started: number; ratio: number } {
  const total = list.length;
  const reached = list.filter((h) => h.reached).length;
  const started = list.filter((h) => !h.reached && h.done > 0).length;
  const sum = list.reduce(
    (n, h) => n + (h.reached ? 1 : ratio(h.done, h.habit.dailyTarget)),
    0,
  );
  return { total, reached, started, ratio: total ? sum / total : 0 };
}
