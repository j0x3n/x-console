import { fail, json, noContent, route } from "../router";
import { at, date, rand } from "./util";

const monitors = [
  {
    id: 1,
    kind: "http",
    name: "X Console",
    target: "https://console.example.com/api/v1/health",
    lastStatus: "up",
    latency: 86,
  },
  {
    id: 2,
    kind: "http",
    name: "博客",
    target: "https://blog.example.com",
    lastStatus: "up",
    latency: 142,
  },
  {
    id: 3,
    kind: "http",
    name: "家里 NAS",
    target: "https://nas.example.com",
    lastStatus: "down",
    latency: 0,
    error: "连接超时（10 秒）",
    failures: 4,
  },
  {
    id: 4,
    kind: "http",
    name: "API 网关",
    target: "https://api.example.com/ping",
    lastStatus: "up",
    latency: 64,
  },
  {
    id: 5,
    kind: "tls",
    name: "console.example.com 证书",
    target: "console.example.com:443",
    lastStatus: "up",
    days: 62,
  },
  {
    id: 6,
    kind: "tls",
    name: "blog.example.com 证书",
    target: "blog.example.com:443",
    lastStatus: "up",
    days: 12,
  },
  {
    id: 7,
    kind: "domain",
    name: "example.com 域名",
    target: "example.com",
    lastStatus: "up",
    days: 45,
  },
  {
    id: 8,
    kind: "domain",
    name: "jo.dev 域名",
    target: "jo.dev",
    lastStatus: "up",
    days: 210,
  },
].map((m) => ({
  id: m.id,
  kind: m.kind,
  name: m.name,
  target: m.target,
  intervalSeconds: m.kind === "http" ? 60 : 86400,
  expectedStatus: 200,
  keyword: "",
  timeoutMs: 10000,
  enabled: true,
  lastStatus: m.lastStatus,
  lastCheckedAt: at(-1),
  lastError: m.error ?? "",
  consecutiveFailures: m.failures ?? 0,
  expiresAt: m.days ? date(m.days) + "T00:00:00Z" : undefined,
  daysLeft: m.days,
  createdAt: at(-60 * 24 * 40),
  latency: m.latency,
}));

const scripts = [
  {
    id: 1,
    name: "清理 Docker",
    description: "删掉没用的镜像和停止的容器",
    shell: "bash",
    body: "docker system prune -f\ndocker image prune -a -f",
    defaultHostIds: ["srv-web1", "srv-nas"],
    timeoutSeconds: 300,
  },
  {
    id: 2,
    name: "备份数据库",
    description: "导出后传到 NAS",
    shell: "bash",
    body: "pg_dump main | gzip > /backup/main-$(date +%F).sql.gz",
    defaultHostIds: ["srv-db"],
    timeoutSeconds: 1800,
  },
  {
    id: 3,
    name: "重启 x-console",
    description: "",
    shell: "sh",
    body: "cd /opt/x-console && docker compose restart",
    defaultHostIds: ["srv-web1"],
    timeoutSeconds: 120,
  },
  {
    id: 4,
    name: "查看磁盘",
    description: "",
    shell: "powershell",
    body: "Get-PSDrive -PSProvider FileSystem",
    defaultHostIds: ["pc-home"],
    timeoutSeconds: 60,
  },
].map((s, i) => ({
  ...s,
  createdAt: at(-60 * 24 * (20 - i)),
  updatedAt: at(-60 * 24 * (5 - i)),
}));
const runs = [
  {
    id: 1,
    scriptId: 1,
    hostId: "srv-web1",
    hostName: "web-1",
    startedAt: at(-60 * 5),
    finishedAt: at(-60 * 5 + 1),
    exitCode: 0,
    status: "ok",
    stdout: "Deleted Images: 4\nTotal reclaimed space: 1.2GB\n",
    stderr: "",
    error: "",
    triggeredBy: "user",
  },
  {
    id: 2,
    scriptId: 1,
    hostId: "srv-nas",
    hostName: "nas",
    startedAt: at(-60 * 5),
    finishedAt: at(-60 * 5 + 2),
    exitCode: 0,
    status: "ok",
    stdout: "Total reclaimed space: 320MB\n",
    stderr: "",
    error: "",
    triggeredBy: "user",
  },
  {
    id: 3,
    scriptId: 2,
    hostId: "srv-db",
    hostName: "db-main",
    startedAt: at(-60 * 26),
    finishedAt: at(-60 * 26 + 3),
    exitCode: 1,
    status: "failed",
    stdout: "",
    stderr: "pg_dump: error: connection refused\n",
    error: "",
    triggeredBy: "automation",
  },
];

