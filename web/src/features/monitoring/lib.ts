import type {
  DockerContainer,
  DockerStats,
  Monitor,
  MonitorResult,
  ScriptRun,
  Subscription,
  SubscriptionCycle,
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

/** 续费还剩几天的颜色。 */
export function renewalTone(daysLeft: number): Tone {
  if (daysLeft < 0) return "danger";
  if (daysLeft <= 7) return "warn";
  return "";
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
  if (cycle === "custom_days") {
    const date = parseDay(value);
    date.setUTCDate(date.getUTCDate() + Math.max(cycleDays, 1));
    return formatDay(date);
  }
  return addMonths(value, 1);
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

/** 跟随日志的缓冲：完整的行和还没收到换行的半行。 */
export interface LogBuffer {
  lines: string[];
  partial: string;
}

export const emptyLog: LogBuffer = { lines: [], partial: "" };

/** 追加一段日志文本。只留最后 max 行，避免页面越来越慢。 */
export function appendLog(
  buf: LogBuffer,
  chunk: string,
  max = 2000,
): LogBuffer {
  const parts = (buf.partial + chunk).replace(/\r\n/g, "\n").split("\n");
  const partial = parts.pop() ?? "";
  let lines = buf.lines.concat(parts);
  if (lines.length > max) lines = lines.slice(lines.length - max);
  return { lines, partial };
}

/** 缓冲里的全部文本，用于显示。 */
export function logText(buf: LogBuffer): string {
  return buf.partial
    ? [...buf.lines, buf.partial].join("\n")
    : buf.lines.join("\n");
}
