import type { NavBadge, NavIconState } from "../../lib/navBadges";
import { useAgentDecisions, useAiAgents } from "../aiagents/api";
import { useTasks } from "./api";

const WAITING = ["review", "failed"] as const;

/** 左栏“Agent”：等你决定的任务和失败的任务（B76）。 */
export function useCodingBadge(): NavBadge | null {
  const tasks = useTasks([...WAITING]).data ?? [];
  // B87：Agent 的权限请求和问题也算等你决定
  const asks = useAgentDecisions().data;
  const decide =
    tasks.filter((x) => x.status === "review").length +
    (Array.isArray(asks) ? asks.length : 0);
  const failed = tasks.filter((x) => x.status === "failed").length;
  const n = decide + failed;
  if (n === 0) return null;
  const parts = [
    decide ? `${decide} 个等你决定` : "",
    failed ? `${failed} 个失败` : "",
  ].filter(Boolean);
  return { count: n, tone: "warn", title: parts.join("，") };
}

/** 左栏“Agent”的图标：有 Agent 在工作时跳动（B86）。 */
export function useAgentsNavIcon(): NavIconState | null {
  const agents = useAiAgents().data ?? [];
  const working = agents.filter((a) => a.runningTasks > 0);
  if (working.length === 0) return null;
  return {
    state: "working",
    title: `${working.map((a) => a.name).join("、")} 正在工作`,
  };
}
