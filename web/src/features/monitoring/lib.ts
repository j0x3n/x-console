import type { LogLine } from "../../components/log/LogViewer";
import type {
  CycleUnit,
  DockerLogLine,
  DockerContainer,
  DockerStats,
  Monitor,
  MonitorResult,
  ScriptRun,
  Subscription,
  SubscriptionCycle,
  SubscriptionSummary,
} from "./api";

/*
 * 监控页的纯逻辑：状态颜色、到期提示、金额、日期推算、图表坐标。
 * 都有单元测试，组件里只管显示。
 */

export type Tone = "ok" | "warn" | "danger" | "";

/** 网站状态的颜色：挂了红色，失败过一次黄色。 */
export function monitorTone(
  m: Pick<Monitor, "enabled" | "lastStatus" | "consecutiveFailures">,
): Tone {
  if (!m.enabled) return "";
  if (m.lastStatus === "down") return "danger";
  if (m.consecutiveFailures > 0) return "warn";
  if (m.lastStatus === "up") return "ok";
  return "";
}

/** 证书剩 14 天内、域名剩 30 天内算快到期。 */
export function expiryTone(
  kind: Monitor["kind"],
  daysLeft: number | undefined,
): Tone {
  if (daysLeft === undefined) return "";
  if (daysLeft <= 0) return "danger";
  const soon = kind === "domain" ? 30 : 14;
  const urgent = kind === "domain" ? 7 : 3;
  if (daysLeft <= urgent) return "danger";
  if (daysLeft <= soon) return "warn";
  return "ok";
}

/** 续费还剩几天的颜色：过期和 3 天内红，7 天内黄（B49）。 */
export function renewalTone(daysLeft: number): Tone {
  if (daysLeft <= 3) return "danger";
  if (daysLeft <= 7) return "warn";
  return "";
}

/** 顶部订阅卡片能切换的币种。 */
export const SPEND_CURRENCIES = ["CNY", "USD"] as const;
export type SpendCurrency = (typeof SPEND_CURRENCIES)[number];

export interface SpendView {
  currency: string;
  monthly: number;
  /** 换算过汇率时为 true */
  converted: boolean;
  /** 没算进去的币种 */
  missing: string[];
}

/**
 * 顶部订阅卡片显示的总额（B49）。后端给了换算结果就用它；
 * 还没有时退回同币种的合计，其他币种列为没算进去。
 */
export function spendView(
  summary: SubscriptionSummary | undefined,
  currency: SpendCurrency,
): SpendView | null {
  if (!summary) return null;
  const conv = summary.converted?.find((x) => x.currency === currency);
  if (conv)
    return {
      currency,
      monthly: conv.monthly,
      converted: true,
      missing: summary.unconverted ?? [],
    };
  if (summary.totals.length === 0) return null;
  const same =
    summary.totals.find((x) => x.currency === currency) ?? summary.totals[0];
  return {
    currency: same.currency,
    monthly: same.monthly,
    converted: false,
    missing: summary.totals
      .map((x) => x.currency)
      .filter((c) => c !== same.currency),
  };
}

/** 金额，保留两位小数，去掉多余的 0。 */
export function formatMoney(amount: number, currency: string): string {
  const text = amount
    .toFixed(2)
    .replace(/\.00$/, "")
    .replace(/(\.\d)0$/, "$1");
  const symbol: Record<string, string> = {
    CNY: "¥",
    USD: "$",
    EUR: "€",
    GBP: "£",
    JPY: "¥",
    HKD: "HK$",
  };
  return symbol[currency]
    ? `${symbol[currency]}${text}`
    : `${text} ${currency}`;
}

function parseDay(value: string): Date {
  const [y, m, d] = value.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
}

function formatDay(date: Date): string {
  return date.toISOString().slice(0, 10);
}

/** 加 n 个月，月底对齐（1 月 31 日加一个月是 2 月 28 日），和服务端一致。 */
export function addMonths(value: string, n: number): string {
  const date = parseDay(value);
  const day = date.getUTCDate();
  const first = new Date(
    Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + n, 1),
  );
  const last = new Date(
    Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0),
  ).getUTCDate();
  first.setUTCDate(Math.min(day, last));
  return formatDay(first);
}

/** 下一个续费日：往后推一个周期。“已续费”按钮用它。 */
export function nextRenewal(
  value: string,
  cycle: SubscriptionCycle,
  cycleDays: number,
): string {
  if (cycle === "yearly") return addMonths(value, 12);
  if (cycle === "custom_days") return addDays(value, Math.max(cycleDays, 1));
  return addMonths(value, 1);
}

function addDays(value: string, n: number): string {
  const date = parseDay(value);
  date.setUTCDate(date.getUTCDate() + n);
  return formatDay(date);
}

