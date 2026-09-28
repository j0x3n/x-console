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
const WIDTHS = arg("widths", "1360,390").split(",").map(Number);
const OUT = join(import.meta.dirname, "..", "screenshots");
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
const page = await ctx.newPage();
await page.goto(BASE);
const status = await page.evaluate(() => fetch("/api/v1/auth/status").then((r) => r.json()));
if (status.setupRequired) {
  await page.getByLabel("用户名").fill(USER);
  await page.getByLabel(/^密码/).fill(PASSWORD);
  await page.getByLabel("再输一次密码").fill(PASSWORD);
  await page.getByRole("button", { name: "下一步" }).click();
  totpSecret = (await page.locator("code.xc-secret").textContent()).trim();
  await page.getByLabel("验证码").fill(totp(totpSecret));
  await page.getByRole("button", { name: "开启并进入" }).click();
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
      ["XC", "X Console", "#cc7752"],
      ["HOME", "家里的网络", "#5cc98b"],
      ["BLOG", "博客改版", "#70b5f7"],
    ];
    for (const [key, name, color] of list) await post("/projects", { key, name, color });
    projects = (await get("/projects")) ?? [];
    const xc = projects.find((p) => p.key === "XC");
    for (const title of ["登录页改版", "云盘上传进度", "修复移动端溢出", "接入 S3"])
      await post(`/projects/${xc.id}/issues`, { title });
    for (const [i, title] of ["周会记录", "读书笔记", "家里网络拓扑", "旅行清单"].entries())
      await post("/notes", { title, body: `示例内容 ${i + 1}\n\n- 第一点\n- 第二点`, pinned: i === 0 });
    const at = (hours) => new Date(Date.now() + hours * 3600e3).toISOString();
    for (const [title, hours] of [["交电费", 3], ["组会", 26], ["体检", 50]])
      await post("/reminders", { title, at: at(hours) });
    await post("/habits", { name: "跑步", unit: "次", dailyTarget: 1 });
  }
  const notes = (await get("/notes"))?.items ?? [];
  const hosts = (await get("/hosts")) ?? [];
  return {
    project: projects[0]?.key,
    note: notes[0]?.id,
    host: hosts.find((x) => x.kind === "server")?.id,
  };
});

const routes = [
  ["today", "/"],
  ["projects", "/projects"],
  ids.project && ["project", `/projects/${ids.project}`],
  ids.project && ["issue", `/projects/${ids.project}/1`],
  ["coding", "/coding"],
  ["notes", "/notes"],
  ids.note && ["note", `/notes/${ids.note}`],
  ["reminders", "/reminders"],
  ["habits", "/habits"],
  ["drive", "/drive"],
  ["calendar", "/calendar"],
  ["servers", "/servers"],
  ids.host && ["server", `/servers/${ids.host}`],
  ["pc", "/pc"],
  ["monitoring", "/monitoring"],
  ["home", "/home"],
  ["automations", "/automations"],
  ["automation-new", "/automations/new"],
  ["coding-repos", "/coding/repos"],
  ["calendar-briefs", "/calendar/briefs"],
  ["calendar-focus", "/calendar/focus"],
  ["calendar-manage", "/calendar/calendars"],
  ["github", "/github"],
  ["settings", "/settings/general"],
  ["settings-security", "/settings/security"],
].filter(Boolean).filter(([, path]) => ONLY.length === 0 || ONLY.includes(path));

// ---- 截图和检查 ----
rmSync(OUT, { recursive: true, force: true });
const cookies = await ctx.cookies();
const problems = [];
for (const width of WIDTHS) {
  mkdirSync(join(OUT, String(width)), { recursive: true });
  const c = await browser.newContext({ viewport: { width, height: 860 } });
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
    if (overflow > 0) bad.push(`横向溢出 ${overflow}px`);
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
