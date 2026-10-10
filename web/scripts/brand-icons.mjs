// 从 favicon.svg 生成安装图标和通用通知图标。
// 用法：cd web && node scripts/brand-icons.mjs（可设 XC_SHOTS_BROWSER）。
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { chromium } from "playwright-core";

const publicDir = join(import.meta.dirname, "..", "public");
const source = readFileSync(join(publicDir, "favicon.svg"), "utf8");
const square = source.replace('rx="16"', 'rx="0"');
const badge = source.replace(/<rect[^>]*\/>/, "");
const outputs = [
  ["icons/icon-192.png", 192, source],
  ["icons/icon-512.png", 512, source],
  ["icons/apple-touch-icon.png", 180, square],
  ["icons/maskable-512.png", 512, square],
  ["icons/notify/app.png", 192, source],
  ["icons/notify/app-badge.png", 96, badge],
];
const browser = await chromium.launch(
  process.env.XC_SHOTS_BROWSER
    ? { executablePath: process.env.XC_SHOTS_BROWSER }
    : {},
);
try {
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  for (const [name, size, svg] of outputs) {
    await page.setContent(
      `<style>body{margin:0}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`,
    );
    await page.locator("svg").screenshot({
      path: join(publicDir, name),
      omitBackground: true,
    });
    console.log(name, size);
  }
} finally {
  await browser.close();
}
