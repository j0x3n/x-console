import type { ExternalReminder, Reminder } from "./api";

/*
 * 提醒页里混排自己的提醒和其他模块的到期事项（B37）。
 */

export type Entry =
  | { kind: "reminder"; key: string; at: string; reminder: Reminder }
  | { kind: "external"; key: string; at: string; item: ExternalReminder };

/** 列表里按哪个时间排：稍后提醒的时间，没有就用第一次的时间。 */
export function reminderTime(r: Reminder): string {
  return r.dueAt ?? r.dtstart;
}

/** 按时间合并成一个列表。时间一样时自己的提醒在前。 */
export function mergeEntries(
  reminders: Reminder[],
  external: ExternalReminder[],
): Entry[] {
  const out: Entry[] = [
    ...reminders.map(
      (r): Entry => ({
        kind: "reminder",
        key: `r${r.id}`,
        at: reminderTime(r),
        reminder: r,
      }),
    ),
    ...external.map(
      (e): Entry => ({
        kind: "external",
        key: `x${e.source}:${e.id}`,
        at: e.at,
        item: e,
      }),
    ),
  ];
  return out.sort(
    (a, b) =>
      Date.parse(a.at) - Date.parse(b.at) ||
      (a.kind === b.kind ? 0 : a.kind === "reminder" ? -1 : 1),
  );
}

/** 来源的显示名，英文原文，中文在 i18n。不认识的用服务端给的 sourceLabel。 */
export const SOURCE_LABELS: Record<string, string> = {
  subscription: "Subscription",
  certificate: "Certificate",
  domain: "Domain",
  issue: "Issue",
};

const KEY = "xc.reminders.external";

/** “显示其他模块”开关，只记在这个浏览器里，默认打开。 */
export function readShowExternal(): boolean {
  try {
    return localStorage.getItem(KEY) !== "off";
  } catch {
    return true;
  }
}

export function writeShowExternal(on: boolean) {
  try {
    localStorage.setItem(KEY, on ? "on" : "off");
  } catch {
    /* 记不住就算了 */
  }
}
