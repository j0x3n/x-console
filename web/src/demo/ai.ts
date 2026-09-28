import { emitDemoEvent } from "../api/events";
import { ago, fail, json, noContent, now, routeFull as route } from "./router";

/*
 * AI 助手：不接真的模型，按关键词给固定的回复，文字一段段推过来。
 * 说“删”会走一次“等你确认”；说“提醒”会直接建一个提醒（假的）。
 */
type Block = Record<string, unknown>;
interface Msg {
  id: number;
  seq: number;
  role: "user" | "assistant";
  content: Block[];
  createdAt: string;
}
interface Pending {
  id: number;
  conversationId: number;
  toolUseId: string;
  action: string;
  input: Record<string, unknown>;
  status: "pending" | "approved" | "rejected" | "done" | "failed";
  result?: unknown;
}
interface Conv {
  id: number;
  title: string;
  createdAt: string;
  updatedAt: string;
  messages: Msg[];
  pending: Pending[];
  running: boolean;
}

const settings = {
  hasApiKey: true,
  model: "claude-opus-5-5",
  confirmAllWrites: false,
};
const tools = [
  ["notes__search", "notes.search", "搜索笔记", "read"],
  ["notes__create", "notes.create", "新建笔记", "write"],
  ["notes__delete", "notes.delete", "删除笔记", "write"],
  ["reminders__list", "reminders.list", "查看提醒", "read"],
  ["reminders__create", "reminders.create", "新建提醒", "write"],
  ["issues__create", "issues.create", "新建 Issue", "write"],
  ["hosts__list", "hosts.list", "查看服务器", "read"],
  ["scripts__run", "scripts.run", "运行脚本", "dangerous"],
].map(([name, action, title, effect]) => ({ name, action, title, effect }));

let ids = 1000;
const nextId = () => ids++;
const convs: Conv[] = [];

function seed() {
  const c: Conv = {
    id: nextId(),
    title: "帮我整理这周的待办",
    createdAt: ago(60 * 20),
    updatedAt: ago(60 * 19),
    messages: [],
    pending: [],
    running: false,
  };
  const push = (role: Msg["role"], content: Block[]) =>
    c.messages.push({
      id: nextId(),
      seq: c.messages.length + 1,
      role,
      content,
      createdAt: ago(60 * 20),
    });
  push("user", [
    { type: "text", text: "帮我整理这周的待办，再建一个周五交电费的提醒" },
  ]);
  push("assistant", [
    { type: "text", text: "好的，我先看一下现在的提醒。" },
    {
      type: "tool_use",
      id: "seed1",
      name: "reminders__list",
      input: { range: "this_week" },
    },
  ]);
  push("user", [
    {
      type: "tool_result",
      tool_use_id: "seed1",
      content: '[{"title":"周三组会"},{"title":"周四体检"}]',
    },
  ]);
  push("assistant", [
    {
      type: "tool_use",
      id: "seed2",
      name: "reminders__create",
      input: { title: "交电费", at: "2026-10-02T09:00:00+08:00" },
    },
  ]);
  push("user", [
    {
      type: "tool_result",
      tool_use_id: "seed2",
      content: '{"id":42,"title":"交电费"}',
    },
  ]);
  push("assistant", [
    {
      type: "text",
      text: "这周一共 3 件事：\n\n1. **周三** 组会\n2. **周四** 体检\n3. **周五 9:00** 交电费（刚建好，[去看看](/reminders)）",
    },
  ]);
  convs.push(c);
}
seed();

const conv = (id: string | number) => convs.find((c) => c.id === Number(id));
const detail = (c: Conv) => ({
  conversation: {
    id: c.id,
    title: c.title,
    createdAt: c.createdAt,
    updatedAt: c.updatedAt,
  },
  messages: c.messages,
  pendingActions: c.pending,
  running: c.running,
});
const add = (c: Conv, role: Msg["role"], content: Block[]) => {
  c.messages.push({
    id: nextId(),
    seq: c.messages.length + 1,
    role,
    content,
    createdAt: now(),
  });
  c.updatedAt = now();
};
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** 把一段文字拆开推过去，再存成一条消息。 */
async function say(c: Conv, text: string, extra: Block[] = []) {
  const parts = text.match(/.{1,6}/gsu) ?? [text];
  for (const p of parts) {
    if (!c.running) return;
    emitDemoEvent("ai.delta", { conversationId: c.id, text: p });
    await wait(70);
  }
  add(c, "assistant", [{ type: "text", text }, ...extra]);
  emitDemoEvent("ai.message_saved", { conversationId: c.id });
}

function finish(c: Conv) {
  c.running = false;
  emitDemoEvent("ai.message_done", { conversationId: c.id });
}

