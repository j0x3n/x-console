import type { Language } from "../types/domain";

/** "3 分钟前" / "3m ago" 这类相对时间。 */
export function relativeTime(
  value: string | Date,
  language: Language = "zh",
): string {
  const date = typeof value === "string" ? new Date(value) : value;
  const seconds = Math.round((Date.now() - date.getTime()) / 1000);
  const rtf = new Intl.RelativeTimeFormat(language === "zh" ? "zh-CN" : "en", {
    numeric: "auto",
  });
  const abs = Math.abs(seconds);
  if (abs < 45) return language === "zh" ? "刚刚" : "just now";
  if (abs < 3600) return rtf.format(-Math.round(seconds / 60), "minute");
  if (abs < 86400) return rtf.format(-Math.round(seconds / 3600), "hour");
  if (abs < 86400 * 30) return rtf.format(-Math.round(seconds / 86400), "day");
  return formatDate(date, language);
}

/** 9月27日 周六 / Sat, Sep 27 */
export function formatDate(
  value: string | Date,
  language: Language = "zh",
): string {
  const date = typeof value === "string" ? new Date(value) : value;
  return date.toLocaleDateString(language === "zh" ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    weekday: "short",
  });
}

/** 14:05 */
export function formatTime(
  value: string | Date,
  language: Language = "zh",
): string {
  const date = typeof value === "string" ? new Date(value) : value;
  return date.toLocaleTimeString(language === "zh" ? "zh-CN" : "en", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/** 1.5 GB */
export function formatBytes(bytes: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}