export interface Cycle {
  count: number;
  unit: CycleUnit;
}

/** 订阅的周期（B23）。新接口有 cycleCount、cycleUnit；旧接口从 cycle 推出来。 */
export function cycleOf(
  sub: Pick<Subscription, "cycle" | "cycleDays" | "cycleCount" | "cycleUnit">,
): Cycle {
  if (sub.cycleUnit && sub.cycleCount)
    return { count: sub.cycleCount, unit: sub.cycleUnit };
  if (sub.cycle === "yearly") return { count: 1, unit: "year" };
  if (sub.cycle === "custom_days") {
    const days = Math.max(sub.cycleDays, 1);
    return days % 7 === 0
      ? { count: days / 7, unit: "week" }
      : { count: days, unit: "day" };
  }
  return { count: 1, unit: "month" };
}

/**
 * 换成旧接口认识的 cycle。旧接口只有每月、每年、每隔几天，
 * 对不上时返回 null（比如每 3 个月、每小时）。
 */
export function legacyCycle({
  count,
  unit,
}: Cycle): { cycle: SubscriptionCycle; cycleDays?: number } | null {
  if (unit === "month" && count === 1) return { cycle: "monthly" };
  if (unit === "year" && count === 1) return { cycle: "yearly" };
  if (unit === "day") return { cycle: "custom_days", cycleDays: count };
  if (unit === "week") return { cycle: "custom_days", cycleDays: count * 7 };
  return null;
}

/**
 * 往后推 N 个单位，结果只保留日期。分钟和小时至少推一天，
 * 不然“已续费”按了没变化。
 */
export function addCycle(value: string, { count, unit }: Cycle): string {
  const n = Math.max(count, 1);
  switch (unit) {
    case "year":
      return addMonths(value, 12 * n);
    case "month":
      return addMonths(value, n);
    case "week":
      return addDays(value, 7 * n);
    case "day":
      return addDays(value, n);
    case "hour":
      return addDays(value, Math.max(1, Math.ceil(n / 24)));
    default:
      return addDays(value, Math.max(1, Math.ceil(n / 1440)));
  }
}

const everyOne: Record<CycleUnit, string> = {
  minute: "Every minute",
  hour: "Every hour",
  day: "Every day",
  week: "Every week",
  month: "Every month",
  year: "Every year",
};
export const unitLabels: Record<CycleUnit, string> = {
  minute: "Minute(s)",
  hour: "Hour(s)",
  day: "Day(s)",
  week: "Week(s)",
  month: "Month(s)",
  year: "Year(s)",
};

/** “每月”“每 3 个月”。t 是翻译函数。 */
export function cycleText(c: Cycle, t: (key: string) => string): string {
  return c.count === 1
    ? t(everyOne[c.unit])
    : `${t("Every")} ${c.count} ${t(unitLabels[c.unit])}`;
}

