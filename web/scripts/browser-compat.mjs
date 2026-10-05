import assert from "node:assert/strict";
import { chromium, firefox, webkit } from "playwright-core";
import { mkdirSync } from "node:fs";
import { join } from "node:path";

const base = process.env.XC_BROWSER_BASE ?? "http://127.0.0.1:5173";
const username = process.env.XC_SHOTS_USER ?? "demo";
const password = process.env.XC_SHOTS_PASSWORD ?? "demo-password-123";
const output = join(import.meta.dirname, "..", "screenshots", "browser-compat");
const engines = (
  process.env.XC_BROWSER_ENGINES ?? "chromium,firefox,webkit"
).split(",");
const routes = [
  "/",
  "/projects",
  "/coding",
  "/notes",
  "/reminders",
  "/habits",
  "/drive",
  "/calendar",
  "/servers",
  "/pc",
  "/monitoring",
  "/home",
  "/automations",
  "/automations/new",
  "/coding/repos",
  "/calendar/briefs",
  "/calendar/focus",
  "/calendar/calendars",
  "/github",
  "/settings/security",
  "/settings/assistant",
  "/settings/storage",
  "/settings/backup",
  "/s/missing-share",
];

async function bounds(page) {
  return page.evaluate(() => ({
    width: innerWidth,
    height: innerHeight,
    overflow:
      document.documentElement.scrollWidth -
      document.documentElement.clientWidth,
    scroll: document.scrollingElement.scrollHeight,
    theme: document.querySelector('meta[name="theme-color"]').content,
    expectedTheme: getComputedStyle(document.documentElement)
      .getPropertyValue(
        matchMedia("(max-width: 720px)").matches ? "--xc-panel" : "--xc-bg",
      )
      .trim(),
  }));
}

async function go(page, url) {
  await page.waitForLoadState("networkidle");
  await page.goto(url);
  await page.waitForLoadState("networkidle");
}

async function profile(page, mobile) {
  if (mobile) await page.locator(".sidebar-toggle").click();
  await page.locator("button.nav-rail-avatar").click();
  await page.getByRole("menu").waitFor();
}

