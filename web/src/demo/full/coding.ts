import { fail, json, noContent, route } from "../router";
import { at } from "./util";

const repos = [
  {
    id: 1,
    agentId: "pc-home",
    agentName: "我的电脑",
    agentOnline: true,
    path: "D:\\code\\x-console",
    name: "x-console",
    defaultBranch: "develop",
    remoteUrl: "git@github.com:j0x3n/x-console.git",
    githubRepo: "j0x3n/x-console",
    createdAt: at(-60 * 24 * 30),
  },
  {
    id: 2,
    agentId: "pc-home",
    agentName: "我的电脑",
    agentOnline: true,
    path: "D:\\code\\blog",
    name: "blog",
    defaultBranch: "main",
    remoteUrl: "git@github.com:j0x3n/blog.git",
    githubRepo: "j0x3n/blog",
    createdAt: at(-60 * 24 * 20),
  },
];
const file = (
  path: string,
  additions: number,
  deletions: number,
  status = "modified",
) => ({ path, status, additions, deletions });
const seed = [
  {
    status: "running",
    title: "云盘上传改成流式写盘",
    issueKey: "XC-1",
    executor: "claude",
    mins: 6,
    files: [],
  },
  {
    status: "review",
    title: "登录页只用密码登录",
    issueKey: "XC-2",
    executor: "claude",
    mins: 40,
    files: [
      file("web/src/auth/AuthGate.tsx", 48, 12),
      file("backend/internal/server/auth/auth.go", 5, 0),
    ],
  },
  {
    status: "queued",
    title: "文章加全文搜索",
    issueKey: "BLOG-2",
    executor: "codex",
    mins: 2,
    files: [],
  },
  {
    status: "pr_opened",
    title: "侧边栏二级菜单",
    issueKey: "XC-4",
    executor: "claude",
    mins: 60 * 20,
    files: [
      file("web/src/components/layout/Sidebar.tsx", 80, 10),
      file("web/src/lib/navChildren.ts", 45, 0, "added"),
    ],
  },
  {
    status: "failed",
    title: "修复看板拖动",
    issueKey: "XC-9",
    executor: "codex",
    mins: 60 * 26,
    files: [],
    error: "测试没过：TestMoveIssue",
  },
  {
    status: "committed",
    title: "暗色主题配色",
    issueKey: "BLOG-3",
    executor: "claude",
    mins: 60 * 30,
    files: [file("src/styles/theme.css", 120, 34)],
  },
];
const tasks = seed.map((s, i) => ({
  id: i + 1,
  repoId: s.issueKey.startsWith("BLOG") ? 2 : 1,
  repoName: s.issueKey.startsWith("BLOG") ? "blog" : "x-console",
  agentId: "pc-home",
  issueKey: s.issueKey,
  executor: s.executor,
  prompt: `${s.title}。先读相关代码，改完跑测试。`,
  title: s.title,
  baseBranch: "develop",
  branch: `claude/task-${i + 1}`,
  baseCommit: "a730db6",
  status: s.status,
  exitCode:
    s.status === "failed"
      ? 1
      : s.status === "running" || s.status === "queued"
        ? undefined
        : 0,
  error: s.error ?? "",
  commitSha:
    s.status === "committed" || s.status === "pr_opened" ? "3f2a9c1" : "",
  prUrl:
    s.status === "pr_opened"
      ? "https://github.com/j0x3n/x-console/pull/17"
      : "",
  changedFiles: s.files,
  queuePosition: s.status === "queued" ? 1 : undefined,
  timeoutMinutes: 60,
  createdAt: at(-s.mins - 1),
  startedAt: s.status === "queued" ? undefined : at(-s.mins),
  finishedAt: ["running", "queued"].includes(s.status)
    ? undefined
    : at(-s.mins + 12),
  updatedAt: at(-Math.max(1, s.mins - 12)),
}));
const events = [
  { kind: "status", text: "开始运行" },
  { kind: "text", text: "我先看一下上传相关的代码。" },
  {
    kind: "tool",
    text: "读取 backend/internal/server/modules/drive/upload.go",
  },
  {
    kind: "text",
    text: "现在是把整个文件读进内存再写盘，改成用 io.Copy 边读边写。",
  },
  { kind: "tool", text: "编辑 upload.go（+32 −18）" },
  { kind: "tool", text: "运行 go test ./internal/server/modules/drive/..." },
  { kind: "text", text: "测试都过了。再加一个 1 GB 文件的测试。" },
];
const diff = `diff --git a/web/src/auth/AuthGate.tsx b/web/src/auth/AuthGate.tsx
--- a/web/src/auth/AuthGate.tsx
+++ b/web/src/auth/AuthGate.tsx
@@ -40,7 +40,9 @@ function LoginPage() {
-  const [form, setForm] = useState({ username: "", password: "", code: "" });
+  const [form, setForm] = useState({ username: "", password: "", code: "" });
+  const [needCode, setNeedCode] = useState(false);
`;

