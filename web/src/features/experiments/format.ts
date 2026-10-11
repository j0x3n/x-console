import type {
  ExperimentPhase,
  ExperimentResult,
  ExperimentVerdict,
} from "./api";

export const PHASE_LABELS: Record<ExperimentPhase, string> = {
  planned: "Experiment planned",
  baseline: "Baseline phase",
  change: "Change phase",
  finished: "Experiment finished",
  stopped: "Experiment stopped",
};

export const PHASE_TONE: Record<ExperimentPhase, string> = {
  planned: "",
  baseline: "info",
  change: "accent",
  finished: "ok",
  stopped: "warn",
};

export const NOTE_TEXT: Record<ExperimentResult["notes"][number], string> = {
  subjective: "这是你自己打的分，容易受预期影响。",
  change_running: "改变期还没结束，结果还会变。",
  sparse_data: "有数据的天数不到六成，结果不稳。",
  retrospective: "这是回头对比，数据先于实验存在，不是事先安排好的。",
  many_metrics: "看的指标越多，越容易有一个碰巧出现差异。",
};

/** 数字显示：最多两位小数，不显示末尾的 0。 */
export function fmt(v: number | undefined | null, digits = 2): string {
  if (v == null || !Number.isFinite(v)) return "—";
  return v.toLocaleString("en-US", { maximumFractionDigits: digits });
}

export function signed(v: number | undefined | null, digits = 2): string {
  if (v == null || !Number.isFinite(v)) return "—";
  return `${v > 0 ? "+" : ""}${fmt(v, digits)}`;
}

/** 结论一句话。先说够不够下结论，再说数字。 */
export function verdictText(
  verdict: ExperimentVerdict,
  r: Pick<ExperimentResult, "diff" | "ciLow" | "ciHigh" | "metric">,
): string {
  const unit = r.metric.unit ? ` ${r.metric.unit}` : "";
  const range = `95% 区间 ${fmt(r.ciLow)} 到 ${fmt(r.ciHigh)}`;
  switch (verdict) {
    case "no_data":
      return "还没有数据。";
    case "too_few":
      return "天数太少，不能下结论。每段至少要有 5 天有数据。";
    case "inconclusive":
      return "差异在随机波动范围内，不能说明有变化。";
    case "higher":
      return `改变期更高，平均高 ${fmt(r.diff)}${unit}（${range}）。`;
    case "lower":
      return `改变期更低，平均低 ${fmt(Math.abs(r.diff ?? 0))}${unit}（${range}）。`;
  }
}

export const VERDICT_TONE: Record<ExperimentVerdict, string> = {
  no_data: "",
  too_few: "warn",
  inconclusive: "info",
  higher: "ok",
  lower: "ok",
};

const dayNumber = (date: string) => {
  const [y, m, d] = date.split("-").map(Number);
  return Math.round(Date.UTC(y, m - 1, d) / 86_400_000);
};

export const CHART_W = 360;
export const CHART_H = 120;
const PAD = 10;

/** 点的坐标。横轴是 from 到 to 的每一天，纵轴按数据的最小最大值撑开。 */
export function scatter(
  points: ExperimentResult["points"],
  from: string,
  to: string,
) {
  const start = dayNumber(from);
  const span = Math.max(dayNumber(to) - start, 1);
  const values = points.map((p) => p.value);
  let lo = Math.min(...values);
  let hi = Math.max(...values);
  if (!Number.isFinite(lo)) lo = hi = 0;
  if (hi === lo) {
    lo -= 1;
    hi += 1;
  }
  const y = (v: number) =>
    CHART_H - PAD - ((v - lo) / (hi - lo)) * (CHART_H - PAD * 2);
  const x = (date: string) =>
    PAD + ((dayNumber(date) - start) / span) * (CHART_W - PAD * 2);
  return {
    lo,
    hi,
    x,
    y,
    dots: points.map((p) => ({ ...p, cx: x(p.date), cy: y(p.value) })),
  };
}
