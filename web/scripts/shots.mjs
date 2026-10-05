// 页面截图和自查（docs/07-design.md 第七节）。
//
// 用法：先起一个干净的本地环境，再跑脚本。
//   cd backend && export XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=$(mktemp -d) && go run ./cmd/server
//   cd web && npm run dev
//   cd web && npm run shots                 # 所有页面
//   cd web && npm run shots -- --only /drive,/notes
//
// 第一次用要装浏览器：npx playwright-core install chromium
// 已经装好的浏览器可以用 XC_SHOTS_BROWSER=/path/to/chromium 指定。
// 服务端已经有账号时，用 XC_SHOTS_USER、XC_SHOTS_PASSWORD、XC_SHOTS_TOTP（两步验证的密钥）登录。
//
// 结果：web/screenshots/<宽度>/<页面>.png。有横向溢出或页面报错时退出码是 1。
import { chromium } from "playwright-core";
import crypto from "node:crypto";
import { mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";

const args = process.argv.slice(2);
const arg = (name, fallback) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : fallback;
};
const BASE = arg("base", "http://127.0.0.1:5173");
const ONLY = arg("only", "")?.split(",").filter(Boolean) ?? [];
const THEME = arg("theme", "");
if (THEME && !["dark", "light"].includes(THEME)) throw new Error("theme 只能是 dark 或 light");
// 主题色（B22，B98 改过）：indigo、ocean、teal、violet、rose、graphite。深浅主题都生效。
const ACCENT = arg("accent", "");
const WIDTHS = arg("widths", "1360,390").split(",").map(Number);
const OUT = join(import.meta.dirname, "..", "screenshots", THEME || "", ACCENT || "");
const USER = process.env.XC_SHOTS_USER ?? "demo";
const PASSWORD = process.env.XC_SHOTS_PASSWORD ?? "demo-password-123";
let totpSecret = process.env.XC_SHOTS_TOTP ?? "";

function totp(secret) {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const c of secret.replace(/=+$/, "").toUpperCase())
    bits += alphabet.indexOf(c).toString(2).padStart(5, "0");
  const key = Buffer.from(
    bits.match(/.{8}/g).map((b) => parseInt(b, 2)),
  );
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)));
  const h = crypto.createHmac("sha1", key).update(counter).digest();
  const o = h[19] & 15;
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1e6).padStart(6, "0");
}

const browser = await chromium.launch(
  process.env.XC_SHOTS_BROWSER ? { executablePath: process.env.XC_SHOTS_BROWSER } : {},
);

// ---- 登录（没有账号时先建一个）----
const ctx = await browser.newContext({ viewport: { width: 1360, height: 860 } });
// Explicit screenshot options must survive the server preference sync.
async function applyScreenshotPreferences(context) {
  if (!THEME && !ACCENT) return;
  await context.route("**/api/v1/me/preferences", async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    if (!response.ok()) return route.fulfill({ response });
    const preferences = await response.json();
    if (THEME) preferences.nightMode = THEME === "dark" ? "on" : "off";
    if (ACCENT) preferences.accent = ACCENT;
    await route.fulfill({ response, json: preferences });
  });
}
await applyScreenshotPreferences(ctx);
if (THEME)
  await ctx.addInitScript((theme) => {
    localStorage.setItem("x-console-theme", theme);
  }, THEME);
if (ACCENT)
  await ctx.addInitScript((accent) => {
    localStorage.setItem("x-console-accent", accent);
  }, ACCENT);
const page = await ctx.newPage();
await page.goto(BASE);
const status = await page.evaluate(() => fetch("/api/v1/auth/status").then((r) => r.json()));
if (status.setupRequired) {
  await page.getByLabel("用户名").fill(USER);
  await page.getByLabel(/^密码/).fill(PASSWORD);
  await page.getByLabel("再输一次密码").fill(PASSWORD);
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("button", { name: "跳过，以后再开" }).click();
} else if (!status.authenticated) {
  await page.getByLabel("用户名").fill(USER);
  await page.getByLabel("密码").fill(PASSWORD);
  await page.getByRole("button", { name: "登录" }).click();
  const code = page.getByLabel(/^两步验证码/);
  await code.waitFor({ timeout: 3000 }).catch(() => {});
  if (await code.count()) {
    if (!totpSecret) throw new Error("这个账号开了两步验证，请设置 XC_SHOTS_TOTP");
    await code.fill(totp(totpSecret));
    await page.getByRole("button", { name: "登录" }).click();
  }
}
await page.locator(".sidebar").waitFor();

