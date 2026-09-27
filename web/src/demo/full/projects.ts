import { fail, json, noContent, route } from "../router";
import { at, date } from "./util";

type Status =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "done"
  | "canceled";
interface Label {
  id: number;
  projectId?: number;
  name: string;
  color: string;
}
interface Issue {
  id: number;
  key: string;
  projectId: number;
  projectKey: string;
  number: number;
  title: string;
  description: string;
  status: Status;
  priority: number;
  dueDate?: string;
  milestoneId?: number;
  sortOrder: number;
  labels: Label[];
  externalSource: string;
  externalId: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
}

const projects = [
  {
    id: 1,
    key: "XC",
    name: "X Console",
    description: "个人控制台：服务器、笔记、云盘、AI 助手",
    color: "#cc7752",
    icon: "",
  },
  {
    id: 2,
    key: "HOME",
    name: "家里的网络",
    description: "路由器、NAS 和智能家居",
    color: "#5cc98b",
    icon: "",
  },
  {
    id: 3,
    key: "BLOG",
    name: "博客改版",
    description: "换成静态站点，加上搜索",
    color: "#70b5f7",
    icon: "",
  },
  {
    id: 4,
    key: "TRIP",
    name: "十月旅行",
    description: "云南七天",
    color: "#e8b454",
    icon: "",
  },
  {
    id: 5,
    key: "READ",
    name: "读书计划",
    description: "今年 12 本",
    color: "#b69cf5",
    icon: "",
  },
].map((p, i) => ({
  ...p,
  createdAt: at(-60 * 24 * (60 - i * 7)),
  updatedAt: at(-60 * (i * 9 + 2)),
  archivedAt: undefined as string | undefined,
}));

const labels: Label[] = [
  { id: 1, name: "bug", color: "#ef6b6b" },
  { id: 2, name: "前端", color: "#70b5f7" },
  { id: 3, name: "后端", color: "#5cc98b" },
  { id: 4, name: "设计", color: "#b69cf5" },
  { id: 5, projectId: 2, name: "硬件", color: "#e8b454" },
];
const milestones = [
  {
    id: 1,
    projectId: 1,
    name: "2.0 上线",
    dueDate: date(12),
    createdAt: at(-60 * 24 * 20),
  },
  {
    id: 2,
    projectId: 1,
    name: "云盘",
    dueDate: date(26),
    createdAt: at(-60 * 24 * 10),
  },
  {
    id: 3,
    projectId: 3,
    name: "新主题",
    dueDate: date(5),
    createdAt: at(-60 * 24 * 8),
  },
];

