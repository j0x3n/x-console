import type { DayPlanItem, DayPlanItemInput } from "./api";

const pad = (n: number) => String(n).padStart(2, "0");

/** 本地日期 YYYY-MM-DD，offset 是相对今天的天数。 */
export function dateOffset(offset: number, from = new Date()): string {
  const d = new Date(
    from.getFullYear(),
    from.getMonth(),
    from.getDate() + offset,
  );
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function minutesOf(hm: string): number | null {
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(hm)) return null;
  return Number(hm.slice(0, 2)) * 60 + Number(hm.slice(3));
}

/** 一行时间显示：“09:00–10:30”，没排上时间写“未排时间”。 */
export function rangeText(start: string, end: string): string {
  return start && end ? `${start}–${end}` : "";
}

/**
 * 检查编辑中的事项。返回第一条错误的说明，没有问题返回空字符串。
 * 起止时间要么都填要么都不填；结束晚于开始；有时间的事项不重叠。
 */
export function checkItems(items: DayPlanItemInput[]): string {
  if (items.length > 8) return "一天最多 8 件事";
  const spans: Array<[number, number]> = [];
  for (const it of items) {
    if (!it.title.trim()) return "每件事都要有名字";
    const start = it.start ?? "";
    const end = it.end ?? "";
    if (!start && !end) continue;
    const s = minutesOf(start);
    const e = minutesOf(end);
    if (s == null || e == null) return "开始和结束时间要一起填";
    if (e <= s) return "结束时间要晚于开始时间";
    if (spans.some(([a, b]) => s < b && a < e)) return "两件事的时间重叠了";
    spans.push([s, e]);
  }
  return "";
}

/** 编辑后的事项和服务端的是否一样（只比标题、时间、完成）。 */
export function sameItems(a: DayPlanItem[], b: DayPlanItemInput[]): boolean {
  if (a.length !== b.length) return false;
  return a.every((it, i) => {
    const x = b[i];
    return (
      it.title === x.title &&
      it.start === (x.start ?? "") &&
      it.end === (x.end ?? "") &&
      it.done === (x.done ?? false)
    );
  });
}

/** 今日页卡片用的统计：几件做完了。 */
export function doneCount(items: DayPlanItem[]): number {
  return items.filter((i) => i.done).length;
}
