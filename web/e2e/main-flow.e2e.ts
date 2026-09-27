import { createHmac } from "node:crypto";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, test } from "@playwright/test";

function totp(secret: string): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = 0;
  let value = 0;
  const bytes: number[] = [];
  for (const char of secret.toUpperCase().replace(/[^A-Z2-7]/g, "")) {
    value = (value << 5) | alphabet.indexOf(char);
    bits += 5;
    if (bits >= 8) {
      bytes.push((value >>> (bits - 8)) & 255);
      bits -= 8;
    }
  }
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30_000)));
  const digest = createHmac("sha1", Buffer.from(bytes)).update(counter).digest();
  const offset = digest[digest.length - 1] & 15;
  return ((digest.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).toString().padStart(6, "0");
}

test("主流程：账号、项目、备忘、提醒、服务器终端、习惯", async ({ page }) => {
  const agentBinary = path.resolve(
    "../backend/bin/e2e-agent" + (process.platform === "win32" ? ".exe" : ""),
  );
  const agentDir = mkdtempSync(path.join(tmpdir(), "x-console-e2e-agent-"));
  const agentConfig = path.join(agentDir, "config.json");
  const apiPort = process.env.XC_E2E_API_PORT ?? "18080";
  let agent: ChildProcess | undefined;
  let secret = "";

  try {
    await test.step("初始化账号并重新登录", async () => {
      await page.goto("/");
      await expect(page.getByText("第一次使用，先创建你的账号")).toBeVisible();
      await page.getByLabel("用户名").fill("e2e-user");
      await page.locator('input[type="password"]').nth(0).fill("E2e-password-123");
      await page.locator('input[type="password"]').nth(1).fill("E2e-password-123");
      await page.getByRole("button", { name: "下一步" }).click();
      secret = (await page.locator("code.xc-secret").textContent())!.trim();
      await page.getByLabel("验证码", { exact: true }).fill(totp(secret));
      await page.getByRole("button", { name: "完成设置" }).click();
      await expect(page.locator(".xc-page").first()).toBeVisible();

      const logout = await page.request.post("/api/v1/auth/logout", {
        headers: { "X-Requested-With": "x-console" },
      });
      expect(logout.ok()).toBeTruthy();
      await page.reload();
      await expect(page.getByRole("button", { name: "登录" })).toBeVisible();
      await page.getByLabel("用户名").fill("e2e-user");
      await page.locator('input[type="password"]').fill("E2e-password-123");
      await page.getByLabel("两步验证码").fill(totp(secret));
      await page.getByRole("button", { name: "登录" }).click();
      await expect(page.locator(".xc-page").first()).toBeVisible();
    });

    await test.step("新建项目和 Issue", async () => {
      await page.goto("/projects");
      await page.getByRole("button", { name: "新建项目" }).click();
      const project = page.getByRole("dialog", { name: "新建项目" });
      await project.getByLabel("名称").fill("E2E 项目");
      await project.getByLabel("Key").fill("EET");
      await project.getByRole("button", { name: "创建项目" }).click();
      await expect(page).toHaveURL(/\/projects\/EET$/);
      await expect(page.locator(".projects-lane")).toHaveCount(6);
      await page.locator(".xc-page-head").getByRole("button", { name: /新建 Issue/ }).click();
      const issue = page.getByRole("dialog", { name: "新建 Issue" });
      await expect(issue).toBeVisible();
      await issue.getByLabel("标题").fill("E2E Issue");
      await issue.getByRole("button", { name: "创建 Issue" }).click();
      await expect(page.getByText("E2E Issue")).toBeVisible();
    });

    await test.step("写备忘并等待保存", async () => {
      await page.goto("/notes");
      await page.getByRole("button", { name: "新建", exact: true }).click();
      await page.getByRole("textbox", { name: "标题" }).fill("E2E 备忘");
      await page.getByRole("textbox", { name: "笔记", exact: true }).fill("用 Playwright 写入的内容");
      await expect(page.locator(".notes-save-state")).toContainText("已保存", { timeout: 15_000 });
      await page.reload();
      await expect(page.getByRole("textbox", { name: "笔记", exact: true })).toHaveValue("用 Playwright 写入的内容");
    });

    await test.step("新建提醒", async () => {
      await page.goto("/reminders?tab=upcoming");
      await page.getByRole("button", { name: "新建提醒" }).click();
      const reminder = page.getByRole("dialog", { name: "新建提醒" });
      await reminder.getByLabel("标题").fill("E2E 提醒");
      const when = new Date(Date.now() + 2 * 24 * 60 * 60 * 1000);
      const localTime = `${when.getFullYear()}-${String(when.getMonth() + 1).padStart(2, "0")}-${String(when.getDate()).padStart(2, "0")}T10:00`;
      await reminder.locator('input[type="datetime-local"]').fill(localTime);
      await reminder.getByRole("button", { name: "保存" }).click();
      await expect(page.getByText("E2E 提醒")).toBeVisible();
    });

    await test.step("配对真实代理并打开终端", async () => {
      const elevate = await page.request.post("/api/v1/auth/elevate", {
        data: { code: totp(secret) },
        headers: { "X-Requested-With": "x-console" },
      });
      expect(elevate.ok()).toBeTruthy();
      await page.goto("/settings/devices");
      await page.getByRole("button", { name: "配对新设备" }).click();
      const pair = page.getByRole("dialog", { name: "配对新设备" });
      await pair.getByLabel("设备名称").fill("E2E agent");
      await pair.getByRole("button", { name: "生成配对码" }).click();
      const code = (await pair.locator("code.xc-secret").first().textContent())!.trim();
      execFileSync(agentBinary, ["pair", "--server", `http://127.0.0.1:${apiPort}`, "--code", code, "--config", agentConfig], { timeout: 20_000 });
      agent = spawn(agentBinary, ["run", "--config", agentConfig], { stdio: "ignore" });
      await page.goto("/servers");
      const card = page.locator(".servers-card").filter({ hasText: "E2E agent" });
      await expect(card).toContainText("在线", { timeout: 30_000 });
      await card.click();
      await page.getByRole("link", { name: "终端" }).click();
      await page.getByRole("button", { name: "连接", exact: true }).click();
      await expect(page.getByText("已连接", { exact: true })).toBeVisible({ timeout: 20_000 });
    });

    await test.step("新建习惯并打卡", async () => {
      await page.goto("/habits");
      await page.getByRole("button", { name: "新建习惯" }).click();
      const habit = page.getByRole("dialog", { name: "新建习惯" });
      await habit.getByLabel("名称").fill("E2E 喝水");
      await habit.getByRole("button", { name: "保存" }).click();
      const card = page.locator(".habits-card").filter({ hasText: "E2E 喝水" });
      await card.getByRole("button", { name: /1/ }).click();
      await expect(card.locator(".habits-ring-label strong")).toHaveText("1");
    });
  } finally {
    agent?.kill();
    rmSync(agentDir, { recursive: true, force: true });
  }
});
