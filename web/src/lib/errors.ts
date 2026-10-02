import { create } from "zustand";
import { API_BASE, ApiError, recentApiErrors } from "../api/client";
import { usePreferencesStore } from "../stores/preferences-store";
import { isChunkLoadError, reloadOnce } from "./chunkReload";
import { translate } from "./i18n";

/*
 * 报错统一出口（B41）。所有报错都走 reportError：
 * - 控制台打印完整对象；
 * - 界面上的报错列表加一条，不自动消失；
 * - 同时写进服务器日志（手机上看不到控制台）。
 */

export interface ErrorNotice {
  id: number;
  /** 去重用：同一个请求、同一个错误码、同一段文字 10 秒内只显示一条。 */
  key: string;
  title: string;
  message: string;
  /** 复制用的完整文本。 */
  detail: string;
  requestId?: string;
  count: number;
  at: number;
  lastAt: number;
}

export interface ReportOptions {
  /** 标题，比如“保存笔记失败”。默认“请求失败”或“页面出错”。 */
  title?: string;
  /** 正文。默认用错误自己的 message。 */
  message?: string;
  /** 只打印到控制台，不在界面上显示。 */
  silent?: boolean;
}

interface ErrorState {
  notices: ErrorNotice[];
  history: ErrorNotice[];
  dismiss: (id: number) => void;
  dismissAll: () => void;
}

export const DEDUPE_MS = 10_000;
const HISTORY_LIMIT = 50;
const MAX_BODY = 4096;

export const useErrorStore = create<ErrorState>()((set) => ({
  notices: [],
  history: [],
  dismiss: (id) =>
    set((s) => ({ notices: s.notices.filter((n) => n.id !== id) })),
  dismissAll: () => set({ notices: [] }),
}));

let sequence = 0;
let now = () => Date.now();

/** 测试用：固定当前时间。 */
export function setErrorClock(fn: () => number) {
  now = fn;
}

/*
 * 页面自己用 toast({ tone: "error" }) 报错时，通常只传了文字。
 * 按文字对上最近的接口报错，把请求详情附上，方便复制。
 */
function findRecentApiError(texts: (string | undefined)[]) {
  const cutoff = now() - 5_000;
  return recentApiErrors.find(
    (error) =>
      (error.request?.at ?? 0) >= cutoff &&
      texts.some((text) => text && text === error.message),
  );
}

function t(text: string) {
  return translate(usePreferencesStore.getState().language, text);
}

/** 这些不算报错：没登录（会跳登录页）、用户主动取消。 */
export function isIgnoredError(error: unknown): boolean {
  if (error instanceof DOMException && error.name === "AbortError") return true;
  if (error instanceof Error && error.name === "AbortError") return true;
  if (error instanceof ApiError && error.status === 401) return true;
  // 需要再次验证时，验证框会弹出来，验证后自动重试。
  if (error instanceof ApiError && error.code === "elevation_required")
    return true;
  return false;
}

function pad(n: number) {
  return String(n).padStart(2, "0");
}

