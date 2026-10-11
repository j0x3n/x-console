import type { NavBadge } from "../../lib/navBadges";
import { useCardSummary } from "./api";

/** 左栏“复习卡片”：今天还要复习的张数。和复习页、今日页共用同一个查询。 */
export function useCardsBadge(): NavBadge | null {
  const q = useCardSummary();
  const n = q.data?.due ?? 0;
  return n > 0 ? { count: n, tone: "warn", title: `${n} 张卡片待复习` } : null;
}
