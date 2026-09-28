import type {
  Host,
  HostDetail,
  MetricsPoint,
  MetricsSample,
  MetricsSeries,
} from "./api";

/*
 * 纯函数：换算、格式化、把实时指标合进缓存。都有单元测试。
 */

export function percent(used: number, total: number): number {
  if (!total) return 0;
  return Math.round((used / total) * 1000) / 10;
}

/** 最满的磁盘使用率。 */
export function fullestDisk(sample: MetricsSample): number {
  let best = 0;
  for (const d of sample.disks) best = Math.max(best, percent(d.used, d.total));
  return best;
}

/** 使用率对应的状态：80% 以上提醒，90% 以上危险。 */
export function usageTone(value: number | undefined): "ok" | "warn" | "danger" {
  if (value === undefined) return "ok";
  if (value >= 90) return "danger";
  if (value >= 80) return "warn";
  return "ok";
}

/** 1.2 MB/s */
export function formatRate(bytesPerSecond: number): string {
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let v = Math.max(0, bytesPerSecond);
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

/** 3 天 4 小时 / 3d 4h */
export function formatUptime(seconds: number, zh = true): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (zh) {
    if (d > 0) return `${d} 天 ${h} 小时`;
    if (h > 0) return `${h} 小时 ${m} 分钟`;
    return `${m} 分钟`;
  }
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function pointFromSample(sample: MetricsSample): MetricsPoint {
  return {
    at: sample.at,
    cpu: sample.cpu,
    memory: percent(sample.memUsed, sample.memTotal),
    memUsed: sample.memUsed,
    memTotal: sample.memTotal,
    disk: fullestDisk(sample),
    netRx: sample.netRx,
    netTx: sample.netTx,
    load1: sample.load1,
  };
}

/** 把一条实时指标合进机器信息。收到指标说明机器在线。 */
export function withSample<T extends Host | HostDetail>(
  host: T,
  sample: MetricsSample,
): T {
  return {
    ...host,
    online: true,
    lastSeenAt: sample.at,
    metrics: sample,
    cpu: sample.cpu,
    memory: percent(sample.memUsed, sample.memTotal),
    disk: fullestDisk(sample),
  };
}

/** 往 1 小时曲线末尾追加一个点，并去掉一小时以前的点。 */
export function appendPoint(
  series: MetricsSeries | undefined,
  point: MetricsPoint,
): MetricsSeries | undefined {
  if (!series || series.range !== "1h") return series;
  const cutoff = new Date(point.at).getTime() - 3600_000;
  const points = series.points.filter(
    (p) => new Date(p.at).getTime() > cutoff && p.at !== point.at,
  );
  points.push(point);
  return { ...series, points };
}

/**
 * 按时间切成连续的几段。两点之间隔了 2.5 个步长以上就断开，
 * 这样离线的那段不会被一条直线连起来。
 */
export function splitSegments<T extends { at: string }>(
  points: T[],
  stepSeconds: number,
): T[][] {
  const out: T[][] = [];
  let cur: T[] = [];
  let prev = 0;
  for (const p of points) {
    const t = new Date(p.at).getTime();
    if (cur.length > 0 && t - prev > stepSeconds * 2500) {
      out.push(cur);
      cur = [];
    }
    cur.push(p);
    prev = t;
  }
  if (cur.length > 0) out.push(cur);
  return out;
}

/** 坐标轴上限：取个整的，至少是 min。 */
export function niceMax(value: number, min = 1): number {
  const v = Math.max(value, min);
  const pow = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * pow >= v) return m * pow;
  }
  return 10 * pow;
}

/** 字节速率的坐标轴上限：先换成 KB/MB 再取整，刻度才是整数。 */
export function niceRateMax(bytesPerSecond: number): number {
  let unit = 1;
  while (bytesPerSecond / unit >= 1024 && unit < 1024 ** 3) unit *= 1024;
  return niceMax(bytesPerSecond / unit, 1) * unit;
}

/** 文件路径的上一级和拼接，按代理报告的分隔符处理。 */
export function joinPath(dir: string, name: string, sep: string): string {
  if (dir.endsWith(sep)) return dir + name;
  return dir + sep + name;
}

/** 面包屑：把路径拆成可以点击的几段。 */
export function breadcrumbs(
  path: string,
  sep: string,
): { label: string; path: string }[] {
  if (!path) return [];
  if (sep === "/") {
    const parts = path.split("/").filter(Boolean);
    const out = [{ label: "/", path: "/" }];
    let acc = "";
    for (const p of parts) {
      acc += "/" + p;
      out.push({ label: p, path: acc });
    }
    return out;
  }
  // Windows: C:\Users\me
  const parts = path.split("\\").filter(Boolean);
  const out: { label: string; path: string }[] = [];
  let acc = "";
  parts.forEach((p, i) => {
    acc = i === 0 ? p + "\\" : joinPath(acc, p, "\\");
    out.push({ label: p, path: acc });
  });
  return out;
}

/** 本机页面默认选中的机器：优先在线的。 */
export function pickDesktop(
  hosts: Host[],
  wanted?: string | null,
): Host | undefined {
  return (
    hosts.find((h) => h.id === wanted) ??
    hosts.find((h) => h.online) ??
    hosts[0]
  );
}

// ---- 流量（B27） ----

type CountMode = "both" | "out" | "in" | "max";

/** 按计算方式算出一天或一段时间的用量。 */
export function trafficUsed(rx: number, tx: number, mode: CountMode): number {
  switch (mode) {
    case "out":
      return tx;
    case "in":
      return rx;
    case "max":
      return Math.max(rx, tx);
    default:
      return rx + tx;
  }
}

/** 每天累计的用量。“取大”要按累计值比较，不能每天各取各的。 */
export function cumulativeUsed(
  days: { rx: number; tx: number }[],
  mode: CountMode,
): number[] {
  let rx = 0;
  let tx = 0;
  return days.map((d) => {
    rx += d.rx;
    tx += d.tx;
    return trafficUsed(rx, tx, mode);
  });
}

/** 周期标题：每月 1 号开始、一个月一个周期叫“本月流量”，其他叫“本期流量”。 */
export function trafficTitle(plan: { startDay: number; periodMonths: number }) {
  return plan.startDay === 1 && plan.periodMonths === 1
    ? "Traffic this month"
    : "Traffic this cycle";
}

/** 从 today 到周期最后一天（含）还有几天。 */
export function daysLeft(cycleEnd: string, today: Date = new Date()): number {
  const [y, m, d] = cycleEnd.split("-").map(Number);
  const end = Date.UTC(y, m - 1, d);
  const now = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  return Math.max(0, Math.round((end - now) / 86_400_000) + 1);
}
