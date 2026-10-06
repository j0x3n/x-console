import type { Language } from "../../types/domain";
import type { QuotaAccount, QuotaKind, QuotaWindow } from "./api";

/* 额度的显示逻辑，纯函数，不依赖 React。 */

export const KIND_ORDER: QuotaKind[] = ["claude", "codex", "grok", "deepseek"];

export const KIND_NAMES: Record<QuotaKind, string> = {
  claude: "Claude",
  codex: "Codex",
  grok: "Grok",
  deepseek: "DeepSeek",
};

export type Tone = "ok" | "warn" | "danger";

export interface WindowView {
  window: QuotaWindow;
  /** 已用百分比。重置时间已过的窗口算 0。 */
  used: number;
  /** 剩余百分比，0 到 100 的整数。 */
  remaining: number;
  /** 重置时间已经过了，数字是上一次读到的旧值之前的，等下次读取 */
  reset: boolean;
  tone: Tone;
}

/** 剩余少于 20% 变黄，少于 5% 变红。 */
export function toneOf(remaining: number): Tone {
  if (remaining < 5) return "danger";
  if (remaining < 20) return "warn";
  return "ok";
}

export function viewWindow(w: QuotaWindow, now: Date): WindowView {
  const reset = !!w.resetsAt && new Date(w.resetsAt).getTime() <= now.getTime();
  const used = reset ? 0 : Math.min(100, Math.max(0, w.usedPercent));
  const remaining = Math.round(100 - used);
  return { window: w, used, remaining, reset, tone: toneOf(remaining) };
}

/** 一个账号最紧张的窗口：不算“用完也不停账号”的按量付费，剩余最少的那个。 */
export function tightestWindow(
  account: QuotaAccount,
  now: Date,
): WindowView | null {
  let best: WindowView | null = null;
  for (const w of account.windows) {
    if (w.aside) continue;
    const v = viewWindow(w, now);
    if (!best || v.remaining < best.remaining) best = v;
  }
  return best;
}

/** 最早要重置的窗口（还没到时间的）。 */
export function nextReset(
  accounts: QuotaAccount[],
  now: Date,
): { account: QuotaAccount; window: QuotaWindow; at: Date } | null {
  let best: { account: QuotaAccount; window: QuotaWindow; at: Date } | null =
    null;
  for (const account of accounts) {
    for (const window of account.windows) {
      if (!window.resetsAt || window.aside) continue;
      const at = new Date(window.resetsAt);
      if (at.getTime() <= now.getTime()) continue;
      if (!best || at.getTime() < best.at.getTime())
        best = { account, window, at };
    }
  }
  return best;
}

/** 距离重置还有多久：“2 小时 14 分”、“3 天 4 小时”、“12 分”。 */
export function durationText(ms: number, language: Language): string {
  const minutes = Math.max(1, Math.round(ms / 60_000));
  const d = Math.floor(minutes / 1440);
  const h = Math.floor((minutes % 1440) / 60);
  const m = minutes % 60;
  const zh = language === "zh";
  if (d > 0)
    return h > 0
      ? zh
        ? `${d} 天 ${h} 小时`
        : `${d}d ${h}h`
      : zh
        ? `${d} 天`
        : `${d}d`;
  if (h > 0)
    return m > 0 && h < 10
      ? zh
        ? `${h} 小时 ${m} 分`
        : `${h}h ${m}m`
      : zh
        ? `${h} 小时`
        : `${h}h`;
  return zh ? `${m} 分` : `${m}m`;
}

/** 窗口下面那行：“2 小时 14 分后重置”。没有重置时间时返回空。 */
export function resetText(
  w: QuotaWindow,
  now: Date,
  language: Language,
): string {
  if (!w.resetsAt) return "";
  const ms = new Date(w.resetsAt).getTime() - now.getTime();
  if (ms <= 0)
    return language === "zh"
      ? "已重置，等待下次读取"
      : "Reset, waiting for the next reading";
  const text = durationText(ms, language);
  return language === "zh" ? `${text}后重置` : `Resets in ${text}`;
}

/** 悬停提示里的具体重置时间。 */
export function resetAtText(w: QuotaWindow, language: Language): string {
  if (!w.resetsAt) return "";
  return new Date(w.resetsAt).toLocaleString(
    language === "zh" ? "zh-CN" : "en",
    { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" },
  );
}

/** 账号里有没有可以显示的数字（失败时也可能留着上一次的）。 */
export function hasNumbers(a: QuotaAccount): boolean {
  return a.windows.length > 0 || a.balances.length > 0 || !!a.credits;
}

/** 机器离线或读取失败，但留着上一次的数字：卡片变灰。 */
export function isStale(a: QuotaAccount): boolean {
  return a.status === "error" && hasNumbers(a);
}

export function groupByKind(
  accounts: QuotaAccount[],
): Array<{ kind: QuotaKind; items: QuotaAccount[] }> {
  return KIND_ORDER.map((kind) => ({
    kind,
    items: accounts.filter((a) => a.kind === kind),
  })).filter((g) => g.items.length > 0);
}

/** 同一种服务里把账号上移或下移一位，返回全部账号的新顺序（编号）。 */
export function moveWithinKind(
  accounts: QuotaAccount[],
  id: number,
  delta: -1 | 1,
): number[] | null {
  const me = accounts.find((a) => a.id === id);
  if (!me) return null;
  const same = accounts.filter((a) => a.kind === me.kind);
  const i = same.findIndex((a) => a.id === id);
  const j = i + delta;
  if (j < 0 || j >= same.length) return null;
  const other = same[j]!;
  const ids = accounts.map((a) => a.id);
  const a = ids.indexOf(id);
  const b = ids.indexOf(other.id);
  ids[a] = other.id;
  ids[b] = id;
  return ids;
}

/** 账号显示用的第二行：机器名，或 DeepSeek。 */
export function accountOrigin(a: QuotaAccount): string {
  if (a.kind === "deepseek") return "API";
  return a.hostName ?? a.hostId;
}