/** 把 "7, 1" 这样的输入变成去重、从大到小的天数。无效输入返回 null。 */
export function parseRemindDays(text: string): number[] | null {
  const parts = text
    .split(/[,，\s]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  const out: number[] = [];
  for (const part of parts) {
    if (!/^\d+$/.test(part)) return null;
    const n = Number(part);
    if (n > 365) return null;
    if (!out.includes(n)) out.push(n);
  }
  return out.sort((a, b) => b - a);
}

/** 按下次续费日排序，已过期的排最前。 */
export function sortSubscriptions(items: Subscription[]): Subscription[] {
  return [...items].sort(
    (a, b) => a.daysLeft - b.daysLeft || a.name.localeCompare(b.name),
  );
}

/** 运行结果的颜色。 */
export function runTone(run: Pick<ScriptRun, "status">): Tone {
  if (run.status === "ok") return "ok";
  if (run.status === "failed") return "danger";
  return "warn";
}

export interface ChartPoint {
  x: number;
  y: number;
  ok: boolean;
}

/**
 * 延迟曲线的坐标。横轴按时间铺满 width，纵轴 0 在底部。
 * 返回点和纵轴上限（取整到好看的数）。
 */
export function latencyPoints(
  items: Pick<MonitorResult, "at" | "latencyMs" | "ok">[],
  width: number,
  height: number,
): { points: ChartPoint[]; max: number } {
  if (items.length === 0) return { points: [], max: 0 };
  const times = items.map((r) => new Date(r.at).getTime());
  const start = times[0];
  const span = Math.max(times[times.length - 1] - start, 1);
  const top = niceCeil(Math.max(...items.map((r) => r.latencyMs), 1));
  const points = items.map((r, i) => ({
    x: items.length === 1 ? width / 2 : ((times[i] - start) / span) * width,
    y: height - (r.latencyMs / top) * height,
    ok: r.ok,
  }));
  return { points, max: top };
}

/**
 * 色带的分段：每个点占到和相邻点的中间，首尾铺到边上，中间没有缝。
 * 返回每段的 [x, 宽度]。
 */
export function stripSegments(xs: number[], width: number): [number, number][] {
  return xs.map((x, i) => {
    const left = i === 0 ? 0 : (xs[i - 1] + x) / 2;
    const right = i === xs.length - 1 ? width : (x + xs[i + 1]) / 2;
    return [left, Math.max(right - left, 1)];
  });
}

/** 取整到 1、2、5 乘 10 的幂。 */
export function niceCeil(value: number): number {
  const exp = Math.pow(10, Math.floor(Math.log10(value)));
  for (const step of [1, 2, 5, 10]) {
    if (value <= step * exp) return step * exp;
  }
  return 10 * exp;
}

export interface ContainerRow extends DockerContainer {
  stats?: DockerStats;
}

/** 把资源占用合进容器列表。运行中的排前面。 */
export function mergeStats(
  containers: DockerContainer[],
  stats: DockerStats[] | undefined,
): ContainerRow[] {
  const byId = new Map((stats ?? []).map((s) => [s.id, s]));
  return containers
    .map((c) => ({ ...c, stats: byId.get(c.id) }))
    .sort(
      (a, b) =>
        Number(b.state === "running") - Number(a.state === "running") ||
        a.name.localeCompare(b.name),
    );
}

/** 容器状态的颜色。 */
export function containerTone(state: string): Tone {
  if (state === "running") return "ok";
  if (state === "restarting" || state === "paused" || state === "created")
    return "warn";
  if (state === "dead") return "danger";
  return "";
}

/** 端口显示成 8080→80/tcp。 */
export function formatPorts(ports: DockerContainer["ports"]): string {
  const seen = new Set<string>();
  for (const p of ports) {
    seen.add(
      p.publicPort
        ? `${p.publicPort}→${p.privatePort}/${p.type}`
        : `${p.privatePort}/${p.type}`,
    );
  }
  return [...seen].join(", ");
}

/** 跟随日志的缓冲（B28）：完整的行和还没收到换行的半行。 */
export interface LogBuffer {
  lines: LogLine[];
  partial: string;
  nextId: number;
}

export const emptyLog: LogBuffer = { lines: [], partial: "", nextId: 1 };

function keepLast(lines: LogLine[], max: number) {
  return lines.length > max ? lines.slice(lines.length - max) : lines;
}

/** 追加一段纯文本日志（旧服务端）。只留最后 max 行，避免页面越来越慢。 */
export function appendLog(
  buf: LogBuffer,
  chunk: string,
  max = 5000,
): LogBuffer {
  const parts = (buf.partial + chunk).replace(/\r\n/g, "\n").split("\n");
  const partial = parts.pop() ?? "";
  let id = buf.nextId;
  const added = parts.map((text) => ({ id: id++, text }));
  return {
    lines: keepLast(buf.lines.concat(added), max),
    partial,
    nextId: id,
  };
}

/** 追加 format=json 的一批日志行，带标准输出和错误输出。 */
export function appendLogLines(
  buf: LogBuffer,
  items: DockerLogLine[],
  max = 5000,
): LogBuffer {
  let id = buf.nextId;
  const added = items.map((l) => ({
    id: id++,
    text: l.text,
    stream: l.stream,
    time: l.time,
  }));
  return {
    lines: keepLast(buf.lines.concat(added), max),
    partial: buf.partial,
    nextId: id,
  };
}

/** 一帧日志：新服务端发 JSON 数组，旧服务端发纯文本。 */
export function parseLogFrame(data: string): DockerLogLine[] | null {
  if (!data.startsWith("[")) return null;
  try {
    const v = JSON.parse(data);
    return Array.isArray(v) && v.every((x) => typeof x?.text === "string")
      ? v
      : null;
  } catch {
    return null;
  }
}

export type ContainerSort = "name" | "state" | "cpu" | "mem";

/** 容器排序（B28）：CPU 和内存从大到小，没有统计的排最后。 */
export function sortContainers(
  rows: ContainerRow[],
  sort: ContainerSort,
): ContainerRow[] {
  const out = [...rows];
  switch (sort) {
    case "cpu":
      return out.sort(
        (a, b) => (b.stats?.cpuPercent ?? -1) - (a.stats?.cpuPercent ?? -1),
      );
    case "mem":
      return out.sort(
        (a, b) => (b.stats?.memUsage ?? -1) - (a.stats?.memUsage ?? -1),
      );
    case "state":
      return out.sort(
        (a, b) =>
          (a.state === "running" ? 0 : 1) - (b.state === "running" ? 0 : 1) ||
          a.name.localeCompare(b.name),
      );
    default:
      return out.sort((a, b) => a.name.localeCompare(b.name));
  }
}