// ---- 示例数据：没有项目时才造 ----
const ids = await page.evaluate(async () => {
  const h = { "Content-Type": "application/json", "X-Requested-With": "x-console" };
  const get = (u) => fetch("/api/v1" + u, { headers: h }).then((r) => (r.ok ? r.json() : null));
  const post = (u, b) =>
    fetch("/api/v1" + u, { method: "POST", headers: h, body: JSON.stringify(b) }).then((r) =>
      r.ok ? r.json() : null,
    );
  let projects = (await get("/projects")) ?? [];
  if (projects.length === 0) {
    const list = [
      ["XC", "X Console", "#e5793b"],
      ["HOME", "家里的网络", "#5cc98b"],
      ["BLOG", "博客改版", "#70b5f7"],
    ];
    for (const [key, name, color] of list) await post("/projects", { key, name, color });
    projects = (await get("/projects")) ?? [];
    const xc = projects.find((p) => p.key === "XC");
    // B101：两张带截止时间，视图页才有内容（一张已过期，一张今天到期）
    const dueIn = (hours) => new Date(Date.now() + hours * 3600e3).toISOString();
    for (const [title, dueAt] of [["登录页改版", dueIn(-48)], ["云盘上传进度", dueIn(1)], ["修复移动端溢出"], ["接入 S3"]])
      await post(`/projects/${xc.id}/issues`, dueAt ? { title, dueAt } : { title });
    for (const [i, title] of ["周会记录", "读书笔记", "家里网络拓扑", "旅行清单"].entries())
      await post("/notes", { title, body: `示例内容 ${i + 1}\n\n- 第一点\n- 第二点`, pinned: i === 0 });
    const at = (hours) => new Date(Date.now() + hours * 3600e3).toISOString();
    for (const [title, hours] of [["交电费", 3], ["组会", 26], ["体检", 50]])
      await post("/reminders", { title, at: at(hours) });
    await post("/habits", { name: "跑步", unit: "次", dailyTarget: 1 });
    await post("/habits", { name: "力量训练", kind: "workout", dailyTarget: 1 });
    await post("/workouts/logs", { durationMinutes: 30, items: [{ name: "深蹲" }] });
    await post("/habits/library/activate", { ids: ["words", "review", "protein", "eyes", "stand"] });
    const dateParts = Object.fromEntries(new Intl.DateTimeFormat("en", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date()).map(({ type, value }) => [type, value]));
    const today = `${dateParts.year}-${dateParts.month}-${dateParts.day}`;
    await post(`/habits/personal/days/${today}/check`, { id: "words", done: true });
    await fetch(`/api/v1/habits/personal/days/${today}`, { method: "PATCH", headers: h, body: JSON.stringify({ weight: "81.5", waist: "91", sleep: "7.5", steps: "6300", energy: "一般", back: "和平时相近", english: "Could you confirm the deadline?", food: "第一餐：鸡蛋、牛奶和燕麦。", sets: { "A1:0:0": true } }) });
    const profile = await get("/habits/personal/profile");
    const logs = Object.fromEntries([1, 3, 5].map((ago, i) => [new Date(Date.parse(`${today}T12:00:00Z`) - ago * 86400000).toISOString().slice(0, 10), { weight: String(81.7 + i * 0.2), sleep: "7.5", back: "和平时相近" }]));
    await post("/habits/personal/backup", { version: 1, profile, logs, checks: {}, sets: {} });
  }
  // B47 的示例 Agent，没有时才造。
  if (((await get("/ai-agents")) ?? []).length === 0) {
    await post("/ai-agents", {
      name: "后端开发",
      kind: "claude_code",
      avatar: "🛠️",
      color: "#2f7fd1",
      instructions: "你是后端开发，只改 backend 目录，做完前跑 go test。",
      monthlyBudgetUsd: 20,
    });
    await post("/ai-agents", { name: "前端", kind: "codex", avatar: "🚀", color: "#8a5cc7", model: "gpt-5-codex" });
    await post("/ai-agents", {
      name: "整理员",
      kind: "builtin",
      avatar: "🧹",
      color: "#3a9a5b",
      model: "1:gpt-5-mini",
      instructions: "把卡片拆成清单，不改代码。",
    });
  }
  const agents = (await get("/ai-agents")) ?? [];
  const notes = (await get("/notes"))?.items ?? [];
  const hosts = (await get("/hosts")) ?? [];
  return {
    agent: agents[0]?.id,
    project: projects[0]?.key,
    note: notes[0]?.id,
    host: hosts.find((x) => x.kind === "server")?.id,
  };
});

