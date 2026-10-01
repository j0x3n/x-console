import type { NavBadge } from "../../lib/navBadges";
import { useGitHubStatus, useRuns } from "./api";
import { latestDefaultRuns, runOutcome } from "./logic";

/** 左栏“仓库”：默认分支最新一次 CI 失败的仓库数（B76）。 */
export function useGitHubBadge(): NavBadge | null {
  const configured = useGitHubStatus().data?.configured === true;
  const runs = useRuns(configured).data ?? [];
  const failing = new Set(
    latestDefaultRuns(runs)
      .filter((r) => runOutcome(r).tone === "danger")
      .map((r) => r.repo),
  ).size;
  return failing > 0
    ? { count: failing, title: `${failing} 个仓库的 CI 失败` }
    : null;
}
