import { chromium } from "../../../web/node_modules/playwright-core/index.mjs";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const dir = import.meta.dirname;
const concepts = [
  ["A", "X 字母", "保留产品名称的识别度。", "a-x.svg"],
  ["B", "终端", "突出命令执行和编码任务。", "b-terminal.svg"],
  ["C", "模块组合", "突出看板、笔记和日常管理。", "c-modules.svg"],
  ["D", "设备连接", "突出服务器、本机和智能家居。", "d-connect.svg"],
];
const icon = (file, size) => readFileSync(join(dir, file), "utf8").replace('<svg ', `<svg width="${size}" height="${size}" `);
const html = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>X Console 站标方案</title>
<style>
*{box-sizing:border-box}body{margin:0;background:#fafafa;color:#18181b;font-family:"Microsoft YaHei",system-ui,sans-serif;padding:34px 40px}
header{display:flex;align-items:center;justify-content:space-between;margin-bottom:24px}h1{font-size:24px;font-weight:600;margin:0}header span{font-size:13px;color:#71717a}
main{display:grid;grid-template-columns:1fr 1fr;gap:20px}.card{background:white;border:1px solid #e4e4e7;border-radius:14px;overflow:hidden}
.head{display:flex;align-items:center;padding:20px 24px 0;gap:10px}.letter{font-size:13px;color:#4f46e5;background:#eef2ff;border-radius:6px;padding:3px 8px;font-weight:600}h2{margin:0;font-size:17px;font-weight:600}.desc{margin:8px 24px 18px;font-size:13px;color:#71717a}
.samples{display:flex;height:146px;padding:0 24px;gap:16px}.sample{flex:1;border:1px solid #ececee;border-radius:10px;display:flex;align-items:center;justify-content:center;background:#fafafa}.dark{background:#111113;border-color:#26262b}
.usage{display:flex;align-items:center;justify-content:space-between;margin:18px 24px 20px}.brand{display:flex;align-items:center;gap:9px;font-size:14px;font-weight:600}.sizes{display:flex;align-items:center;gap:16px}.sizes>span{display:grid;justify-items:center;gap:5px}.sizes small{font-size:10px;color:#71717a}footer{margin-top:22px;color:#71717a;font-size:12px}
</style><header><h1>X Console · 站标方案</h1><span>默认靛蓝 / SVG 矢量 / 深浅背景与小尺寸预览</span></header><main>
${concepts.map(([id,title,desc,file])=>`<article class="card"><div class="head"><span class="letter">${id}</span><h2>${title}</h2></div><p class="desc">${desc}</p><div class="samples"><div class="sample">${icon(file,100)}</div><div class="sample dark">${icon(file,100)}</div></div><div class="usage"><div class="brand">${icon(file,32)}<span>X Console</span></div><div class="sizes">${[16,24,32].map(size=>`<span>${icon(file,size)}<small>${size}px</small></span>`).join("")}</div></div></article>`).join("")}
</main><footer>选定后统一用于浏览器标签、安装图标、登录页和左栏标识。</footer></html>`;
writeFileSync(join(dir,"preview.html"),html);
const browser = await chromium.launch(process.env.XC_SHOTS_BROWSER ? {executablePath:process.env.XC_SHOTS_BROWSER} : {});
try {
  const page = await browser.newPage({viewport:{width:1200,height:820},deviceScaleFactor:1});
  await page.setContent(html);
  await page.screenshot({path:join(dir,"preview.png"),fullPage:true});
  console.log("站标预览已生成："+join(dir,"preview.png"));
} finally { await browser.close(); }