for (const [name, engine] of Object.entries({ chromium, firefox, webkit })) {
  if (!engines.includes(name)) continue;
  const browser = await engine.launch();
  try {
    for (const size of [
      { width: 1360, height: 860 },
      { width: 390, height: 844 },
      { width: 844, height: 390 },
    ]) {
      const mobile = size.width <= 720;
      const context = await browser.newContext({
        viewport: size,
        hasTouch: mobile,
      });
      const errors = [];
      const page = await context.newPage();
      page.on("pageerror", (error) => errors.push(error.message));
      await go(page, base);
      await page.getByLabel("用户名").fill(username);
      await page.getByLabel("密码", { exact: true }).fill(password);
      await page.getByRole("button", { name: "登录", exact: true }).click();
      await page.locator(".sidebar").waitFor({ state: "attached" });
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
      for (const route of routes) {
        await go(page, base + route);
        await page
          .locator(route.startsWith("/s/") ? ".share-page" : ".main-panel")
          .waitFor();
        await page.waitForLoadState("networkidle");
        await page.waitForTimeout(180);
        const result = await bounds(page);
        assert.ok(
          result.overflow <= 0,
          `${name} ${size.width} ${route}: 横向溢出 ${result.overflow}`,
        );
        assert.equal(
          result.theme,
          result.expectedTheme,
          `${name} ${route}: 浏览器主题色`,
        );
      }
      await go(page, base + "/settings/security");
      await page.locator(".main-panel").waitFor();
      await profile(page, mobile);
      await page.locator('.profile-segmented [role="radio"]').nth(0).click();
      await page.waitForTimeout(80);
      assert.equal(
        (await bounds(page)).theme,
        (await bounds(page)).expectedTheme,
      );
      await page.locator('.profile-segmented [role="radio"]').nth(1).click();
      await page.locator('.profile-accents [role="radio"]').nth(1).click();
      await page.waitForTimeout(80);
      assert.equal(
        (await bounds(page)).theme,
        (await bounds(page)).expectedTheme,
      );
      await page.getByRole("menuitem", { name: "安装应用" }).click();
      await page.getByRole("dialog", { name: "安装应用" }).waitFor();
      await page.getByRole("button", { name: "知道了" }).click();
      if (mobile) {
        await page.locator('.sidebar a[href="/reminders"]').first().click();
        await page.waitForURL("**/reminders");
        assert.equal(
          await page
            .locator(".sidebar")
            .evaluate((element) => element.classList.contains("open")),
          false,
        );
        await go(page, base + "/settings/backup");
        await page.getByRole("button", { name: "导出全部数据" }).waitFor();
        await page.setViewportSize({ width: size.width, height: 400 });
        await page.waitForTimeout(300);
        await page.mouse.move(200, 200);
        await page.mouse.wheel(0, 1200);
        await page.waitForTimeout(500);
        assert.ok(
          await page.evaluate(() => scrollY > 0),
          `${name}: 手机文档滚动 ${JSON.stringify(await bounds(page))}`,
        );
        await page.setViewportSize(size);
      }
      await page.evaluate(() => {
        scrollTo(0, 0);
        document.documentElement.style.setProperty("--xc-safe-top", "47px");
        document.documentElement.style.setProperty("--xc-safe-bottom", "34px");
        document.documentElement.style.setProperty("--xc-safe-left", "24px");
        document.documentElement.style.setProperty("--xc-safe-right", "24px");
      });
      await page.keyboard.press("Control+k");
      await page.locator(".command-dialog").waitFor();
      await page.locator(".command-input input").fill("笔记");
      await page.setViewportSize({ width: size.width, height: 360 });
      await page.waitForTimeout(150);
      const dialog = await page.locator(".command-dialog").boundingBox();
      assert.ok(
        dialog.y >= 47 && dialog.y + dialog.height <= 326,
        `${name}: 安全区弹窗 ${JSON.stringify(dialog)}`,
      );
      await page.keyboard.press("Escape");
      await page.setViewportSize(size);
      await go(page, base + "/notes");
      await page.locator(".notes-layout").waitFor();
      await page.evaluate(() => {
        document.documentElement.style.setProperty("--xc-safe-top", "47px");
        document.documentElement.style.setProperty("--xc-safe-bottom", "34px");
      });
      await page.waitForTimeout(100);
      const notes = await page.locator(".notes-layout").boundingBox();
      assert.ok(
        notes.y + notes.height <= size.height - 34 + 1,
        `${name}: 笔记底部 ${JSON.stringify(notes)}`,
      );
      await page.getByRole("button", { name: "新建笔记" }).first().click();
      await page.waitForURL(/\/notes\/\d+/);
      const title = `浏览器适配 ${name} ${size.width}`;
      await page
        .getByRole("textbox", { name: "标题", exact: true })
        .fill(title);
      await page
        .getByRole("textbox", { name: "笔记", exact: true })
        .fill("安全区和动态高度下也能编辑保存。");
      await page.getByText("已保存", { exact: true }).waitFor();
      await page.reload();
      await page.getByRole("textbox", { name: "标题", exact: true }).waitFor();
      assert.equal(
        await page
          .getByRole("textbox", { name: "标题", exact: true })
          .inputValue(),
        title,
      );
      await go(page, base + "/drive");
      await page.locator('[data-testid="drive-file-input"]').setInputFiles({
        name: `${name}-${size.width}.txt`,
        mimeType: "text/plain",
        buffer: Buffer.from("跨浏览器预览内容"),
      });
      const file = page
        .getByRole("button", { name: `${name}-${size.width}.txt`, exact: true })
        .first();
      await file.waitFor();
      await file.click();
      await page
        .getByRole("dialog", { name: `${name}-${size.width}.txt` })
        .getByText("跨浏览器预览内容", { exact: false })
        .waitFor();
      await page.setViewportSize({ width: size.width, height: 360 });
      await page.evaluate(() => {
        document.documentElement.style.setProperty("--xc-safe-top", "47px");
        document.documentElement.style.setProperty("--xc-safe-bottom", "34px");
      });
      await page.waitForTimeout(100);
      const viewerHead = await page.locator(".drive-viewer-head").boundingBox();
      assert.ok(viewerHead.y >= 47, `${name}: 查看器顶部安全区`);
      await page
        .getByRole("dialog", { name: `${name}-${size.width}.txt` })
        .getByRole("button", { name: "关闭", exact: true })
        .click();
      await page.setViewportSize(size);
      await page.locator(".ai-fab").click();
      await page.getByRole("dialog", { name: "AI", exact: true }).waitFor();
      await page.setViewportSize({ width: size.width, height: 360 });
      await page.waitForTimeout(100);
      const ai = await page.locator(".ai-panel").boundingBox();
      assert.ok(
        ai.y >= 0 && ai.y + ai.height <= 360,
        `${name}: AI 浮窗 ${JSON.stringify(ai)}`,
      );
      await page
        .getByRole("dialog", { name: "AI", exact: true })
        .getByRole("button", { name: "关闭", exact: true })
        .click();
      await page.setViewportSize(size);
      assert.deepEqual(errors, [], `${name}: 页面报错`);
      mkdirSync(join(output, name), { recursive: true });
      await page.screenshot({
        path: join(output, name, `notes-${size.width}.png`),
      });
      console.log(
        `通过 ${name} ${size.width}×${size.height}: 全页面、导航、主题、滚动、弹窗、安全区`,
      );
      await context.close();
    }
    const context = await browser.newContext({
      viewport: { width: 390, height: 844 },
    });
    await context.addInitScript(() => {
      Object.defineProperty(navigator, "standalone", { value: true });
      const viewport = new EventTarget();
      Object.assign(viewport, { height: 844, offsetTop: 0, scale: 1 });
      Object.defineProperty(window, "visualViewport", { value: viewport });
    });
    const page = await context.newPage();
    await go(page, base);
    await page.getByLabel("用户名").fill(username);
    await page.getByLabel("密码", { exact: true }).fill(password);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.locator(".sidebar").waitFor({ state: "attached" });
    await profile(page, true);
    assert.equal(
      await page.getByRole("menuitem", { name: "安装应用" }).count(),
      0,
    );
    await page.locator(".mobile-scrim").click({ position: { x: 350, y: 60 } });
    await page.keyboard.press("Control+k");
    await page.locator(".command-dialog").waitFor();
    await page.locator(".command-input input").fill("提醒");
    await page.evaluate(() => {
      Object.assign(visualViewport, { height: 300, offsetTop: 90 });
      visualViewport.dispatchEvent(new Event("resize"));
    });
    await page.waitForTimeout(120);
    const dialog = await page.locator(".command-dialog").boundingBox();
    assert.ok(
      dialog.y >= 90 && dialog.y + dialog.height <= 390,
      `${name}: 软键盘可见区域`,
    );
    await page.evaluate(() => {
      Object.assign(visualViewport, { height: 150, offsetTop: 0, scale: 2 });
      visualViewport.dispatchEvent(new Event("resize"));
    });
    await page.waitForTimeout(120);
    assert.equal(
      await page.evaluate(() =>
        document.documentElement.style.getPropertyValue("--xc-visible-height"),
      ),
      "300px",
    );
    await context.close();
    console.log(`通过 ${name}: 主屏幕应用、软键盘可见区域、保留用户缩放`);
  } finally {
    await browser.close();
  }
}
