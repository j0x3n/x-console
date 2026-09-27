import { emitDemoEvent } from "../api/events";
import { ago, fail, json, noContent, now, route as addRoute } from "./router";
import { demoFull } from "./mode";

const route: typeof addRoute = (method, path, handler) =>
  addRoute(method, path, (request) => (demoFull ? handler(request) : undefined));

/* 自动化：规则和运行记录都在内存里。“立即运行”一秒后给出一条成功的记录。 */
const catalog = {
  topics: [
    { topic: "monitor.down", title: "网站挂了" },
    { topic: "monitor.up", title: "网站恢复" },
    { topic: "host.offline", title: "服务器离线" },
    { topic: "issue.created", title: "新建了 Issue" },
    { topic: "reminder.fired", title: "提醒到时间" },
  ],
  actions: [
    {
      name: "notify.send",
      title: "发通知",
      effect: "write",
      description: "发到所有打开的通知渠道（浏览器、Telegram、Bark）。",
      input: {
        type: "object",
        properties: {
          title: { type: "string" },
          body: { type: "string" },
          priority: {
            type: "string",
            enum: ["low", "normal", "high", "urgent"],
          },
        },
        required: ["title"],
      },
    },
    {
      name: "ai.ask",
      title: "问 AI",
      effect: "read",
      description:
        "把提示词和触发数据交给 Claude，回答作为下一步的输入（{{steps.N.result}}）。",
      input: {
        type: "object",
        properties: { prompt: { type: "string" } },
        required: ["prompt"],
      },
    },
    {
      name: "notes.create",
      title: "新建笔记",
      effect: "write",
      input: {
        type: "object",
        properties: { title: { type: "string" }, body: { type: "string" } },
      },
    },
    {
      name: "reminders.create",
      title: "新建提醒",
      effect: "write",
      input: {
        type: "object",
        properties: { title: { type: "string" }, at: { type: "string" } },
        required: ["title", "at"],
      },
    },
    {
      name: "ha.call_service",
      title: "控制智能家居",
      effect: "write",
      input: {
        type: "object",
        properties: {
          entityId: { type: "string" },
          service: { type: "string", enum: ["turn_on", "turn_off", "toggle"] },
        },
        required: ["entityId", "service"],
      },
    },
    {
      name: "scripts.run",
      title: "运行脚本",
      effect: "dangerous",
      description: "在选中的服务器上运行保存好的脚本。",
      input: {
        type: "object",
        properties: {
          scriptId: { type: "integer" },
          hosts: { type: "array", items: { type: "string" } },
        },
      },
    },
  ],
};

type Rule = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
let nextId = 10;
const rules: Rule[] = [
  {
    id: 1,
    name: "CPU 太高时提醒我",
    enabled: true,
    authorized: true,
    trigger: { type: "metric", metric: "cpu", op: ">", value: 90 },
    conditions: [],
    actions: [
      {
        action: "notify.send",
        input: {
          title: "{{trigger.data.host}} CPU {{trigger.data.cpu}}%",
          priority: "high",
        },
      },
    ],
    cooldownSeconds: 600,
    createdAt: ago(60 * 24 * 7),
    updatedAt: ago(60 * 24 * 2),
  },
  {
    id: 2,
    name: "每天早上总结昨天的告警",
    enabled: true,
    authorized: true,
    trigger: { type: "schedule", cron: "0 9 * * *" },
    conditions: [],
    actions: [
      { action: "ai.ask", input: { prompt: "用三句话总结昨天的告警" } },
      {
        action: "notify.send",
        input: { title: "昨日告警", body: "{{steps.0.result}}" },
      },
    ],
    cooldownSeconds: 0,
    createdAt: ago(60 * 24 * 5),
    updatedAt: ago(60 * 24 * 5),
  },
  {
    id: 3,
    name: "网站挂了就重启服务",
    enabled: false,
    authorized: false,
    trigger: {
      type: "event",
      topic: "monitor.down",
      match: 'data.url == "https://example.com"',
    },
    conditions: [{ field: "data.status", op: ">=", value: "500" }],
    actions: [{ action: "scripts.run", input: { scriptId: 3 } }],
    cooldownSeconds: 1800,
    createdAt: ago(60 * 24 * 3),
    updatedAt: ago(60 * 24 * 3),
  },
];
const runs: Rule[] = [
  {
    id: 101,
    automationId: 1,
    startedAt: ago(60 * 5),
    finishedAt: ago(60 * 5 - 0.02),
    status: "ok",
    triggerData: { data: { host: "web-1", cpu: 96 } },
    steps: [
      {
        action: "notify.send",
        input: { title: "web-1 CPU 96%" },
        result: "已发送到 2 个渠道",
      },
    ],
  },
  {
    id: 102,
    automationId: 2,
    startedAt: ago(60 * 3),
    finishedAt: ago(60 * 3 - 0.2),
    status: "failed",
    triggerData: {},
    steps: [
      {
        action: "ai.ask",
        input: { prompt: "用三句话总结昨天的告警" },
        result: "昨天有 2 次 CPU 告警，都在 5 分钟内恢复。",
      },
      {
        action: "notify.send",
        input: { title: "昨日告警" },
        error: "没有可用的通知渠道，请在设置 → 通知里打开一个",
      },
    ],
  },
  {
    id: 103,
    automationId: 2,
    startedAt: ago(60 * 27),
    finishedAt: ago(60 * 27 - 0.2),
    status: "ok",
    triggerData: {},
    steps: [
      { action: "ai.ask", input: {}, result: "昨天一切正常。" },
      { action: "notify.send", input: {}, result: "已发送到 1 个渠道" },
    ],
  },
];

