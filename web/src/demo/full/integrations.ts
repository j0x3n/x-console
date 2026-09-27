import { json, noContent, route } from "../router";
import { at } from "./util";

/* 智能家居 */
type State = {
  entityId: string;
  state: string;
  attributes: Record<string, unknown>;
  lastChanged: string;
};
const s = (
  entityId: string,
  state: string,
  name: string,
  extra: Record<string, unknown> = {},
  min = 30,
): State => ({
  entityId,
  state,
  attributes: { friendly_name: name, ...extra },
  lastChanged: at(-min),
});
const states: State[] = [
  s("light.living_room", "on", "客厅灯", { brightness: 200 }, 12),
  s("light.bedroom", "off", "卧室灯", {}, 300),
  s("light.desk", "on", "台灯", { brightness: 120 }, 40),
  s("switch.water_heater", "off", "热水器", {}, 90),
  s("switch.fish_tank", "on", "鱼缸灯", {}, 600),
  s("fan.purifier", "on", "空气净化器", { percentage: 40 }, 200),
  s(
    "climate.living_room",
    "cool",
    "客厅空调",
    { temperature: 26, current_temperature: 27.5, unit_of_measurement: "°C" },
    45,
  ),
  s("cover.curtain", "open", "客厅窗帘", { current_position: 100 }, 480),
  s("lock.front_door", "locked", "大门锁", {}, 120),
  s(
    "sensor.living_temperature",
    "25.8",
    "客厅温度",
    { unit_of_measurement: "°C", device_class: "temperature" },
    2,
  ),
  s(
    "sensor.living_humidity",
    "56",
    "客厅湿度",
    { unit_of_measurement: "%", device_class: "humidity" },
    2,
  ),
  s(
    "sensor.power",
    "312",
    "当前用电",
    { unit_of_measurement: "W", device_class: "power" },
    1,
  ),
  s("binary_sensor.door", "off", "门磁", { device_class: "door" }, 180),
  s(
    "binary_sensor.motion_hall",
    "on",
    "走廊人体",
    { device_class: "motion" },
    3,
  ),
  s("media_player.tv", "playing", "客厅电视", { media_title: "新闻联播" }, 20),
  s("scene.movie", at(-60 * 20), "观影模式", {}, 60 * 24),
  s("scene.sleep", at(-600), "睡眠模式", {}, 60 * 24),
  s("script.good_morning", "off", "早安", {}, 60 * 10),
  s("input_boolean.guest_mode", "off", "访客模式", {}, 60 * 48),
];
let favorites = [
  "light.living_room",
  "climate.living_room",
  "lock.front_door",
  "sensor.living_temperature",
  "scene.movie",
  "switch.water_heater",
];
const toggle: Record<string, [string, string]> = {
  turn_on: ["on", "on"],
  turn_off: ["off", "off"],
};

/* GitHub */
const repos = ["j0x3n/x-console", "j0x3n/blog"];
const pulls = [
  {
    repo: repos[0],
    number: 17,
    title: "侧边栏二级菜单",
    author: "j0x3n",
    headRef: "claude/task-4",
    baseRef: "develop",
    draft: false,
    reviewState: "approved",
    checkState: "success",
    issueKeys: ["XC-4"],
    codingTaskId: 4,
    h: 5,
  },
  {
    repo: repos[0],
    number: 18,
    title: "云盘上传改成流式写盘",
    author: "j0x3n",
    headRef: "claude/task-1",
    baseRef: "develop",
    draft: true,
    reviewState: "pending",
    checkState: "pending",
    issueKeys: ["XC-1"],
    h: 1,
  },
  {
    repo: repos[0],
    number: 15,
    title: "B2 今日页前端",
    author: "gpt-dev",
    headRef: "task/B2-today",
    baseRef: "develop",
    draft: false,
    reviewState: "changes_requested",
    checkState: "failure",
    issueKeys: [],
    h: 26,
  },
  {
    repo: repos[1],
    number: 9,
    title: "暗色主题",
    author: "j0x3n",
    headRef: "dark-theme",
    baseRef: "main",
    draft: false,
    reviewState: "none",
    checkState: "success",
    issueKeys: ["BLOG-3"],
    h: 30,
  },
].map(({ h, ...p }) => ({
  ...p,
  url: `https://github.com/${p.repo}/pull/${p.number}`,
  createdAt: at(-60 * (h + 24)),
  updatedAt: at(-60 * h),
}));
const runs = [
  ["CI", "develop", "push", "completed", "success", 3],
  ["Deploy", "develop", "push", "completed", "success", 8],
  ["CI", "claude/task-1", "pull_request", "in_progress", "", 1],
  ["CI", "task/B2-today", "pull_request", "completed", "failure", 30],
  ["Deploy", "main", "push", "completed", "success", 60 * 20],
].map(([name, branch, event, status, conclusion, m], i) => ({
  id: 1000 + i,
  repo: i === 4 ? repos[1] : repos[0],
  name,
  branch,
  event,
  status,
  conclusion,
  url: `https://github.com/${repos[0]}/actions/runs/${1000 + i}`,
  defaultBranch: branch === "main",
  createdAt: at(-(m as number) - 3),
  updatedAt: at(-(m as number)),
}));
const ghIssues = [
  {
    repo: repos[0],
    number: 21,
    title: "手机上命令面板太高",
    author: "j0x3n",
    assignees: ["j0x3n"],
    labels: ["bug"],
    relation: "both",
    h: 4,
  },
  {
    repo: repos[1],
    number: 5,
    title: "RSS 里图片地址不对",
    author: "reader42",
    assignees: ["j0x3n"],
    labels: ["bug"],
    relation: "assigned",
    h: 50,
  },
].map(({ h, ...x }) => ({
  ...x,
  url: `https://github.com/${x.repo}/issues/${x.number}`,
  createdAt: at(-60 * (h + 10)),
  updatedAt: at(-60 * h),
}));

