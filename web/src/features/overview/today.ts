/*
 * “今日”页用到的纯逻辑：问候语、按日期挑 Issue、日程排序。
 * 这里不依赖 React，方便单元测试。
 */

/** 按本地时间的小时选问候语，返回英文原文。 */
export function greetingKey(now: Date): string {
  const h = now.getHours();
  if (h >= 5 && h < 11) return "Good morning";
  if (h >= 11 && h < 13) return "Good noon";
  if (h >= 13 && h < 18) return "Good afternoon";
  return "Good evening";
}

/** 本地日期 YYYY-MM-DD，和 Issue 的 dueDate 格式一样。 */
export function localDateKey(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

interface Dated {
  dueDate?: string;
}

/** 截止日期是今天的。 */
export function dueToday<T extends Dated>(items: T[], now: Date): T[] {
  const today = localDateKey(now);
  return items.filter((i) => i.dueDate === today);
}

/** 截止日期在今天之前的，最早的排前面。 */
export function overdue<T extends Dated>(items: T[], now: Date): T[] {
  const today = localDateKey(now);
  return items
    .filter((i) => i.dueDate !== undefined && i.dueDate < today)
    .sort((a, b) => (a.dueDate ?? "").localeCompare(b.dueDate ?? ""));
}

/** 逾期几天。dueDate 是今天或以后时返回 0。 */
export function daysLate(dueDate: string, now: Date): number {
  const [y, m, d] = dueDate.split("-").map(Number);
  const due = new Date(y, m - 1, d).getTime();
  const today = new Date(
    now.getFullYear(),
    now.getMonth(),
    now.getDate(),
  ).getTime();
  return Math.max(0, Math.round((today - due) / 86_400_000));
}

interface EventLike {
  start: string;
  end: string;
  allDay: boolean;
}

/** 全天事件在前，其余按开始时间排。 */
export function sortEvents<T extends EventLike>(events: T[]): T[] {
  return [...events].sort((a, b) => {
    if (a.allDay !== b.allDay) return a.allDay ? -1 : 1;
    return a.start.localeCompare(b.start);
  });
}

/** 下一项还没结束的非全天日程。 */
export function nextEvent<T extends EventLike>(
  events: T[],
  now: Date,
): T | undefined {
  const t = now.getTime();
  return sortEvents(events).find(
    (e) => !e.allDay && new Date(e.end).getTime() > t,
  );
}

export interface DaySummary {
  dueToday: number;
  reminders: number;
  serversTotal: number;
  serversOffline: number;
  alerts: number;
}

/**
 * 问候下面那一行概况，比如“今天 3 个待办，2 个提醒，服务器全部在线”。
 * 某项数据还没取到时传 undefined，那一段就不显示。
 */
export function summaryLine(
  s: Partial<DaySummary>,
  language: "zh" | "en",
): string {
  const zh = language === "zh";
  const parts: string[] = [];
  if (s.dueToday !== undefined)
    parts.push(zh ? `今天 ${s.dueToday} 个待办` : `${s.dueToday} due today`);
  if (s.reminders !== undefined)
    parts.push(zh ? `${s.reminders} 个提醒` : `${s.reminders} reminders`);
  if (s.serversTotal) {
    const off = s.serversOffline ?? 0;
    const alerts = s.alerts ?? 0;
    if (off > 0)
      parts.push(zh ? `${off} 台服务器离线` : `${off} servers offline`);
    else if (alerts > 0)
      parts.push(zh ? `服务器有 ${alerts} 条告警` : `${alerts} server alerts`);
    else parts.push(zh ? "服务器全部在线" : "all servers online");
  }
  return parts.join(zh ? "，" : ", ");
}
