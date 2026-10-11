import type { GoalPeriod, GoalSource, GoalState } from "./api";

/* 目标页用的纯函数：季度、数字、文字、日期。 */

export const SOURCES: GoalSource[] = [
  "manual",
  "habit_checkins",
  "workouts",
  "focus_minutes",
  "screen_minutes",
  "github_commits",
  "github_merged_prs",
];

/** 来源的名称，英文原文，界面里用 t()。 */
export const SOURCE_LABELS: Record<GoalSource, string> = {
  manual: "Fill in by hand",
  habit_checkins: "Habit check-ins",
  workouts: "Workouts",
  focus_minutes: "Focus minutes",
  screen_minutes: "Computer minutes",
  github_commits: "GitHub commits",
  github_merged_prs: "GitHub merged PRs",
};

/** 选了来源后单位框里先填什么。 */
export const SOURCE_UNITS: Record<GoalSource, string> = {
  manual: "",
  habit_checkins: "次",
  workouts: "次",
  focus_minutes: "分钟",
  screen_minutes: "分钟",
  github_commits: "个",
  github_merged_prs: "个",
};

export const STATE_LABELS: Record<GoalState, string> = {
  done: "Reached",
  on_track: "On track",
  behind: "Behind",
  upcoming: "Not started",
  missed: "Missed",
};

export function stateTone(state: GoalState): string {
  switch (state) {
    case "done":
      return "ok";
    case "behind":
      return "warn";
    case "missed":
      return "danger";
    case "on_track":
      return "info";
    default:
      return "";
  }
}

export function currentPeriod(now: Date = new Date()): GoalPeriod {
  return {
    year: now.getFullYear(),
    quarter: Math.floor(now.getMonth() / 3) + 1,
  };
}

/** 往前或往后移若干个季度。 */
export function shiftPeriod(p: GoalPeriod, delta: number): GoalPeriod {
  const index = p.year * 4 + (p.quarter - 1) + delta;
  return { year: Math.floor(index / 4), quarter: (index % 4) + 1 };
}

export function samePeriod(a: GoalPeriod, b: GoalPeriod): boolean {
  return a.year === b.year && a.quarter === b.quarter;
}

export function quarterLabel(p: GoalPeriod): string {
  return `${p.year} 年第 ${p.quarter} 季度`;
}

/** 最多一位小数，去掉多余的 0：12 → "12"，12.50 → "12.5"。 */
export function fmtNum(v: number): string {
  return String(Math.round(v * 10) / 10);
}

export function fmtPercent(v: number | null | undefined): string {
  return v == null ? "–" : `${fmtNum(v)}%`;
}

/** 带正负号的变化："+2.5"、"-1"、"0"。 */
export function fmtChange(v: number | null | undefined): string {
  if (v == null) return "–";
  const n = Math.round(v * 10) / 10;
  if (n === 0) return "0";
  return n > 0 ? `+${n}` : String(n);
}

/** "41.5 / 80 次" */
export function valueText(
  value: number | null | undefined,
  target: number,
  unit: string,
): string {
  const v = value == null ? "–" : fmtNum(value);
  return `${v} / ${fmtNum(target)}${unit ? ` ${unit}` : ""}`;
}

/** 本季度进行中的目标，平均进度和落后的数量，给概要卡片。 */
export function summarize(
  goals: { progress: number | null; state: GoalState; status: string }[],
) {
  const active = goals.filter((g) => g.status === "active");
  const measured = active.filter((g) => g.progress != null);
  const average = measured.length
    ? Math.round(
        (measured.reduce((sum, g) => sum + (g.progress as number), 0) /
          measured.length) *
          10,
      ) / 10
    : null;
  return {
    active: active.length,
    average,
    behind: active.filter((g) => g.state === "behind").length,
  };
}

const DAY = 24 * 60 * 60 * 1000;

function parseDay(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}

/** "11 月 9 日到 11 月 15 日" */
export function weekLabel(start: string, end: string): string {
  const a = parseDay(start);
  const b = parseDay(end);
  return `${a.getMonth() + 1} 月 ${a.getDate()} 日到 ${b.getMonth() + 1} 月 ${b.getDate()} 日`;
}

/** 前一周或后一周的周一，格式 YYYY-MM-DD。 */
export function shiftWeek(start: string, delta: number): string {
  const d = parseDay(start);
  const next = new Date(d.getFullYear(), d.getMonth(), d.getDate() + delta * 7);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${next.getFullYear()}-${pad(next.getMonth() + 1)}-${pad(next.getDate())}`;
}

/** 两个 YYYY-MM-DD 之间相差几周（后减前）。 */
export function weeksBetween(from: string, to: string): number {
  return Math.round(
    (parseDay(to).getTime() - parseDay(from).getTime()) / (7 * DAY),
  );
}

/** 进度条上的两个位置：已到的和按时间应到的，限制在 0 到 100。 */
export function barWidths(
  progress: number | null | undefined,
  expected: number,
): { fill: number; mark: number } {
  const clamp = (v: number) => Math.max(0, Math.min(100, v));
  return { fill: clamp(progress ?? 0), mark: clamp(expected) };
}

/** 不能为空的数字输入：返回数字，不合格返回 null。 */
export function parseNumber(text: string): number | null {
  const s = text.trim();
  if (s === "") return null;
  const n = Number(s);
  return Number.isFinite(n) ? n : null;
}