const subs = [
  {
    id: 1,
    name: "阿里云 ECS",
    category: "server",
    amount: 99,
    currency: "CNY",
    cycle: "monthly",
    days: 5,
    url: "https://ecs.console.aliyun.com",
    autoRenew: true,
  },
  {
    id: 2,
    name: "Hetzner CX22",
    category: "server",
    amount: 4.5,
    currency: "EUR",
    cycle: "monthly",
    days: 18,
    url: "https://console.hetzner.cloud",
    autoRenew: true,
  },
  {
    id: 3,
    name: "example.com",
    category: "domain",
    amount: 78,
    currency: "CNY",
    cycle: "yearly",
    days: 45,
    url: "",
    autoRenew: false,
  },
  {
    id: 4,
    name: "jo.dev",
    category: "domain",
    amount: 12,
    currency: "USD",
    cycle: "yearly",
    days: 210,
    url: "",
    autoRenew: true,
  },
  {
    id: 5,
    name: "ChatGPT Plus",
    category: "saas",
    amount: 20,
    currency: "USD",
    cycle: "monthly",
    days: 9,
    url: "",
    autoRenew: true,
  },
  {
    id: 6,
    name: "1Password",
    category: "saas",
    amount: 36,
    currency: "USD",
    cycle: "yearly",
    days: 120,
    url: "",
    autoRenew: true,
  },
].map((s, i) => ({
  id: s.id,
  name: s.name,
  category: s.category,
  amount: s.amount,
  currency: s.currency,
  cycle: s.cycle,
  cycleDays: 0,
  nextRenewal: date(s.days),
  remindDaysBefore: [7, 1],
  url: s.url,
  note: "",
  autoRenew: s.autoRenew,
  daysLeft: s.days,
  monthlyCost:
    s.cycle === "yearly" ? Math.round((s.amount / 12) * 100) / 100 : s.amount,
  createdAt: at(-60 * 24 * (100 + i)),
  updatedAt: at(-60 * 24 * i),
}));

