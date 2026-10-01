import type { NavBadge } from "../../lib/navBadges";
import { useExternalReminders, useReminders } from "./api";
import { readShowExternal } from "./external";

/** 左栏“提醒”：已经到点还没处理的提醒，加上其他模块已经到期的事项（B76）。 */
export function useRemindersBadge(): NavBadge | null {
  const showExternal = readShowExternal();
  const today = useReminders("today");
  const external = useExternalReminders("today", showExternal);
  const now = Date.now();
  const own = (today.data ?? []).filter((r) => r.status === "pending").length;
  const ext = showExternal
    ? (external.data ?? []).filter(
        (x) => !x.done && new Date(x.at).getTime() <= now,
      ).length
    : 0;
  const n = own + ext;
  return n > 0 ? { count: n, title: `${n} 条提醒已到期` } : null;
}
