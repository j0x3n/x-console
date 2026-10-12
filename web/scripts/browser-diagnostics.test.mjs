import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import test from "node:test";
import { watchBrowserErrors } from "./browser-diagnostics.mjs";
import { chromium } from "playwright-core";

test("正常操作返回结果且移除监听器", async () => {
  const page = new EventEmitter();
  const diagnostics = watchBrowserErrors(page);
  assert.equal(await diagnostics.run(() => 42), 42);
  diagnostics.dispose();
  assert.equal(page.listenerCount("pageerror"), 0);
  assert.equal(page.listenerCount("console"), 0);
});

test("页面异常立即中断等待", async () => {
  const page = new EventEmitter();
  const diagnostics = watchBrowserErrors(page);
  const pending = diagnostics.run(() => new Promise(() => {}));
  page.emit("pageerror", new Error("缺少更新时间"));
  await assert.rejects(pending, /页面异常：缺少更新时间/);
  assert.deepEqual(diagnostics.errors, ["缺少更新时间"]);
  diagnostics.dispose();
});

test("React 捕获的渲染异常中断，预期接口报错保留", async () => {
  const page = new EventEmitter();
  const diagnostics = watchBrowserErrors(page);
  page.emit("console", {
    type: () => "error",
    text: () => "Failed to load resource: 500",
  });
  assert.equal(await diagnostics.run(() => "正常"), "正常");
  const pending = diagnostics.run(() => new Promise(() => {}));
  page.emit("console", {
    type: () => "error",
    text: () => "TypeError: 缺少字段",
  });
  await assert.rejects(pending, /缺少字段/);
  diagnostics.dispose();
});

test(
  "真实浏览器脚本异常立即结束检查",
  { skip: !process.env.XC_SHOTS_BROWSER },
  async () => {
    const browser = await chromium.launch({
      executablePath: process.env.XC_SHOTS_BROWSER,
    });
    try {
      const page = await browser.newPage();
      const diagnostics = watchBrowserErrors(page);
      const started = Date.now();
      const pending = diagnostics.run(async () => {
        await page.setContent(
          '<script>throw new Error("故意缺少天气字段")</script>',
        );
        await page.getByText("永远不会出现").waitFor({ timeout: 10000 });
      });
      await assert.rejects(pending, /故意缺少天气字段/);
      assert.ok(Date.now() - started < 2000);
      diagnostics.dispose();
    } finally {
      await browser.close();
    }
  },
);
