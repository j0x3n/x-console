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
  const rank = (p: GitHubPull) =>
    p.state !== "open" ? 2 : p.checkState === "failure" ? 0 : 1;
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

/* ---- B70：多仓库 ---- */

/** 一个仓库在页面里的唯一键：Git 账号 + owner/name。GitHub 和 Forgejo 可能有同名仓库。 */
export function repoKey(connectionId: number | undefined, repo: string) {
  return `${connectionId ?? 0}:${repo}`;
}

/** 地址栏里的 ?repo=3:owner/name。没有冒号时当作旧链接，账号是 0。 */
export function parseRepoKey(key: string): {
  connectionId: number;
  repo: string;
} {
  const i = key.indexOf(":");
  if (i < 0) return { connectionId: 0, repo: key };
  const id = Number(key.slice(0, i));
  return { connectionId: Number.isFinite(id) ? id : 0, repo: key.slice(i + 1) };
}

/** 列表里的条目是不是这个仓库。账号是 0（不知道账号）时只比仓库名。 */
export function sameRepo(
  item: { repo: string; connectionId?: number },
  sel: { repo: string; connectionId: number },
) {
  if (item.repo.toLowerCase() !== sel.repo.toLowerCase()) return false;
  return !sel.connectionId || !item.connectionId
    ? true
    : item.connectionId === sel.connectionId;
}

export function splitRepo(repo: string): { owner: string; name: string } {
  const i = repo.indexOf("/");
  return i < 0
    ? { owner: "", name: repo }
    : { owner: repo.slice(0, i), name: repo.slice(i + 1) };
}

// 仓库颜色：按名字算一个固定的颜色，同一个仓库每次都一样，深浅主题都看得清。
const REPO_COLORS = [
  "#e0663f",
  "#3f8fd8",
  "#2f9e66",
  "#9b6bd6",
  "#d2558a",
  "#c9a227",
  "#20a5b5",
  "#6b6fd6",
  "#a0785a",
  "#7a9a2e",
];

export function repoColor(repo: string): string {
  let h = 0;
  for (const c of repo.toLowerCase()) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return REPO_COLORS[h % REPO_COLORS.length];
}

/** 仓库列表里的一行，接口没上线时由旧数据拼出来，字段和 WatchedRepo 一样。 */
export interface RepoRow {
  key: string;
  connectionId: number;
  connectionName?: string;
  forge: "github" | "forgejo";
  repo: string;
  url: string;
  defaultBranch: string;
  private: boolean;
  description?: string;
  openPulls: number;
  openIssues: number;
  pushedAt?: string;
  ci?: { status: string; conclusion: string; runId?: number };
  lastCommit?: { sha: string; message: string; author: string; at: string };
  notifyCustom: boolean;
  notifyOff: boolean;
  syncError?: string;
}

/** CI 状态的分数，失败的排前面。 */
function ciRank(row: Pick<RepoRow, "ci">): number {
  if (!row.ci) return 2;
  const tone = runOutcome(row.ci).tone;
  return tone === "danger" ? 0 : tone === "warn" ? 1 : 2;
}

/** CI 失败的在前，然后按最近活动（推送、最新提交）倒序，最后按名字。 */
export function sortRepos<T extends RepoRow>(rows: T[]): T[] {
  const at = (r: RepoRow) =>
    new Date(r.pushedAt ?? r.lastCommit?.at ?? 0).getTime() || 0;
  return [...rows].sort(
    (a, b) =>
      ciRank(a) - ciRank(b) || at(b) - at(a) || a.repo.localeCompare(b.repo),
  );
}

/** 搜索仓库列表：仓库名、说明、账号名里包含关键字。 */
export function filterRepoRows<T extends RepoRow>(rows: T[], q: string): T[] {
  const s = q.trim().toLowerCase();
  if (!s) return rows;
  return rows.filter(
    (r) =>
      r.repo.toLowerCase().includes(s) ||
      (r.description ?? "").toLowerCase().includes(s) ||
      (r.connectionName ?? "").toLowerCase().includes(s),
  );
}

/** 仓库多的时候按 owner 分组，组按名字排，组内保持原顺序。 */
export function groupByOwner<T extends RepoRow>(
  rows: T[],
): { owner: string; rows: T[] }[] {
  const map = new Map<string, T[]>();
  for (const r of rows) {
    const owner = splitRepo(r.repo).owner;
    const list = map.get(owner);
    if (list) list.push(r);
    else map.set(owner, [r]);
  }
  return [...map.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([owner, list]) => ({ owner, rows: list }));
}

/**
 * /github/repos 还没上线时，用配置、PR 和运行记录拼出仓库列表。
 * 关注的仓库来自 watches（新）或 repos（旧，账号是 connectionId）。
 */
