import type { components } from "../../api/gen/projects";

/*
 * 项目模块的纯逻辑：筛选、分组、排序、看板拖动。和界面无关，方便测试。
 * 看板顺序的算法和后端 modules/projects/sortorder.go 一致。
 */

export type Issue = components["schemas"]["Issue"];
export type IssueStatus = components["schemas"]["IssueStatus"];
export type Category = components["schemas"]["ProjectCategory"];
export type Checklist = components["schemas"]["Checklist"];
export type ChecklistItem = components["schemas"]["ChecklistItem"];
export type DueRemind = components["schemas"]["DueRemind"];

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

export const OPEN_STATUSES: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
];

export type DueFilter = "today" | "week" | "overdue";

export interface IssueFilter {
  statuses: IssueStatus[];
  priority: number | null;
  labelId: number | null;
  milestoneId: number | null;
  /** 分类。一级分类包含它下面的二级；UNCATEGORIZED 表示未分类。 */
  categoryId: number | null;
  due: DueFilter | null;
  q: string;
}

export const UNCATEGORIZED = 0;

export const emptyFilter: IssueFilter = {
  statuses: [],
  priority: null,
  labelId: null,
  milestoneId: null,
  categoryId: null,
  due: null,
  q: "",
};

export function isFilterActive(filter: IssueFilter) {
  return (
    filter.statuses.length > 0 ||
    filter.priority !== null ||
    filter.labelId !== null ||
    filter.milestoneId !== null ||
    filter.categoryId !== null ||
    filter.due !== null ||
    filter.q.trim() !== ""
  );
}

/** 按截止时间筛选。已完成和已取消的不算到期。 */
export function matchDue(issue: Issue, due: DueFilter, now = new Date()) {
  const at = issueDue(issue);
  if (!at || isClosed(issue.status)) return false;
  if (due === "overdue") return at < now;
  if (due === "today") return localDate(at) === localDate(now);
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const end = new Date(start.getTime());
  end.setDate(end.getDate() + 7);
  return at >= start && at < end;
}

/**
 * 在已加载的 Issue 里筛选。q 匹配标题或 key，不区分大小写。
 * categories 用来把一级分类展开成它和它的二级分类。
 */
export function filterIssues(
  issues: Issue[],
  filter: IssueFilter,
  categories: Category[] = [],
  now = new Date(),
): Issue[] {
  const q = filter.q.trim().toLowerCase();
  const scope =
    filter.categoryId === null || filter.categoryId === UNCATEGORIZED
      ? null
      : categoryScope(categories, filter.categoryId);
  return issues.filter(
    (issue) =>
      (filter.statuses.length === 0 ||
        filter.statuses.includes(issue.status)) &&
      (filter.priority === null || issue.priority === filter.priority) &&
      (filter.labelId === null ||
        issue.labels.some((l) => l.id === filter.labelId)) &&
      (filter.milestoneId === null ||
        issue.milestoneId === filter.milestoneId) &&
      (filter.categoryId === null ||
        (scope
          ? issue.categoryId !== undefined && scope.has(issue.categoryId)
          : !issue.categoryId)) &&
      (filter.due === null || matchDue(issue, filter.due, now)) &&
      (q === "" ||
        issue.title.toLowerCase().includes(q) ||
        issue.key.toLowerCase() === q),
  );
}

/* ---- 分类（B36） ---- */

export interface CategoryNode {
  category: Category;
  depth: 0 | 1;
  /** “一级 / 二级” */
  path: string;
}

const byPosition = (a: Category, b: Category) =>
  a.position - b.position || a.id - b.id;

/** 按树的顺序排好：一级分类，然后是它下面的二级分类。 */
export function categoryTree(list: Category[]): CategoryNode[] {
  const out: CategoryNode[] = [];
  const top = list.filter((c) => !c.parentId).sort(byPosition);
  for (const parent of top) {
    out.push({ category: parent, depth: 0, path: parent.name });
    for (const child of list
      .filter((c) => c.parentId === parent.id)
      .sort(byPosition))
      out.push({
        category: child,
        depth: 1,
        path: `${parent.name} / ${child.name}`,
      });
  }
  return out;
}

