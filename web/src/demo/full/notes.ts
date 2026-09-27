import { fail, json, noContent, route } from "../router";
import { createIssueFromNote } from "./projects";
import { addReminder } from "./reminders";
import { at } from "./util";

interface Note {
  id: number;
  title: string;
  body: string;
  pinned: boolean;
  tags: string[];
  archivedAt?: string;
  createdAt: string;
  updatedAt: string;
}

const raw: [string, string, string[], boolean?, boolean?][] = [
  [
    "周会记录 9/26",
    "## 本周\n\n- 云盘前端做完了\n- 两步验证改成可选\n\n## 下周\n\n- [ ] 开始做后端\n- [ ] 把演示数据去掉\n- [x] 定好部署规则",
    ["工作"],
    true,
  ],
  [
    "家里网络拓扑",
    "光猫 → 主路由（192.168.1.1）→ 交换机\n\n- NAS：192.168.1.10\n- Home Assistant：192.168.1.20\n- 打印机：192.168.1.30",
    ["家里"],
    true,
  ],
  [
    "读书笔记：原则",
    "> 把痛苦当成信号。\n\n1. 目标要写下来\n2. 问题要分清是设计问题还是执行问题\n3. 每周复盘一次",
    ["读书"],
  ],
  [
    "云南旅行清单",
    "- [x] 身份证\n- [x] 充电宝\n- [ ] 防晒霜\n- [ ] 雨伞\n- [ ] 相机电池",
    ["旅行"],
  ],
  [
    "Go 里 context 的坑",
    "写 websocket 时不要传会被取消的 context，它会关掉整条连接。\n\n```go\nctx := context.WithoutCancel(r.Context())\n```",
    ["技术"],
  ],
  ["买菜", "- 西红柿\n- 鸡蛋\n- 牛奶\n- 面包", []],
  ["报税材料", "- 工资单\n- 房租合同\n- 医疗发票", ["家里"]],
  [
    "博客文章想法",
    "1. 一个人的控制台怎么设计\n2. SQLite 在小项目里够不够用\n3. 让 AI 写代码的一些经验",
    ["写作"],
  ],
  ["面试题整理", "- 进程和线程\n- TCP 三次握手\n- 数据库索引", ["技术"]],
  [
    "健身计划",
    "周一：胸和三头\n周三：背和二头\n周五：腿\n\n每次 45 分钟左右。",
    ["健康"],
  ],
  ["电影想看", "- 沙丘 3\n- 流浪地球 3", [], false, true],
  ["旧的服务器密码提示", "已经换了，这条可以删。", [], false, true],
];
let nextId = 1;
const notes: Note[] = raw.map(([title, body, tags, pinned, archived], i) => ({
  id: nextId++,
  title,
  body,
  pinned: !!pinned,
  tags,
  archivedAt: archived ? at(-60 * 24 * 20) : undefined,
  createdAt: at(-60 * 24 * (i * 3 + 1)),
  updatedAt: at(-60 * (i * i * 2 + 1)),
}));

const plain = (s: string) =>
  s
    .replace(/[#>*`\-[\]]/g, "")
    .replace(/\s+/g, " ")
    .trim();
const summary = (n: Note, q?: string) => {
  const text = plain(n.body);
  let snippet: string | undefined;
  if (q) {
    const i = `${n.title} ${text}`.toLowerCase().indexOf(q.toLowerCase());
    if (i >= 0) {
      const src = `${n.title} ${text}`;
      const start = Math.max(0, i - 20);
      snippet =
        src.slice(start, i) +
        "" +
        src.slice(i, i + q.length) +
        "" +
        src.slice(i + q.length, i + q.length + 40);
    }
  }
  return {
    id: n.id,
    title: n.title,
    excerpt: text.slice(0, 140),
    snippet,
    pinned: n.pinned,
    tags: n.tags,
    archivedAt: n.archivedAt,
    createdAt: n.createdAt,
    updatedAt: n.updatedAt,
  };
};

export function register() {
  route("GET", "/notes", ({ query }) => {
    const q = query.get("q") ?? "";
    const tag = query.get("tag");
    const archived = query.get("archived") === "true";
    const pinned = query.get("pinned") === "true";
    const list = notes
      .filter((n) => !!n.archivedAt === archived)
      .filter((n) => !pinned || n.pinned)
      .filter((n) => !tag || n.tags.includes(tag))
      .filter(
        (n) =>
          !q || `${n.title} ${n.body}`.toLowerCase().includes(q.toLowerCase()),
      )
      .sort(
        (a, b) =>
          (q ? 0 : Number(b.pinned) - Number(a.pinned)) ||
          b.updatedAt.localeCompare(a.updatedAt),
      );
    return json({ items: list.map((n) => summary(n, q || undefined)) });
  });
  route("GET", "/notes/tags", () => {
    const counts = new Map<string, number>();
    for (const n of notes)
      if (!n.archivedAt)
        for (const t of n.tags) counts.set(t, (counts.get(t) ?? 0) + 1);
    return json(
      [...counts]
        .map(([tag, count]) => ({ tag, count }))
        .sort((a, b) => b.count - a.count),
    );
  });
  route("POST", "/notes", ({ body }) => {
    const n: Note = {
      id: nextId++,
      title: body?.title ?? "",
      body: body?.body ?? "",
      pinned: !!body?.pinned,
      tags: body?.tags ?? [],
      createdAt: at(0),
      updatedAt: at(0),
    };
    notes.push(n);
    return json(n, 201);
  });
  route("GET", "/notes/:id", ({ params }) => {
    const n = notes.find((x) => x.id === Number(params.id));
    return n ? json(n) : fail(404, "not_found", "资源不存在");
  });
  route("PATCH", "/notes/:id", ({ params, body }) => {
    const n = notes.find((x) => x.id === Number(params.id));
    if (!n) return fail(404, "not_found", "资源不存在");
    for (const k of ["title", "body", "pinned", "tags"] as const)
      if (body?.[k] !== undefined)
        (n as unknown as Record<string, unknown>)[k] = body[k];
    if (body?.archived !== undefined)
      n.archivedAt = body.archived ? at(0) : undefined;
    n.updatedAt = at(0);
    return json(n);
  });
  route("DELETE", "/notes/:id", ({ params }) => {
    const i = notes.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) notes.splice(i, 1);
    return noContent();
  });
  route("GET", "/notes/:id/attachments", () => json([]));
  route("POST", "/notes/:id/to-issue", ({ params, body }) => {
    const n = notes.find((x) => x.id === Number(params.id));
    if (!n) return fail(404, "not_found", "资源不存在");
    const issue = createIssueFromNote(
      body.projectId,
      n.title || "来自笔记",
      n.body,
    );
    n.body += `\n\n[${issue.key}](/projects/${issue.projectKey}/${issue.number})`;
    n.updatedAt = at(0);
    return json(
      {
        issueKey: issue.key,
        issueUrl: `/projects/${issue.projectKey}/${issue.number}`,
        note: n,
      },
      201,
    );
  });
  route("POST", "/notes/:id/to-reminder", ({ params, body }) => {
    const n = notes.find((x) => x.id === Number(params.id));
    if (!n) return fail(404, "not_found", "资源不存在");
    return json(
      addReminder(
        n.title || "笔记提醒",
        body.at,
        body.rrule ?? "",
        `/notes/${n.id}`,
      ),
      201,
    );
  });
}