async function reply(
  c: Conv,
  text: string,
  page?: { path?: string; title?: string },
) {
  await wait(500);
  if (text.includes("删")) {
    await say(c, "我先找一下相关的笔记。", [
      {
        type: "tool_use",
        id: `s${c.id}${c.messages.length}`,
        name: "notes__search",
        input: { q: text.replace(/删掉?|删除/g, "").trim() || "周报" },
      },
    ]);
    const searchId = c.messages.at(-1)!.content[1].id as string;
    await wait(400);
    add(c, "user", [
      {
        type: "tool_result",
        tool_use_id: searchId,
        content: '[{"id":7,"title":"8 月周报"}]',
      },
    ]);
    emitDemoEvent("ai.message_saved", { conversationId: c.id });
    const useId = `d${c.id}${c.messages.length}`;
    await say(c, "找到 1 条：8 月周报。删除之前需要你确认。", [
      { type: "tool_use", id: useId, name: "notes__delete", input: { id: 7 } },
    ]);
    c.pending.push({
      id: nextId(),
      conversationId: c.id,
      toolUseId: useId,
      action: "notes.delete",
      input: { id: 7 },
      status: "pending",
    });
    emitDemoEvent("ai.action_pending", { conversationId: c.id });
    return; // 等用户确认，这一轮还没结束
  }
  if (text.includes("提醒")) {
    const useId = `r${c.id}${c.messages.length}`;
    await say(c, "好的，我来建这个提醒。", [
      {
        type: "tool_use",
        id: useId,
        name: "reminders__create",
        input: { title: text.slice(0, 20), at: "明天 09:00" },
      },
    ]);
    await wait(500);
    add(c, "user", [
      { type: "tool_result", tool_use_id: useId, content: '{"id":43}' },
    ]);
    emitDemoEvent("ai.message_saved", { conversationId: c.id });
    await say(c, "建好了，明天早上 9 点提醒你。[打开提醒](/reminders)");
    return finish(c);
  }
  const where = page?.title
    ? `你现在在“${page.title}”页面（${page.path}）。`
    : "";
  await say(
    c,
    `${where}这是演示回复，还没接上真的模型。\n\n可以试试：\n- “删掉周报笔记”：会先让你确认\n- “明天提醒我交电费”：会直接建提醒`,
  );
  finish(c);
}

route("GET", "/ai/settings", () => json(settings));
route("PUT", "/ai/settings", ({ body }) => {
  if (body?.apiKey !== undefined) settings.hasApiKey = body.apiKey !== "";
  if (body?.model) settings.model = body.model;
  if (body?.confirmAllWrites !== undefined)
    settings.confirmAllWrites = body.confirmAllWrites;
  return json(settings);
});
route("GET", "/ai/tools", () => json(tools));
route("GET", "/ai/conversations", () =>
  json(
    [...convs]
      .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
      .map((c) => detail(c).conversation),
  ),
);
route("POST", "/ai/conversations", () => {
  const c: Conv = {
    id: nextId(),
    title: "",
    createdAt: now(),
    updatedAt: now(),
    messages: [],
    pending: [],
    running: false,
  };
  convs.push(c);
  return json(detail(c).conversation, 201);
});
route("GET", "/ai/conversations/:id", ({ params }) => {
  const c = conv(params.id);
  return c ? json(detail(c)) : fail(404, "not_found", "资源不存在");
});
route("DELETE", "/ai/conversations/:id", ({ params }) => {
  const i = convs.findIndex((c) => c.id === Number(params.id));
  if (i >= 0) convs.splice(i, 1);
  return noContent();
});
route("POST", "/ai/conversations/:id/messages", ({ params, body }) => {
  const c = conv(params.id);
  if (!c) return fail(404, "not_found", "资源不存在");
  if (c.running) return fail(409, "conflict", "上一条还没回复完");
  const text: string = body?.text ?? "";
  if (!c.title) c.title = text.slice(0, 24);
  add(c, "user", [{ type: "text", text }]);
  c.running = true;
  void reply(c, text, body?.context);
  return noContent(202);
});
route("POST", "/ai/conversations/:id/stop", ({ params }) => {
  const c = conv(params.id);
  if (c) {
    c.running = false;
    for (const p of c.pending)
      if (p.status === "pending") p.status = "rejected";
    emitDemoEvent("ai.message_done", { conversationId: c.id });
  }
  return noContent();
});
async function decide(id: string, approve: boolean) {
  const c = convs.find((x) => x.pending.some((p) => p.id === Number(id)));
  const p = c?.pending.find((x) => x.id === Number(id));
  if (!c || !p || p.status !== "pending") return;
  p.status = approve ? "done" : "rejected";
  p.result = approve ? "已删除" : "用户拒绝了";
  add(c, "user", [
    {
      type: "tool_result",
      tool_use_id: p.toolUseId,
      content: p.result,
      is_error: !approve,
    },
  ]);
  emitDemoEvent("ai.message_saved", { conversationId: c.id });
  await say(c, approve ? "删好了。" : "好的，不删了。");
  finish(c);
}
route("POST", "/ai/actions/:id/approve", ({ params }) => {
  void decide(params.id, true);
  return noContent();
});
route("POST", "/ai/actions/:id/reject", ({ params }) => {
  void decide(params.id, false);
  return noContent();
});