const routes = [
  ["today", "/"],
  ["mail", "/mail"],
  ["projects", "/projects"],
  ["projects-overdue", "/projects/views/overdue"],
  ["projects-mine", "/projects/views/mine"],
  ids.project && ["project", `/projects/${ids.project}`],
  ids.project && ["issue", `/projects/${ids.project}/1`],
  ["agents", "/coding"],
  ["coding", "/coding/tasks"],
  ids.agent && ["agent", `/coding/agents/${ids.agent}`],
  ["settings-git", "/settings/git"],
  ["notes", "/notes"],
  ids.note && ["note", `/notes/${ids.note}`],
  ["reminders", "/reminders"],
  ["habits", "/habits"],
  ["habits-fitness", "/habits/fitness"],
  ...["overview", "training", "daily", "food", "english", "records", "settings", "reference"].map((section) => [
    `habits-personal-${section}`, `/habits/plan?section=${section}`,
  ]),
  ["drive", "/drive"],
  ["calendar", "/calendar"],
  ["servers", "/servers"],
  ids.host && ["server", `/servers/${ids.host}`],
  ["pc", "/pc"],
  ["monitoring", "/monitoring"],
  ["home", "/home"],
  ["router", "/router"],
  ["automations", "/automations"],
  ["automation-new", "/automations/new"],
  ["coding-repos", "/coding/repos"],
  ["calendar-briefs", "/calendar/briefs"],
  ["calendar-focus", "/calendar/focus"],
  ["calendar-manage", "/calendar/calendars"],
  ["github", "/github"],
  ["settings", "/settings/security"],
  ["settings-ai", "/settings/assistant"],
  ["settings-ai-usage", "/settings/ai-usage"],
  ["settings-errors", "/settings/errors"],
  ["settings-remote", "/settings/remote"],
  ["settings-storage", "/settings/storage"],
  ["settings-backup", "/settings/backup"],
  ["settings-router", "/settings/router"],
].filter(Boolean).filter(([, path]) => ONLY.length === 0 || ONLY.includes(path));

// ---- 截图和检查 ----
rmSync(OUT, { recursive: true, force: true });
const cookies = await ctx.cookies();
const problems = [];
for (const width of WIDTHS) {
  mkdirSync(join(OUT, String(width)), { recursive: true });
  const c = await browser.newContext({ viewport: { width, height: 860 } });
  await applyScreenshotPreferences(c);
  if (THEME)
    await c.addInitScript((theme) => {
      localStorage.setItem("x-console-theme", theme);
    }, THEME);
  await c.addCookies(cookies);
  const p = await c.newPage();
  let errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  for (const [name, path] of routes) {
    errors = [];
    await p.goto(BASE + path);
    await p.waitForLoadState("networkidle").catch(() => {});
    await p.waitForTimeout(400);
    const overflow = await p.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    await p.screenshot({ path: join(OUT, String(width), `${name}.png`), fullPage: true });
    const bad = [];
    const actualTheme = await p.locator("html").getAttribute("data-theme");
    if (THEME && actualTheme !== THEME) bad.push(`主题不对：${actualTheme}`);
    if (overflow > 0) bad.push(`横向溢出 ${overflow}px`);
    // B44：手机宽度下检查字号。文字不小于 11px，输入框等于 16px。
    if (width <= 720) {
      const fonts = await p.evaluate(() => {
        const small = new Set();
        const inputs = new Set();
        const visible = (el) => {
          const r = el.getBoundingClientRect();
          return r.width > 0 && r.height > 0 && getComputedStyle(el).visibility !== "hidden";
        };
        const label = (el) =>
          `${el.tagName.toLowerCase()}${el.className && typeof el.className === "string" ? "." + el.className.trim().split(/\s+/).join(".") : ""}`;
        for (const el of document.querySelectorAll("#main *, .sidebar *")) {
          const own = [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent.trim());
          if (!own || !visible(el)) continue;
          const size = parseFloat(getComputedStyle(el).fontSize);
          // 0 是手机上故意藏起文字、只显示图标的按钮。
          if (size > 0 && size < 11) small.add(`${label(el)} ${size}px`);
        }
        for (const el of document.querySelectorAll("#main input:not([type=checkbox]):not([type=radio]):not([type=range]):not([type=color]), #main select, #main textarea")) {
          if (!visible(el)) continue;
          const size = parseFloat(getComputedStyle(el).fontSize);
          if (size !== 16) inputs.add(`${label(el)} ${size}px`);
        }
        return { small: [...small].slice(0, 5), inputs: [...inputs].slice(0, 5) };
      });
      if (fonts.small.length) bad.push(`文字小于 11px：${fonts.small.join("、")}`);
      if (fonts.inputs.length) bad.push(`输入框不是 16px：${fonts.inputs.join("、")}`);
    }
    if (errors.length) bad.push(`页面报错：${errors.join("；")}`);
    console.log(`${bad.length ? "✗" : "✓"} ${String(width).padStart(4)}  ${path.padEnd(28)} ${bad.join("，")}`);
    if (bad.length) problems.push(`${width} ${path}：${bad.join("，")}`);
  }
  await c.close();
}
await browser.close();

console.log(`\n截图在 ${OUT}`);
if (problems.length) {
  console.log(`\n有 ${problems.length} 个问题：\n${problems.join("\n")}`);
  process.exit(1);
}
console.log("没有横向溢出，也没有页面报错。别忘了自己看一遍截图。");
