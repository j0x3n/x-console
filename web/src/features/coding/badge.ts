import type { NavBadge } from "../../lib/navBadges";
import { useTasks } from "./api";

const WAITING = ["review", "failed"] as const;

/** 左栏“Agent”：等你决定的任务和失败的任务（B76）。 */
export function useCodingBadge(): NavBadge | null {
  const tasks = useTasks([...WAITING]).data ?? [];
  const review = tasks.filter((x) => x.status === "review").length;
  const failed = tasks.filter((x) => x.status === "failed").length;
  const n = review + failed;
  if (n === 0) return null;
  const parts = [
    review ? `${review} 个等你决定` : "",
    failed ? `${failed} 个失败` : "",
  ].filter(Boolean);
  return { count: n, tone: "warn", title: parts.join("，") };
}
