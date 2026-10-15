import type { Agent } from "../../api/core";
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

/** X 任务的地址换成看得懂的说法：@账号、搜索：…、推文 123 的回复。 */
export function xLabel(url: string) {
  try {
    const u = new URL(url);
    if (u.pathname === "/search")
      return `搜索：${u.searchParams.get("q") ?? ""}`;
    const post = u.pathname.match(/\/status\/(\d+)/);
    if (post) return `推文 ${post[1]} 的回复`;
    return `@${u.pathname.replace(/^\/|\/$/g, "")}`;
  } catch {
    return url;
  }
}

export function money(v: number | undefined) {
  if (v == null) return "—";
  return `$${v < 1 ? v.toFixed(3) : v.toFixed(2)}`;
}

/** B208：出口下拉框里“服务器直接访问”和“填代理地址”的值，机器用它的 id。 */
export const EGRESS_DIRECT = "";
export const EGRESS_PROXY = ":proxy";
const CAP_EGRESS = "egress";

export function egressOf(w: Pick<Watcher, "egressAgentId" | "hasProxy">) {
  if (w.egressAgentId) return w.egressAgentId;
  return w.hasProxy ? EGRESS_PROXY : EGRESS_DIRECT;
}

/** 能当出口的机器。任务已经选了的那台即使现在不支持也列出来，下拉框才显示得对。 */
export function egressAgents(agents: Agent[], current?: string) {
  return agents.filter(
    (a) => a.capabilities.includes(CAP_EGRESS) || a.id === current,
  );
}

/**
 * B213：新建时不再选类型，按填的网址认。规则和后端一样：
 * @ 开头或者 x.com、twitter.com 的地址是 X；其他 http(s) 地址是网页；
 * 不带 . 也不带 / 的文字是 X 的搜索词。认不出来回 null。
 */
export type GuessedKind = { kind: "web" | "x"; label: string };

const X_HOSTS = /^(?:(?:www|mobile)\.)?(?:x|twitter)\.com$/i;

export function guessKind(input: string): GuessedKind | null {
  const text = input.trim();
  if (!text) return null;
  if (text.startsWith("@")) return { kind: "x", label: "X 账号" };
  const withScheme = /^https?:\/\//i.test(text) ? text : `https://${text}`;
  try {
    const u = new URL(withScheme);
    if (X_HOSTS.test(u.hostname)) {
      if (u.pathname === "/search") return { kind: "x", label: "X 搜索" };
      if (/\/status\/\d+/.test(u.pathname))
        return { kind: "x", label: "X 推文的回复" };
      return { kind: "x", label: "X 账号" };
    }
  } catch {
    // 不是网址，往下按搜索词或者缺了 https:// 处理
  }
  if (/^https?:\/\/\S+$/i.test(text)) return { kind: "web", label: "网页" };
  if (!text.includes(".") && !text.includes("/"))
    return { kind: "x", label: "X 搜索" };
  return null;
}
/**
 * B234：连续几次一样的运行（同类、同结果、同说明）合成一组，
 * 页面上显示成一行“同样 N 次”。列表从新到旧，每组的第一条是最近的一次。
 * 正在跑的运行不合并。
 */
export interface RunGroup<T> {
  latest: T;
  count: number;
}

export function groupRuns<
  T extends { kind: string; outcome: string; detail: string },
>(runs: T[]): RunGroup<T>[] {
  const out: RunGroup<T>[] = [];
  for (const r of runs) {
    const last = out[out.length - 1];
    if (
      last &&
      r.outcome !== "running" &&
      last.latest.outcome !== "running" &&
      last.latest.kind === r.kind &&
      last.latest.outcome === r.outcome &&
      last.latest.detail === r.detail
    )
      last.count++;
    else out.push({ latest: r, count: 1 });
  }
  return out;
}

/** 一次运行里带截图的步骤，按顺序。页内看大图时左右切换用。 */
export function runShots(steps: { text: string; shotId?: number }[]) {
  return steps.flatMap((s) =>
    s.shotId != null ? [{ id: s.shotId, text: s.text }] : [],
  );
}

/** B234：还没读到值时，卡片上写现在在干什么。 */
export function emptyValue(w: Pick<Watcher, "status" | "statusDetail">) {
  if (w.status === "new") return "等第一次检查";
  if (w.status === "exploring") return "AI 在找要盯的内容";
  if (w.statusDetail) return w.statusDetail;
  return STATUS[w.status].label;
}
