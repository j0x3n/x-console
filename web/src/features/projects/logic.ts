import type { components } from "../../api/gen/projects";

/*
 * 项目模块的纯逻辑：筛选、分组、排序、看板拖动。和界面无关，方便测试。
 * 看板顺序的算法和后端 modules/projects/sortorder.go 一致。
 */

export type Issue = components["schemas"]["Issue"];
export type IssueStatus = components["schemas"]["IssueStatus"];

export const STATUSES: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
  "done",
  "canceled",
];

export const STATUS_LABELS: Record<IssueStatus, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In progress",
  in_review: "In review",
  done: "Done",
  canceled: "Canceled",
};

/** 0 无，1 紧急，2 高，3 中，4 低。 */
export const PRIORITIES = [1, 2, 3, 4, 0] as const;

export const PRIORITY_LABELS: Record<number, string> = {
  0: "No priority",
  1: "Urgent",
  2: "High",
  3: "Medium",
  4: "Low",
};

export const isClosed = (status: IssueStatus) =>
  status === "done" || status === "canceled";

export interface IssueFilter {
  statuses: IssueStatus[];
  priority: number | null;
  labelId: number | null;
  milestoneId: number | null;
  q: string;
}

export const emptyFilter: IssueFilter = {
  statuses: [],
  priority: null,
  labelId: null,
  milestoneId: null,
  q: "",
};

export function isFilterActive(filter: IssueFilter) {
  return (
    filter.statuses.length > 0 ||
    filter.priority !== null ||
    filter.labelId !== null ||
    filter.milestoneId !== null ||
    filter.q.trim() !== ""
  );
}

/** 在已加载的 Issue 里筛选。q 匹配标题或 key，不区分大小写。 */
export function filterIssues(issues: Issue[], filter: IssueFilter): Issue[] {
  const q = filter.q.trim().toLowerCase();
  return issues.filter(
    (issue) =>
      (filter.statuses.length === 0 ||
        filter.statuses.includes(issue.status)) &&
      (filter.priority === null || issue.priority === filter.priority) &&
      (filter.labelId === null ||
        issue.labels.some((l) => l.id === filter.labelId)) &&
      (filter.milestoneId === null ||
        issue.milestoneId === filter.milestoneId) &&
      (q === "" ||
        issue.title.toLowerCase().includes(q) ||
        issue.key.toLowerCase() === q),
  );
}

export type SortKey = "manual" | "updated" | "priority" | "due";

/** 无优先级排在最后。 */
const priorityRank = (p: number) => (p === 0 ? 5 : p);

export function compareIssues(sort: SortKey) {
  return (a: Issue, b: Issue): number => {
    switch (sort) {
      case "manual":
        return a.sortOrder - b.sortOrder || a.id - b.id;
      case "updated":
        return b.updatedAt.localeCompare(a.updatedAt) || b.id - a.id;
      case "priority":
        return (
          priorityRank(a.priority) - priorityRank(b.priority) ||
          b.updatedAt.localeCompare(a.updatedAt)
        );
      case "due": {
        if (a.dueDate && b.dueDate && a.dueDate !== b.dueDate)
          return a.dueDate.localeCompare(b.dueDate);
        if (!a.dueDate !== !b.dueDate) return a.dueDate ? -1 : 1;
        return priorityRank(a.priority) - priorityRank(b.priority) || a.id - b.id;
      }
    }
  };
}

export function sortIssues(issues: Issue[], sort: SortKey): Issue[] {
  return [...issues].sort(compareIssues(sort));
}

export type GroupBy = "status" | "priority" | "none";

export interface IssueGroup {
  id: string;
  label: string;
  status?: IssueStatus;
  priority?: number;
  issues: Issue[];
}

/** 按状态或优先级分组，组的顺序固定，空组也保留（界面决定显不显示）。 */
export function groupIssues(
  issues: Issue[],
  groupBy: GroupBy,
  sort: SortKey,
): IssueGroup[] {
  const sorted = sortIssues(issues, sort);
  if (groupBy === "status")
    return STATUSES.map((status) => ({
      id: status,
      label: STATUS_LABELS[status],
      status,
      issues: sorted.filter((i) => i.status === status),
    }));
  if (groupBy === "priority")
    return PRIORITIES.map((priority) => ({
      id: `p${priority}`,
      label: PRIORITY_LABELS[priority],
      priority,
      issues: sorted.filter((i) => i.priority === priority),
    }));
  return [{ id: "all", label: "All issues", issues: sorted }];
}

