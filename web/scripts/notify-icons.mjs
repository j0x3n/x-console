// 生成通知图标（B91）：彩色底白色图案 192px，和 Android 用的单色 badge 96px。
// 用法：cd web && XC_SHOTS_BROWSER=<chromium 路径> node scripts/notify-icons.mjs
// 生成后把 public/icons/notify/ 下的大图标同步复制到
// backend/internal/server/modules/reminders/icon_assets/（后端画进度环和图标要用）。
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import * as L from "lucide-react";
import { chromium } from "playwright-core";

// 底色用站标的两个颜色：紫是系统类，粉是个人类。
// app（通用图标）由 brand-icons.mjs 从 favicon.svg 生成，这里不要再生成。
const PINK = "#e0218a";
const PURPLE = "#7c3aed";
const kinds = {
  server: ["Server", PURPLE],
  monitor: ["Radar", PURPLE],
  mail: ["Mail", PURPLE],
  repo: ["GitBranch", PURPLE],
  agent: ["Bot", PURPLE],
  drive: ["HardDrive", PURPLE],
  router: ["Router", PURPLE],
  home: ["House", PURPLE],
  weather: ["CloudSun", PINK],
  reminder: ["Bell", PINK],
  habit: ["HeartPulse", PINK],
  water: ["Droplets", PINK],
  eyes: ["Eye", PINK],
  move: ["Footprints", PINK],
  medicine: ["Pill", PINK],
  brief: ["Newspaper", PINK],
  calendar: ["CalendarDays", PINK],
};
const browser = await chromium.launch({
  executablePath: process.env.XC_SHOTS_BROWSER,
});
const page = await browser.newPage({ deviceScaleFactor: 1 });
for (const [name, [icon, color]] of Object.entries(kinds)) {
  const glyph = (size, stroke) =>
    renderToStaticMarkup(
      createElement(L[icon], { size, color: "#fff", strokeWidth: stroke }),
    );
  // 大图标：圆角方块
  await page.setContent(
    `<div id=i style="width:192px;height:192px;border-radius:44px;background:${color};display:grid;place-items:center">${glyph(112, 2)}</div>`,
  );
  await page.locator("#i").screenshot({
    path: `public/icons/notify/${name}.png`,
    omitBackground: true,
  });
  // 单色 badge：透明底白色图案
  await page.setContent(
    `<div id=i style="width:96px;height:96px;display:grid;place-items:center">${glyph(80, 2.4)}</div>`,
  );
  await page.locator("#i").screenshot({
    path: `public/icons/notify/${name}-badge.png`,
    omitBackground: true,
  });
}
await browser.close();
console.log("ok", Object.keys(kinds).length);
