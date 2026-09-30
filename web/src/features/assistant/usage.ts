/*
 * AI 用量页（B42）的纯函数：时间范围、数字格式、和上一段时间比。
 * 日期都是本地日期 YYYY-MM-DD，服务器按它的时区算（和浏览器一般一致）。
 */

export type UsageRange =
  | "today"
  | "7d"
  | "30d"
  | "month"
  | "lastMonth"
  | "custom";

function pad(n: number) {
  return String(n).padStart(2, "0");
}

export function isoDate(d: Date) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 选中的时间范围对应的起止日期（都包含）。 */
export function rangeDates(
  range: Exclude<UsageRange, "custom">,
  now = new Date(),
): { from: string; to: string } {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const back = (days: number) =>
    new Date(today.getFullYear(), today.getMonth(), today.getDate() - days);
  switch (range) {
    case "today":
      return { from: isoDate(today), to: isoDate(today) };
    case "7d":
      return { from: isoDate(back(6)), to: isoDate(today) };
    case "30d":
      return { from: isoDate(back(29)), to: isoDate(today) };
    case "month":
      return {
        from: isoDate(new Date(today.getFullYear(), today.getMonth(), 1)),
        to: isoDate(today),
      };
    case "lastMonth":
      return {
        from: isoDate(new Date(today.getFullYear(), today.getMonth() - 1, 1)),
        to: isoDate(new Date(today.getFullYear(), today.getMonth(), 0)),
      };
  }
}

/** 升降百分比，上一段为 0 时没有可比的数字。 */
export function change(current: number, previous: number | undefined) {
  if (!previous) return undefined;
  return (current - previous) / previous;
}

export function formatChange(v: number | undefined) {
  if (v === undefined || !Number.isFinite(v)) return "";
  const pct = Math.round(v * 100);
  if (pct === 0) return "±0%";
  return `${pct > 0 ? "↑" : "↓"}${Math.abs(pct)}%`;
}

export function formatRate(v: number | undefined) {
  if (v === undefined) return "-";
  return `${Math.round(v * 1000) / 10}%`;
}

/** 柱状图的刻度：取一个整齐的上限，给 4 条网格线。 */
export function niceMax(v: number) {
  if (v <= 0) return 1;
  const exp = Math.pow(10, Math.floor(Math.log10(v)));
  for (const step of [1, 2, 2.5, 5, 10]) {
    if (step * exp >= v) return step * exp;
  }
  return 10 * exp;
}