const seed: [
  number,
  string,
  Status,
  number,
  number | null,
  number[],
  number?,
][] = [
  // 项目, 标题, 状态, 优先级(0 无 1 紧急 2 高 3 中 4 低), 截止(天), 标签, 里程碑
  [1, "云盘上传大文件时内存占满", "in_progress", 1, 0, [1, 3], 2],
  [1, "登录页支持只用密码登录", "in_review", 2, 1, [2, 3], 1],
  [1, "AI 助手接上真的模型", "todo", 2, 6, [3], 1],
  [1, "侧边栏二级菜单", "done", 3, -2, [2, 4], 1],
  [1, "手机上命令面板太高", "todo", 3, 3, [1, 2], 1],
  [1, "S3 同步失败时重试", "backlog", 3, null, [3], 2],
  [1, "自动化规则支持 Webhook", "backlog", 4, null, [3]],
  [1, "早报加上续费提醒", "todo", 3, 9, [3], 1],
  [1, "看板拖动支持手机", "backlog", 4, null, [2]],
  [1, "隐藏内容的审计日志", "in_progress", 2, 2, [3], 1],
  [1, "设置页改回顶部标签", "done", 3, -1, [2, 4]],
  [1, "旧的备忘接口删掉", "canceled", 4, null, [3]],
  [2, "NAS 换新硬盘", "todo", 2, 4, [5]],
  [2, "路由器固件升级", "done", 3, -5, [5]],
  [2, "客厅灯接入 Home Assistant", "in_progress", 3, 7, []],
  [2, "门口摄像头掉线", "todo", 1, -1, [1, 5]],
  [3, "选一个静态站点生成器", "done", 3, -10, [4]],
  [3, "文章加全文搜索", "in_progress", 2, 5, [2]],
  [3, "暗色主题", "todo", 3, 5, [4, 2]],
  [3, "旧文章图片迁移", "backlog", 4, null, []],
  [4, "订酒店", "done", 1, -3, []],
  [4, "买机票", "in_review", 1, 0, []],
  [4, "列行李清单", "todo", 3, 8, []],
  [5, "《原则》读完写笔记", "in_progress", 3, 4, []],
  [5, "选下个月的书", "todo", 4, 20, []],
];
const counters: Record<number, number> = {};
let nextId = 1;
const issues: Issue[] = seed.map(
  ([pid, title, status, priority, due, labelIds, ms], i) => {
    const p = projects.find((x) => x.id === pid)!;
    const number = (counters[pid] = (counters[pid] ?? 0) + 1);
    const created = at(-60 * (200 - i * 6));
    return {
      id: nextId++,
      key: `${p.key}-${number}`,
      projectId: pid,
      projectKey: p.key,
      number,
      title,
      description:
        i % 3 === 0
          ? `## 背景\n\n${title}。\n\n## 要做的\n\n- [x] 先查原因\n- [ ] 写修复\n- [ ] 加测试`
          : "",
      status,
      priority,
      dueDate: due == null ? undefined : date(due),
      milestoneId: ms,
      sortOrder: i,
      labels: labels.filter((l) => labelIds.includes(l.id)),
      externalSource: i === 2 ? "linear" : "",
      externalId: i === 2 ? "LIN-42" : "",
      createdAt: created,
      updatedAt: at(-60 * (i * 3 + 1)),
      completedAt: status === "done" ? at(-60 * (i + 5)) : undefined,
    };
  },
);
const comments = [
  {
    id: 1,
    issueId: 1,
    body: "复现了：1 GB 的文件会把内存吃满。要改成流式写盘。",
    createdAt: at(-60 * 5),
  },
  { id: 2, issueId: 1, body: "改好了，等验收。", createdAt: at(-60 * 2) },
  {
    id: 3,
    issueId: 2,
    body: "PR 已经提了，CI 是绿的。",
    createdAt: at(-60 * 20),
  },
];
const links = [
  {
    id: 1,
    issueId: 2,
    kind: "pull_request",
    title: "B12 两步验证改为可选",
    url: "https://github.com/j0x3n/x-console/pull/16",
    ref: "j0x3n/x-console#16",
    createdAt: at(-60 * 20),
  },
  {
    id: 2,
    issueId: 1,
    kind: "coding_task",
    title: "编码任务 #3",
    url: "/coding/3",
    ref: "3",
    createdAt: at(-60 * 3),
  },
];

const withCounts = (p: (typeof projects)[number]) => {
  const list = issues.filter((i) => i.projectId === p.id);
  return {
    ...p,
    issueCount: list.length,
    openCount: list.filter(
      (i) => i.status !== "done" && i.status !== "canceled",
    ).length,
  };
};
const byKey = (key: string) => issues.find((i) => i.key === key);
const open = (i: Issue) => i.status !== "done" && i.status !== "canceled";

