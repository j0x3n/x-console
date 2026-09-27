import { fail, json, noContent, route } from "../router";
import { at, GB, MB, rand } from "./util";

const caps = [
  "system.info",
  "metrics",
  "processes",
  "services",
  "pty",
  "files",
  "exec",
  "docker",
];
const hostsSeed = [
  {
    id: "srv-web1",
    name: "web-1",
    kind: "server",
    os: "linux",
    arch: "amd64",
    hostname: "web-1.internal",
    online: true,
    cpu: 34,
    mem: 62,
    disk: 48,
    caps,
    sys: ["Ubuntu", "24.04", "6.8.0-45-generic", "AMD EPYC 7B13", 4, 8 * GB],
  },
  {
    id: "srv-db",
    name: "db-main",
    kind: "server",
    os: "linux",
    arch: "amd64",
    hostname: "db.internal",
    online: true,
    cpu: 12,
    mem: 78,
    disk: 71,
    caps: caps.filter((c) => c !== "docker"),
    sys: ["Debian", "12", "6.1.0-25-amd64", "Intel Xeon E-2386G", 6, 32 * GB],
  },
  {
    id: "srv-nas",
    name: "nas",
    kind: "server",
    os: "linux",
    arch: "arm64",
    hostname: "nas.local",
    online: true,
    cpu: 7,
    mem: 41,
    disk: 86,
    caps,
    sys: ["Debian", "12", "6.6.31", "Cortex-A76", 4, 4 * GB],
  },
  {
    id: "srv-old",
    name: "vps-old",
    kind: "server",
    os: "linux",
    arch: "amd64",
    hostname: "vps-old",
    online: false,
    cpu: 0,
    mem: 0,
    disk: 55,
    caps: ["system.info", "metrics"],
    sys: ["CentOS", "7", "3.10.0", "Intel Xeon", 1, 1 * GB],
  },
  {
    id: "pc-home",
    name: "我的电脑",
    kind: "desktop",
    os: "windows",
    arch: "amd64",
    hostname: "DESKTOP-JO",
    online: true,
    cpu: 21,
    mem: 55,
    disk: 63,
    caps: [
      ...caps.filter((c) => c !== "docker"),
      "clipboard",
      "power",
      "open",
      "coding",
    ],
    sys: [
      "Windows 11",
      "24H2",
      "10.0.26100",
      "AMD Ryzen 7 7840HS",
      16,
      32 * GB,
    ],
  },
] as const;

function sample(h: (typeof hostsSeed)[number]) {
  const total = h.sys[5];
  return {
    at: at(0),
    cpu: h.cpu,
    cpuPerCore: Array.from({ length: h.sys[4] }, (_, i) =>
      Math.max(0, Math.min(100, h.cpu + ((i * 17) % 30) - 15)),
    ),
    memUsed: Math.round((total * h.mem) / 100),
    memTotal: total,
    swapUsed: 0,
    swapTotal: 2 * GB,
    disks: [
      {
        mount: h.os === "windows" ? "C:" : "/",
        fsType: h.os === "windows" ? "NTFS" : "ext4",
        used: Math.round(200 * GB * (h.disk / 100)),
        total: 200 * GB,
      },
    ],
    netRx: 120_000 + h.cpu * 1000,
    netTx: 80_000 + h.cpu * 800,
    diskRead: 2 * MB,
    diskWrite: 1 * MB,
    load1: h.cpu / 25,
    load5: h.cpu / 30,
    load15: h.cpu / 35,
    uptimeSeconds: 86400 * 23 + 3600 * 5,
    procs: 180 + h.cpu,
  };
}
const toHost = (h: (typeof hostsSeed)[number]) => ({
  id: h.id,
  name: h.name,
  kind: h.kind,
  source: "agent",
  online: h.online,
  os: h.os,
  arch: h.arch,
  hostname: h.hostname,
  version: "0.9.0",
  capabilities: [...h.caps],
  lastSeenAt: h.online ? at(0) : at(-60 * 26),
  metrics: h.online ? sample(h) : undefined,
  cpu: h.online ? h.cpu : undefined,
  memory: h.online ? h.mem : undefined,
  disk: h.disk,
  activeAlerts: h.id === "srv-nas" ? 1 : 0,
});
const find = (id: string) => hostsSeed.find((h) => h.id === id);

