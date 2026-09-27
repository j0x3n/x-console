// B8：用真实服务端和 Linux 代理跑浏览器主流程。运行前先 npm ci、安装 Chromium。
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import net from "node:net";
import os from "node:os";
import { join, resolve } from "node:path";
import { chromium } from "playwright-core";

const webDir = resolve(import.meta.dirname, "..");
const backendDir = resolve(webDir, "../backend");
const artifacts = join(webDir, "e2e-artifacts");
const temp = mkdtempSync(join(os.tmpdir(), "xc-e2e-"));
const binary = (name) => join(temp, `${name}${process.platform === "win32" ? ".exe" : ""}`);
const children = [];
const logs = new Map();
let browser;
let page;
let stage = "启动";

function start(name, command, args, options = {}) {
  const child = spawn(command, args, {
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  });
  children.push(child);
  let output = "";
  const collect = (data) => {
    output += data.toString();
    logs.set(name, output);
    if (output.length > 200_000) output = output.slice(-100_000);
  };
  child.stdout.on("data", collect);
  child.stderr.on("data", collect);
  child.on("error", (error) => collect(`\n${error.stack}\n`));
  child.on("exit", (code) => {
    if (code !== null && code !== 0) collect(`\nexit code: ${code}\n`);
  });
  return child;
}

async function run(name, command, args, options = {}) {
  const child = start(name, command, args, options);
  const code = await new Promise((resolveCode, reject) => {
    child.once("error", reject);
    child.once("exit", resolveCode);
  });
  if (code !== 0) throw new Error(`${name} 失败 (${code})\n${logs.get(name) ?? ""}`);
}

async function freePort() {
  const server = net.createServer();
  await new Promise((resolveReady) => server.listen(0, "127.0.0.1", resolveReady));
  const port = server.address().port;
  await new Promise((resolveClosed) => server.close(resolveClosed));
  return port;
}

async function until(label, check, timeout = 30_000) {
  const deadline = Date.now() + timeout;
  let last;
  while (Date.now() < deadline) {
    try {
      const value = await check();
      if (value) return value;
    } catch (error) {
      last = error;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 250));
  }
  throw new Error(`等待${label}超时${last ? `: ${last.message}` : ""}`);
}

async function api(path) {
  const response = await page.context().request.get(`${base}/api/v1${path}`);
  assert.equal(response.status(), 200, `${path}: ${await response.text()}`);
  return response.json();
}

function dialog(title) {
  return page.getByRole("dialog", { name: title });
}

async function verifyIfAsked() {
  const prompt = dialog("再次验证");
  await prompt.waitFor({ state: "visible", timeout: 3000 }).catch(() => {});
  if (await prompt.isVisible()) {
    await prompt.getByLabel("密码").fill(password);
    await prompt.getByRole("button", { name: "验证" }).click();
    await prompt.waitFor({ state: "hidden" });
  }
}

const username = "e2e";
const password = `E2e-${crypto.randomBytes(12).toString("hex")}`;
let base;

