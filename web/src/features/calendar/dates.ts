/* 日历视图的纯计算：日期范围、全天事件、同一天里事件的排布。时间都按浏览器本地时区。 */

export type CalendarView = "day" | "week";

/** 事件里用到的字段，方便测试时构造。 */
export interface EventLike {
  id: string;
  title: string;
  start: string;
  end: string;
  allDay: boolean;
  startDate?: string;
  endDate?: string;
}

export const DAY_MINUTES = 24 * 60;

export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/** 按日历天加减，跨夏令时也不会错位。 */
export function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

/** 本周一。 */
export function startOfWeek(d: Date): Date {
  const day = startOfDay(d);
  const offset = (day.getDay() + 6) % 7;
  return addDays(day, -offset);
}

/** 本地日期 YYYY-MM-DD。 */
export function dayKey(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

/** 解析 YYYY-MM-DD 为本地零点，格式不对时返回 null。 */
export function parseDayKey(s: string | null | undefined): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s ?? "");
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  return Number.isNaN(d.getTime()) ? null : d;
}

export function viewRange(
  view: CalendarView,
  anchor: Date,
): { from: Date; to: Date; days: Date[] } {
  const from = view === "week" ? startOfWeek(anchor) : startOfDay(anchor);
  const count = view === "week" ? 7 : 1;
  const days = Array.from({ length: count }, (_, i) => addDays(from, i));
  return { from, to: addDays(from, count), days };
}

/** 全天事件是否落在这一天（结束日期不含）。 */
export function coversDay(e: EventLike, day: Date): boolean {
  const key = dayKey(day);
  const start = e.startDate ?? dayKey(new Date(e.start));
  const end = e.endDate ?? dayKey(new Date(e.end));
  return start <= key && key < end;
}

export function allDayEvents<T extends EventLike>(events: T[], day: Date): T[] {
  return events.filter((e) => e.allDay && coversDay(e, day));
}

export interface Block<T extends EventLike = EventLike> {
  event: T;
  /** 当天零点起的分钟数 */
  top: number;
  /** 分钟数，至少 15 */
  height: number;
  column: number;
  columns: number;
  continuesBefore: boolean;
  continuesAfter: boolean;
}

/**
 * 把一天里有具体时间的事件排成列：互相重叠的事件并排显示，
 * 不重叠的事件占满整宽。跨天的事件只取当天那一段。
 */
export function layoutDay<T extends EventLike>(
  events: T[],
  day: Date,
): Block<T>[] {
  const dayStart = startOfDay(day).getTime();
  const dayEnd = addDays(day, 1).getTime();
  const items = events
    .filter((e) => !e.allDay)
    .map((e) => {
      const s = new Date(e.start).getTime();
      const en = Math.max(new Date(e.end).getTime(), s);
      return { e, s, en };
    })
    .filter(({ s, en }) =>
      en === s ? s >= dayStart && s < dayEnd : s < dayEnd && en > dayStart,
    )
    .sort(
      (a, b) => a.s - b.s || b.en - a.en || a.e.title.localeCompare(b.e.title),
    );

  const blocks: Block<T>[] = [];
  let cluster: Block<T>[] = [];
  let colEnds: number[] = [];
  let clusterEnd = -Infinity;
  const flush = () => {
    for (const b of cluster) b.columns = colEnds.length;
    cluster = [];
    colEnds = [];
  };
  for (const { e, s, en } of items) {
    const top = Math.max(0, (s - dayStart) / 60000);
    const bottom = Math.min(DAY_MINUTES, (en - dayStart) / 60000);
    const height = Math.max(15, bottom - top);
    // 按显示高度判断重叠，短事件也不会叠在一起。
    const visStart = top;
    const visEnd = Math.min(DAY_MINUTES, top + height);
    if (visStart >= clusterEnd) flush();
    let column = colEnds.findIndex((end) => end <= visStart);
    if (column === -1) {
      column = colEnds.length;
      colEnds.push(visEnd);
    } else {
      colEnds[column] = visEnd;
    }
    clusterEnd = cluster.length === 0 ? visEnd : Math.max(clusterEnd, visEnd);
    const block: Block<T> = {
      event: e,
      top,
      height: Math.min(height, DAY_MINUTES - top),
      column,
      columns: 1,
      continuesBefore: s < dayStart,
      continuesAfter: en > dayEnd,
    };
    cluster.push(block);
    blocks.push(block);
  }
  flush();
  return blocks;
}

/** 视图标题：9月27日 或 9月22日 – 28日。 */
export function rangeTitle(
  view: CalendarView,
  days: Date[],
  language: "zh" | "en",
): string {
  const locale = language === "zh" ? "zh-CN" : "en";
  const first = days[0];
  const last = days[days.length - 1];
  if (view === "day") {
    return first.toLocaleDateString(locale, {
      year: "numeric",
      month: "long",
      day: "numeric",
      weekday: "short",
    });
  }
  const fmt: Intl.DateTimeFormatOptions = { month: "short", day: "numeric" };
  const a = first.toLocaleDateString(locale, { year: "numeric", ...fmt });
  const b = last.toLocaleDateString(locale, fmt);
  return `${a} – ${b}`;
}

/** 番茄钟剩余时间 mm:ss，超时后是 00:00。 */
export function formatRemaining(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

/** 秒数写成 1 小时 5 分 / 25 分。 */
export function formatDuration(seconds: number, language: "zh" | "en"): string {
  const minutes = Math.round(seconds / 60);
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (language === "zh") {
    if (h === 0) return `${m} 分钟`;
    return m === 0 ? `${h} 小时` : `${h} 小时 ${m} 分`;
  }
  if (h === 0) return `${m} min`;
  return m === 0 ? `${h} h` : `${h} h ${m} min`;
}