export function fallbackRepoRows(
  config: {
    repos: string[];
    connectionId?: number;
    watches?: { connectionId: number; repo: string }[];
  },
  pulls: GitHubPull[],
  runs: GitHubRun[],
  apiUrl = "https://api.github.com",
): RepoRow[] {
  const watches =
    config.watches ??
    config.repos.map((repo) => ({
      connectionId: config.connectionId ?? 0,
      repo,
    }));
  const web = apiUrl.includes("api.github.com")
    ? "https://github.com"
    : apiUrl.replace(/\/api(\/v\d+)?\/?$/, "");
  const heads = latestDefaultRuns(runs);
  return watches.map((w) => {
    const sel = { repo: w.repo, connectionId: w.connectionId };
    const head = heads.find((r) => sameRepo(r, sel));
    const defaultRun = runs.find((r) => r.defaultBranch && sameRepo(r, sel));
    return {
      key: repoKey(w.connectionId, w.repo),
      connectionId: w.connectionId,
      forge: "github",
      repo: w.repo,
      url: `${web}/${w.repo}`,
      defaultBranch: defaultRun?.branch ?? "",
      private: false,
      openPulls: pulls.filter((p) => p.state === "open" && sameRepo(p, sel))
        .length,
      openIssues: 0,
      pushedAt: defaultRun?.createdAt,
      ci: head
        ? { status: head.status, conclusion: head.conclusion, runId: head.id }
        : undefined,
      notifyCustom: false,
      notifyOff: false,
    };
  });
}

/** 一条运行所有 job 的步骤进度：做完几步、一共几步、正在跑哪一步。 */
export function jobsProgress(
  jobs: {
    status: string;
    steps: { name: string; status: string; number: number }[];
  }[],
): { done: number; total: number; current?: string; currentIndex?: number } {
  let done = 0;
  let total = 0;
  let current: string | undefined;
  let currentIndex: number | undefined;
  for (const job of jobs) {
    for (const step of job.steps) {
      total++;
      if (step.status === "completed") done++;
      else if (step.status === "in_progress" && current == null) {
        current = step.name;
        currentIndex = total;
      }
    }
  }
  return { done, total, current, currentIndex };
}

/** 一步或一个 job 的状态，给图标用。 */
export function stepState(s: {
  status: string;
  conclusion: string;
}): "done" | "failed" | "running" | "waiting" | "skipped" {
  if (s.status === "in_progress") return "running";
  if (s.status !== "completed") return "waiting";
  switch (s.conclusion) {
    case "success":
      return "done";
    case "skipped":
    case "cancelled":
    case "neutral":
      return "skipped";
    default:
      return "failed";
  }
}

/** 用了多久：“1 分 20 秒”。没开始返回空。 */
export function duration(
  start?: string,
  end?: string,
  now = Date.now(),
): string {
  if (!start) return "";
  const ms = (end ? new Date(end).getTime() : now) - new Date(start).getTime();
  if (!(ms >= 0)) return "";
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s} 秒`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} 分 ${s % 60} 秒`;
  return `${Math.floor(m / 60)} 小时 ${m % 60} 分`;
}

/* ---- B71：通知事件 ---- */

export const NOTIFY_GROUPS: { label: string; events: string[] }[] = [
  {
    label: "CI",
    events: [
      "ci_started",
      "ci_succeeded",
      "ci_failed",
      "ci_cancelled",
      "ci_recovered",
    ],
  },
  { label: "Commits", events: ["push"] },
  {
    label: "Pull requests",
    events: ["pr_opened", "pr_merged", "pr_closed", "pr_review"],
  },
  {
    label: "Issues and releases",
    events: ["issue_opened", "issue_assigned", "release"],
  },
];

export const NOTIFY_LABELS: Record<string, string> = {
  ci_started: "CI started",
  ci_succeeded: "CI passed",
  ci_failed: "CI failed",
  ci_cancelled: "CI cancelled",
  ci_recovered: "CI fixed again",
  push: "New commits",
  pr_opened: "New pull request",
  pr_merged: "Pull request merged",
  pr_closed: "Pull request closed without merging",
  pr_review: "Review result",
  issue_opened: "New repo issue",
  issue_assigned: "Issue assigned to me",
  release: "New release",
};

/** 规格 B71 的默认值。 */
export const DEFAULT_NOTIFY = {
  events: [
    "ci_failed",
    "ci_recovered",
    "pr_opened",
    "pr_merged",
    "pr_review",
    "issue_assigned",
    "release",
  ],
  ciBranches: "default" as const,
};

/** B109：本月 CI 时长用了多少。80% 变黄，100% 变红。 */
export function ciUsageLevel(
  used: number,
  included: number,
): { percent: number; tone?: "warn" | "danger" } {
  const percent = included > 0 ? Math.floor((used * 100) / included) : 0;
  if (percent >= 100) return { percent, tone: "danger" };
  if (percent >= 80) return { percent, tone: "warn" };
  return { percent };
}
