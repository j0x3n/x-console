import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import net from "node:net";
import os from "node:os";
import { join, resolve } from "node:path";
import { chromium } from "playwright-core";
import { verifyWeatherReminder } from "./weather-reminder-flow.mjs";
import {
  saveBrowserFailure,
  watchBrowserErrors,
} from "./browser-diagnostics.mjs";

const webDir = resolve(import.meta.dirname, "..");
const backendDir = resolve(webDir, "../backend");
const temp = mkdtempSync(join(os.tmpdir(), "xc-weather-"));
const children = [];
const logs = new Map();
const started = Date.now();
let browser;
let page;
let diagnostics;
let stage = "编译服务端";

function start(name, command, args, options = {}) {
  const child = spawn(command, args, {
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  });
  children.push(child);
  const collect = (data) =>
    logs.set(name, ((logs.get(name) ?? "") + data).slice(-12000));
  child.stdout.on("data", collect);
  child.stderr.on("data", collect);
  child.on("error", (error) => collect(error.stack));
  return child;
}

async function port() {
  const listener = net.createServer();
  await new Promise((done) => listener.listen(0, "127.0.0.1", done));
  const value = listener.address().port;
  await new Promise((done) => listener.close(done));
  return value;
}

async function ready(url, child) {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`进程退出：${url}`);
    if (
      await fetch(url, { signal: AbortSignal.timeout(1000) })
        .then((r) => r.ok)
        .catch(() => false)
    )
      return;
    await new Promise((done) => setTimeout(done, 100));
  }
  throw new Error(`服务未就绪：${url}`);
}

async function stop(child) {
  if (child.exitCode !== null) return;
  if (process.platform === "win32") {
    await new Promise((done) => {
      const killer = spawn(
        "taskkill",
        ["/pid", String(child.pid), "/t", "/f"],
        { stdio: "ignore" },
      );
      killer.on("exit", done);
      killer.on("error", done);
    });
  } else {
    child.kill("SIGTERM");
    await new Promise((done) => {
      const timer = setTimeout(() => {
        child.kill("SIGKILL");
        done();
      }, 2000);
      child.once("exit", () => {
        clearTimeout(timer);
        done();
      });
    });
  }
}

try {
  const binary = join(
    temp,
    `server${process.platform === "win32" ? ".exe" : ""}`,
  );
  const compiler = start(
    "build",
    "go",
    ["build", "-o", binary, "./cmd/server"],
    { cwd: backendDir },
  );
  const code = await new Promise((done, reject) => {
    compiler.once("exit", done);
    compiler.once("error", reject);
  });
  assert.equal(code, 0, logs.get("build"));
  const apiPort = await port();
  const webPort = await port();
  const api = `http://127.0.0.1:${apiPort}`;
  const base = `http://127.0.0.1:${webPort}`;
  stage = "启动服务";
  const server = start("server", binary, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      XC_DEV: "1",
      XC_ADDR: `127.0.0.1:${apiPort}`,
      XC_DATA_DIR: join(temp, "data"),
      XC_MASTER_KEY: crypto.randomBytes(32).toString("base64"),
    },
  });
  const vite = start(
    "vite",
    process.execPath,
    [
      join(webDir, "node_modules/vite/bin/vite.js"),
      "--port",
      String(webPort),
      "--strictPort",
    ],
    { cwd: webDir, env: { ...process.env, XC_API: api } },
  );
  await Promise.all([ready(`${api}/api/v1/health`, server), ready(base, vite)]);
  browser = await chromium.launch(
    process.env.XC_SHOTS_BROWSER
      ? { executablePath: process.env.XC_SHOTS_BROWSER }
      : {},
  );
  page = await browser.newPage({ viewport: { width: 1360, height: 860 } });
  page.setDefaultTimeout(5000);
  diagnostics = watchBrowserErrors(page);
  await diagnostics.run(async () => {
    await page.route("**/api/v1/weather", (route) =>
      route.fulfill({
        json: {
          temperature: 22,
          summary: "多云",
          low: 18,
          high: 25,
          humidity: 65,
          precipitationChance: 60,
          weatherCode: 2,
          isDay: true,
          source: "qweather",
          location: "上海",
          fetchedAt: new Date().toISOString(),
        },
      }),
    );
    await page.route("**/api/v1/weather/extra", (route) =>
      route.fulfill({
        json: {
          configured: true,
          warnings: [],
          indices: [],
          earthquakes: [],
          fetchedAt: new Date().toISOString(),
        },
      }),
    );
    await page.route("**/api/v1/weather/qweather", (route) =>
      route.fulfill({ json: { apiHost: "test.qweather.com", keySet: true } }),
    );
    stage = "初始化账号";
    await page.goto(base);
    await page.getByLabel("用户名").fill("weather-test");
    const password = crypto.randomBytes(16).toString("hex");
    await page.getByLabel(/^密码/).fill(password);
    await page.getByLabel("再输一次密码").fill(password);
    await page.getByRole("button", { name: "下一步" }).click();
    await page.getByRole("button", { name: "跳过，以后再开" }).click();
    await page.locator(".sidebar").waitFor();
    const open = async () => {
      await page.locator(".today-weather-strip").click();
      await page
        .getByRole("dialog", { name: "天气", exact: true })
        .getByRole("button", { name: "天气设置" })
        .click();
    };
    stage = "降水提醒完整流程";
    await verifyWeatherReminder(page, base, open);
    stage = "桌面和手机布局";
    for (const width of [1360, 390]) {
      await page.setViewportSize({ width, height: 860 });
      await open();
      const dialog = page.getByRole("dialog", { name: "天气设置" });
      await dialog
        .getByRole("checkbox", { name: "降水提醒", exact: true })
        .check();
      for (const mode of ["minutely", "probability"]) {
        await dialog.getByLabel("提醒方式").selectOption(mode);
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth,
          ),
          false,
        );
        assert.equal(
          await dialog.evaluate((el) => el.scrollWidth > el.clientWidth + 1),
          false,
        );
      }
      await dialog.getByRole("button", { name: "取消", exact: true }).click();
    }
  });
  console.log(
    `天气浏览器检查通过，用时 ${((Date.now() - started) / 1000).toFixed(1)} 秒。`,
  );
} catch (error) {
  const evidence = mkdtempSync(join(os.tmpdir(), "xc-weather-failure-"));
  const body = page ? await saveBrowserFailure(page, evidence) : "";
  const details = `阶段：${stage}\n${error.stack}\n页面异常：${diagnostics?.errors.join("\n") ?? ""}\n${body}\n${[...logs].map(([name, text]) => `${name}\n${text}`).join("\n")}`;
  writeFileSync(join(evidence, "failure.txt"), details);
  console.error(details);
  console.error(`失败记录：${evidence}`);
  process.exitCode = 1;
} finally {
  diagnostics?.dispose();
  await browser?.close();
  for (const child of children.reverse()) await stop(child);
  rmSync(temp, { recursive: true, force: true });
}