/* 通知、审计、代理 */
const notifications = [
  {
    kind: "monitor.down",
    title: "家里 NAS 访问不了",
    body: "连接超时（10 秒），已经连续 4 次",
    link: "/monitoring",
    priority: "high",
    source: "monitoring",
    m: 12,
    read: false,
  },
  {
    kind: "alert",
    title: "nas 磁盘用了 86%",
    body: "超过 85% 的告警线",
    link: "/servers/srv-nas",
    priority: "normal",
    source: "hosts",
    m: 180,
    read: false,
  },
  {
    kind: "reminder",
    title: "取快递",
    body: "",
    link: "/reminders",
    priority: "normal",
    source: "reminders",
    m: 60,
    read: false,
  },
  {
    kind: "coding.review",
    title: "编码任务等你看：登录页只用密码登录",
    body: "改了 2 个文件",
    link: "/coding/2",
    priority: "normal",
    source: "coding",
    m: 40,
    read: true,
  },
  {
    kind: "subscription",
    title: "阿里云 ECS 5 天后续费",
    body: "99 元",
    link: "/monitoring/subscriptions",
    priority: "low",
    source: "monitoring",
    m: 60 * 20,
    read: true,
  },
  {
    kind: "brief",
    title: "今天的早报",
    body: "3 个日程、4 个要到期的 Issue",
    link: "/calendar/briefs",
    priority: "low",
    source: "brief",
    m: 60 * 5,
    read: true,
  },
].map(({ m, read, ...n }, i) => ({
  id: i + 1,
  createdAt: at(-m),
  readAt: read ? at(-m + 5) : undefined,
  ...n,
}));
const audit = [
  ["jo", "auth.login", "192.168.1.66", "ok"],
  ["jo", "hosts.exec", "srv-web1", "ok"],
  ["automation:1", "notify.send", "web-1 CPU", "ok"],
  ["jo", "scripts.run", "清理 Docker", "ok"],
  ["ai", "notes.create", "周会记录", "ok"],
  ["jo", "auth.elevate", "", "ok"],
  ["jo", "agents.revoke", "old-laptop", "ok"],
  ["unknown", "auth.login", "203.0.113.9", "denied"],
].map(([actor, action, target, result], i) => ({
  id: 100 - i,
  at: at(-(i * 47 + 3)),
  actor,
  action,
  target,
  result,
  detail: {},
}));
const agents = [
  {
    id: "srv-web1",
    name: "web-1",
    kind: "server",
    os: "linux",
    arch: "amd64",
    hostname: "web-1.internal",
    version: "0.9.0",
    online: true,
  },
  {
    id: "srv-db",
    name: "db-main",
    kind: "server",
    os: "linux",
    arch: "amd64",
    hostname: "db.internal",
    version: "0.9.0",
    online: true,
  },
  {
    id: "srv-nas",
    name: "nas",
    kind: "server",
    os: "linux",
    arch: "arm64",
    hostname: "nas.local",
    version: "0.9.0",
    online: true,
  },
  {
    id: "srv-old",
    name: "vps-old",
    kind: "server",
    os: "linux",
    arch: "amd64",
    hostname: "vps-old",
    version: "0.8.2",
    online: false,
  },
  {
    id: "pc-home",
    name: "我的电脑",
    kind: "desktop",
    os: "windows",
    arch: "amd64",
    hostname: "DESKTOP-JO",
    version: "0.9.0",
    online: true,
  },
].map((a, i) => ({
  ...a,
  capabilities: [],
  createdAt: at(-60 * 24 * (40 - i)),
  lastSeenAt: a.online ? at(0) : at(-60 * 26),
}));