export function register() {
  route("GET", "/monitors", ({ query }) => {
    const kind = query.get("kind");
    return json(monitors.filter((m) => !kind || m.kind === kind));
  });
  route("GET", "/monitors/:id", ({ params }) => {
    const m = monitors.find((x) => x.id === Number(params.id));
    return m ? json(m) : fail(404, "not_found", "资源不存在");
  });
  route("POST", "/monitors/:id/check", ({ params }) => {
    const m = monitors.find((x) => x.id === Number(params.id));
    if (!m) return fail(404, "not_found", "资源不存在");
    m.lastCheckedAt = at(0);
    return json({
      at: at(0),
      ok: m.lastStatus === "up",
      statusCode: m.lastStatus === "up" ? 200 : undefined,
      latencyMs: m.latency,
      error: m.lastError,
      detail: {},
    });
  });
  route("PATCH", "/monitors/:id", ({ params, body }) => {
    const m = monitors.find((x) => x.id === Number(params.id));
    return m
      ? json(Object.assign(m, body))
      : fail(404, "not_found", "资源不存在");
  });
  route("GET", "/monitors/:id/results", ({ params, query }) => {
    const m = monitors.find((x) => x.id === Number(params.id));
    if (!m) return fail(404, "not_found", "资源不存在");
    const range = query.get("range") ?? "24h";
    const [n, step] =
      range === "30d"
        ? [120, 21600]
        : range === "7d"
          ? [168, 3600]
          : [144, 600];
    const r = rand(m.id);
    const items = Array.from({ length: n }, (_, i) => {
      const ok = m.lastStatus === "down" ? i < n - 5 && r() > 0.02 : r() > 0.01;
      return {
        at: new Date(Date.now() - (n - i) * step * 1000).toISOString(),
        ok,
        statusCode: ok ? 200 : undefined,
        latencyMs: ok ? Math.round((m.latency || 90) * (0.7 + r() * 0.6)) : 0,
        error: ok ? "" : "连接超时",
        detail: {},
      };
    });
    const up = items.filter((x) => x.ok);
    return json({
      items,
      uptime: up.length / items.length,
      avgLatencyMs: Math.round(
        up.reduce((s, x) => s + x.latencyMs, 0) / Math.max(1, up.length),
      ),
      total: items.length,
      stepSeconds: step,
    });
  });
  route("GET", "/scripts", () => json(scripts));
  route("GET", "/scripts/:id", ({ params }) => {
    const s = scripts.find((x) => x.id === Number(params.id));
    return s ? json(s) : fail(404, "not_found", "资源不存在");
  });
  route("GET", "/scripts/:id/runs", ({ params }) =>
    json({ items: runs.filter((x) => x.scriptId === Number(params.id)) }),
  );
  route("POST", "/scripts/:id/run", ({ params, body }) => {
    const s = scripts.find((x) => x.id === Number(params.id));
    if (!s) return fail(404, "not_found", "资源不存在");
    const ids: string[] = body?.hostIds ?? s.defaultHostIds;
    const created = ids.map((hostId) => {
      const run = {
        id: runs.length + 1,
        scriptId: s.id,
        hostId,
        hostName: hostId.replace(/^srv-|^pc-/, ""),
        startedAt: at(0),
        finishedAt: at(0),
        exitCode: 0,
        status: "ok",
        stdout: "（演示数据，没有真的执行）\n",
        stderr: "",
        error: "",
        triggeredBy: "user",
      };
      runs.unshift(run);
      return run;
    });
    return json(created, 202);
  });
  route("GET", "/script-runs/:id", ({ params }) => {
    const run = runs.find((x) => x.id === Number(params.id));
    return run ? json(run) : fail(404, "not_found", "资源不存在");
  });
  route("GET", "/subscriptions", ({ query }) =>
    json(query.get("archived") === "true" ? [] : subs),
  );
  route("GET", "/subscriptions/summary", () => {
    const byCur = new Map<string, { monthly: number; count: number }>();
    for (const s of subs) {
      const t = byCur.get(s.currency) ?? { monthly: 0, count: 0 };
      t.monthly += s.monthlyCost;
      t.count++;
      byCur.set(s.currency, t);
    }
    return json({
      totals: [...byCur].map(([currency, t]) => ({
        currency,
        monthly: Math.round(t.monthly * 100) / 100,
        yearly: Math.round(t.monthly * 1200) / 100,
        count: t.count,
      })),
      byCategory: ["server", "domain", "saas"].map((category) => {
        const list = subs.filter((s) => s.category === category);
        const monthly = list.reduce((a, s) => a + s.monthlyCost, 0);
        return {
          category,
          currency: list[0]?.currency ?? "CNY",
          monthly,
          yearly: monthly * 12,
        };
      }),
    });
  });
  route("GET", "/subscriptions/:id", ({ params }) => {
    const s = subs.find((x) => x.id === Number(params.id));
    return s ? json(s) : fail(404, "not_found", "资源不存在");
  });
  route("GET", "/subscriptions/:id/events", ({ params }) =>
    json([
      {
        id: 1,
        subscriptionId: Number(params.id),
        at: at(-60 * 24 * 100),
        kind: "created",
        detail: "新建",
      },
      {
        id: 2,
        subscriptionId: Number(params.id),
        at: at(-60 * 24 * 30),
        kind: "renewed",
        detail: "已续费",
      },
      {
        id: 3,
        subscriptionId: Number(params.id),
        at: at(-60 * 24 * 2),
        kind: "reminded",
        detail: "提前 7 天提醒",
      },
    ]),
  );
  route("DELETE", "/monitors/:id", ({ params }) => {
    const i = monitors.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) monitors.splice(i, 1);
    return noContent();
  });
}
