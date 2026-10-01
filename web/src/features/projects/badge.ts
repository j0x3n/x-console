import type { NavBadge } from "../../lib/navBadges";
import { useMyIssues } from "./api";
import { issueDueState } from "./logic";

/** 左栏“项目”：没完成、已经过了截止时间的卡片（B76）。 */
export function useProjectsBadge(): NavBadge | null {
  const issues = useMyIssues().data ?? [];
  const now = new Date();
  const n = issues.filter((i) => issueDueState(i, now) === "overdue").length;
  return n > 0 ? { count: n, title: `${n} 张卡片已过期` } : null;
}