export function register() {
  route("GET", "/ha/status", () =>
    json({
      configured: true,
      connected: true,
      mode: "direct",
      version: "2026.9.2",
      entityCount: states.length,
      connectedAt: at(-60 * 5),
    }),
  );
  route("GET", "/ha/config", () =>
    json({
      url: "http://192.168.1.20:8123",
      mode: "direct",
      agentId: "",
      hasToken: true,
      token: "eyJh…demo",
    }),
  );
  route("GET", "/ha/states", ({ query }) => {
    const domain = query.get("domain");
    const q = query.get("q")?.toLowerCase();
    return json(
      states.filter(
        (x) =>
          (!domain || x.entityId.startsWith(`${domain}.`)) &&
          (!q ||
            `${x.entityId} ${x.attributes.friendly_name}`
              .toLowerCase()
              .includes(q)),
      ),
    );
  });
  route("GET", "/ha/states/:id", ({ params }) => {
    const x = states.find((st) => st.entityId === params.id);
    return x ? json(x) : undefined;
  });
  route("POST", "/ha/services/:domain/:service", ({ params, body }) => {
    const x = states.find((st) => st.entityId === body?.entityId);
    if (x) {
      if (params.service === "toggle")
        x.state = x.state === "on" ? "off" : "on";
      else if (toggle[params.service]) x.state = toggle[params.service][0];
      else if (params.service === "lock") x.state = "locked";
      else if (params.service === "unlock") x.state = "unlocked";
      else if (params.service === "open_cover") x.state = "open";
      else if (params.service === "close_cover") x.state = "closed";
      x.lastChanged = at(0);
    }
    return json(x ? [x] : []);
  });
  route("GET", "/ha/favorites", () =>
    json(
      favorites.map((entityId, i) => ({
        entityId,
        alias: "",
        sortOrder: i,
        state: states.find((x) => x.entityId === entityId),
      })),
    ),
  );
  route("PUT", "/ha/favorites", ({ body }) => {
    favorites = (body ?? []).map((f: { entityId: string }) => f.entityId);
    return json(
      favorites.map((entityId, i) => ({
        entityId,
        alias: "",
        sortOrder: i,
        state: states.find((x) => x.entityId === entityId),
      })),
    );
  });

  route("GET", "/github/config", () =>
    json({
      hasToken: true,
      token: "github_pat_…demo",
      repos,
      apiUrl: "https://api.github.com",
      login: "j0x3n",
    }),
  );
  route("GET", "/github/status", () =>
    json({
      configured: true,
      syncing: false,
      repoCount: repos.length,
      login: "j0x3n",
      lastSyncAt: at(-3),
      rateLimitRemaining: 4876,
    }),
  );
  route("POST", "/github/sync", () => noContent(202));
  route("GET", "/github/pulls", ({ query }) =>
    json(
      pulls.filter((p) => !query.get("repo") || p.repo === query.get("repo")),
    ),
  );
  route("GET", "/github/runs", ({ query }) =>
    json(
      runs.filter((r) => !query.get("repo") || r.repo === query.get("repo")),
    ),
  );
  route("GET", "/github/issues", () => json(ghIssues));
  route("GET", "/linear/status", () =>
    json({ configured: false, syncing: false, mappingCount: 0 }),
  );
  route("GET", "/linear/config", () =>
    json({
      hasKey: false,
      apiKey: "",
      apiUrl: "https://api.linear.app/graphql",
      mappings: [],
    }),
  );

  route("GET", "/notifications", ({ query }) => {
    const unread = query.get("unread") === "true";
    return json({
      items: notifications.filter((n) => !unread || !n.readAt),
      unreadCount: notifications.filter((n) => !n.readAt).length,
    });
  });
  route("POST", "/notifications/read-all", () => {
    for (const n of notifications) n.readAt ??= at(0);
    return noContent();
  });
  route("POST", "/notifications/:id/read", ({ params }) => {
    const n = notifications.find((x) => x.id === Number(params.id));
    if (n) n.readAt ??= at(0);
    return noContent();
  });
  route("DELETE", "/notifications/:id", ({ params }) => {
    const i = notifications.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) notifications.splice(i, 1);
    return noContent();
  });
  route("GET", "/audit", () => json({ items: audit }));
  route("GET", "/agents", () => json(agents));
}
