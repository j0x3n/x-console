import type { Localize } from "../types/domain";
export function relativeTime(value: string, L: Localize) {
  const match = /^(\d+)(m|h|d)$/.exec(value);
  if (!match) {
    const date =
      /^(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) (\d+)$/.exec(value);
    if (!date) return L(value);
    const month =
      [
        "Jan",
        "Feb",
        "Mar",
        "Apr",
        "May",
        "Jun",
        "Jul",
        "Aug",
        "Sep",
        "Oct",
        "Nov",
        "Dec",
      ].indexOf(date[1]) + 1;
    return L(value, `${month} 月 ${date[2]} 日`);
  }
  const units: Record<string, string> = { m: "分钟", h: "小时", d: "天" };
  const unit = units[match[2]];
  return L(value, `${match[1]} ${unit}前`);
}

export function durationTime(value: string, L: Localize) {
  const match = /^(?:(\d+)m )?(\d+)s$/.exec(value);
  return match
    ? L(value, `${match[1] ? `${match[1]} 分 ` : ""}${match[2]} 秒`)
    : L(value);
}
