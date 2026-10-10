import type { ScreenCategory, ScreenRange } from "./api";

export const CATEGORIES: ScreenCategory[] = [
  "coding",
  "ai",
  "chat",
  "web",
  "entertainment",
  "office",
  "other",
];

/** 英文原文，界面里用 t()。 */
export const CATEGORY_LABELS: Record<ScreenCategory, string> = {
  coding: "Coding",
  ai: "AI tools",
  chat: "Messaging",
  web: "Web",
  entertainment: "Entertainment",
  office: "Office",
  other: "Other",
};

/** 每个类别的颜色。只用主题令牌，深浅主题都能看。 */
export const CATEGORY_COLORS: Record<ScreenCategory, string> = {
  coding: "var(--xc-accent)",
  ai: "var(--xc-info)",
  chat: "var(--xc-ok)",
  web: "var(--xc-warn)",
  entertainment: "var(--xc-danger)",
  office: "var(--xc-text-2)",
  other: "var(--xc-faint)",
};

type T = (key: string) => string;

/** 2 小时 5 分钟、35 分钟。 */
export function durationText(t: T, minutes: number): string {
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m} ${t("min")}`;
  if (m === 0) return `${h} ${t("hr")}`;
  return `${h} ${t("hr")} ${m} ${t("min")}`;
}

/** 概要卡片的大数字：不到一小时写分钟，其余写一位小数的小时。 */
export function statValue(
  t: T,
  minutes: number,
): { value: string; unit: string } {
  if (minutes < 60) return { value: String(minutes), unit: t("min") };
  const hours = Math.round(minutes / 6) / 10;
  return { value: String(hours), unit: t("hr") };
}

/** Code.exe 显示成 Code。 */
export function appName(app: string): string {
  return app.replace(/\.exe$/i, "");
}

// 日期都是 YYYY-MM-DD 的文本，按 UTC 算天数，不受浏览器时区影响。
function parse(date: string): Date {
  return new Date(`${date}T00:00:00Z`);
}

function format(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** 前后翻一天、一周或一个月。from 是当前范围的第一天。 */
export function shiftRange(
  from: string,
  range: ScreenRange,
  delta: -1 | 1,
): string {
  const d = parse(from);
  if (range === "month") d.setUTCMonth(d.getUTCMonth() + delta);
  else d.setUTCDate(d.getUTCDate() + delta * (range === "week" ? 7 : 1));
  return format(d);
}

/** 浏览器本地的今天，只用来判断“下一页”还有没有。 */
export function localToday(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

/** 顶部显示的范围：10月10日 周六、10月5日 到 10月11日、2026年10月。 */
export function rangeLabel(
  range: ScreenRange,
  from: string,
  to: string,
  language: string,
): string {
  const locale = language === "zh" ? "zh-CN" : "en";
  const fmt = (date: string, opts: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat(locale, { timeZone: "UTC", ...opts }).format(
      parse(date),
    );
  if (range === "month") return fmt(from, { year: "numeric", month: "long" });
  if (range === "day")
    return fmt(from, { month: "short", day: "numeric", weekday: "short" });
  const a = fmt(from, { month: "short", day: "numeric" });
  const b = fmt(to, { month: "short", day: "numeric" });
  return `${a} – ${b}`;
}

/** 柱状图下面的日期：周视图写周几，月视图写几号。 */
export function dayLabel(
  date: string,
  range: ScreenRange,
  language: string,
): string {
  if (range === "month") return String(Number(date.slice(8)));
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en", {
    timeZone: "UTC",
    weekday: "short",
  }).format(parse(date));
}
