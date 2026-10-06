// B8：用真实服务端和 Linux 代理跑浏览器主流程。运行前先 npm ci、安装 Chromium。
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, utimesSync, writeFileSync } from "node:fs";
import net from "node:net";
import http from "node:http";
import https from "node:https";
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
let weatherProxy;
let weatherTargetPort;
const weatherProxySockets = new Set();
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
  await run("build-fakedav", "go", ["build", "-o", binary("fakedav"), "./cmd/fakedav"], { cwd: backendDir });
  if (process.env.XC_E2E_USE_BUILD !== "1")
    await run("build-web", process.execPath, [join(webDir, "node_modules/vite/bin/vite.js"), "build"], { cwd: webDir });

  const apiPort = await freePort();
  // QWeather uses HTTPS. Trust only this local test certificate in the server.
  const weatherCert = join(temp, "qweather.pem");
  const weatherKey = join(temp, "qweather.key");
  await run("qweather-cert", "openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", weatherKey, "-out", weatherCert, "-days", "1", "-subj", "/CN=weather.e2e.test", "-addext", "subjectAltName=DNS:weather.e2e.test"]);
  // Route the test hostname to a local TLS server without DNS or privileged ports.
  weatherProxy = http.createServer((_request, response) => response.writeHead(502).end());
  weatherProxy.on("connection", (socket) => {
    weatherProxySockets.add(socket);
    socket.on("close", () => weatherProxySockets.delete(socket));
  });
  weatherProxy.on("connect", (request, socket, head) => {
    if (request.url !== "weather.e2e.test:443" || !weatherTargetPort) {
      socket.end("HTTP/1.1 502 Bad Gateway\r\n\r\n");
      return;
    }
    const upstream = net.connect(weatherTargetPort, "127.0.0.1", () => {
      socket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
      if (head.length) upstream.write(head);
      socket.pipe(upstream);
      upstream.pipe(socket);
    });
    upstream.on("error", () => socket.destroy());
    socket.on("error", () => upstream.destroy());
    socket.on("close", () => upstream.destroy());
  });
  await new Promise((ready) => weatherProxy.listen(0, "127.0.0.1", ready));
  const serverUrl = `http://127.0.0.1:${apiPort}`;
  base = serverUrl;
  const serverEnv = {
    ...process.env,
    XC_ADDR: `127.0.0.1:${apiPort}`,
    XC_DATA_DIR: join(temp, "data"),
    XC_WEB_DIR: join(webDir, "dist"),
    XC_MASTER_KEY: crypto.randomBytes(32).toString("base64"),
    XC_DEV: "1",
    SSL_CERT_FILE: weatherCert,
    HTTPS_PROXY: `http://127.0.0.1:${weatherProxy.address().port}`,
    NO_PROXY: "127.0.0.1,localhost",
  };
  stage = "启动服务";
  start("server", binary("server"), [], { cwd: backendDir, env: serverEnv });
  await until("服务端", async () => (await fetch(`${serverUrl}/api/v1/auth/status`)).ok);
  await until("前端", async () => (await fetch(base)).ok);

  // 本地浏览器和 playwright-core 版本不一致时，用 XC_SHOTS_BROWSER 指定 Chromium（和 shots 一样）。
  browser = await chromium.launch(
    process.env.XC_SHOTS_BROWSER ? { executablePath: process.env.XC_SHOTS_BROWSER } : {},
  );
  const context = await browser.newContext({ viewport: { width: 1360, height: 860 } });
  // 旧浏览器可能留有演示开关；正式页面仍应读写真实接口。
  await context.addInitScript(() => localStorage.setItem("xc.demo.full", "on"));
  page = await context.newPage();
  const eventFrames = { sent: [], received: [] };
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/events")) return;
    socket.on("framesent", ({ payload }) => {
      try { eventFrames.sent.push(JSON.parse(payload)); } catch { /* binary frame */ }
    });
    socket.on("framereceived", ({ payload }) => {
      try { eventFrames.received.push(JSON.parse(payload)); } catch { /* binary frame */ }
    });
  });
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
  assert.equal(await page.getByText(/演示数据：/).count(), 0);
  assert.equal((await api("/auth/status")).authenticated, true);

  stage = "退出和登录";
  await page.locator("button.nav-rail-avatar").click();
  await page.getByRole("menuitem", { name: "退出登录" }).click();
  await until("退出登录", async () => !(await api("/auth/status")).authenticated);
  await page.reload();
  await page.getByRole("button", { name: "登录" }).waitFor();
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill(password);
  await page.getByRole("button", { name: "登录" }).click();
  await page.locator(".sidebar").waitFor();

  stage = "B32 AI 供应商和浮窗";
  const aiFake = http.createServer(async (request, response) => {
    if (request.url === "/v1/models") {
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ data: [{ id: "e2e-model" }] }));
      return;
    }
    if (request.url === "/v1/chat/completions") {
      const parts = [];
      for await (const part of request) parts.push(part);
      const body = JSON.parse(Buffer.concat(parts).toString());
      if (body.stream !== true) {
        response.writeHead(200, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ id: "chat_note", object: "chat.completion", created: 1, model: "e2e-model", choices: [{ index: 0, message: { role: "assistant", content: JSON.stringify({ title: "端到端标题", tags: ["测试标签"] }) }, finish_reason: "stop" }], usage: { prompt_tokens: 20, completion_tokens: 10 } }));
        return;
      }
      response.writeHead(200, { "Content-Type": "text/event-stream" });
      response.write(`data: ${JSON.stringify({ id: "chat_e2e", object: "chat.completion.chunk", created: 1, model: "e2e-model", choices: [{ index: 0, delta: { content: "AI 已收到测试消息" } }] })}\n\n`);
      response.write(`data: ${JSON.stringify({ id: "chat_e2e", object: "chat.completion.chunk", created: 1, model: "e2e-model", choices: [], usage: { prompt_tokens: 4, completion_tokens: 5 } })}\n\n`);
      response.end("data: [DONE]\n\n");
      return;
    }
    response.writeHead(404).end();
  });
  await new Promise((ready) => aiFake.listen(0, "127.0.0.1", ready));
  const aiFakePort = aiFake.address().port;
  try {
    const elevated = await page.context().request.post(`${base}/api/v1/auth/elevate`, {
      headers: { "X-Requested-With": "x-console" }, data: { password },
    });
    assert.equal(elevated.status(), 200, await elevated.text());
    const created = await page.context().request.post(`${base}/api/v1/ai/providers`, {
      headers: { "X-Requested-With": "x-console" },
      data: { name: "端到端模型", baseUrl: `http://127.0.0.1:${aiFakePort}/v1` },
    });
    assert.equal(created.status(), 201, await created.text());
    const aiProvider = await created.json();
    const refreshed = await page.context().request.post(`${base}/api/v1/ai/providers/${aiProvider.id}/models`, {
      headers: { "X-Requested-With": "x-console" },
    });
    assert.equal(refreshed.status(), 200, await refreshed.text());
    const settings = await page.context().request.put(`${base}/api/v1/ai/model-settings`, {
      headers: { "X-Requested-With": "x-console" },
      data: { agent: { providerId: aiProvider.id, model: "e2e-model" }, fast: null, reasoningEffort: "off" },
    });
    assert.equal(settings.status(), 200, await settings.text());
    const conversation = await page.context().request.post(`${base}/api/v1/ai/conversations`, {
      headers: { "X-Requested-With": "x-console" }, data: {},
    });
    assert.equal(conversation.status(), 201, await conversation.text());
    const aiConversation = await conversation.json();
    const sent = await page.context().request.post(`${base}/api/v1/ai/conversations/${aiConversation.id}/messages`, {
      headers: { "X-Requested-With": "x-console" }, data: { text: "你好" },
    });
    assert.equal(sent.status(), 202, await sent.text());
    await until("AI 浮窗回复", async () => {
      const detail = await api(`/ai/conversations/${aiConversation.id}`);
      return !detail.running && detail.messages.some((message) => message.role === "assistant" && message.content.some((block) => block.text === "AI 已收到测试消息"));
    });
    assert.equal((await api("/ai/usage")).calls, 1);
    stage = "B32 笔记自动标题和标签";
    const aiNoteBody = "这是一篇用于端到端验证的笔记。".repeat(12);
    const noteResponse = await page.context().request.post(`${base}/api/v1/notes`, {
      headers: { "X-Requested-With": "x-console" }, data: { body: aiNoteBody },
    });
    assert.equal(noteResponse.status(), 201, await noteResponse.text());
    const aiNote = await noteResponse.json();
    await until("笔记自动标题和建议标签", async () => {
      const note = await api(`/notes/${aiNote.id}`);
      return note.title === "端到端标题" && note.suggestedTags?.[0] === "测试标签";
    }, 20_000);
    const removeNote = await page.context().request.delete(`${base}/api/v1/notes/${aiNote.id}`, {
      headers: { "X-Requested-With": "x-console" },
    });
    assert.equal(removeNote.status(), 204, await removeNote.text());
    const removeProvider = await page.context().request.delete(`${base}/api/v1/ai/providers/${aiProvider.id}`, {
      headers: { "X-Requested-With": "x-console" },
    });
    assert.equal(removeProvider.status(), 204, await removeProvider.text());
  } finally {
    await new Promise((done) => aiFake.close(done));
  }

  stage = "B65 路由器";
  // 假的 OpenWrt ubus：登录、系统信息、接口、网卡计数、DHCP 租约和 host hints。
  const ubusFake = http.createServer(async (request, response) => {
    let raw = "";
    for await (const chunk of request) raw += chunk;
    const { id, params } = JSON.parse(raw || "{}");
    const [, object, method] = params ?? [];
    const results = {
      "session.login": [0, { ubus_rpc_session: "e".repeat(32), expires: 300 }],
      "system.board": [0, { hostname: "OpenWrt", model: "端到端路由器", release: { description: "OpenWrt 24.10.0" } }],
      "system.info": [0, { uptime: 7200, load: [0, 0, 0], memory: { total: 268435456, available: 134217728 } }],
      "network.interface.dump": [0, { interface: [
        { interface: "lan", up: true, uptime: 7200, proto: "static", device: "br-lan", "ipv4-address": [{ address: "192.168.1.1", mask: 24 }] },
        { interface: "wan", up: true, uptime: 3600, proto: "dhcp", device: "eth1", "ipv4-address": [{ address: "100.64.9.9", mask: 32 }] },
      ] }],
      "network.device.status": [0, { statistics: { rx_bytes: 1000, tx_bytes: 100 } }],
      "luci-rpc.getDHCPLeases": [0, { dhcp_leases: [{ expires: 600, hostname: "端到端手机", macaddr: "aa:bb:cc:dd:ee:01", ipaddr: "192.168.1.50" }] }],
      "luci-rpc.getHostHints": [0, {}],
    };
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ jsonrpc: "2.0", id, result: results[`${object}.${method}`] ?? [3] }));
  });
  await new Promise((ready) => ubusFake.listen(0, "127.0.0.1", ready));
  try {
    const elevatedForRouter = await page.context().request.post(`${base}/api/v1/auth/elevate`, {
      headers: { "X-Requested-With": "x-console" }, data: { password },
    });
    assert.equal(elevatedForRouter.status(), 200, await elevatedForRouter.text());
    const routerSaved = await page.context().request.put(`${base}/api/v1/router/config`, {
      headers: { "X-Requested-With": "x-console" },
      data: { url: `http://127.0.0.1:${ubusFake.address().port}`, username: "xconsole", password: "e2e", mode: "direct" },
    });
    assert.equal(routerSaved.status(), 200, await routerSaved.text());
    await page.goto(`${base}/router`);
    await page.getByText("100.64.9.9", { exact: true }).waitFor();
    await page.getByText("端到端手机").waitFor();
    const routerCleared = await page.context().request.put(`${base}/api/v1/router/config`, {
      headers: { "X-Requested-With": "x-console" }, data: { url: "" },
    });
    assert.equal(routerCleared.status(), 200, await routerCleared.text());
  } finally {
    await new Promise((done) => ubusFake.close(done));
  }

  stage = "AI 额度";
  // B110、B111：添加一个 DeepSeek 账号。CI 连不上 DeepSeek 时卡片显示读取失败，
  // 连得上时显示余额，两种都算页面正常。
  const quotaElevate = await page.context().request.post(`${base}/api/v1/auth/elevate`, {
    headers: { "X-Requested-With": "x-console" }, data: { password },
  });
  assert.equal(quotaElevate.status(), 200, await quotaElevate.text());
  await page.goto(`${base}/quotas`);
  await page.getByText("还没有额度账号").waitFor();
  await page.getByRole("button", { name: "添加账号" }).first().click();
  await dialog("添加额度账号").getByLabel("服务").selectOption("deepseek");
  await dialog("添加额度账号").getByLabel("备注名").fill("端到端 DeepSeek");
  await dialog("添加额度账号").getByLabel("DeepSeek API Key").fill("sk-e2e-not-real");
  await dialog("添加额度账号").getByRole("button", { name: "保存" }).click();
  const quotaCard = page.getByRole("article", { name: "端到端 DeepSeek" });
  await quotaCard.waitFor();
  await quotaCard.getByText(/余额|读取失败|登录失效|正在读取/).first().waitFor();
  const quotaList = await page.context().request.get(`${base}/api/v1/quotas`);
  assert.ok(!(await quotaList.text()).includes("sk-e2e-not-real"), "接口不能返回 API Key");
  await page.context().request.delete(`${base}/api/v1/quotas/${(await quotaList.json()).items[0].id}`, {
    headers: { "X-Requested-With": "x-console" },
  });

  stage = "通知静音规则";
  // B113：加一条规则，设置 → 通知里能看到，点删除后消失。邮箱 99 不存在，所以显示成已删除的邮箱。
  const muteCreated = await page.context().request.post(`${base}/api/v1/notify/mutes`, {
    headers: { "X-Requested-With": "x-console" },
    data: { kindPattern: "mail.new", scope: "mail:99", target: "bark" },
  });
  assert.equal(muteCreated.status(), 201, await muteCreated.text());
  await page.goto(`${base}/settings/notifications`);
  await page.getByText("已删除的邮箱 → Bark").waitFor();
  await page.getByRole("button", { name: "删除 新邮件" }).click();
  await page.getByText("还没有静音规则").waitFor();

  stage = "新建项目和卡片";
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
  await dialog("新建卡片").getByRole("textbox", { name: "标题" }).fill("端到端 Issue");
  await dialog("新建卡片").getByRole("button", { name: "创建卡片" }).click();
  await page.waitForURL(/\/projects\/EET\/\d+$/);
  const issueKey = `EET-${page.url().split("/").at(-1)}`;
  assert.equal((await api(`/issues/${issueKey}`)).title, "端到端 Issue");
  stage = "B36 分类和截止时间";
  const categoryResponse = await page.context().request.post(`${base}/api/v1/projects/${project.id}/categories`, {
    headers: { "X-Requested-With": "x-console" }, data: { name: "后端" },
  });
  assert.equal(categoryResponse.status(), 201, await categoryResponse.text());
  const parentCategory = await categoryResponse.json();
  const childResponse = await page.context().request.post(`${base}/api/v1/projects/${project.id}/categories`, {
    headers: { "X-Requested-With": "x-console" }, data: { name: "服务器", parentId: parentCategory.id },
  });
  assert.equal(childResponse.status(), 201, await childResponse.text());
  const childCategory = await childResponse.json();
  const dueAt = new Date(Math.ceil(Date.now() / 60_000) * 60_000 + 60 * 60 * 1000).toISOString();
  const dueResponse = await page.context().request.patch(`${base}/api/v1/issues/${issueKey}`, {
    headers: { "X-Requested-With": "x-console" },
    data: { categoryId: childCategory.id, dueAt, dueRemind: "15m" },
  });
  assert.equal(dueResponse.status(), 200, await dueResponse.text());
  const dueIssue = await dueResponse.json();
  assert.equal(dueIssue.categoryId, childCategory.id);
  assert.equal(dueIssue.dueRemind, "15m");
  assert.equal(new Date(dueIssue.dueAt).getTime(), new Date(dueAt).getTime());
  // B46 起分类不在界面上显示，接口保留到下个版本。
  stage = "B36 检查清单";
  const checklistResponse = await page.context().request.post(`${base}/api/v1/issues/${issueKey}/checklists`, {
    headers: { "X-Requested-With": "x-console" }, data: { title: "端到端检查" },
  });
  assert.equal(checklistResponse.status(), 201, await checklistResponse.text());
  const checklist = await checklistResponse.json();
  const checklistItemResponse = await page.context().request.post(`${base}/api/v1/issues/${issueKey}/checklists/${checklist.id}/items`, {
    headers: { "X-Requested-With": "x-console" }, data: { text: "核对接口" },
  });
  assert.equal(checklistItemResponse.status(), 201, await checklistItemResponse.text());
  const checklistItem = await checklistItemResponse.json();
  const checkedResponse = await page.context().request.patch(`${base}/api/v1/issues/${issueKey}/checklist-items/${checklistItem.id}`, {
    headers: { "X-Requested-With": "x-console" }, data: { done: true },
  });
  assert.equal(checkedResponse.status(), 200, await checkedResponse.text());
  await until("检查清单进度", async () => (await api(`/issues/${issueKey}`)).checklistDone === 1);
  await page.reload();
  await page.getByText("核对接口").waitFor();
  stage = "B36 图片上传和归属";
  const sampleImage = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/nwAAAABJRU5ErkJggg==", "base64");
  const uploadImage = async (name) => {
    const response = await page.context().request.post(`${base}/api/v1/files?scope=projects`, {
      headers: { "X-Requested-With": "x-console" },
      multipart: { file: { name, mimeType: "image/png", buffer: sampleImage } },
    });
    assert.equal(response.status(), 201, await response.text());
    return response.json();
  };
  const issueImage = await uploadImage("issue.png");
  const commentImage = await uploadImage("comment.png");
  const imagePatch = await page.context().request.patch(`${base}/api/v1/issues/${issueKey}`, {
    headers: { "X-Requested-With": "x-console" },
    data: { description: `![截图](${issueImage.url})` },
  });
  assert.equal(imagePatch.status(), 200, await imagePatch.text());
  const imageComment = await page.context().request.post(`${base}/api/v1/issues/${issueKey}/comments`, {
    headers: { "X-Requested-With": "x-console" },
    data: { body: `![评论截图](${commentImage.url})` },
  });
  assert.equal(imageComment.status(), 201, await imageComment.text());
  await page.reload();
  await page.locator(`img[src*="/api/v1/files/${issueImage.id}"]`).waitFor();
  assert.equal((await api(`/issues/${issueKey}`)).description.includes(issueImage.url), true);
  assert.equal((await api(`/issues/${issueKey}/comments`)).some((item) => item.body.includes(commentImage.url)), true);
  assert.equal((await page.context().request.get(`${base}${issueImage.url}`)).status(), 200);
  const progressResponse = await page
    .context()
    .request.patch(`${base}/api/v1/issues/${issueKey}`, {
      headers: { "X-Requested-With": "x-console" },
      data: { status: "in_progress" },
    });
  assert.equal(progressResponse.status(), 200, await progressResponse.text());
  await page.goto(`${base}/projects`);
  const projectStats = page.locator('section.xc-stats[aria-label="项目"]');
  await until("项目概要", async () =>
    (await projectStats.locator(".xc-stat").count()) === 5,
  );
  const progressCard = projectStats.locator(".xc-stat").filter({ hasText: "正在处理的卡片" });
  assert.equal((await progressCard.locator(".xc-stat-value").textContent()).trim(), "1");

  stage = "B101 已过期视图";
  const overdueResponse = await page.context().request.post(`${base}/api/v1/projects/${project.id}/issues`, {
    headers: { "X-Requested-With": "x-console" },
    data: { title: "端到端过期卡片", dueAt: new Date(Date.now() - 2 * 86_400_000).toISOString() },
  });
  assert.equal(overdueResponse.status(), 201, await overdueResponse.text());
  await page.goto(`${base}/projects/views/overdue`);
  await page.locator(".projects-row").filter({ hasText: "端到端过期卡片" }).waitFor();
  assert.equal(await page.locator(".projects-row").filter({ hasText: "端到端 Issue" }).count(), 0);
  const overdueLink = page.locator(".nav-panel").getByRole("link", { name: /已过期/ });
  await until("左栏的已过期数量", async () => (await overdueLink.locator("small").textContent())?.trim() === "1");

  stage = "B46 新建看板、加卡片、拖到另一个列表、加清单";
  await page.goto(`${base}/projects/EET`);
  await page.getByRole("button", { name: "新建看板" }).click();
  await dialog("新建看板").getByLabel("名称").fill("端到端看板");
  await dialog("新建看板").getByRole("button", { name: "创建看板" }).click();
  await page.getByRole("tab", { name: /端到端看板/ }).waitFor();
  // 新看板的标签先出现，切过去要等页面拿到新看板
  await until("切到新看板", async () =>
    (await page.getByRole("tab", { name: /端到端看板/ }).getAttribute("aria-selected")) === "true",
  );
  const lanes = page.locator(".projects-board.lists > section.projects-lane[data-list-id]");
  await until("三个列表", async () => (await lanes.count()) === 3);
  await lanes.nth(0).getByRole("button", { name: "添加卡片" }).click();
  const cardInput = page.getByPlaceholder("卡片标题，可以直接粘贴图片");
  await cardInput.fill("看板里的卡片");
  await cardInput.press("Enter");
  const boardCard = lanes.nth(0).locator("[data-issue-key]").filter({ hasText: "看板里的卡片" });
  await boardCard.waitFor();
  await cardInput.press("Escape");
  const boardCardKey = await boardCard.getAttribute("data-issue-key");
  assert.equal((await api(`/issues/${boardCardKey}`)).status, "todo");
  await boardCard.dragTo(lanes.nth(1));
  await until("拖到进行中", async () => (await api(`/issues/${boardCardKey}`)).status === "in_progress");
  await lanes.nth(1).locator(`[data-issue-key="${boardCardKey}"]`).click();
  await page.waitForURL(/\/projects\/EET\/\d+$/);
  await page.getByRole("button", { name: "添加检查清单" }).click();
  await page.getByLabel("清单标题").fill("看板清单");
  await page.getByLabel("清单标题").press("Enter");
  await until("清单已建", async () => (await api(`/issues/${boardCardKey}/checklists`)).length === 1);

  stage = "B55 锁定看板结构后不能加列表";
  await page.goto(`${base}/projects/EET`);
  await page.getByRole("button", { name: "添加列表" }).waitFor();
  await page.getByRole("button", { name: "锁定看板结构" }).click();
  await page.getByRole("button", { name: "添加列表" }).waitFor({ state: "detached" });
  assert.equal(await page.getByRole("button", { name: "新建看板" }).count(), 0);
  await page.getByRole("button", { name: "解锁看板结构" }).click();
  await page.getByRole("button", { name: "添加列表" }).waitFor();

  stage = "B47 新建 Agent，把卡片分配给内置 Agent";
  await page.goto(`${base}/coding`);
  await page.getByRole("button", { name: "新建 Agent" }).first().click();
  await dialog("新建 Agent").getByLabel("名称").fill("端到端 Agent");
  await dialog("新建 Agent").getByRole("button", { name: "创建 Agent" }).click();
  await page.locator(".aiagent-card").filter({ hasText: "端到端 Agent" }).waitFor();
  const e2eAgents = await api("/ai-agents");
  assert.equal(e2eAgents.find((a) => a.name === "端到端 Agent")?.kind, "claude_code");
  // 内置 Agent 要选模型，端到端环境没有 AI 供应商，直接用接口建。
  const builtinResponse = await page.request.post(`${base}/api/v1/ai-agents`, {
    headers: { "X-Requested-With": "x-console" },
    data: { name: "端到端整理员", kind: "builtin", model: "1:none" },
  });
  assert.equal(builtinResponse.status(), 201, await builtinResponse.text());
  await page.goto(`${base}/projects/EET/${boardCardKey.split("-")[1]}`);
  await page.getByRole("button", { name: "分配给 Agent" }).click();
  await dialog("分配给 Agent").getByRole("radio", { name: /端到端整理员/ }).check();
  await dialog("分配给 Agent").getByRole("button", { name: "分配", exact: true }).click();
  // 没有 AI 供应商：Agent 先说开始，再说没做完。
  await until("Agent 的评论", async () => {
    const list = await api(`/issues/${boardCardKey}/comments`);
    return list.some((c) => c.author?.startsWith("agent:") && c.body.includes("开始处理")) &&
      list.some((c) => c.author?.startsWith("agent:") && c.body.includes("没做完"));
  });
  assert.ok((await api(`/issues/${boardCardKey}`)).members.some((m) => m.kind === "agent"));

  stage = "B86 Agent 执行记录和日志";
  const runs = await api(`/ai-agents/runs?issueKey=${boardCardKey}`);
  assert.equal(runs[0].status, "failed");
  const runEvents = await api(`/ai-agents/runs/${runs[0].id}/events`);
  assert.ok(runEvents.items.some((event) => event.kind === "error"));
  assert.equal((await api(`/ai-agents/runs/${runs[0].id}/events?after=${runEvents.lastSeq}`)).items.length, 0);

  stage = "B87 Agent 通知开关和待决定接口";
  const notifySettings = await api("/ai-agents/notify");
  assert.equal(notifySettings.received, false);
  assert.equal(notifySettings.decision, true);
  const changedNotify = { ...notifySettings, done: false };
  const notifyResponse = await page.request.put(`${base}/api/v1/ai-agents/notify`, {
    headers: { "X-Requested-With": "x-console" }, data: changedNotify,
  });
  assert.equal(notifyResponse.status(), 200);
  assert.equal((await api("/ai-agents/notify")).done, false);
  assert.deepEqual(await api("/ai-agents/decisions"), []);

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
  // B20 以后“新建笔记”在顶栏。
  await page.getByRole("button", { name: "新建笔记" }).first().click();
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

  stage = "B72 笔记外链分享";
  {
    const elevate = await page.context().request.post(`${base}/api/v1/auth/elevate`, {
      headers: { "X-Requested-With": "x-console" }, data: { password },
    });
    assert.equal(elevate.status(), 200, await elevate.text());
    const shared = await page.context().request.put(`${base}/api/v1/notes/${noteId}/share`, {
      headers: { "X-Requested-With": "x-console" }, data: { expiresIn: "7d", password: "e2e-note" },
    });
    assert.equal(shared.status(), 200, await shared.text());
    const share = await shared.json();
    const publicBase = `${base}/api/v1/public/notes/${share.token}`;
    assert.equal((await fetch(publicBase)).status, 401);
    const unlocked = await fetch(`${publicBase}/unlock`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: "e2e-note" }),
    });
    assert.equal(unlocked.status, 200);
    const access = (await unlocked.json()).access;
    const publicNote = await fetch(`${publicBase}?t=${encodeURIComponent(access)}`);
    assert.equal(publicNote.status, 200);
    assert.equal((await publicNote.json()).body, currentNote.body);
    assert.equal((await api(`/notes/${noteId}`)).shared, true);
    const stopped = await page.context().request.delete(`${base}/api/v1/notes/${noteId}/share`, {
      headers: { "X-Requested-With": "x-console" },
    });
    assert.equal(stopped.status(), 204, await stopped.text());
    assert.equal((await fetch(publicBase)).status, 404);
  }

  stage = "B73 便签 API 主流程";
  {
    const created = await page.context().request.post(`${base}/api/v1/notes`, {
      headers: { "X-Requested-With": "x-console" },
      data: { title: "便签主流程", body: "便签检索主流程", kind: "memo", tags: ["便签主流程"] },
    });
    assert.equal(created.status(), 201, await created.text());
    const memo = await created.json();
    assert.equal(memo.kind, "memo");
    const listed = await api("/notes?kind=memo&tag=" + encodeURIComponent("便签主流程"));
    assert.equal(listed.items[0].body, memo.body);
    assert.ok((await api("/notes/counts")).memos > 0);
    const converted = await page.context().request.patch(`${base}/api/v1/notes/${memo.id}`, {
      headers: { "X-Requested-With": "x-console" }, data: { kind: "note" },
    });
    assert.equal(converted.status(), 200, await converted.text());
    assert.equal((await converted.json()).kind, "note");
    assert.equal((await api("/notes?kind=memo&tag=" + encodeURIComponent("便签主流程"))).items.length, 0);
    const removed = await page.context().request.delete(`${base}/api/v1/notes/${memo.id}`, {
      headers: { "X-Requested-With": "x-console" },
    });
    assert.equal(removed.status(), 204, await removed.text());
  }

  stage = "B84 看板绑定仓库和同步 Issue API 主流程";
  {
    const gitFake = http.createServer((request, response) => {
      response.setHeader("Content-Type", "application/json");
      if (request.url === "/user") return response.end(JSON.stringify({ login: "e2e" }));
      if (request.url === "/repos/e2e/board") return response.end(JSON.stringify({ full_name: "e2e/board", html_url: "https://example.test/e2e/board", clone_url: "https://example.test/e2e/board.git", default_branch: "main" }));
      if (request.url.startsWith("/repos/e2e/board/issues?")) return response.end(JSON.stringify([{ number: 1, title: "同步来的卡片", body: "仓库正文", state: "open", html_url: "https://example.test/e2e/board/issues/1", labels: [{ name: "测试标签" }] }]));
      response.writeHead(404).end();
    });
    await new Promise((ready) => gitFake.listen(0, "127.0.0.1", ready));
    try {
      const send = async (method, path, data) => {
        const response = await page.request.fetch(`${base}/api/v1${path}`, { method, headers: { "X-Requested-With": "x-console" }, data });
        assert.ok(response.ok(), `${path}: ${await response.text()}`);
        return response.status() === 204 ? null : response.json();
      };
      await send("POST", "/auth/elevate", { password });
      const connection = await send("POST", "/git-connections", { name: "同步测试", kind: "github", baseUrl: `http://127.0.0.1:${gitFake.address().port}`, token: "e2e-token" });
      const board = (await api(`/projects/${project.id}/boards`))[0];
      const path = `/boards/${board.id}/repo`;
      const bound = await send("PUT", path, { connectionId: connection.connection.id, fullName: "e2e/board" });
      assert.equal(bound.repo.syncedCount, 1);
      await send("POST", `${path}/sync`);
      const synced = (await api(`/issues?boardId=${board.id}`)).items.filter((card) => card.externalId === "e2e/board#1");
      assert.equal(synced.length, 1);
      assert.equal(synced[0].externalUrl, "https://example.test/e2e/board/issues/1");
      await send("DELETE", path);
      assert.equal((await api(`/issues/${synced[0].key}`)).externalSource, "");
      await send("DELETE", `/issues/${synced[0].key}`);
      await send("DELETE", `/git-connections/${connection.connection.id}`);
    } finally {
      await new Promise((done) => gitFake.close(done));
    }
  }

  stage = "B90 和风地点搜索、选择和天气";
  {
    const geoQueries = [];
    const weatherFake = https.createServer({ key: readFileSync(weatherKey), cert: readFileSync(weatherCert) }, (request, response) => {
      assert.equal(request.headers["x-qw-api-key"], "e2e-weather-key");
      const url = new URL(request.url, "https://weather.e2e.test");
      response.setHeader("Content-Type", "application/json");
      if (url.pathname === "/geo/v2/city/lookup") {
        geoQueries.push(url.searchParams.get("location"));
        return response.end(JSON.stringify({ code: "200", location: [{ id: "101191002", name: "东海", adm1: "江苏省", adm2: "连云港市", country: "中国", lat: "34.5225", lon: "118.7666" }] }));
      }
      if (url.pathname === "/v7/weather/now") {
        assert.equal(url.searchParams.get("location"), "101191002");
        return response.end(JSON.stringify({ code: "200", now: { temp: "21", humidity: "88", text: "小雨", icon: "305" } }));
      }
      if (url.pathname === "/v7/weather/3d") return response.end(JSON.stringify({ code: "200", daily: [{ tempMax: "25", tempMin: "17", pop: "75", sunrise: "00:00", sunset: "23:59" }] }));
      if (url.pathname === "/airquality/v1/current/34.52/118.77") return response.end(JSON.stringify({ indexes: [{ code: "cn-mee", aqi: 40, level: "1", category: "优" }] }));
      response.writeHead(500).end();
    });
    await new Promise((ready, reject) => {
      weatherFake.once("error", reject);
      weatherFake.listen(0, "127.0.0.1", ready);
    });
    weatherTargetPort = weatherFake.address().port;
    const original = await api("/briefs/settings");
    delete original.aiAvailable;
    delete original.availableChannels;
    delete original.nextRunAt;
    try {
      const configured = await page.request.put(`${base}/api/v1/weather/qweather`, {
        headers: { "X-Requested-With": "x-console" }, data: { apiHost: "weather.e2e.test", apiKey: "e2e-weather-key" },
      });
      assert.equal(configured.status(), 200, await configured.text());
      await page.goto(base);
      await page.locator(".today-weather-strip").click();
      if (!(await dialog("天气设置").isVisible())) await dialog("天气").getByRole("button", { name: "天气设置" }).click();
      const settingsDialog = dialog("天气设置");
      await settingsDialog.getByLabel("搜索城市").fill("东海");
      await settingsDialog.getByRole("button", { name: "搜索", exact: true }).click();
      await settingsDialog.getByRole("button", { name: /东海.*连云港市.*江苏省/ }).click();
      await page.context().grantPermissions(["geolocation"]);
      await page.context().setGeolocation({ latitude: 34.54, longitude: 118.75 });
      await settingsDialog.getByRole("button", { name: "用当前位置" }).click();
      await until("和风反查当前位置", () => geoQueries.includes("118.75,34.54"));
      await until("定位完成", () => settingsDialog.getByRole("button", { name: "用当前位置" }).isEnabled());
      await settingsDialog.getByRole("button", { name: "保存", exact: true }).click();
      await settingsDialog.waitFor({ state: "hidden" });
      const selected = await api("/briefs/settings");
      assert.equal(selected.location.id, "101191002");
      assert.equal(selected.location.lat, 34.5225);
      assert.equal(selected.location.lon, 118.7666);
      const weather = await api("/weather");
      assert.equal(weather.source, "qweather");
      assert.equal(weather.humidity, 88);
      assert.equal(weather.weatherCode, 61);
      assert.equal(weather.isDay, true);
      const extra = await api("/weather/extra");
      assert.equal(extra.air.aqi, 40);
      // The same search endpoint reverse-geocodes browser coordinates.
      const nearby = await api("/weather/places?q=118.75,34.54");
      assert.equal(nearby[0].id, "101191002");
      for (const width of [1360, 390]) {
        await page.setViewportSize({ width, height: 860 });
        await page.locator(".today-weather-strip").click();
        await dialog("天气").getByRole("button", { name: "天气设置" }).click();
        await page.waitForTimeout(250); // Wait for the dialog's entrance animation.
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
        await page.screenshot({ path: join(artifacts, `weather-settings-${width}.png`), fullPage: true, animations: "disabled" });
        await dialog("天气设置").getByRole("button", { name: "取消", exact: true }).click();
      }
      await page.setViewportSize({ width: 1360, height: 860 });
    } finally {
      await page.request.put(`${base}/api/v1/briefs/settings`, { headers: { "X-Requested-With": "x-console" }, data: original });
      await page.request.put(`${base}/api/v1/weather/qweather`, { headers: { "X-Requested-With": "x-console" }, data: { apiHost: "", clearKey: true } });
      await new Promise((done) => weatherFake.close(done));
    }
  }
  stage = "B70 仓库关注 API 主流程";
  {
    const original = await api("/github/config");
    const saved = await page.context().request.put(`${base}/api/v1/github/config`, {
      headers: { "X-Requested-With": "x-console" }, data: { watches: [] },
    });
    assert.equal(saved.status(), 200, await saved.text());
    assert.deepEqual((await saved.json()).watches, []);
    assert.deepEqual(await api("/github/repos"), []);
    assert.deepEqual(await api("/github/commits"), []);
    assert.deepEqual(await api("/github/pulls"), []);
    const restored = await page.context().request.put(`${base}/api/v1/github/config`, {
      headers: { "X-Requested-With": "x-console" }, data: { repos: original.repos, connectionId: original.connectionId ?? 0 },
    });
    assert.equal(restored.status(), 200, await restored.text());
  }

  stage = "B71 仓库通知设置 API 主流程";
  {
    const original = await api("/github/notify");
    const saved = await page.context().request.put(`${base}/api/v1/github/notify`, {
      headers: { "X-Requested-With": "x-console" },
      data: { defaults: { events: ["ci_started", "push"], ciBranches: "all" }, repos: [] },
    });
    assert.equal(saved.status(), 200, await saved.text());
    assert.deepEqual((await api("/github/notify")).defaults, { events: ["ci_started", "push"], ciBranches: "all" });
    const invalid = await page.context().request.put(`${base}/api/v1/github/notify`, {
      headers: { "X-Requested-With": "x-console" },
      data: { defaults: { events: ["unknown"], ciBranches: "default" }, repos: [] },
    });
    assert.equal(invalid.status(), 400, await invalid.text());
    const restored = await page.context().request.put(`${base}/api/v1/github/notify`, {
      headers: { "X-Requested-With": "x-console" }, data: original,
    });
    assert.equal(restored.status(), 200, await restored.text());
  }

  stage = "B74 笔记背景色 API 主流程";
  {
    const colored = await page.context().request.patch(`${base}/api/v1/notes/${noteId}`, {
      headers: { "X-Requested-With": "x-console" }, data: { color: "teal" },
    });
    assert.equal(colored.status(), 200, await colored.text());
    assert.equal((await api(`/notes/${noteId}`)).color, "teal");
    const invalid = await page.context().request.patch(`${base}/api/v1/notes/${noteId}`, {
      headers: { "X-Requested-With": "x-console" }, data: { color: "#ffffff" },
    });
    assert.equal(invalid.status(), 400, await invalid.text());
    const shared = await page.context().request.put(`${base}/api/v1/notes/${noteId}/share`, {
      headers: { "X-Requested-With": "x-console" }, data: { expiresIn: "1d" },
    });
    assert.equal(shared.status(), 200, await shared.text());
    const share = await shared.json();
    const publicNote = await fetch(`${base}/api/v1/public/notes/${share.token}`);
    assert.equal(publicNote.status, 200);
    assert.equal((await publicNote.json()).color, "teal");
    const stopped = await page.context().request.delete(`${base}/api/v1/notes/${noteId}/share`, {
      headers: { "X-Requested-With": "x-console" },
    });
    assert.equal(stopped.status(), 204, await stopped.text());
  }

  stage = "新建提醒";
  await page.goto(`${base}/reminders`);
  await page
    .locator('section.xc-stats[aria-label="提醒"] .xc-stat')
    .first()
    .waitFor();
  const reminderLabels = await page
    .locator('section.xc-stats[aria-label="提醒"] .xc-stat-top span:first-child')
    .allTextContents();
  assert.deepEqual(reminderLabels, ["今天", "下一个提醒", "即将到来", "已完成"]);
  await page.getByRole("button", { name: "新建提醒" }).click();
  await dialog("新建提醒").getByRole("textbox", { name: "标题" }).fill("端到端提醒");
  await dialog("新建提醒").getByRole("button", { name: "选择图标" }).click();
  await dialog("新建提醒").getByRole("button", { name: "💧", exact: true }).click();
  await dialog("新建提醒").getByRole("button", { name: "保存" }).click();
  await dialog("新建提醒").waitFor({ state: "hidden" });
  const reminders = [
    ...(await api("/reminders?range=today")).items,
    ...(await api("/reminders?range=upcoming")).items,
  ];
  const reminder = reminders.find((item) => item.title === "端到端提醒");
  assert.ok(reminder, "真实接口里没有新建的提醒");
  assert.equal(reminder.icon, "💧");
  const invalidIcon = await page.request.get(`${base}/api/v1/notify/icons/emoji-1f4a7.png?sig=00`);
  assert.equal(invalidIcon.status(), 403);

  stage = "本地日历写入";
  const calendarResponse = await page
    .context()
    .request.post(`${base}/api/v1/calendars`, {
      headers: { "X-Requested-With": "x-console" },
      data: { name: "端到端日历", kind: "local", url: "" },
    });
  assert.equal(calendarResponse.status(), 201, await calendarResponse.text());
  const calendar = await calendarResponse.json();
  assert.equal(calendar.writable, true);
  const eventStart = new Date(Date.now() + 10 * 60_000);
  const eventEnd = new Date(eventStart.getTime() + 30 * 60_000);
  const eventResponse = await page
    .context()
    .request.post(`${base}/api/v1/calendar/events`, {
      headers: { "X-Requested-With": "x-console" },
      data: {
        calendarId: calendar.id,
        title: "端到端日程",
        allDay: false,
        start: eventStart.toISOString(),
        end: eventEnd.toISOString(),
      },
    });
  assert.equal(eventResponse.status(), 201, await eventResponse.text());
  const calendarEvent = await eventResponse.json();
  const dateKey = `${eventStart.getFullYear()}-${String(eventStart.getMonth() + 1).padStart(2, "0")}-${String(eventStart.getDate()).padStart(2, "0")}`;
  await page.goto(`${base}/calendar?view=day&date=${dateKey}`);
  await page.getByText("端到端日程").first().waitFor();
  const movedResponse = await page
    .context()
    .request.patch(`${base}/api/v1/calendar/events/${calendarEvent.eventId}`, {
      headers: { "X-Requested-With": "x-console" },
      data: { title: "端到端日程已修改" },
    });
  assert.equal(movedResponse.status(), 200, await movedResponse.text());
  await page.reload();
  await page.getByText("端到端日程已修改").first().waitFor();
  const deleteResponse = await page
    .context()
    .request.delete(`${base}/api/v1/calendar/events/${calendarEvent.eventId}`, {
      headers: { "X-Requested-With": "x-console" },
    });
  assert.equal(deleteResponse.status(), 204, await deleteResponse.text());

  stage = "自动化规则运行";
  const automationResponse = await page.context().request.post(`${base}/api/v1/automations`, {
    headers: { "X-Requested-With": "x-console" },
    data: {
      name: "端到端自动化",
      enabled: true,
      trigger: { type: "event", topic: "e2e.never" },
      conditions: [],
      actions: [{ action: "notify.send", input: { title: "端到端自动化通知" } }],
      cooldownSeconds: 0,
    },
  });
  assert.equal(automationResponse.status(), 201, await automationResponse.text());
  const automation = await automationResponse.json();
  await page.goto(`${base}/automations`);
  await page.getByText("端到端自动化").waitFor();
  const runResponse = await page.context().request.post(`${base}/api/v1/automations/${automation.id}/run`, {
    headers: { "X-Requested-With": "x-console" },
  });
  assert.equal(runResponse.status(), 202, await runResponse.text());
  await until("自动化执行", async () =>
    (await api(`/automations/${automation.id}/runs`))[0]?.status === "ok",
  );

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
  stage = "云盘历史版本";
  const savedVersion = await page.context().request.put(
    `${base}/api/v1/drive/items/${driveFile.id}/content`,
    {
      headers: { "X-Requested-With": "x-console", "Content-Type": "text/plain; charset=utf-8" },
      data: "历史版本测试",
    },
  );
  assert.equal(savedVersion.status(), 200, await savedVersion.text());
  const previousVersion = (await api(`/drive/items/${driveFile.id}/versions`)).items[0];
  assert.ok(previousVersion?.id);
  const restoredVersion = await page.context().request.post(
    `${base}/api/v1/drive/items/${driveFile.id}/versions/${previousVersion.id}/restore`,
    { headers: { "X-Requested-With": "x-console" } },
  );
  assert.equal(restoredVersion.status(), 200, await restoredVersion.text());
  const restoredContent = await page.context().request.get(
    `${base}/api/v1/drive/items/${driveFile.id}/content`,
  );
  assert.equal(await restoredContent.text(), "云盘内容可以预览。");
  stage = "云盘分享管理";
  const shareElevation = await page.context().request.post(`${base}/api/v1/auth/elevate`, {
    headers: { "X-Requested-With": "x-console" },
    data: { password },
  });
  assert.equal(shareElevation.status(), 200, await shareElevation.text());
  stage = "B75 云盘分享预览和下载记录 API 主流程";
  const shareResponse = await page.context().request.post(`${base}/api/v1/drive/shares`, {
    headers: { "X-Requested-With": "x-console" },
    data: { itemId: driveFile.id, expiresIn: "7d", code: "分享密码123456" },
  });
  assert.equal(shareResponse.status(), 201, await shareResponse.text());
  const driveShare = await shareResponse.json();
  assert.equal((await api(`/drive/shares?itemId=${driveFile.id}`)).items[0]?.id, driveShare.id);
  const drivePublicBase = `${base}/api/v1/public/shares/${driveShare.token}`;
  assert.equal((await fetch(drivePublicBase)).status, 401);
  const driveUnlocked = await fetch(`${drivePublicBase}/unlock`, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code: "分享密码123456" }),
  });
  assert.equal(driveUnlocked.status, 200);
  const driveAccess = encodeURIComponent((await driveUnlocked.json()).access);
  const publicShare = await page.context().request.get(`${drivePublicBase}?t=${driveAccess}`);
  assert.equal(publicShare.status(), 200, await publicShare.text());
  for (let i = 0; i < 3; i++) {
    const preview = await page.context().request.get(`${drivePublicBase}/content?preview=true&t=${driveAccess}`);
    assert.equal(preview.status(), 200, await preview.text());
    assert.equal(await preview.text(), "云盘内容可以预览。");
    assert.match(preview.headers()["content-disposition"], /^inline/);
  }
  const publicHead = await page.context().request.head(`${drivePublicBase}/content?t=${driveAccess}`);
  assert.equal(publicHead.status(), 200);
  assert.equal((await api(`/drive/shares?itemId=${driveFile.id}`)).items[0].downloads, 0);
  const publicContent = await page.context().request.get(`${drivePublicBase}/content?t=${driveAccess}`);
  assert.equal(publicContent.status(), 200);
  assert.equal(await publicContent.text(), "云盘内容可以预览。");
  const publicRange = await page.context().request.get(`${drivePublicBase}/content?t=${driveAccess}`, {
    headers: { Range: "bytes=3-" },
  });
  assert.equal(publicRange.status(), 206);
  assert.equal((await api(`/drive/shares?itemId=${driveFile.id}`)).items[0].downloads, 1);
  const driveHistory = await api(`/drive/shares/${driveShare.id}/downloads`);
  assert.equal(driveHistory.items.length, 1);
  assert.equal(driveHistory.items[0].ip, "127.0.*.*");
  assert.equal(driveHistory.items[0].itemName, driveFile.name);
  const unshareResponse = await page.context().request.delete(
    `${base}/api/v1/drive/shares/${driveShare.id}`,
    { headers: { "X-Requested-With": "x-console" } },
  );
  assert.equal(unshareResponse.status(), 204);
  stage = "云盘日志实时";
  await page.evaluate(
    ({ id, offset }) =>
      new Promise((resolve, reject) => {
        window.__driveFollowFrames = [];
        const socket = new WebSocket(
          `${location.origin.replace(/^http/, "ws")}/api/v1/drive/items/${id}/follow?offset=${offset}`,
        );
        window.__driveFollowSocket = socket;
        socket.onmessage = (event) => window.__driveFollowFrames.push(JSON.parse(event.data));
        socket.onopen = resolve;
        socket.onerror = reject;
      }),
    { id: driveFile.id, offset: Buffer.byteLength("云盘内容可以预览。") },
  );
  const appendLog = await page.context().request.put(
    `${base}/api/v1/drive/items/${driveFile.id}/content`,
    {
      headers: { "X-Requested-With": "x-console", "Content-Type": "text/plain; charset=utf-8" },
      data: "云盘内容可以预览。\n实时追加",
    },
  );
  assert.equal(appendLog.status(), 200, await appendLog.text());
  await until("云盘日志追加", async () =>
    page.evaluate(() => window.__driveFollowFrames.some((frame) => frame.type === "append" && frame.data === "\n实时追加")),
  );
  const resetLog = await page.context().request.put(
    `${base}/api/v1/drive/items/${driveFile.id}/content`,
    {
      headers: { "X-Requested-With": "x-console", "Content-Type": "text/plain; charset=utf-8" },
      data: "短",
    },
  );
  assert.equal(resetLog.status(), 200, await resetLog.text());
  await until("云盘日志重置", async () =>
    page.evaluate(() => window.__driveFollowFrames.some((frame) => frame.type === "reset")),
  );
  await page.evaluate(() => window.__driveFollowSocket.close());
  const restoreLog = await page.context().request.put(
    `${base}/api/v1/drive/items/${driveFile.id}/content`,
    {
      headers: { "X-Requested-With": "x-console", "Content-Type": "text/plain; charset=utf-8" },
      data: "云盘内容可以预览。",
    },
  );
  assert.equal(restoreLog.status(), 200, await restoreLog.text());
  stage = "云盘打包下载";
  const zipResponse = await page.context().request.get(`${base}/api/v1/drive/zip?ids=${driveFile.id}`);
  assert.equal(zipResponse.status(), 200);
  assert.equal(zipResponse.headers()["content-type"], "application/zip");
  assert.equal((await zipResponse.body()).subarray(0, 2).toString(), "PK");
  stage = "云盘批量复制和移动";
  const transferFolderResponse = await page.context().request.post(`${base}/api/v1/drive/folders`, {
    headers: { "X-Requested-With": "x-console" },
    data: { name: "端到端目标" },
  });
  assert.equal(transferFolderResponse.status(), 201, await transferFolderResponse.text());
  const transferFolder = await transferFolderResponse.json();
  const copyResponse = await page.context().request.post(`${base}/api/v1/drive/batch/copy`, {
    headers: { "X-Requested-With": "x-console" },
    data: { ids: [driveFile.id], targetId: transferFolder.id, conflict: "rename" },
  });
  assert.equal(copyResponse.status(), 202, await copyResponse.text());
  const copyTask = await copyResponse.json();
  await until("云盘复制", async () =>
    (await api("/drive/tasks")).items.find((task) => task.id === copyTask.id)?.state === "done",
  );
  const copiedFile = (await api(`/drive/items?parent=${transferFolder.id}`)).items.find(
    (item) => item.name === driveFile.name,
  );
  assert.ok(copiedFile && copiedFile.id !== driveFile.id);
  const movedFolderResponse = await page.context().request.post(`${base}/api/v1/drive/folders`, {
    headers: { "X-Requested-With": "x-console" },
    data: { name: "端到端移动目标" },
  });
  assert.equal(movedFolderResponse.status(), 201, await movedFolderResponse.text());
  const movedFolder = await movedFolderResponse.json();
  const moveResponse = await page.context().request.post(`${base}/api/v1/drive/batch/move`, {
    headers: { "X-Requested-With": "x-console" },
    data: { ids: [copiedFile.id], targetId: movedFolder.id, conflict: "rename" },
  });
  assert.equal(moveResponse.status(), 202, await moveResponse.text());
  const moveTask = await moveResponse.json();
  await until("云盘移动", async () =>
    (await api("/drive/tasks")).items.find((task) => task.id === moveTask.id)?.state === "done",
  );
  assert.equal((await api(`/drive/items?parent=${transferFolder.id}`)).items.length, 0);
  assert.equal((await api(`/drive/items?parent=${movedFolder.id}`)).items[0]?.id, copiedFile.id);
  stage = "云盘压缩";
  const archiveResponse = await page.context().request.post(`${base}/api/v1/drive/archive`, {
    headers: { "X-Requested-With": "x-console" },
    data: { ids: [driveFile.id], name: "端到端压缩", format: "zip", parentId: transferFolder.id },
  });
  assert.equal(archiveResponse.status(), 202, await archiveResponse.text());
  const archiveTask = await archiveResponse.json();
  const finishedArchive = await until("云盘压缩", async () =>
    (await api("/drive/tasks")).items.find((task) => task.id === archiveTask.id && task.state === "done"),
  );
  const archived = await page.context().request.get(
    `${base}/api/v1/drive/items/${finishedArchive.resultId}/content`,
  );
  assert.equal(archived.status(), 200);
  assert.equal((await archived.body()).subarray(0, 2).toString(), "PK");
  stage = "云盘解压";
  const extractResponse = await page.context().request.post(
    `${base}/api/v1/drive/items/${finishedArchive.resultId}/extract`,
    { headers: { "X-Requested-With": "x-console" } },
  );
  assert.equal(extractResponse.status(), 202, await extractResponse.text());
  const extractTask = await extractResponse.json();
  const finishedExtract = await until("云盘解压", async () =>
    (await api("/drive/tasks")).items.find((task) => task.id === extractTask.id && task.state === "done"),
  );
  assert.equal(
    (await api(`/drive/items?parent=${finishedExtract.resultId}`)).items[0]?.name,
    driveFile.name,
  );
  const finishedTasks = page.locator(".drive-tasks");
  if (await finishedTasks.isVisible())
    await finishedTasks.getByRole("button", { name: "关闭" }).click();
  const finishedUploads = page.locator(".drive-uploads:not(.drive-tasks)");
  if (await finishedUploads.isVisible())
    await finishedUploads.getByRole("button", { name: "关闭" }).click();
  stage = "手机云盘上传、预览和删除";
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
  // B21 以后删除要在确认框里再点一次。
  await dialog("删除“端到端文件.txt”？").getByRole("button", { name: "删除", exact: true }).click();
  await until("云盘删除", async () =>
    !(await api("/drive/items")).items.some((item) => item.id === driveFile.id),
  );
  await page.setViewportSize({ width: 1360, height: 860 });

  stage = "B69 在存储页加 WebDAV 账号，云盘二级菜单出现网盘";
  const davPort = await freePort();
  start("fakedav", binary("fakedav"), ["-addr", `127.0.0.1:${davPort}`, "-user", "me", "-password", "dav-pw"]);
  await until("WebDAV", async () => (await fetch(`http://127.0.0.1:${davPort}/dav/`)).status === 401);
  await page.goto(`${base}/settings/storage`);
  await page.getByRole("button", { name: "添加网盘账号" }).first().click();
  const davDialog = dialog("添加网盘账号");
  await davDialog.getByLabel("名称").fill("端到端网盘");
  await davDialog.getByLabel(/^地址/).fill(`http://127.0.0.1:${davPort}/dav/`);
  await davDialog.getByLabel("用户名").fill("me");
  await davDialog.getByLabel(/^密码/).fill("dav-pw");
  await davDialog.getByRole("button", { name: "保存" }).click();
  await verifyIfAsked();
  await davDialog.waitFor({ state: "hidden" });
  await page.locator(".storage-remote-row").filter({ hasText: "端到端网盘" }).waitFor();
  await page.goto(`${base}/drive`);
  // B104：网盘的入口在左栏二级菜单里
  await page.locator(".nav-panel").getByRole("link", { name: "端到端网盘" }).click();
  await page.getByRole("button", { name: "docs", exact: true }).click();
  await page.getByText("hello.txt").first().waitFor();

  const send = async (method, path, data) => {
    const response = await page.context().request.fetch(`${base}/api/v1${path}`, {
      method,
      headers: { "X-Requested-With": "x-console" },
      data,
    });
    assert.ok(response.status() < 300, `${method} ${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? undefined : response.json();
  };
  stage = "B81 增量备份 API 主流程";
  const previousBackupSettings = await api("/backups/settings");
  const backupRemote = (await api("/storage/remotes")).items.find((item) => item.name === "端到端网盘");
  await send("PUT", "/backups/settings", { target: "remote", remoteId: backupRemote.id, mode: "incremental", webdav: { folder: "e2e-backups" }, retention: { last: 7, daily: 14, weekly: 8, monthly: 12 } });
  const backupProbe = join(serverEnv.XC_DATA_DIR, "files", "projects", "backup-e2e.txt");
  mkdirSync(join(serverEnv.XC_DATA_DIR, "files", "projects"), { recursive: true });
  writeFileSync(backupProbe, "backup-e2e");
  await send("POST", "/backups/run");
  const firstBackupJob = await until("首个增量备份", async () => {
    const result = await api("/backups/job");
    return result.state !== "running" && result;
  });
  assert.equal(firstBackupJob.state, "done", firstBackupJob.error);
  rmSync(backupProbe);
  await send("POST", "/backups/run");
  const secondBackupJob = await until("第二个增量备份", async () => {
    const result = await api("/backups/job");
    return result.state !== "running" && result;
  });
  assert.equal(secondBackupJob.state, "done", secondBackupJob.error);
  const backupSnapshots = await api("/backups/snapshots");
  assert.equal(backupSnapshots.items.length, 2);
  assert.ok(backupSnapshots.stats.uniqueBytes > 0);
  const backupChanges = await api(`/backups/snapshots/${secondBackupJob.backupId}/changes`);
  assert.ok(backupChanges.items.some((item) => item.path === "projects/backup-e2e.txt" && item.kind === "deleted"));
  await send("POST", "/backups/check");
  const checkedBackup = await until("检查增量备份", async () => {
    const result = await api("/backups/job");
    return result.state !== "running" && result;
  });
  assert.equal(checkedBackup.state, "done", checkedBackup.error);
  assert.equal(checkedBackup.check.snapshots, 2);
  const { lastRuns: previousBackupRuns, nextRunAt: previousBackupNext, ...previousBackupInput } = previousBackupSettings;
  await send("PUT", "/backups/settings", previousBackupInput);

  stage = "B83 作息和健康提醒 API 主流程";
  const previousSchedule = await api("/habits/schedule");
  await send("PUT", "/habits/schedule", { workDays: [1, 2, 3, 4, 5], wakeTime: "12:00", sleepTime: "03:30", timezone: "Asia/Shanghai", idleMinutes: 5 });
  assert.equal((await api("/habits/schedule")).sleepTime, "03:30");
  const healthHabit = await send("POST", "/habits", { name: "端到端喝水", template: "water" });
  assert.deepEqual(healthHabit.remindWhen, ["awake"]);
  assert.equal(healthHabit.dailyTarget, 8);
  assert.ok(healthHabit.nextRemindAt);
  await send("POST", "/notify/actions", { actionId: `habit.snooze:${healthHabit.id}` });
  await send("POST", `/habits/${healthHabit.id}/checkin`, { amount: 1 });
  assert.equal((await api("/habits/today")).find((item) => item.habit.id === healthHabit.id).done, 1);
  assert.ok(Array.isArray(await api("/habits/presence")));
  await send("DELETE", `/habits/${healthHabit.id}`);
  await send("PUT", "/habits/schedule", previousSchedule);

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

  stage = "训练自动打卡";
  const workoutHabitResponse = await page.context().request.post(`${base}/api/v1/habits`, {
    headers: { "X-Requested-With": "x-console" },
    data: { name: "端到端健身", kind: "workout", dailyTarget: 1 },
  });
  assert.equal(workoutHabitResponse.status(), 201, await workoutHabitResponse.text());
  const workoutHabit = await workoutHabitResponse.json();
  assert.equal(workoutHabit.kind, "workout");
  const workoutLogResponse = await page.context().request.post(`${base}/api/v1/workouts/logs`, {
    headers: { "X-Requested-With": "x-console" },
    data: { durationMinutes: 20, items: [{ name: "端到端深蹲" }] },
  });
  assert.equal(workoutLogResponse.status(), 201, await workoutLogResponse.text());
  const workoutLog = await workoutLogResponse.json();
  await page.reload();
  const workoutCard = page.locator(".habits-card").filter({ hasText: "端到端健身" });
  await workoutCard.waitFor();
  assert.equal(await workoutCard.getByRole("button", { name: "撤销" }).count(), 0);
  assert.equal((await api("/habits/today")).find((item) => item.habit.id === workoutHabit.id).done, 1);
  const removeWorkoutResponse = await page.context().request.delete(`${base}/api/v1/workouts/logs/${workoutLog.id}`, {
    headers: { "X-Requested-With": "x-console" },
  });
  assert.equal(removeWorkoutResponse.status(), 204, await removeWorkoutResponse.text());
  assert.equal((await api("/habits/today")).find((item) => item.habit.id === workoutHabit.id).done, 0);

  stage = "B97 个人计划主流程";
  const library = await api("/habits/library");
  assert.equal(library.exercises.length, 66);
  assert.equal(library.articles.length, 26);
  const initialPersonalProfile = await api("/habits/personal/profile");
  await page.goto(`${base}/habits/plan?section=english&date=${initialPersonalProfile.start}`);
  await page.getByRole("button", { name: "全部加入习惯" }).click();
  const personalWords = library.habits.find((h) => h.id === "words");
  await page.getByRole("checkbox", { name: personalWords.name, exact: true }).click();
  const personalProfile = await until("个人计划打卡", async () => {
    const p = await api("/habits/personal/profile");
    return p.habitIds.words && (await api("/habits/today")).find((h) => h.habit.id === p.habitIds.words)?.done === 1 && p;
  });
  await page.reload();
  assert.equal(await page.getByRole("checkbox", { name: personalWords.name, exact: true }).isChecked(), true);
  await page.getByRole("checkbox", { name: personalWords.name, exact: true }).click();
  await until("撤销个人打卡", async () => (await api("/habits/today")).find((h) => h.habit.id === personalProfile.habitIds.words)?.done === 0);
  // 个人计划的栏目在左栏二级菜单里（2026-10-05）
  await page.locator(".nav-panel").getByRole("link", { name: "个人记录", exact: true }).click();
  await page.getByLabel("体重（公斤）", { exact: true }).fill("81.5");
  await page.getByRole("button", { name: "保存当天记录" }).click();
  const personalDate = await page.getByLabel("计划日期").inputValue();
  await until("身体记录持久化", async () => (await api(`/habits/personal/days/${personalDate}`)).weight === "81.5");
  await page.reload();
  assert.equal(await page.getByLabel("体重（公斤）", { exact: true }).inputValue(), "81.5");
  await page.locator(".nav-panel").getByRole("link", { name: "跟练", exact: true }).click();
  await page.getByLabel("训练模板").selectOption("A1");
  await page.locator(".habits-set-row input").first().click();
  await until("完成训练组", async () => (await api(`/habits/personal/days/${personalDate}`)).sets["A1:0:0"] === true);
  await page.getByRole("button", { name: "保存已完成训练" }).click();
  const personalWorkout = await until("保存跟练", async () => { const d = await api(`/habits/personal/days/${personalDate}`); return d.workoutLogId && d; });
  await page.getByRole("button", { name: "更新当天训练" }).click();
  await page.getByRole("button", { name: "更新当天训练" }).waitFor();
  assert.equal((await api(`/habits/personal/days/${personalDate}`)).workoutLogId, personalWorkout.workoutLogId);
  assert.equal((await api("/habits/today")).find((h) => h.habit.id === workoutHabit.id).done, 1);
  const savedPersonalWorkout = (await api("/workouts/logs?days=30")).find((w) => w.id === personalWorkout.workoutLogId);
  assert.ok(savedPersonalWorkout.items[0].exerciseId && savedPersonalWorkout.items[0].prescription);
  await page.goto(`${base}/habits/fitness?view=library`);
  const picked = library.exercises[0];
  await page.locator(".habits-fit-exercises").getByRole("button", { name: new RegExp(picked.name) }).click();
  await page.locator(".habits-fit-detail").getByRole("button", { name: "添加动作到周计划" }).click();
  await dialog("添加动作到周计划").getByRole("button", { name: "添加", exact: true }).click();
  await dialog("添加动作到周计划").waitFor({ state: "hidden" });
  assert.ok((await api("/workouts/plans")).some((p) => p.items.some((i) => i.exerciseId === picked.id && i.prescription)));

  stage = "健身方案加入习惯、推荐习惯";
  await page.goto(`${base}/habits/fitness?program=low-back`);
  await page.locator(".habits-fit-head").getByRole("button", { name: "加入习惯" }).click();
  await dialog("新建习惯").getByRole("button", { name: "保存" }).click();
  await dialog("新建习惯").waitFor({ state: "hidden" });
  const backHabit = await until("方案建成习惯", async () => (await api("/habits")).find((h) => h.name === "护腰练习"));
  assert.equal(backHabit.remindMode, "times");
  assert.deepEqual(backHabit.remindTimes, ["12:30"]);
  await page.locator(".habits-fit-head").getByRole("button", { name: "已添加" }).waitFor();
  await page.goto(`${base}/habits`);
  await page.locator(".habits-card").filter({ hasText: "护腰练习" }).getByRole("link", { name: "看动作" }).click();
  await page.waitForURL(/\/habits\/fitness\?program=low-back/);
  await page.goto(`${base}/habits/plan?cat=mind`);
  await page.getByRole("button", { name: "添加：冥想 10 分钟" }).click();
  await dialog("新建习惯").getByRole("button", { name: "保存" }).click();
  await dialog("新建习惯").waitFor({ state: "hidden" });
  await until("推荐建成习惯", async () => (await api("/habits")).some((h) => h.name === "冥想 10 分钟" && h.dailyTarget === 10));
  await page.locator(".habits-rec-card").filter({ hasText: "冥想 10 分钟" }).getByRole("button", { name: "已添加" }).waitFor();
  await send("DELETE", `/habits/${backHabit.id}`);
  await send("DELETE", `/habits/${(await api("/habits")).find((h) => h.name === "冥想 10 分钟").id}`);
  await page.goto(`${base}/habits/plan?section=food`);
  await page.getByRole("heading", { name: library.articles.find((a) => a.category === "food").title, exact: true }).waitFor();
  const personalBackup = await api("/habits/personal/backup");
  await send("POST", "/habits/personal/backup", personalBackup);
  assert.equal((await api(`/habits/personal/days/${personalDate}`)).weight, "81.5");

  stage = "续费进入早报";
  const dateParts = Object.fromEntries(
    new Intl.DateTimeFormat("en", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" })
      .formatToParts(new Date(Date.now() + 3 * 86_400_000)).map(({ type, value }) => [type, value]),
  );
  const renewalDate = `${dateParts.year}-${dateParts.month}-${dateParts.day}`;
  const subscriptionResponse = await page.context().request.post(`${base}/api/v1/subscriptions`, {
    headers: { "X-Requested-With": "x-console" },
    data: { name: "端到端续费", amount: 68, currency: "CNY", cycle: "monthly", nextRenewal: renewalDate },
  });
  assert.equal(subscriptionResponse.status(), 201, await subscriptionResponse.text());
  const briefResponse = await page.context().request.post(`${base}/api/v1/briefs/generate`, {
    headers: { "X-Requested-With": "x-console" },
    data: { send: false },
  });
  assert.equal(briefResponse.status(), 200, await briefResponse.text());
  const generatedBrief = await briefResponse.json();
  assert.ok(generatedBrief.sections.some((section) => section.key === "renewals" && section.markdown.includes("端到端续费")));

  stage = "B37 提醒页汇总";
  await page.goto(`${base}/reminders?tab=upcoming`);
  const renewalRow = page.locator(".reminders-external").filter({ hasText: "端到端续费 续费" });
  await renewalRow.waitFor();
  await page.getByRole("checkbox", { name: "显示其他模块" }).uncheck();
  await renewalRow.waitFor({ state: "hidden" });
  await page.getByRole("checkbox", { name: "显示其他模块" }).check();
  await renewalRow.getByRole("link", { name: "端到端续费 续费" }).click();
  await page.waitForURL(/\/monitoring\/subscriptions$/);

  stage = "B50 只填域名添加网站，顺带建证书和域名监控";
  await page.goto(`${base}/monitoring?new=1`);
  await dialog("添加网站").getByLabel("网址").fill("www.e2e-site.example.com");
  await dialog("添加网站").getByRole("button", { name: "保存" }).click();
  await page.locator(".monitoring-row", { hasText: "https://www.e2e-site.example.com" }).waitFor();
  // 证书和域名监控在网站存好以后才建，建完弹窗才关
  await dialog("添加网站").waitFor({ state: "hidden" });
  const siteMonitors = (await api("/monitors")).filter((m) => m.name === "www.e2e-site.example.com");
  assert.deepEqual(siteMonitors.map((m) => `${m.kind} ${m.target}`).sort(), [
    "domain example.com",
    "http https://www.e2e-site.example.com",
    "tls www.e2e-site.example.com",
  ]);
  await page.goto(`${base}/monitoring/certs`);
  await page.locator(".monitoring-row", { hasText: "example.com" }).getByRole("button", { name: /证书/ }).waitFor();

  stage = "B57 锁定后被隐藏的模块像不存在一样";

  stage = "B79 维护 API 主流程";
  const maintenanceOverview = await api("/maintenance/overview?refresh=true");
  assert.ok(maintenanceOverview.version && maintenanceOverview.process && maintenanceOverview.machine);
  assert.ok(maintenanceOverview.storage.some((row) => row.key === "database"));
  assert.ok(Array.isArray(await api("/maintenance/metrics")));
  const maintenanceGarbage = join(serverEnv.XC_DATA_DIR, "tmp", "maintenance-e2e.tmp");
  writeFileSync(maintenanceGarbage, "garbage");
  const maintenanceOld = new Date(Date.now() - 48 * 3600 * 1000);
  utimesSync(maintenanceGarbage, maintenanceOld, maintenanceOld);
  await send("POST", "/maintenance/scan");
  const maintenanceScan = await until("维护扫描", async () => {
    const result = await api("/maintenance/scan");
    return result.state !== "running" && result;
  });
  assert.equal(maintenanceScan.state, "done");
  for (const group of maintenanceScan.groups) {
    assert.ok(group.items.length <= 50);
    if (["missing_records", "audit_logs"].includes(group.kind)) assert.equal(group.selected, false);
  }
  await send("POST", "/maintenance/cleanup", { kinds: ["temporary_files"], scanId: maintenanceScan.id });
  const maintenanceCleanup = await until("维护清理", async () => {
    const result = await api("/maintenance/cleanup");
    return result.state !== "running" && result;
  });
  assert.equal(maintenanceCleanup.state, "done");
  assert.ok(maintenanceCleanup.result.deleted >= 1);
  assert.equal(existsSync(maintenanceGarbage), false);
  stage = "B57 锁定后被隐藏的模块像不存在一样";
  await send("POST", "/vault/setup", { password: "e2e-vault-secret" });
  await send("PUT", "/vault/modules", { hidden: ["github"] });
  await send("POST", "/vault/lock");
  await page.goto(`${base}/`);
  await page.locator(".sidebar").getByRole("link", { name: "项目" }).waitFor();
  assert.equal(await page.locator(".sidebar").getByRole("link", { name: "仓库" }).count(), 0);
  await page.goto(`${base}/github`);
  await page.getByText("页面不存在").first().waitFor();
  assert.equal((await page.context().request.get(`${base}/api/v1/vault/modules`)).status(), 404);
  // B68：锁定时设置里看不到隐藏密码卡片；没隐藏云盘时网盘标签照常（B69 前面加了一个）
  await page.goto(`${base}/settings/security`);
  await page.getByRole("heading", { name: /安全|Security/ }).first().waitFor().catch(() => {});
  await page.waitForLoadState("networkidle").catch(() => {});
  assert.equal(await page.getByRole("heading", { name: "隐藏密码" }).count(), 0);
  const remotes = await api("/storage/remotes?drive=true");
  assert.deepEqual(remotes.items.map((item) => item.name), ["端到端网盘"]);
  // 恢复，后面的步骤还要打开 GitHub 页面
  await send("POST", "/vault/unlock", { password: "e2e-vault-secret" });
  await send("PUT", "/vault/modules", { hidden: [] });
  await send("POST", "/vault/lock");

  stage = "配对 Linux 代理";
  await page.goto(`${base}/settings/devices`);
  // B30 以后入口叫“添加设备”，生成配对码后显示安装命令和配对码。
  await page.getByRole("button", { name: "添加设备" }).click();
  await dialog("添加服务器").getByLabel("设备名称").fill("e2e-linux");
  await dialog("添加服务器").getByRole("button", { name: "生成配对码" }).click();
  await verifyIfAsked();
  const code = (
    await dialog("添加服务器").locator("p", { hasText: "配对码是" }).locator("code").first().textContent()
  ).trim();
  const agentConfig = join(temp, "agent.json");
  await run("pair-agent", binary("agent"), ["pair", "--server", serverUrl, "--code", code, "--config", agentConfig]);
  const agentProcess = start("agent", binary("agent"), ["run", "--config", agentConfig]);
  const host = await until("代理上线", async () => (await api("/hosts")).find((item) => item.name === "e2e-linux" && item.online));
  assert.equal("capabilities" in host, false, "列表接口带了详情字段");
  stage = "B82 服务器信息 API 主流程";
  await send("PATCH", `/hosts/${host.id}`, { info: { ownership: "client", client: "端到端客户", username: "operator", password: "e2e-host-password", note: "测试备注", tags: ["测试"] } });
  const hostInfo = await api(`/hosts/${host.id}`);
  assert.equal(hostInfo.info.client, "端到端客户");
  assert.equal(hostInfo.info.hasPassword, true);
  assert.ok(Array.isArray(hostInfo.addresses));
  assert.equal((await api(`/hosts/${host.id}/password`)).password, "e2e-host-password");
  await send("PUT", "/hosts/order", { kind: "server", ids: [host.id, ...(await api("/hosts?kind=server")).filter((item) => item.id !== host.id).map((item) => item.id)] });
  assert.equal((await api("/hosts?kind=server"))[0].id, host.id);

  stage = "查看远端日志文件";
  const remoteLog = join(temp, "e2e-remote.log");
  writeFileSync(remoteLog, "远端日志主流程\n");
  await page.goto(`${base}/servers/${host.id}/files`);
  await page.getByRole("textbox", { name: "路径" }).fill(temp);
  await page.getByRole("textbox", { name: "路径" }).press("Enter");
  await page.getByRole("button", { name: "e2e-remote.log", exact: true }).click();
  await dialog("e2e-remote.log").getByText("远端日志主流程").waitFor();
  await page.keyboard.press("Escape");

  stage = "B33 服务器 Agent 只读命令";
  const hostFake = http.createServer(async (request, response) => {
    if (request.url === "/v1/models") {
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ data: [{ id: "e2e-model" }] }));
      return;
    }
    if (request.url !== "/v1/chat/completions") { response.writeHead(404).end(); return; }
    const parts = [];
    for await (const part of request) parts.push(part);
    const body = JSON.parse(Buffer.concat(parts).toString());
    response.writeHead(200, { "Content-Type": "text/event-stream" });
    const hasResult = body.messages?.some((message) => message.role === "tool");
    const delta = hasResult
      ? { content: "运行时间已读取" }
      : { tool_calls: [{ index: 0, id: "host-e2e-tool", type: "function", function: { name: "host__run_command", arguments: JSON.stringify({ command: "uptime", reason: "检查运行时间" }) } }] };
    response.write(`data: ${JSON.stringify({ id: "host_e2e", object: "chat.completion.chunk", created: 1, model: "e2e-model", choices: [{ index: 0, delta }] })}\n\n`);
    response.write(`data: ${JSON.stringify({ id: "host_e2e", object: "chat.completion.chunk", created: 1, model: "e2e-model", choices: [], usage: { prompt_tokens: 4, completion_tokens: 5 } })}\n\n`);
    response.end("data: [DONE]\n\n");
  });
  await new Promise((ready) => hostFake.listen(0, "127.0.0.1", ready));
  try {
    const created = await page.context().request.post(`${base}/api/v1/ai/providers`, {
      headers: { "X-Requested-With": "x-console" },
      data: { name: "机器 Agent 测试模型", baseUrl: `http://127.0.0.1:${hostFake.address().port}/v1` },
    });
    assert.equal(created.status(), 201, await created.text());
    const provider = await created.json();
    const refreshed = await page.context().request.post(`${base}/api/v1/ai/providers/${provider.id}/models`, { headers: { "X-Requested-With": "x-console" } });
    assert.equal(refreshed.status(), 200, await refreshed.text());
    const settings = await page.context().request.put(`${base}/api/v1/ai/model-settings`, {
      headers: { "X-Requested-With": "x-console" }, data: { agent: { providerId: provider.id, model: "e2e-model" } },
    });
    assert.equal(settings.status(), 200, await settings.text());
    await page.goto(`${base}/servers/${host.id}/agent`);
    await page.locator(".hagent-mode select").selectOption("read_auto");
    await page.locator(".hagent-composer textarea").fill("uptime");
    await page.locator(".hagent-composer textarea").press("Enter");
    await page.locator(".hagent-body").getByText("运行时间已读取").waitFor();
    const hostConversations = await api(`/ai/host-agent/${host.id}/conversations`);
    assert.equal(hostConversations.length, 1);
    const hostDetail = await api(`/ai/conversations/${hostConversations[0].id}`);
    assert.equal(hostDetail.pendingActions[0]?.action, "host.run_command");
    assert.equal(hostDetail.pendingActions[0]?.status, "done");
    assert.equal(hostDetail.pendingActions[0]?.effect, "read");
    assert.equal((await api("/ai/conversations")).some((item) => item.id === hostConversations[0].id), false);
    const removeProvider = await page.context().request.delete(`${base}/api/v1/ai/providers/${provider.id}`, { headers: { "X-Requested-With": "x-console" } });
    assert.equal(removeProvider.status(), 204, await removeProvider.text());
  } finally {
    await new Promise((done) => hostFake.close(done));
  }

  stage = "按需订阅服务器指标";
  await page.goto(`${base}/servers`);
  await page.locator(".servers-card").filter({ hasText: "e2e-linux" }).getByText("在线").waitFor();
  await until("列表指标订阅", () => eventFrames.sent.some((frame) => frame.type === "subscribe" && frame.topics?.includes("host.metrics")));
  await page.goto(`${base}/servers/${host.id}`);
  await until("详情指标订阅", () => eventFrames.sent.some((frame) => frame.type === "subscribe" && frame.topics?.includes(`host.metrics:${host.id}`)));
  const beforeDetail = eventFrames.received.filter((frame) => frame.topic === "host.metrics" && frame.data?.hostId === host.id).length;
  await until("详情 5 秒指标", () => eventFrames.received.filter((frame) => frame.topic === "host.metrics" && frame.data?.hostId === host.id).length >= beforeDetail + 2, 15_000);
  const detailFrame = [...eventFrames.received].reverse().find((frame) => frame.topic === "host.metrics" && frame.data?.hostId === host.id);
  assert.ok(Array.isArray(detailFrame.data.sample.netInterfaces), "详情指标缺少逐网卡速率");
  await page.getByRole("heading", { name: "网卡" }).waitFor();
  await page.screenshot({ path: join(artifacts, "server-detail-1360.png"), fullPage: true });
  await page.getByRole("heading", { name: "网卡" }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: join(artifacts, "server-network-1360.png") });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await page.locator(".servers-stats").waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, "手机服务器详情横向溢出");
  await page.screenshot({ path: join(artifacts, "server-detail-390.png"), fullPage: true });
  await page.getByRole("heading", { name: "网卡" }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: join(artifacts, "server-network-390.png") });
  await page.setViewportSize({ width: 1360, height: 860 });

  stage = "查看服务器并打开终端";
  await page.goto(`${base}/servers/${host.id}/terminal`);
  await page.getByRole("button", { name: "连接", exact: true }).click();
  await verifyIfAsked();
  await page.locator(".servers-terminal-card .xc-badge.ok").getByText("已连接").waitFor();

  // C1：每个路由页面都要在真实服务端上至少完成一次渲染。
  for (const path of [
    "/",
    "/projects",
    "/projects/EET",
    `/projects/EET/${issueKey.split("-")[1]}`,
    "/coding",
    "/coding/tasks",
    "/settings/git",
    "/coding/repos",
    "/coding/999999",
    "/notes",
    `/notes/${noteId}`,
    "/reminders",
    "/habits",
    "/drive",
    "/calendar",
    "/calendar/briefs",
    "/calendar/focus",
    "/calendar/calendars",
    "/servers",
    `/servers/${host.id}`,
    "/pc",
    "/monitoring",
    "/home",
    "/automations",
    `/automations/${automation.id}`,
    "/automations/new",
    "/github",
    "/settings/security",
  ]) {
    stage = `页面渲染 ${path}`;
    await page.goto(`${base}${path}`);
    await page.locator("#main .xc-page, #main .notes-layout").first().waitFor();
  }
  stage = "B41 报错提示常驻、可以展开和复制";
  {
    const consoleErrors = [];
    const onConsole = (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    };
    page.on("console", onConsole);
    await page.route(/\/api\/v1\/notes(\?|$)/, (route) =>
      route.fulfill({
        status: 500,
        headers: { "Content-Type": "application/json", "X-Request-Id": "e2e-req-1" },
        body: JSON.stringify({ code: "internal", message: "端到端假错误", requestId: "e2e-req-1" }),
      }),
    );
    await page.goto(`${base}/notes`);
    const notice = page.locator(".error-notice").filter({ hasText: "端到端假错误" }).first();
    await notice.waitFor({ timeout: 20_000 });
    await page.waitForTimeout(6_000);
    assert.equal(await notice.isVisible(), true, "报错提示 6 秒后自己消失了");
    await notice.locator(".error-notice-text").click();
    const detail = await notice.locator(".error-notice-text").innerText();
    assert.match(detail, /请求编号：e2e-req-1/);
    assert.match(detail, /状态：500 internal/);
    assert.ok(consoleErrors.some((text) => text.includes("[X Console]")), "控制台没有 [X Console] 输出");
    await page.unroute(/\/api\/v1\/notes(\?|$)/);
    page.off("console", onConsole);
    await page.getByRole("button", { name: /全部关闭|关闭/ }).first().click();
  }
  stage = "手机命令面板";
  await page.setViewportSize({ width: 390, height: 180 });
  await page.keyboard.press("Control+k");
  await page.locator(".command-dialog").waitFor();
  const paletteBounds = await page.evaluate(() => {
    const dialog = document.querySelector(".command-dialog").getBoundingClientRect();
    const footer = document.querySelector(".command-footer").getBoundingClientRect();
    return { dialogBottom: dialog.bottom, footerBottom: footer.bottom, viewport: innerHeight };
  });
  assert.ok(paletteBounds.dialogBottom <= paletteBounds.viewport - 4, `命令面板超出屏幕：${JSON.stringify(paletteBounds)}`);
  assert.ok(paletteBounds.footerBottom <= paletteBounds.viewport - 4, `前缀提示超出屏幕：${JSON.stringify(paletteBounds)}`);
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 1360, height: 860 });
  stage = "吊销代理并停止进程";
  await page.goto(`${base}/settings/devices`);
  const agentRow = page.locator("tr").filter({ hasText: "e2e-linux" });
  await agentRow.waitFor();
  await agentRow.getByRole("button", { name: "吊销" }).click();
  // B21 以后用页面里的确认框，不再是浏览器自带的。
  await dialog("吊销“e2e-linux”？").getByRole("button", { name: "吊销", exact: true }).click();
  await verifyIfAsked();
  await until("代理以吊销状态退出", () => agentProcess.exitCode === 3, 10_000);
  await until("代理从列表移除", async () => !(await api("/hosts")).some((item) => item.id === host.id));
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
  for (const socket of weatherProxySockets) socket.destroy();
  if (weatherProxy) await new Promise((done) => weatherProxy.close(done));
  rmSync(temp, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
}