export function formatTime(ms: number) {
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function errorText(error: unknown): string {
  if (error instanceof Error) return error.message || error.name;
  if (typeof error === "string") return error;
  try {
    return JSON.stringify(error);
  } catch {
    return String(error);
  }
}

/** 拼出复制用的纯文本，格式固定，方便贴给 AI 或写进 Issue。 */
export function buildDetail(
  title: string,
  message: string,
  error: unknown,
  at: number,
): { detail: string; requestId?: string } {
  const lines = [title, `时间：${formatTime(at)}`];
  let requestId: string | undefined;
  if (error instanceof ApiError) {
    const info = error.request;
    requestId = info?.requestId;
    if (info) lines.push(`请求：${info.method} ${info.path}`);
    lines.push(`状态：${error.status} ${error.code}`);
  }
  lines.push(`信息：${message}`);
  if (requestId) lines.push(`请求编号：${requestId}`);
  if (typeof location !== "undefined")
    lines.push(`页面：${location.pathname}${location.search}`);
  if (typeof navigator !== "undefined")
    lines.push(`浏览器：${navigator.userAgent}`);
  if (error instanceof ApiError) {
    const body = error.request?.body;
    if (body) lines.push(`响应：${body.slice(0, MAX_BODY)}`);
  }
  if (error instanceof Error && error.stack)
    lines.push(`调用栈：\n${error.stack}`);
  return { detail: lines.join("\n"), requestId };
}

/** 已经显示过的错误对象对应哪条提示。同一个错误再报一次时只更新标题。 */
const noticeOf = new WeakMap<object, number>();

export function reportError(error: unknown, options: ReportOptions = {}) {
  if (isIgnoredError(error)) return;
  const at = now();
  if (error && typeof error === "object" && noticeOf.has(error)) {
    // 全局兜底先报了“请求失败”，页面随后用自己的标题又报一次：合并成一条。
    const id = noticeOf.get(error);
    if (options.title) {
      const rename = (n: ErrorNotice) =>
        n.id === id
          ? {
              ...n,
              title: options.title!,
              detail: n.detail.replace(/^[^\n]*/, options.title!),
            }
          : n;
      useErrorStore.setState((s) => ({
        notices: s.notices.map(rename),
        history: s.history.map(rename),
      }));
    }
    return;
  }
  const api = error instanceof ApiError ? error : undefined;
  const info = api?.request;
  const title =
    options.title ?? t(api || info ? "Request failed" : "Page error");
  const message = options.message ?? errorText(error);
  console.error("[X Console]", title, message, error);
  if (options.silent) return;

  // 去重不看查询参数：同一个列表的几个查询一起失败时只显示一条。
  const pathname = info?.path.split("?")[0];
  const key = [info?.method, pathname, api?.code, title, message].join("|");
  const state = useErrorStore.getState();
  const same = state.notices.find(
    (n) => n.key === key && at - n.lastAt < DEDUPE_MS,
  );
  if (same) {
    useErrorStore.setState({
      notices: state.notices.map((n) =>
        n.id === same.id ? { ...n, count: n.count + 1, lastAt: at } : n,
      ),
    });
    return;
  }
  const { detail, requestId } = buildDetail(title, message, error, at);
  const notice: ErrorNotice = {
    id: ++sequence,
    key,
    title,
    message,
    detail,
    requestId,
    count: 1,
    at,
    lastAt: at,
  };
  if (error && typeof error === "object") noticeOf.set(error, notice.id);
  useErrorStore.setState({
    notices: [notice, ...state.notices],
    history: [notice, ...state.history].slice(0, HISTORY_LIMIT),
  });
  if (!info?.path.includes("/client-errors")) sendToServer(notice);
}

/**
 * toast({ tone: "error" }) 转过来的报错。页面只给了文字，
 * 能对上最近的接口报错时，用那个错误的详情。
 */
export function reportErrorToast(message: string, subtitle?: string) {
  const api = findRecentApiError([subtitle, message]);
  const body = subtitle && subtitle !== message ? subtitle : undefined;
  reportError(api ?? new Error(body ?? message), {
    title: body ? message : undefined,
    message: body ?? message,
  });
}

function sendToServer(notice: ErrorNotice) {
  if (typeof fetch === "undefined" || import.meta.env?.MODE === "test") return;
  const page =
    typeof location !== "undefined" ? location.pathname + location.search : "";
  void fetch(API_BASE + "/client-errors", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "x-console",
    },
    body: JSON.stringify({
      title: notice.title.slice(0, 200),
      message: notice.message.slice(0, 2000),
      detail: notice.detail.slice(0, 8192),
      page: page.slice(0, 500),
      requestId: notice.requestId?.slice(0, 100),
    }),
  }).catch(() => undefined);
}

/** 页面级的报错：未处理的异常和 Promise。只装一次。 */
let installed = false;
export function installGlobalErrorHandlers() {
  if (installed || typeof window === "undefined") return;
  installed = true;
  window.addEventListener("error", (event) => {
    // 图片等资源加载失败也会触发 error 事件，但没有 error 对象，不算。
    if (!event.error && !event.message) return;
    reportError(event.error ?? new Error(event.message));
  });
  window.addEventListener("unhandledrejection", (event) => {
    if (isChunkLoadError(event.reason) && reloadOnce()) return;
    reportError(event.reason);
  });
  // Vite 预加载按需文件失败时发这个事件，刷新一次拿新版本
  window.addEventListener("vite:preloadError", (event) => {
    if (reloadOnce()) event.preventDefault();
  });
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // 没有剪贴板权限（比如非 HTTPS）时用旧办法。
    try {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      const ok = document.execCommand("copy");
      area.remove();
      return ok;
    } catch {
      return false;
    }
  }
}