export function register() {
  route("GET", "/coding/executors", () =>
    json([
      {
        name: "claude",
        available: true,
        path: "C:\\Users\\jo\\.local\\bin\\claude.exe",
        version: "2.3.1",
      },
      {
        name: "codex",
        available: true,
        path: "C:\\Users\\jo\\AppData\\npm\\codex.cmd",
        version: "0.48.0",
      },
    ]),
  );
  route("GET", "/coding/repos", () => json(repos));
  route("GET", "/coding/repos/discover", () =>
    json({
      roots: ["D:\\code"],
      items: repos.map((r) => ({
        path: r.path,
        name: r.name,
        currentBranch: r.defaultBranch,
        defaultBranch: r.defaultBranch,
        remoteUrl: r.remoteUrl,
        repoId: r.id,
      })),
    }),
  );
  route("GET", "/coding/tasks", ({ query }) => {
    const status = query.getAll("status").flatMap((s) => s.split(","));
    const issueKey = query.get("issueKey");
    const repoId = query.get("repoId");
    return json({
      items: tasks.filter(
        (t) =>
          (!status.length || status.includes(t.status)) &&
          (!issueKey || t.issueKey === issueKey) &&
          (!repoId || t.repoId === Number(repoId)),
      ),
    });
  });
  route("POST", "/coding/tasks", ({ body }) => {
    const repo = repos.find((r) => r.id === body.repoId) ?? repos[0];
    const t = {
      ...tasks[2],
      id: tasks.length + 1,
      repoId: repo.id,
      repoName: repo.name,
      issueKey: body.issueKey,
      executor: body.executor ?? "claude",
      prompt: body.prompt,
      title: (body.prompt ?? "新任务").slice(0, 30),
      status: "queued",
      queuePosition: 2,
      createdAt: at(0),
      updatedAt: at(0),
      startedAt: undefined,
      finishedAt: undefined,
      changedFiles: [],
    };
    tasks.unshift(t);
    return json(t, 201);
  });
  route("GET", "/coding/tasks/:id", ({ params }) => {
    const t = tasks.find((x) => x.id === Number(params.id));
    return t ? json(t) : fail(404, "not_found", "资源不存在");
  });
  route("GET", "/coding/tasks/:id/events", ({ params }) => {
    const t = tasks.find((x) => x.id === Number(params.id));
    if (!t) return fail(404, "not_found", "资源不存在");
    const list = events.map((e, i) => ({
      seq: i + 1,
      at: at(-20 + i * 2),
      ...e,
    }));
    if (t.status !== "running" && t.status !== "queued")
      list.push({
        seq: list.length + 1,
        at: at(-5),
        kind: t.status === "failed" ? "error" : "done",
        text: t.status === "failed" ? t.error : "完成",
      });
    return json({ items: list, lastSeq: list.length });
  });
  route("GET", "/coding/tasks/:id/diff", ({ params }) => {
    const t = tasks.find((x) => x.id === Number(params.id));
    return t
      ? json({ files: t.changedFiles, diff, truncated: false })
      : fail(404, "not_found", "资源不存在");
  });
  for (const action of ["cancel", "commit", "push", "pr", "discard"]) {
    route("POST", `/coding/tasks/:id/${action}`, ({ params }) => {
      const t = tasks.find((x) => x.id === Number(params.id));
      if (!t) return fail(404, "not_found", "资源不存在");
      t.status = {
        cancel: "canceled",
        commit: "committed",
        push: "pushed",
        pr: "pr_opened",
        discard: "discarded",
      }[action]!;
      if (action === "pr")
        t.prUrl = "https://github.com/j0x3n/x-console/pull/18";
      t.updatedAt = at(0);
      return json(t);
    });
  }
  route("GET", "/coding/settings", () =>
    json({ maxConcurrent: 2, defaultTimeoutMinutes: 60, prAvailable: true }),
  );
  route("DELETE", "/coding/repos/:id", () => noContent());
}