try {
  rmSync(artifacts, { recursive: true, force: true });
  mkdirSync(artifacts, { recursive: true });
  stage = "编译服务端和代理";
  await run("build-server", "go", ["build", "-o", binary("server"), "./cmd/server"], { cwd: backendDir });
  await run("build-agent", "go", ["build", "-o", binary("agent"), "./cmd/agent"], { cwd: backendDir });
  if (process.env.XC_E2E_USE_BUILD !== "1")
    await run("build-web", process.execPath, [join(webDir, "node_modules/vite/bin/vite.js"), "build"], { cwd: webDir });

  const apiPort = await freePort();
  const serverUrl = `http://127.0.0.1:${apiPort}`;
  base = serverUrl;
  const serverEnv = {
    ...process.env,
    XC_ADDR: `127.0.0.1:${apiPort}`,
    XC_DATA_DIR: join(temp, "data"),
    XC_WEB_DIR: join(webDir, "dist"),
    XC_MASTER_KEY: crypto.randomBytes(32).toString("base64"),
    XC_DEV: "1",
  };
  stage = "启动服务";
  start("server", binary("server"), [], { cwd: backendDir, env: serverEnv });
  await until("服务端", async () => (await fetch(`${serverUrl}/api/v1/auth/status`)).ok);
  await until("前端", async () => (await fetch(base)).ok);

  browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1360, height: 860 } });
  await context.addInitScript(() => localStorage.setItem("xc.demo.full", "off"));
  page = await context.newPage();
  page.setDefaultTimeout(10_000);
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));

  stage = "初始化账号";
  await page.goto(base);
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel(/^密码/).fill(password);
  await page.getByLabel("再输一次密码").fill(password);
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("button", { name: "跳过，以后再开" }).click();
  await page.locator(".sidebar").waitFor();
  assert.equal((await api("/auth/status")).authenticated, true);

  stage = "退出和登录";
  await page.locator("button.profile").click();
  await page.getByRole("menuitem", { name: "退出登录" }).click();
  await until("退出登录", async () => !(await api("/auth/status")).authenticated);
  await page.reload();
  await page.getByRole("button", { name: "登录" }).waitFor();
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill(password);
  await page.getByRole("button", { name: "登录" }).click();
  await page.locator(".sidebar").waitFor();

  stage = "新建项目和 Issue";
  await page.goto(`${base}/projects`);
  await page.getByRole("button", { name: "新建项目" }).click();
  await dialog("新建项目").getByLabel("名称").fill("端到端项目");
  await dialog("新建项目").getByLabel("Key").fill("EET");
  await dialog("新建项目").getByRole("button", { name: "创建项目" }).click();
  await page.waitForURL(/\/projects\/EET$/);
  const projects = await api("/projects");
  const project = projects.find((item) => item.key === "EET");
  assert.ok(project, "真实接口里没有新建的项目");
  await page.goto(`${base}/projects?new=1`);
  await dialog("新建 Issue").getByRole("textbox", { name: "标题" }).fill("端到端 Issue");
  await dialog("新建 Issue").getByRole("button", { name: "创建 Issue" }).click();
  await page.waitForURL(/\/projects\/EET\/\d+$/);
  const issueKey = `EET-${page.url().split("/").at(-1)}`;
  assert.equal((await api(`/issues/${issueKey}`)).title, "端到端 Issue");

  stage = "写笔记";
  const noteResponses = [];
  page.on("request", (request) => {
    if (request.method() === "PATCH" && request.url().includes("/api/v1/notes/"))
      noteResponses.push(`started ${request.url()}`);
  });
  page.on("response", (response) => {
    if (response.request().method() === "PATCH" && response.url().includes("/api/v1/notes/"))
      noteResponses.push(`${response.status()} ${response.url()}`);
  });
  page.on("requestfailed", (request) => {
    if (request.method() === "PATCH" && request.url().includes("/api/v1/notes/"))
      noteResponses.push(`failed ${request.failure()?.errorText}`);
  });
  await page.goto(`${base}/notes`);
  await page.locator(".notes-list-head button").click();
  await page.waitForURL(/\/notes\/\d+/);
  const noteId = page.url().match(/\/notes\/(\d+)/)[1];
  await page.getByRole("textbox", { name: "标题" }).fill("端到端笔记");
  await page.getByRole("textbox", { name: "笔记", exact: true }).fill("这是写入真实数据库的内容。");
  let currentNote;
  try {
    await page.getByText("已保存", { exact: true }).waitFor({ timeout: 10_000 });
    await until("笔记自动保存", async () => {
      currentNote = await api(`/notes/${noteId}`);
      return currentNote.title === "端到端笔记" && currentNote.body === "这是写入真实数据库的内容。";
    }, 5_000);
  } catch (error) {
    throw new Error(`${error.message}\n当前笔记：${JSON.stringify(currentNote)}\n请求：${noteResponses.join(", ")}\n页面异常：${pageErrors.join("；")}`);
  }

  stage = "新建提醒";
  await page.goto(`${base}/reminders`);
  await page.getByRole("button", { name: "新建提醒" }).click();
  await dialog("新建提醒").getByRole("textbox", { name: "标题" }).fill("端到端提醒");
  await dialog("新建提醒").getByRole("button", { name: "保存" }).click();
  await dialog("新建提醒").waitFor({ state: "hidden" });
  const reminders = [
    ...(await api("/reminders?range=today")).items,
    ...(await api("/reminders?range=upcoming")).items,
  ];
  const reminder = reminders.find((item) => item.title === "端到端提醒");
  assert.ok(reminder, "真实接口里没有新建的提醒");

  stage = "手机云盘上传、预览和删除";
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/drive`);
  await page.locator('[data-testid="drive-file-input"]').setInputFiles({
    name: "端到端文件.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("云盘内容可以预览。"),
  });
  const driveFile = await until("云盘上传", async () =>
    (await api("/drive/items")).items.find((item) => item.name === "端到端文件.txt"),
  );
  const driveRow = page.locator(".drive-row").filter({ hasText: "端到端文件.txt" });
  await driveRow.getByRole("button", { name: "端到端文件.txt", exact: true }).click();
  await dialog("端到端文件.txt").getByText("云盘内容可以预览。").waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "手机云盘横向溢出");
  await dialog("端到端文件.txt").getByRole("button", { name: "关闭" }).click();
  await page.screenshot({ path: join(artifacts, "drive-390.png"), fullPage: true });
  await page.setViewportSize({ width: 1360, height: 860 });
  await page.screenshot({ path: join(artifacts, "drive-1360.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await driveRow.getByRole("button", { name: /更多.*端到端文件\.txt/ }).click();
  await page.getByRole("menuitem", { name: "删除" }).click();
  await until("云盘删除", async () =>
    !(await api("/drive/items")).items.some((item) => item.id === driveFile.id),
  );
  await page.setViewportSize({ width: 1360, height: 860 });

  stage = "习惯打卡";
  await page.goto(`${base}/habits`);
  await page.getByRole("button", { name: "新建习惯" }).click();
  await dialog("新建习惯").getByLabel("名称").fill("端到端习惯");
  await dialog("新建习惯").getByRole("button", { name: "保存" }).click();
  await dialog("新建习惯").waitFor({ state: "hidden" });
  const habitCard = page.locator(".habits-card").filter({ hasText: "端到端习惯" });
  await habitCard.getByRole("button", { name: "1 次" }).click();
  await until("习惯打卡", async () => {
    const habit = (await api("/habits/today")).find((item) => item.habit.name === "端到端习惯");
    return habit?.reached && habit.logs.length === 1;
  });

  stage = "配对 Linux 代理";
  await page.goto(`${base}/settings/devices`);
  await page.getByRole("button", { name: "配对新设备" }).click();
  await dialog("配对新设备").getByLabel("设备名称").fill("e2e-linux");
  await dialog("配对新设备").getByRole("button", { name: "生成配对码" }).click();
  await verifyIfAsked();
  const code = (await dialog("配对新设备").locator("code.xc-secret").first().textContent()).trim();
  const agentConfig = join(temp, "agent.json");
  await run("pair-agent", binary("agent"), ["pair", "--server", serverUrl, "--code", code, "--config", agentConfig]);
  start("agent", binary("agent"), ["run", "--config", agentConfig]);
  const host = await until("代理上线", async () => (await api("/hosts")).find((item) => item.name === "e2e-linux" && item.online));

  stage = "查看服务器并打开终端";
  await page.goto(`${base}/servers`);
  await page.locator(".servers-card").filter({ hasText: "e2e-linux" }).getByText("在线").waitFor();
  await page.goto(`${base}/servers/${host.id}/terminal`);
  await page.getByRole("button", { name: "连接", exact: true }).click();
  await verifyIfAsked();
  await page.locator(".servers-terminal-card .xc-badge.ok").getByText("已连接").waitFor();
  assert.deepEqual(pageErrors, [], `浏览器异常：${pageErrors.join("；")}`);

  console.log("B8 端到端主流程通过");
} catch (error) {
  console.error(`B8 失败于「${stage}」:`, error);
  if (page) {
    await page.screenshot({ path: join(artifacts, "failure.png"), fullPage: true }).catch(() => {});
    writeFileSync(join(artifacts, "url.txt"), page.url());
  }
  writeFileSync(join(artifacts, "failure.txt"), `${stage}\n${error.stack ?? error}\n`);
  for (const [name, output] of logs) writeFileSync(join(artifacts, `${name}.log`), output);
  process.exitCode = 1;
} finally {
  if (browser) await browser.close().catch(() => {});
  await Promise.all(children.reverse().map(async (child) => {
    if (child.exitCode !== null) return;
    const exited = new Promise((resolveExit) => child.once("exit", resolveExit));
    child.kill();
    await Promise.race([exited, new Promise((resolveWait) => setTimeout(resolveWait, 3000))]);
  }));
  rmSync(temp, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
}
