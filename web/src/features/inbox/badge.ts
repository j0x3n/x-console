import type { NavBadge } from "../../lib/navBadges";
import { useInbox } from "./api";

/** 左栏“收集箱”：没处理的条数（B134）。和页面共用查询缓存。 */
export function useInboxBadge(): NavBadge | null {
  const n = useInbox().data?.openCount ?? 0;
  if (n === 0) return null;
  return { count: n, tone: "warn", title: `${n} 条没处理` };
}