/** 分类 id → “一级 / 二级”。 */
export function categoryPaths(list: Category[]): Map<number, string> {
  return new Map(categoryTree(list).map((n) => [n.category.id, n.path]));
}

/** 这个分类和它的二级分类的 id。 */
export function categoryScope(list: Category[], id: number): Set<number> {
  const ids = new Set([id]);
  for (const c of list) if (c.parentId === id) ids.add(c.id);
  return ids;
}

/** 同级里上移或下移一位，返回接口要的 afterId / beforeId。到头了返回 null。 */
export function planCategoryStep(
  list: Category[],
  id: number,
  delta: 1 | -1,
): { afterId?: number; beforeId?: number } | null {
  const self = list.find((c) => c.id === id);
  if (!self) return null;
  const siblings = list
    .filter((c) => (c.parentId ?? 0) === (self.parentId ?? 0))
    .sort(byPosition);
  const at = siblings.findIndex((c) => c.id === id);
  const to = at + delta;
  if (to < 0 || to >= siblings.length) return null;
  const others = siblings.filter((c) => c.id !== id);
  return { afterId: others[to - 1]?.id, beforeId: others[to]?.id };
}

/* ---- 截止时间（B36） ---- */

/** 截止时间。旧数据只有日期时按当天 23:59 算。 */
export function issueDue(issue: Pick<Issue, "dueAt" | "dueDate">): Date | null {
  if (issue.dueAt) {
    const at = new Date(issue.dueAt);
    return Number.isNaN(at.getTime()) ? null : at;
  }
  if (issue.dueDate) return new Date(`${issue.dueDate}T23:59:00`);
  return null;
}

export type DueState = "overdue" | "today" | "soon" | "later";

/** 截止时间状态：overdue 已过期，today 今天，soon 三天内。 */
export function issueDueState(issue: Issue, now = new Date()): DueState | null {
  const at = issueDue(issue);
  if (!at) return null;
  if (isClosed(issue.status)) return "later";
  if (at < now) return "overdue";
  if (localDate(at) === localDate(now)) return "today";
  return at.getTime() - now.getTime() <= 3 * 86_400_000 ? "soon" : "later";
}

/** 过期多久，用最大的单位：不到 1 小时按分钟，不到 2 天按小时。 */
export function overdueBy(
  at: Date,
  now = new Date(),
): { value: number; unit: "minutes" | "hours" | "days" } {
  const minutes = Math.max(
    1,
    Math.floor((now.getTime() - at.getTime()) / 60_000),
  );
  if (minutes < 60) return { value: minutes, unit: "minutes" };
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return { value: hours, unit: "hours" };
  return { value: Math.floor(hours / 24), unit: "days" };
}

const pad = (n: number) => String(n).padStart(2, "0");

/** 本地时间 HH:mm。 */
export const localTime = (d: Date) =>
  `${pad(d.getHours())}:${pad(d.getMinutes())}`;

/** 截止时间拆成本地的日期和时间，给输入框用。 */
export function splitDue(issue: Pick<Issue, "dueAt" | "dueDate">): {
  date: string;
  time: string;
} {
  const at = issueDue(issue);
  return at
    ? { date: localDate(at), time: localTime(at) }
    : { date: "", time: "" };
}

/** 本地日期和时间合成 UTC 时间。只有日期时按 23:59。没有日期返回 null。 */
export function joinDue(date: string, time: string): string | null {
  if (!date) return null;
  const at = new Date(`${date}T${time || "23:59"}:00`);
  return Number.isNaN(at.getTime()) ? null : at.toISOString();
}

export const DUE_REMINDS: DueRemind[] = ["none", "at_due", "15m", "1h", "1d"];