/** 看板一列，按 sortOrder 排好。 */
export function column(issues: Issue[], status: IssueStatus): Issue[] {
  return sortIssues(
    issues.filter((i) => i.status === status),
    "manual",
  );
}

const SORT_GAP = 1024;
const MIN_GAP = 1e-6;

/** 和后端 between() 一样：取两个邻居中间。太挤时返回 null，交给服务端重排。 */
export function sortOrderBetween(
  prev: number | undefined,
  next: number | undefined,
): number | null {
  if (prev !== undefined && next !== undefined)
    return next - prev <= MIN_GAP ? null : (prev + next) / 2;
  if (prev !== undefined) return prev + SORT_GAP;
  if (next !== undefined) return next - SORT_GAP;
  return 0;
}

export interface DropTarget {
  status: IssueStatus;
  /** 在目标列里（不含被拖动的 Issue）插入的位置。 */
  index: number;
}

export interface MovePlan {
  status: IssueStatus;
  afterKey?: string;
  beforeKey?: string;
  /** 乐观更新用的新 sortOrder；null 表示等服务端算。 */
  sortOrder: number | null;
}

/** 把拖放位置换成接口需要的 afterKey / beforeKey，并算出乐观的 sortOrder。 */
export function planMove(
  issues: Issue[],
  key: string,
  target: DropTarget,
): MovePlan {
  const others = column(issues, target.status).filter((i) => i.key !== key);
  const index = Math.max(0, Math.min(target.index, others.length));
  const above = others[index - 1];
  const below = others[index];
  return {
    status: target.status,
    afterKey: above?.key,
    beforeKey: below?.key,
    sortOrder:
      above || below
        ? sortOrderBetween(above?.sortOrder, below?.sortOrder)
        : 0,
  };
}

/** 判断拖放后位置有没有变化，没变就不发请求。 */
export function isNoopMove(issues: Issue[], key: string, plan: MovePlan) {
  const issue = issues.find((i) => i.key === key);
  if (!issue || issue.status !== plan.status) return false;
  const col = column(issues, plan.status);
  const at = col.findIndex((i) => i.key === key);
  return (
    col[at - 1]?.key === plan.afterKey && col[at + 1]?.key === plan.beforeKey
  );
}

/** 乐观更新：返回移动后的新数组。sortOrder 为 null 时放在 afterKey 后面一点点。 */
export function applyMove(
  issues: Issue[],
  key: string,
  plan: MovePlan,
): Issue[] {
  let order = plan.sortOrder;
  if (order === null) {
    const above = issues.find((i) => i.key === plan.afterKey);
    const below = issues.find((i) => i.key === plan.beforeKey);
    order = above && below ? (above.sortOrder + below.sortOrder) / 2 : 0;
  }
  const now = new Date().toISOString();
  return issues.map((issue) =>
    issue.key === key
      ? {
          ...issue,
          status: plan.status,
          sortOrder: order,
          updatedAt: now,
          completedAt: isClosed(plan.status)
            ? (issue.completedAt ?? now)
            : undefined,
        }
      : issue,
  );
}

/** 截止日期状态：overdue 已过期，today 今天，soon 三天内。 */
export function dueState(
  dueDate: string | undefined,
  today: string,
  status?: IssueStatus,
): "overdue" | "today" | "soon" | "later" | null {
  if (!dueDate) return null;
  if (status && isClosed(status)) return "later";
  if (dueDate < today) return "overdue";
  if (dueDate === today) return "today";
  const diff =
    (Date.parse(`${dueDate}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)) /
    86_400_000;
  return diff <= 3 ? "soon" : "later";
}

/** 本地日期 YYYY-MM-DD。 */
export function localDate(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

/** "XC-12" → { projectKey: "XC", number: 12 }。 */
export function parseIssueKey(
  key: string,
): { projectKey: string; number: number } | null {
  const match = /^([A-Za-z]{2,5})-(\d+)$/.exec(key.trim());
  if (!match) return null;
  return { projectKey: match[1].toUpperCase(), number: Number(match[2]) };
}

export const issuePath = (key: string) => {
  const parsed = parseIssueKey(key);
  return parsed
    ? `/projects/${parsed.projectKey}/${parsed.number}`
    : "/projects";
};

/** 键盘 J/K 用：在当前可见顺序里移动选中项。 */
export function stepSelection(
  keys: string[],
  current: string | null,
  delta: 1 | -1,
): string | null {
  if (keys.length === 0) return null;
  const at = current ? keys.indexOf(current) : -1;
  if (at === -1) return delta === 1 ? keys[0] : keys[keys.length - 1];
  return keys[Math.max(0, Math.min(keys.length - 1, at + delta))];
}
