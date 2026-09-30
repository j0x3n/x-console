import { useState } from "react";
import { copyText } from "../../lib/errors";

/*
 * B45：打开了 ?safari= 测试开关时，在页面顶部显示当前开关，
 * 点按钮复制视口数据发给审查者。不是固定定位，不影响 Safari 判断。
 */
function measure(unit: string) {
  const probe = document.createElement("div");
  probe.style.cssText = `position:absolute;visibility:hidden;height:100${unit}`;
  document.body.appendChild(probe);
  const h = probe.getBoundingClientRect().height;
  probe.remove();
  return Math.round(h);
}

function safeAreas() {
  const probe = document.createElement("div");
  probe.style.cssText =
    "position:absolute;visibility:hidden;padding:env(safe-area-inset-top) env(safe-area-inset-right) env(safe-area-inset-bottom) env(safe-area-inset-left)";
  document.body.appendChild(probe);
  const s = getComputedStyle(probe);
  const out = `上 ${s.paddingTop} 右 ${s.paddingRight} 下 ${s.paddingBottom} 左 ${s.paddingLeft}`;
  probe.remove();
  return out;
}

export function viewportReport(flags: string) {
  const mode = ["fullscreen", "standalone", "minimal-ui", "browser"].find(
    (m) => matchMedia(`(display-mode: ${m})`).matches,
  );
  const scroller = document.scrollingElement;
  const vv = window.visualViewport;
  const meta = (name: string) =>
    document.querySelector(`meta[name="${name}"]`)?.getAttribute("content") ??
    "（没有）";
  return [
    `开关：${flags}`,
    `浏览器：${navigator.userAgent}`,
    `显示模式：${mode ?? "未知"}`,
    `innerHeight ${innerHeight}，outerHeight ${outerHeight}，screen.height ${screen.height}`,
    `visualViewport ${vv ? `${Math.round(vv.height)}，offsetTop ${Math.round(vv.offsetTop)}` : "（没有）"}`,
    `100vh ${measure("vh")}，100svh ${measure("svh")}，100lvh ${measure("lvh")}，100dvh ${measure("dvh")}`,
    `安全区：${safeAreas()}`,
    `文档滚动：scrollHeight ${scroller?.scrollHeight}，clientHeight ${scroller?.clientHeight}，scrollY ${Math.round(scrollY)}`,
    `viewport：${meta("viewport")}`,
    `theme-color：${meta("theme-color")}`,
    `背景：html ${getComputedStyle(document.documentElement).backgroundColor}，body ${getComputedStyle(document.body).backgroundColor}`,
  ].join("\n");
}

export default function SafariProbe() {
  const flags = document.documentElement.dataset.safari;
  const [copied, setCopied] = useState(false);
  if (!flags) return null;
  return (
    <div className="safari-probe">
      <span>Safari 测试开关：{flags}</span>
      <button
        type="button"
        onClick={async () => {
          if (await copyText(viewportReport(flags))) {
            setCopied(true);
            window.setTimeout(() => setCopied(false), 2000);
          }
        }}
      >
        {copied ? "已复制" : "复制视口数据"}
      </button>
      <a href="?safari=off">关闭开关</a>
    </div>
  );
}
