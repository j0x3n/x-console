import type { CheckinSeries } from "./api";

export const CHART_W = 320;
export const CHART_H = 84;
const PAD = 8;

const dayNumber = (date: string) => {
  const [y, m, d] = date.split("-").map(Number);
  return Math.round(Date.UTC(y, m - 1, d) / 86_400_000);
};

/** 一条折线的坐标。横轴是 from 到 to 的每一天，纵轴是 1 到 5 分。 */
export function linePoints(
  points: CheckinSeries["points"],
  from: string,
  to: string,
): Array<{ x: number; y: number; date: string; average: number }> {
  const start = dayNumber(from);
  const span = Math.max(dayNumber(to) - start, 1);
  return points.map((p) => ({
    date: p.date,
    average: p.average,
    x: PAD + ((dayNumber(p.date) - start) / span) * (CHART_W - PAD * 2),
    y: CHART_H - PAD - ((p.average - 1) / 4) * (CHART_H - PAD * 2),
  }));
}

export const pathOf = (pts: Array<{ x: number; y: number }>) =>
  pts
    .map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)} ${p.y.toFixed(1)}`)
    .join(" ");

export const fmtScore = (v: number) =>
  v === 0 ? "—" : v.toLocaleString("en-US", { maximumFractionDigits: 1 });