const alerts = [
  {
    id: 1,
    ruleId: 2,
    hostId: "srv-nas",
    hostName: "nas",
    metric: "disk",
    severity: "warning",
    value: 86,
    message: "磁盘用了 86%，超过 85%",
    firedAt: at(-60 * 3),
  },
  {
    id: 2,
    ruleId: 1,
    hostId: "srv-web1",
    hostName: "web-1",
    metric: "cpu",
    severity: "critical",
    value: 96,
    message: "CPU 96%，持续 5 分钟",
    firedAt: at(-60 * 26),
    resolvedAt: at(-60 * 25),
  },
  {
    id: 3,
    hostId: "srv-old",
    hostName: "vps-old",
    metric: "offline",
    severity: "critical",
    value: 0,
    message: "离线超过 10 分钟",
    firedAt: at(-60 * 26),
  },
];
const rules = [
  {
    id: 1,
    metric: "cpu",
    op: "gt",
    threshold: 90,
    durationSeconds: 300,
    severity: "critical",
    enabled: true,
    createdAt: at(-60 * 24 * 30),
  },
  {
    id: 2,
    metric: "disk",
    op: "gt",
    threshold: 85,
    durationSeconds: 0,
    severity: "warning",
    enabled: true,
    createdAt: at(-60 * 24 * 30),
  },
  {
    id: 3,
    metric: "offline",
    op: "gt",
    threshold: 0,
    durationSeconds: 600,
    severity: "critical",
    enabled: true,
    createdAt: at(-60 * 24 * 30),
  },
];
const sshHosts = [
  {
    id: 1,
    hostId: "ssh:1",
    name: "路由器",
    address: "192.168.1.1",
    port: 22,
    username: "root",
    auth: "key",
    hostKeyFingerprint: "SHA256:k3J9…",
    createdAt: at(-60 * 24 * 10),
  },
];

const procNames = [
  "nginx",
  "postgres",
  "node",
  "containerd",
  "dockerd",
  "sshd",
  "systemd",
  "x-console-agent",
  "redis-server",
  "cron",
];
const services = [
  ["nginx", "高性能 Web 服务器", "active", "running", true],
  ["docker", "Docker 容器引擎", "active", "running", true],
  ["ssh", "OpenSSH 服务", "active", "running", true],
  ["x-console-agent", "X Console 代理", "active", "running", true],
  ["postgresql", "PostgreSQL 数据库", "active", "running", true],
  ["cron", "定时任务", "active", "running", true],
  ["bluetooth", "蓝牙", "inactive", "dead", false],
  ["cups", "打印服务", "failed", "failed", true],
] as const;

const containers = [
  {
    id: "a1b2c3",
    name: "x-console",
    image: "ghcr.io/j0x3n/x-console:latest",
    state: "running",
    status: "Up 3 days",
    ports: [
      { ip: "127.0.0.1", privatePort: 8080, publicPort: 17380, type: "tcp" },
    ],
  },
  {
    id: "d4e5f6",
    name: "caddy",
    image: "caddy:2",
    state: "running",
    status: "Up 3 days",
    ports: [
      { privatePort: 443, publicPort: 443, type: "tcp" },
      { privatePort: 80, publicPort: 80, type: "tcp" },
    ],
  },
  {
    id: "g7h8i9",
    name: "postgres",
    image: "postgres:16",
    state: "running",
    status: "Up 12 days",
    ports: [{ privatePort: 5432, type: "tcp" }],
  },
  {
    id: "j1k2l3",
    name: "uptime-kuma",
    image: "louislam/uptime-kuma:1",
    state: "exited",
    status: "Exited (0) 2 days ago",
    ports: [],
  },
].map((c) => ({ ...c, created: at(-60 * 24 * 12) }));

let clipboard = "上次复制的内容：docker compose logs -f x-console";

