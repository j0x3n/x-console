/*
 * 演示数据（临时）。后端还没做的接口在浏览器里拦下来，返回内存里的假数据，
 * 方便先看界面效果。用户确认界面后整个目录删掉，同时删掉：
 * - main.tsx 第一行的 import "./demo"
 * - api/events.ts 的 emitDemoEvent
 * - features/drive/api.ts 的 demoFileUrl
 *
 * 覆盖：今日页布局、隐藏内容（含隐藏笔记）、云盘、AI 助手、自动化。
 * 不覆盖：登录和两步验证（假的会让人误以为安全设置生效了）、笔记图片附件。
 */
import { findRoute } from "./router";
import { fileUrl } from "./files";
import { demoUpload } from "./drive";
import "./vault";
import "./ai";
import "./automations";
import "./dashboard";

const API = "/api/v1";
const realFetch = globalThis.fetch.bind(globalThis);

async function handle(input: RequestInfo | URL, init?: RequestInit) {
  // 先只看地址和方法。不拦的请求一点都不碰，免得读掉它的请求体。
  const isReq = input instanceof Request;
  const url = new URL(isReq ? input.url : String(input), location.href);
  const method = (isReq ? input.method : (init?.method ?? "GET")).toUpperCase();
  if (url.origin !== location.origin || !url.pathname.startsWith(`${API}/`))
    return undefined;
  const path = url.pathname.slice(API.length);
  const hit = findRoute(method, path);
  if (!hit) return undefined;
  let body: unknown = undefined;
  if (method !== "GET") {
    const copy = isReq ? input.clone() : new Request(url, init);
    const text = await copy.text();
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = text;
      }
    }
  }
  const res = await hit.handler({
    method,
    path,
    query: url.searchParams,
    params: hit.params,
    body,
  });
  if (res) await new Promise((r) => setTimeout(r, 120)); // 像真的请求一样有一点延迟
  return res;
}

globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) =>
  (await handle(input, init)) ?? realFetch(input, init)) as typeof fetch;

// 云盘的图片、PDF 用本地 blob 地址显示。
(
  globalThis as { xcDemoFileUrl?: (id: number) => string | undefined }
).xcDemoFileUrl = fileUrl;

// 云盘上传用的是 XMLHttpRequest，只拦上传地址，其余交给真的。
const RealXHR = globalThis.XMLHttpRequest;
class DemoXHR extends RealXHR {
  private demoUrl: URL | null = null;
  private demoStatus = 0;
  private demoText = "";
  open(method: string, url: string | URL, ...rest: unknown[]) {
    const u = new URL(String(url), location.href);
    if (method === "POST" && u.pathname === `${API}/drive/upload`)
      this.demoUrl = u;
    else (super.open as (...a: unknown[]) => void)(method, url, ...rest);
  }
  setRequestHeader(name: string, value: string) {
    if (!this.demoUrl) super.setRequestHeader(name, value);
  }
  get status() {
    return this.demoUrl ? this.demoStatus : super.status;
  }
  get responseText() {
    return this.demoUrl ? this.demoText : super.responseText;
  }
  send(body?: Document | XMLHttpRequestBodyInit | null) {
    if (!this.demoUrl) return super.send(body);
    const file = body instanceof FormData ? (body.get("file") as File) : null;
    const total = file?.size ?? 0;
    let loaded = 0;
    const step = () => {
      loaded = Math.min(total, loaded + Math.max(total / 8, 1));
      const onProgress = this.upload.onprogress as
        | ((e: ProgressEvent) => void)
        | null;
      onProgress?.(
        new ProgressEvent("progress", {
          loaded,
          total,
          lengthComputable: true,
        }),
      );
      if (loaded < total) return void setTimeout(step, 120);
      const item = file ? demoUpload(this.demoUrl!.searchParams, file) : null;
      this.demoStatus = 201;
      this.demoText = JSON.stringify({ items: item ? [item] : [] });
      const onLoad = this.onload as ((e: ProgressEvent) => void) | null;
      onLoad?.(new ProgressEvent("load"));
    };
    setTimeout(step, 120);
  }
}
globalThis.XMLHttpRequest = DemoXHR as typeof XMLHttpRequest;

// 页面左下角的小标记，提醒这些是假数据。
function badge() {
  const el = document.createElement("div");
  el.textContent = "演示数据";
  el.title =
    "云盘、AI 助手、自动化、隐藏内容、今日页布局用的是假数据，刷新页面会重置。";
  el.setAttribute(
    "style",
    "position:fixed;left:50%;transform:translateX(-50%);bottom:10px;z-index:60;padding:3px 8px;border-radius:999px;" +
      "background:#e8b454;color:#1d1d21;font:500 11px/1.4 system-ui,sans-serif;opacity:.85;pointer-events:auto;cursor:help",
  );
  document.body.appendChild(el);
}
if (document.readyState === "loading")
  document.addEventListener("DOMContentLoaded", badge);
else badge();
