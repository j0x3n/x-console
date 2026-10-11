import type { RoiGroup, RoiOutcome, RoiTask } from "./api";

type T = (key: string) => string;

/** 没有记录的格子。服务端给 null，不是 0。 */
export const NO_RECORD = "No record";

/** 美元。不到一毛的小额保留四位，其余两位。 */
export function moneyText(v: number | null | undefined): string | null {
  if (v == null) return null;
  if (v === 0) return "$0.00";
  return `$${v >= 0.1 ? v.toFixed(2) : v.toFixed(4)}`;
}

/** 12、3.4k、1.2M。 */
export function tokensText(v: number | null | undefined): string | null {
  if (v == null) return null;
  if (v < 1000) return String(v);
  if (v < 1_000_000) return `${(v / 1000).toFixed(v < 10_000 ? 1 : 0)}k`;
  return `${(v / 1_000_000).toFixed(1)}M`;
}

/** 分钟数写成 35 分钟、2 小时 5 分钟。 */
export function minutesText(
  t: T,
  minutes: number | null | undefined,
): string | null {
  if (minutes == null) return null;
  const total = Math.round(minutes);
  if (total < 60) return `${total} ${t("min")}`;
  const h = Math.floor(total / 60);
  const m = total % 60;
  return m === 0 ? `${h} ${t("hr")}` : `${h} ${t("hr")} ${m} ${t("min")}`;
}

export function secondsText(
  t: T,
  seconds: number | null | undefined,
): string | null {
  if (seconds == null) return null;
  if (seconds < 60) return `${Math.round(seconds)} ${t("sec")}`;
  return minutesText(t, seconds / 60);
}

export function percentText(v: number | null | undefined): string | null {
  if (v == null) return null;
  return `${Math.round(v * 100)}%`;
}

/** 审查分钟除以运行分钟，写成 1.5 倍。 */
export function ratioText(v: number | null | undefined): string | null {
  if (v == null) return null;
  return `${v.toFixed(v < 10 ? 1 : 0)}×`;
}

export const OUTCOME_LABELS: Record<RoiOutcome, string> = {
  merged: "Merged",
  reverted: "Reverted",
  canceled: "Canceled",
  failed: "Failed",
  waiting: "Waiting to merge",
  active: "In progress",
};

export const OUTCOME_TONES: Record<RoiOutcome, string> = {
  merged: "ok",
  reverted: "warn",
  canceled: "",
  failed: "danger",
  waiting: "info",
  active: "accent",
};

/** 分组名下面那一行：还没有结果的和没成的，只写不为 0 的。 */
export function breakdown(t: T, g: RoiGroup): string {
  const parts: string[] = [];
  const add = (n: number, label: string) => {
    if (n > 0) parts.push(`${n} ${t(label)}`);
  };
  add(g.waiting, "Waiting to merge");
  add(g.active, "In progress");
  add(g.canceled, "Canceled");
  add(g.failed, "Failed");
  return parts.join(" · ");
}

// ---------- 排序 ----------

export interface Sort {
  key: string;
  desc: boolean;
}

/** 点同一列在升序、降序间切换；换列先降序（数字大的在前）。 */
export function nextSort(sort: Sort, key: string): Sort {
  if (sort.key === key) return { key, desc: !sort.desc };
  return { key, desc: true };
}

type Cell = number | string | null | undefined;

/** 按一列排序。没有记录的永远排在最后，不管升降序。 */
export function sortRows<R>(
  rows: R[],
  sort: Sort,
  value: (row: R, key: string) => Cell,
): R[] {
  const sign = sort.desc ? -1 : 1;
  return rows
    .map((row, index) => ({ row, index, v: value(row, sort.key) }))
    .sort((a, b) => {
      const an = a.v == null;
      const bn = b.v == null;
      if (an || bn) return an === bn ? a.index - b.index : an ? 1 : -1;
      if (typeof a.v === "number" && typeof b.v === "number")
        return (a.v - b.v) * sign || a.index - b.index;
      return String(a.v).localeCompare(String(b.v)) * sign || a.index - b.index;
    })
    .map((x) => x.row);
}

/** 分组表每一列的值。 */
export function groupCell(g: RoiGroup, key: string): Cell {
  switch (key) {
    case "label":
      return g.label;
    case "tasks":
      return g.tasks;
    case "merged":
      return g.merged;
    case "reverted":
      return g.reverted;
    case "cost":
      return g.cost;
    case "run":
      return g.runSeconds;
    case "review":
      return g.reviewMinutes;
    case "rounds":
      return g.rounds;
    case "costPerMerge":
      return g.costPerMerge;
    case "runPerMerge":
      return g.runSecondsPerMerge;
    case "reviewPerMerge":
      return g.reviewMinutesPerMerge;
    default:
      return null;
  }
}

/** 任务表每一列的值。 */
export function taskCell(task: RoiTask, key: string): Cell {
  switch (key) {
    case "title":
      return task.title;
    case "outcome":
      return task.outcome;
    case "cost":
      return task.cost;
    case "tokens":
      return task.inputTokens == null
        ? null
        : task.inputTokens + (task.outputTokens ?? 0);
    case "run":
      return task.runSeconds;
    case "review":
      return task.reviewMinutes;
    case "rounds":
      return task.rounds;
    case "reviewPerRun":
      return task.reviewPerRun;
    case "created":
      return task.createdAt;
    default:
      return null;
  }
}