const withLastRun = (r: Rule) => ({
  ...r,
  lastRun: runs
    .filter((x) => x.automationId === r.id)
    .sort((a, b) => b.startedAt.localeCompare(a.startedAt))[0],
});
const find = (id: string) => rules.find((r) => r.id === Number(id));
const dangerous = (r: Rule) =>
  r.actions.some(
    (s: Rule) =>
      catalog.actions.find((a) => a.name === s.action)?.effect === "dangerous",
  );
const save = (r: Rule, body: Rule) => {
  Object.assign(r, body, {
    updatedAt: now(),
    authorized: dangerous(body) ? true : r.authorized,
  });
  if (r.trigger?.type === "webhook" && !r.trigger.webhookPath)
    r.trigger = {
      ...r.trigger,
      webhookPath: `/hooks/${Math.random().toString(36).slice(2, 14)}`,
    };
  emitDemoEvent("automation.updated", { automationId: r.id });
  return withLastRun(r);
};

route("GET", "/automations/catalog", () => json(catalog));
route("GET", "/automations", () => json(rules.map(withLastRun)));
route("POST", "/automations", ({ body }) => {
  const r: Rule = { id: nextId++, createdAt: now(), authorized: false };
  rules.push(r);
  return json(save(r, body), 201);
});
route("GET", "/automations/:id", ({ params }) => {
  const r = find(params.id);
  return r ? json(withLastRun(r)) : fail(404, "not_found", "资源不存在");
});
route("PUT", "/automations/:id", ({ params, body }) => {
  const r = find(params.id);
  return r ? json(save(r, body)) : fail(404, "not_found", "资源不存在");
});
route("PATCH", "/automations/:id", ({ params, body }) => {
  const r = find(params.id);
  if (!r) return fail(404, "not_found", "资源不存在");
  r.enabled = !!body?.enabled;
  return json(withLastRun(r));
});
route("DELETE", "/automations/:id", ({ params }) => {
  const i = rules.findIndex((r) => r.id === Number(params.id));
  if (i >= 0) rules.splice(i, 1);
  return noContent();
});
route("POST", "/automations/:id/run", ({ params }) => {
  const r = find(params.id);
  if (!r) return fail(404, "not_found", "资源不存在");
  const run: Rule = {
    id: nextId++,
    automationId: r.id,
    startedAt: now(),
    status: "running",
    triggerData: {},
    steps: [],
  };
  runs.push(run);
  setTimeout(() => {
    run.status = "ok";
    run.finishedAt = now();
    run.steps = r.actions.map((s: Rule) => ({
      action: s.action,
      input: s.input,
      result: "完成（演示）",
    }));
    emitDemoEvent("automation.run", {
      automationId: r.id,
      runId: run.id,
      status: "ok",
    });
  }, 1000);
  return json({ runId: run.id }, 202);
});
route("GET", "/automations/:id/runs", ({ params }) =>
  json(
    runs
      .filter((x) => x.automationId === Number(params.id))
      .sort((a, b) => b.startedAt.localeCompare(a.startedAt)),
  ),
);