export function register() {
  route("GET", "/hosts", ({ query }) => {
    const kind = query.get("kind");
    return json(hostsSeed.filter((h) => !kind || h.kind === kind).map(toHost));
  });
  route("GET", "/hosts/:id", ({ params }) => {
    const h = find(params.id);
    if (!h) return undefined;
    return json({
      ...toHost(h),
      address:
        h.kind === "desktop"
          ? "192.168.1.66"
          : "10.0.0." + (hostsSeed.indexOf(h) + 10),
      systemInfo: {
        hostname: h.hostname,
        os: h.os,
        platform: h.sys[0],
        platformVersion: h.sys[1],
        kernelVersion: h.sys[2],
        arch: h.arch,
        cpuModel: h.sys[3],
        cpuCores: h.sys[4],
        memoryTotal: h.sys[5],
        uptimeSeconds: 86400 * 23,
      },
    });
  });
  route("GET", "/hosts/:id/metrics", ({ params, query }) => {
    const h = find(params.id);
    if (!h) return undefined;
    const range = query.get("range") ?? "1h";
    const [n, step] =
      range === "7d" ? [168, 3600] : range === "24h" ? [144, 600] : [60, 60];
    const r = rand(hostsSeed.indexOf(h) + 3);
    const points = Array.from({ length: n }, (_, i) => {
      const wave = Math.sin(i / 8) * 10;
      const cpu = Math.max(1, Math.min(100, h.cpu + wave + (r() - 0.5) * 12));
      const memory = Math.max(1, Math.min(100, h.mem + (r() - 0.5) * 4));
      return {
        at: new Date(Date.now() - (n - i) * step * 1000).toISOString(),
        cpu,
        memory,
        memUsed: Math.round((h.sys[5] * memory) / 100),
        memTotal: h.sys[5],
        disk: h.disk,
        netRx: 100_000 + r() * 400_000,
        netTx: 60_000 + r() * 200_000,
        load1: cpu / 25,
      };
    });
    return json({ range, stepSeconds: step, points });
  });
  route("GET", "/hosts/:id/processes", ({ params }) => {
    const h = find(params.id);
    if (!h) return undefined;
    const r = rand(11);
    const items = Array.from({ length: 24 }, (_, i) => ({
      pid: 100 + i * 37,
      ppid: 1,
      name: procNames[i % procNames.length],
      user: i % 3 ? "root" : "www-data",
      cpu: Math.round(r() * (i < 3 ? 40 : 5) * 10) / 10,
      memRss: Math.round((r() * 300 + 10) * MB),
      memPercent: Math.round(r() * 80) / 10,
      cmdline: `/usr/bin/${procNames[i % procNames.length]}`,
      startedAt: at(-60 * 24 * (i % 5)),
      status: "S",
    })).sort((a, b) => b.cpu - a.cpu);
    return json({ items, total: 186 });
  });
  route("POST", "/hosts/:id/processes/:pid/kill", () => noContent());
  route("GET", "/hosts/:id/services", () =>
    json({
      items: services.map(([name, description, state, subState, enabled]) => ({
        name,
        description,
        state,
        subState,
        enabled,
        startType: enabled ? "enabled" : "disabled",
      })),
    }),
  );
  route("POST", "/hosts/:id/services/:name/:action", () => noContent());
  route("GET", "/hosts/:id/services/:name/logs", ({ params }) =>
    json({
      lines: Array.from(
        { length: 30 },
        (_, i) =>
          `${new Date(Date.now() - (30 - i) * 60000).toISOString().slice(0, 19)} ${params.name}[1234]: 演示日志第 ${i + 1} 行`,
      ),
    }),
  );
  route("GET", "/hosts/:id/files", ({ query }) => {
    const path = query.get("path") || "/";
    const dirs =
      path === "/" ? ["etc", "home", "srv", "var"] : ["logs", "backup"];
    const files =
      path === "/" ? [] : ["README.md", "docker-compose.yml", ".env"];
    return json({
      path,
      parent: path === "/" ? "" : path.replace(/\/[^/]+\/?$/, "") || "/",
      sep: "/",
      entries: [
        ...dirs.map((d) => ({
          name: d,
          path: `${path.replace(/\/$/, "")}/${d}`,
          type: "dir",
          size: 4096,
          modTime: at(-60 * 24),
          mode: "drwxr-xr-x",
        })),
        ...files.map((f, i) => ({
          name: f,
          path: `${path.replace(/\/$/, "")}/${f}`,
          type: "file",
          size: 800 + i * 300,
          modTime: at(-60 * (i + 2)),
          mode: "-rw-r--r--",
        })),
      ],
    });
  });
  route(
    "GET",
    "/hosts/:id/files/content",
    () =>
      new Response("# 演示文件\n\n这是假数据。\n", {
        headers: { "Content-Type": "text/plain; charset=utf-8" },
      }),
  );
  route("POST", "/hosts/:id/exec", ({ body }) =>
    json({
      exitCode: 0,
      stdout: `$ ${body?.command}\n（演示数据，没有真的执行）\n`,
      stderr: "",
      timedOut: false,
      truncated: false,
      durationMs: 42,
    }),
  );
  route("GET", "/hosts/:id/clipboard", () => json({ text: clipboard }));
  route("PUT", "/hosts/:id/clipboard", ({ body }) => {
    clipboard = body?.text ?? "";
    return json({ text: clipboard });
  });
  route("POST", "/hosts/:id/power", () => noContent(202));
  route("POST", "/hosts/:id/open", () => noContent(202));
  route("GET", "/hosts/:id/docker/containers", ({ query }) =>
    json({
      items:
        query.get("all") === "false"
          ? containers.filter((c) => c.state === "running")
          : containers,
    }),
  );
  route("POST", "/hosts/:id/docker/containers/:cid/:action", () =>
    noContent(202),
  );
  route("GET", "/hosts/:id/docker/containers/:cid/logs", () =>
    json({
      lines: Array.from(
        { length: 20 },
        (_, i) =>
          `2026-09-27T08:${String(i).padStart(2, "0")}:00Z INFO 请求 GET /api/v1/health 200 1ms`,
      ),
    }),
  );
  route("GET", "/hosts/:id/docker/stats", () =>
    json({
      items: containers
        .filter((c) => c.state === "running")
        .map((c, i) => ({
          id: c.id,
          name: c.name,
          cpuPercent: [3.2, 0.4, 1.8][i] ?? 0.1,
          memUsage: [180, 40, 320][i] * MB,
          memLimit: 8 * GB,
          memPercent: [2.2, 0.5, 3.9][i] ?? 0,
          netRx: 30 * MB,
          netTx: 12 * MB,
          blockRead: 5 * MB,
          blockWrite: 20 * MB,
          pids: [12, 8, 22][i] ?? 1,
        })),
    }),
  );
  route("GET", "/hosts/:id/docker/images", () =>
    json({
      items: containers.map((c, i) => ({
        id: `sha256:${c.id}`,
        tags: [c.image],
        size: [60, 45, 420, 180][i] * MB,
        created: at(-60 * 24 * 30),
        containers: 1,
      })),
    }),
  );
  route("GET", "/ssh-hosts", () => json(sshHosts));
  route("GET", "/alert-rules", () => json(rules));
  route("GET", "/alerts", ({ query }) => {
    const active = query.get("active") === "true";
    const hostId = query.get("hostId");
    return json({
      items: alerts.filter(
        (a) => (!active || !a.resolvedAt) && (!hostId || a.hostId === hostId),
      ),
    });
  });
  route("POST", "/alert-rules", ({ body }) => {
    const rr = {
      id: rules.length + 1,
      op: "gt",
      threshold: 90,
      durationSeconds: 0,
      severity: "warning",
      enabled: true,
      createdAt: at(0),
      ...body,
    };
    rules.push(rr);
    return json(rr, 201);
  });
  route("PUT", "/alert-rules/:id", ({ params, body }) => {
    const rr = rules.find((x) => x.id === Number(params.id));
    return rr
      ? json(Object.assign(rr, body))
      : fail(404, "not_found", "资源不存在");
  });
  route("DELETE", "/alert-rules/:id", ({ params }) => {
    const i = rules.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) rules.splice(i, 1);
    return noContent();
  });
}

/** Agent 任务里的代理列表用。 */
export const desktopAgent = { id: "pc-home", name: "我的电脑" };