export const DUE_REMIND_LABELS: Record<DueRemind, string> = {
  none: "No reminder",
  at_due: "When due",
  "15m": "15 minutes before",
  "1h": "1 hour before",
  "1d": "1 day before",
};

/* ---- 检查清单（B36） ---- */

/** 所有清单加起来的进度。 */
export function checklistProgress(lists: Checklist[]): {
  done: number;
  total: number;
} {
  let done = 0;
  let total = 0;
  for (const list of lists)
    for (const item of list.items) {
      total++;
      if (item.done) done++;
    }
  return { done, total };
}

export function sortItems(items: ChecklistItem[]): ChecklistItem[] {
  return [...items].sort((a, b) => a.position - b.position || a.id - b.id);
}

/** 条目拖到 index（不含被拖的那条）时，接口要的 afterId / beforeId。 */
export function planItemMove(
  items: ChecklistItem[],
  id: number,
  index: number,
): { afterId?: number; beforeId?: number } {
  const others = sortItems(items).filter((i) => i.id !== id);
  const at = Math.max(0, Math.min(index, others.length));
  return { afterId: others[at - 1]?.id, beforeId: others[at]?.id };
}

/** 拖动后顺序有没有变化。 */
export function isNoopItemMove(
  items: ChecklistItem[],
  id: number,
  plan: { afterId?: number; beforeId?: number },
) {
  const sorted = sortItems(items);
  const at = sorted.findIndex((i) => i.id === id);
  return (
    sorted[at - 1]?.id === plan.afterId && sorted[at + 1]?.id === plan.beforeId
  );
}

/** 乐观更新：把条目挪到 afterId 和 beforeId 中间。 */
export function applyItemMove(
  items: ChecklistItem[],
  id: number,
  plan: { afterId?: number; beforeId?: number },
): ChecklistItem[] {
  const above = items.find((i) => i.id === plan.afterId);
  const below = items.find((i) => i.id === plan.beforeId);
  const position =
    sortOrderBetween(above?.position, below?.position) ??
    ((above?.position ?? 0) + (below?.position ?? 0)) / 2;
  return items.map((i) => (i.id === id ? { ...i, position } : i));
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
        const da = issueDue(a)?.getTime();
        const db = issueDue(b)?.getTime();
        if (da !== undefined && db !== undefined && da !== db) return da - db;
        if ((da === undefined) !== (db === undefined))
          return da !== undefined ? -1 : 1;
        return (
          priorityRank(a.priority) - priorityRank(b.priority) || a.id - b.id
        );
      }
    }
  };
}

export function sortIssues(issues: Issue[], sort: SortKey): Issue[] {
  return [...issues].sort(compareIssues(sort));
}

export type GroupBy = "status" | "priority" | "category" | "none";

export interface IssueGroup {
  id: string;
  label: string;
  /** label 是用户起的名字，不用翻译 */
  raw?: boolean;
  status?: IssueStatus;
  priority?: number;
  issues: Issue[];
}

/** 按状态、优先级或分类分组，组的顺序固定，空组也保留（界面决定显不显示）。 */
export function groupIssues(
  issues: Issue[],
  groupBy: GroupBy,
  sort: SortKey,
  categories: Category[] = [],
): IssueGroup[] {
  const sorted = sortIssues(issues, sort);
  if (groupBy === "category") {
    const tree = categoryTree(categories);
    const known = new Set(tree.map((n) => n.category.id));
    return [
      ...tree.map((n) => ({
        id: `c${n.category.id}`,
        label: n.path,
        raw: true,
        issues: sorted.filter((i) => i.categoryId === n.category.id),
      })),
      {
        id: "c0",
        label: "Uncategorized",
        issues: sorted.filter((i) => !i.categoryId || !known.has(i.categoryId)),
      },
    ];
  }
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
      above || below ? sortOrderBetween(above?.sortOrder, below?.sortOrder) : 0,
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
