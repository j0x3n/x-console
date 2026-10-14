import type { Watcher, WatcherStatus } from "./api";

/** 状态的中文名和颜色。 */
export const STATUS: Record<
  WatcherStatus,
  { label: string; tone: "ok" | "warn" | "danger" | "info" | "" }
> = {
  new: { label: "还没跑", tone: "" },
  exploring: { label: "AI 在摸索", tone: "info" },
  checking: { label: "检查中", tone: "info" },
  ok: { label: "正常", tone: "ok" },
  waiting: { label: "等你处理", tone: "warn" },
  login: { label: "需要登录", tone: "warn" },
  error: { label: "出错", tone: "danger" },
  paused: { label: "已暂停", tone: "" },
};

/** 要用户动手的任务：等处理和需要登录。 */
export function needsUser(w: Pick<Watcher, "status">) {
  return w.status === "waiting" || w.status === "login";
}

export const INTERVALS = [15, 30, 60, 120, 360, 720, 1440];
export const KEEPALIVES = [0, 10, 20, 30];

/** 15 分钟、1 小时、1 天 */
export function minutesText(m: number) {
  if (m === 0) return "不保活";
  if (m % 1440 === 0) return `${m / 1440} 天`;
  if (m % 60 === 0) return `${m / 60} 小时`;
  return `${m} 分钟`;
}

/** 输入框里的域名：逗号、空格、换行都能分开。 */
export function parseHosts(text: string) {
  return text
    .split(/[\s,，]+/)
    .map((h) => h.trim())
    .filter(Boolean);
}

/** 列表排序：要你处理的在前，然后出错的，其余按名字。 */
export function sortWatchers<T extends Pick<Watcher, "status" | "name">>(
  list: T[],
) {
  const rank = (w: T) =>
    needsUser(w) ? 0 : w.status === "error" ? 1 : w.status === "paused" ? 3 : 2;
  return [...list].sort(
    (a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name, "zh-CN"),
  );
}

/** 接管画面上的点换成页面坐标。 */
export function toPage(
  clientX: number,
  clientY: number,
  rect: { left: number; top: number; width: number; height: number },
  page: { width: number; height: number },
) {
  const x = ((clientX - rect.left) / rect.width) * page.width;
  const y = ((clientY - rect.top) / rect.height) * page.height;
  return {
    x: Math.max(0, Math.min(page.width, Math.round(x))),
    y: Math.max(0, Math.min(page.height, Math.round(y))),
  };
}

export function money(v: number | undefined) {
  if (v == null) return "—";
  return `$${v < 1 ? v.toFixed(3) : v.toFixed(2)}`;
}
