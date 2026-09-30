// B8：用真实服务端和 Linux 代理跑浏览器主流程。运行前先 npm ci、安装 Chromium。
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import net from "node:net";
import http from "node:http";
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
  await page.locator("button.profile").click();
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
  await page.reload();
  await page.getByText("服务器", { exact: true }).first().waitFor();
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
  const progressCard = projectStats.locator(".xc-stat").filter({ hasText: "正在处理的 Issue" });
  assert.equal((await progressCard.locator(".xc-stat-value").textContent()).trim(), "1");

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
  await dialog("新建提醒").getByRole("button", { name: "保存" }).click();
  await dialog("新建提醒").waitFor({ state: "hidden" });
  const reminders = [
    ...(await api("/reminders?range=today")).items,
    ...(await api("/reminders?range=upcoming")).items,
  ];
  const reminder = reminders.find((item) => item.title === "端到端提醒");
  assert.ok(reminder, "真实接口里没有新建的提醒");

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
  const shareResponse = await page.context().request.post(`${base}/api/v1/drive/shares`, {
    headers: { "X-Requested-With": "x-console" },
    data: { itemId: driveFile.id, expiresIn: "7d" },
  });
  assert.equal(shareResponse.status(), 201, await shareResponse.text());
  const driveShare = await shareResponse.json();
  assert.equal((await api(`/drive/shares?itemId=${driveFile.id}`)).items[0]?.id, driveShare.id);
  const publicShare = await page.context().request.get(
    `${base}/api/v1/public/shares/${driveShare.token}`,
  );
  assert.equal(publicShare.status(), 200, await publicShare.text());
  const publicContent = await page.context().request.get(
    `${base}/api/v1/public/shares/${driveShare.token}/content`,
  );
  assert.equal(publicContent.status(), 200);
  assert.equal(await publicContent.text(), "云盘内容可以预览。");
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
  rmSync(temp, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
}
