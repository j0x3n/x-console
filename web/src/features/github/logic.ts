import type { CheckState, GitHubPull, GitHubRun, ReviewState } from "./api";

export type Tone = "ok" | "warn" | "danger" | "info" | "";

export interface RepoGroup<T> {
  repo: string;
  items: T[];
}

/** 按仓库分组，仓库按名字排序，组内保持原来的顺序。 */
export function groupByRepo<T extends { repo: string }>(
  items: T[],
): RepoGroup<T>[] {
  const map = new Map<string, T[]>();
  for (const item of items) {
    const list = map.get(item.repo);
    if (list) list.push(item);
    else map.set(item.repo, [item]);
  }
  return [...map.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([repo, list]) => ({ repo, items: list }));
}

export function checkTone(state: CheckState): Tone {
  switch (state) {
    case "success":
      return "ok";
    case "failure":
      return "danger";
    case "pending":
      return "warn";
    default:
      return "";
  }
}

export const checkLabel: Record<CheckState, string> = {
  success: "Checks passed",
  failure: "Checks failed",
  pending: "Checks running",
  none: "No checks",
};

export function reviewTone(state: ReviewState): Tone {
  switch (state) {
    case "approved":
      return "ok";
    case "changes_requested":
      return "danger";
    case "pending":
      return "warn";
    default:
      return "";
  }
}

export const reviewLabel: Record<ReviewState, string> = {
  approved: "Approved",
  changes_requested: "Changes requested",
  pending: "Review requested",
  commented: "Commented",
  none: "No review",
};

/** 一次运行的结果：没结束时看 status，结束了看 conclusion。 */
export function runOutcome(run: Pick<GitHubRun, "status" | "conclusion">): {
  label: string;
  tone: Tone;
} {
  if (run.status !== "completed") {
    return {
      label: run.status === "queued" ? "Queued" : "Running",
      tone: "warn",
    };
  }
  switch (run.conclusion) {
    case "success":
      return { label: "Passed", tone: "ok" };
    case "failure":
    case "timed_out":
    case "startup_failure":
      return { label: "Failed", tone: "danger" };
    case "cancelled":
      return { label: "Cancelled", tone: "" };
    case "skipped":
      return { label: "Skipped", tone: "" };
    default:
      return { label: run.conclusion || "Finished", tone: "" };
  }
}

/** "XC-12" → "/projects/XC/12"。 */
export function issuePath(key: string): string {
  const i = key.lastIndexOf("-");
  if (i <= 0) return "/projects";
  return `/projects/${key.slice(0, i)}/${key.slice(i + 1)}`;
}

/** 文本框里的仓库列表：按行、逗号或空格分开，去掉空项和重复项。 */
export function parseRepos(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of text.split(/[\s,]+/)) {
    const repo = part.trim();
    if (!repo) continue;
    const key = repo.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(repo);
  }
  return out;
}

/** 失败的 PR 放前面，然后按更新时间倒序。 */
export function sortPulls(pulls: GitHubPull[]): GitHubPull[] {
  const rank = (p: GitHubPull) => (p.checkState === "failure" ? 0 : 1);
  return [...pulls].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
  );
}

/** 团队和项目的对应关系有什么问题；没问题返回 null。返回英文原文，给 t() 用。 */
export function mappingProblem(
  rows: { teamId: string; projectId: number }[],
): string | null {
  const teams = new Set<string>();
  const projects = new Set<number>();
  for (const r of rows) {
    if (!r.teamId || !r.projectId)
      return "Choose a team and a project in every row";
    if (teams.has(r.teamId) || projects.has(r.projectId))
      return "Each team and each project can be used only once";
    teams.add(r.teamId);
    projects.add(r.projectId);
  }
  return null;
}

/** 每个仓库默认分支上每个 workflow 的最近一次结果，用来显示“主干是不是绿的”。 */
export function latestDefaultRuns(runs: GitHubRun[]): GitHubRun[] {
  const seen = new Set<string>();
  const out: GitHubRun[] = [];
  for (const run of runs) {
    if (!run.defaultBranch) continue;
    const key = `${run.repo}\n${run.name}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(run);
  }
  return out;
}

/** 关注的仓库最多 50 个，和接口的 maxItems 一致。 */
export const MAX_REPOS = 50;

/** owner/name 的格式。 */
export function isRepoName(text: string): boolean {
  return /^[A-Za-z0-9-]+\/[A-Za-z0-9._-]+$/.test(text);
}

/** 搜索仓库：名字或说明里包含关键字，不区分大小写。 */
export function filterRepos<
  T extends { fullName: string; description?: string },
>(repos: T[], q: string): T[] {
  const s = q.trim().toLowerCase();
  if (!s) return repos;
  return repos.filter(
    (r) =>
      r.fullName.toLowerCase().includes(s) ||
      (r.description ?? "").toLowerCase().includes(s),
  );
}
