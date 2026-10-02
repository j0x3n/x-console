// 生成通知图标（B91）：彩色底白色图案 192px，和 Android 用的单色 badge 96px。
// 用法：cd web && XC_SHOTS_BROWSER=<chromium 路径> node scripts/notify-icons.mjs
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import * as L from "lucide-react";
import { chromium } from "playwright-core";

const kinds = {
  app: ["X", "#cc7752"],
  server: ["Server", "#4b7bd8"],
  monitor: ["Radar", "#d1663d"],
  mail: ["Mail", "#3a8fd6"],
  repo: ["GitBranch", "#6b5bd2"],
  agent: ["Bot", "#c2653d"],
  weather: ["CloudSun", "#2f9fd6"],
  reminder: ["Bell", "#d6a12f"],
  habit: ["HeartPulse", "#d8505f"],
  water: ["Droplets", "#2f8fe0"],
  eyes: ["Eye", "#3aa37a"],
  move: ["Footprints", "#7c8a2f"],
  medicine: ["Pill", "#b5578c"],
  brief: ["Newspaper", "#5b6b80"],
  calendar: ["CalendarDays", "#3a9a5b"],
  drive: ["HardDrive", "#5b6b80"],
  router: ["Router", "#2f7f8f"],
  home: ["House", "#3a9a5b"],
};
const browser = await chromium.launch({
  executablePath: process.env.XC_SHOTS_BROWSER,
});
const page = await browser.newPage({ deviceScaleFactor: 1 });
for (const [name, [icon, color]] of Object.entries(kinds)) {
  const glyph = (size, stroke) =>
    icon === "X"
      ? `<span style="font:700 ${size * 0.62}px/1 system-ui,sans-serif;color:#fff">X</span>`
      : renderToStaticMarkup(
          createElement(L[icon], { size, color: "#fff", strokeWidth: stroke }),
        );
  // 大图标：圆角方块
  await page.setContent(
    `<div id=i style="width:192px;height:192px;border-radius:44px;background:${color};display:grid;place-items:center">${glyph(112, 2)}</div>`,
  );
  await page
    .locator("#i")
    .screenshot({
      path: `public/icons/notify/${name}.png`,
      omitBackground: true,
    });
  // 单色 badge：透明底白色图案
  await page.setContent(
    `<div id=i style="width:96px;height:96px;display:grid;place-items:center">${glyph(80, 2.4)}</div>`,
  );
  await page
    .locator("#i")
    .screenshot({
      path: `public/icons/notify/${name}-badge.png`,
      omitBackground: true,
    });
}
await browser.close();
console.log("ok", Object.keys(kinds).length);
