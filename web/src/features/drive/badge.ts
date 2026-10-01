import type { NavBadge } from "../../lib/navBadges";
import { useDriveTasks } from "./api";

/** 左栏“云盘”：失败的后台任务（B76）。 */
export function useDriveBadge(): NavBadge | null {
  const tasks = useDriveTasks().data ?? [];
  const n = tasks.filter((x) => x.state === "failed").length;
  return n > 0
    ? { count: n, tone: "warn", title: `${n} 个后台任务失败` }
    : null;
}