export function register() {
  route("GET", "/projects", ({ query }) => {
    const archived = query.get("archived") === "true";
    return json(
      projects.filter((p) => !!p.archivedAt === archived).map(withCounts),
    );
  });
  route("POST", "/projects", ({ body }) => {
    if (projects.some((p) => p.key === body.key))
      return fail(409, "conflict", "这个 key 已经用过了");
    const p = {
      id: projects.length + 1,
      key: body.key,
      name: body.name,
      description: body.description ?? "",
      color: body.color ?? "",
      icon: "",
      createdAt: at(0),
      updatedAt: at(0),
      archivedAt: undefined,
    };
    projects.push(p);
    return json(withCounts(p), 201);
  });
  route("GET", "/projects/:id", ({ params }) => {
    const p = projects.find((x) => x.id === Number(params.id));
    return p ? json(withCounts(p)) : fail(404, "not_found", "资源不存在");
  });
  route("PATCH", "/projects/:id", ({ params, body }) => {
    const p = projects.find((x) => x.id === Number(params.id));
    if (!p) return fail(404, "not_found", "资源不存在");
    Object.assign(p, {
      name: body.name ?? p.name,
      description: body.description ?? p.description,
      color: body.color ?? p.color,
      updatedAt: at(0),
    });
    if (body.archived !== undefined)
      p.archivedAt = body.archived ? at(0) : undefined;
    return json(withCounts(p));
  });
  route("GET", "/projects/:id/labels", ({ params }) =>
    json(
      labels.filter((l) => !l.projectId || l.projectId === Number(params.id)),
    ),
  );
  route("GET", "/projects/:id/milestones", ({ params }) =>
    json(milestones.filter((m) => m.projectId === Number(params.id))),
  );
  route("POST", "/projects/:id/issues", ({ params, body }) => {
    const p = projects.find((x) => x.id === Number(params.id));
    if (!p) return fail(404, "not_found", "资源不存在");
    const number = (counters[p.id] = (counters[p.id] ?? 0) + 1);
    const i: Issue = {
      id: nextId++,
      key: `${p.key}-${number}`,
      projectId: p.id,
      projectKey: p.key,
      number,
      title: body.title,
      description: body.description ?? "",
      status: body.status ?? "todo",
      priority: body.priority ?? 0,
      dueDate: body.dueDate,
      milestoneId: body.milestoneId,
      sortOrder: Math.max(0, ...issues.map((x) => x.sortOrder)) + 1,
      labels: labels.filter((l) => (body.labelIds ?? []).includes(l.id)),
      externalSource: "",
      externalId: "",
      createdAt: at(0),
      updatedAt: at(0),
    };
    issues.push(i);
    return json(i, 201);
  });

  route("GET", "/issues", ({ query }) => {
    const today = date(0);
    const week = date(7);
    let list = issues.filter((i) => {
      if (
        query.get("projectId") &&
        i.projectId !== Number(query.get("projectId"))
      )
        return false;
      const status = query.getAll("status").flatMap((s) => s.split(","));
      if (status.length && !status.includes(i.status)) return false;
      if (query.get("priority") && i.priority !== Number(query.get("priority")))
        return false;
      if (
        query.get("labelId") &&
        !i.labels.some((l) => l.id === Number(query.get("labelId")))
      )
        return false;
      if (
        query.get("milestoneId") &&
        i.milestoneId !== Number(query.get("milestoneId"))
      )
        return false;
      const due = query.get("due");
      if (due === "today" && !(i.dueDate && i.dueDate <= today && open(i)))
        return false;
      if (due === "week" && !(i.dueDate && i.dueDate <= week && open(i)))
        return false;
      if (due === "overdue" && !(i.dueDate && i.dueDate < today && open(i)))
        return false;
      const q = query.get("q")?.toLowerCase();
      if (q && !`${i.key} ${i.title}`.toLowerCase().includes(q)) return false;
      return true;
    });
    const sort = query.get("sort") ?? "updated";
    const pr = (i: Issue) => (i.priority === 0 ? 9 : i.priority);
    list = [...list].sort((a, b) =>
      sort === "priority"
        ? pr(a) - pr(b)
        : sort === "due"
          ? (a.dueDate ?? "9999").localeCompare(b.dueDate ?? "9999")
          : sort === "manual"
            ? a.sortOrder - b.sortOrder
            : b.updatedAt.localeCompare(a.updatedAt),
    );
    const limit = Number(query.get("limit") ?? 200);
    return json({ items: list.slice(0, limit) });
  });
  route("GET", "/issues/:key", ({ params }) => {
    const i = byKey(params.key);
    return i ? json(i) : fail(404, "not_found", "资源不存在");
  });
  route("PATCH", "/issues/:key", ({ params, body }) => {
    const i = byKey(params.key);
    if (!i) return fail(404, "not_found", "资源不存在");
    for (const k of [
      "title",
      "description",
      "status",
      "priority",
      "dueDate",
      "milestoneId",
    ] as const)
      if (body[k] !== undefined)
        (i as unknown as Record<string, unknown>)[k] =
          body[k] === "" ? undefined : body[k];
    if (body.labelIds)
      i.labels = labels.filter((l) => body.labelIds.includes(l.id));
    if (body.status === "done") i.completedAt = at(0);
    i.updatedAt = at(0);
    return json(i);
  });
  route("DELETE", "/issues/:key", ({ params }) => {
    const idx = issues.findIndex((i) => i.key === params.key);
    if (idx >= 0) issues.splice(idx, 1);
    return noContent();
  });
  route("POST", "/issues/:key/move", ({ params, body }) => {
    const i = byKey(params.key);
    if (!i) return fail(404, "not_found", "资源不存在");
    i.status = body.status;
    const after = body.afterKey ? byKey(body.afterKey) : undefined;
    const before = body.beforeKey ? byKey(body.beforeKey) : undefined;
    i.sortOrder =
      after && before
        ? (after.sortOrder + before.sortOrder) / 2
        : after
          ? after.sortOrder + 1
          : before
            ? before.sortOrder - 1
            : i.sortOrder;
    if (i.status === "done") i.completedAt = at(0);
    i.updatedAt = at(0);
    return json(i);
  });
  route("GET", "/issues/:key/comments", ({ params }) => {
    const i = byKey(params.key);
    return json(i ? comments.filter((c) => c.issueId === i.id) : []);
  });
  route("POST", "/issues/:key/comments", ({ params, body }) => {
    const i = byKey(params.key);
    if (!i) return fail(404, "not_found", "资源不存在");
    const c = {
      id: comments.length + 1,
      issueId: i.id,
      body: body.body,
      createdAt: at(0),
    };
    comments.push(c);
    return json(c, 201);
  });
  route("DELETE", "/issues/:key/comments/:cid", ({ params }) => {
    const idx = comments.findIndex((c) => c.id === Number(params.cid));
    if (idx >= 0) comments.splice(idx, 1);
    return noContent();
  });
  route("GET", "/issues/:key/links", ({ params }) => {
    const i = byKey(params.key);
    return json(i ? links.filter((l) => l.issueId === i.id) : []);
  });
  route("POST", "/issues/:key/links", ({ params, body }) => {
    const i = byKey(params.key);
    if (!i) return fail(404, "not_found", "资源不存在");
    const l = {
      id: links.length + 1,
      issueId: i.id,
      kind: body.kind,
      title: body.title ?? body.url,
      url: body.url,
      ref: body.ref ?? "",
      createdAt: at(0),
    };
    links.push(l);
    return json(l, 201);
  });
  route("DELETE", "/issues/:key/links/:lid", ({ params }) => {
    const idx = links.findIndex((l) => l.id === Number(params.lid));
    if (idx >= 0) links.splice(idx, 1);
    return noContent();
  });
}

/** 笔记转 Issue 时用。 */
export function createIssueFromNote(
  projectId: number,
  title: string,
  description: string,
) {
  const p = projects.find((x) => x.id === projectId) ?? projects[0];
  const number = (counters[p.id] = (counters[p.id] ?? 0) + 1);
  const i: Issue = {
    id: nextId++,
    key: `${p.key}-${number}`,
    projectId: p.id,
    projectKey: p.key,
    number,
    title,
    description,
    status: "todo",
    priority: 0,
    sortOrder: issues.length,
    labels: [],
    externalSource: "",
    externalId: "",
    createdAt: at(0),
    updatedAt: at(0),
  };
  issues.push(i);
  return i;
}
