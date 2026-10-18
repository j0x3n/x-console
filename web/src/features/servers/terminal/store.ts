import { create } from "zustand";

/*
 * 后台终端的列表（B249）。顶栏的“后台”按钮和隐藏容器只读这个文件，不引 xterm，
 * xterm 和连接在 sessions.ts 里，打开终端页时才加载。
 * 每台机器最多一个会话，按机器 id 记。
 */

export type TermStatus = "connecting" | "open" | "closed";

export interface TermInfo {
  hostId: string;
  hostName: string;
  /** 回到这个终端的地址 */
  path: string;
  status: TermStatus;
  /** shell 设的窗口标题，没有时是空字符串 */
  title: string;
  /** 进后台的时间（毫秒），正在看时是 null */
  parkedAt: number | null;
}

export const MAX_BACKGROUND = 4;
export const BACKGROUND_TTL_MS = 30 * 60 * 1000;
/** 这么久内有输出就当作命令还在跑 */
const BUSY_MS = 3000;

interface TermState {
  sessions: Record<string, TermInfo>;
}

export const useTerminals = create<TermState>()(() => ({ sessions: {} }));

export function patchSession(hostId: string, change: Partial<TermInfo>) {
  useTerminals.setState((s) => {
    const cur = s.sessions[hostId];
    if (!cur) return s;
    return { sessions: { ...s.sessions, [hostId]: { ...cur, ...change } } };
  });
}

export function removeSession(hostId: string) {
  useTerminals.setState((s) => {
    if (!s.sessions[hostId]) return s;
    const next = { ...s.sessions };
    delete next[hostId];
    return { sessions: next };
  });
}

export function sessionOf(hostId: string): TermInfo | undefined {
  return useTerminals.getState().sessions[hostId];
}

/** 留在后台的会话，最早进后台的在前。 */
export function backgroundSessions(
  sessions: Record<string, TermInfo>,
): TermInfo[] {
  return Object.values(sessions)
    .filter((s) => s.parkedAt !== null)
    .sort((a, b) => (a.parkedAt ?? 0) - (b.parkedAt ?? 0));
}

/** sessions.ts 为每个会话登记的操作。 */
export interface Handle {
  close: () => void;
  lastOutputAt: () => number;
}
const handles = new Map<string, Handle>();

export function registerHandle(hostId: string, handle: Handle) {
  handles.set(hostId, handle);
}
export function unregisterHandle(hostId: string, handle: Handle) {
  if (handles.get(hostId) === handle) handles.delete(hostId);
}

/** 断开并丢掉会话，代理端的 shell 随之退出。 */
export function disconnect(hostId: string) {
  const handle = handles.get(hostId);
  if (handle) handle.close();
  else removeSession(hostId);
}

/** 最近几秒有输出，当作命令还在跑。 */
export function isBusy(hostId: string, now = Date.now()): boolean {
  const handle = handles.get(hostId);
  return !!handle && now - handle.lastOutputAt() < BUSY_MS;
}

/** 后台超过 30 分钟没看的会话断开。返回断开了哪些。 */
export function expireSessions(now = Date.now()): TermInfo[] {
  const old = backgroundSessions(useTerminals.getState().sessions).filter(
    (s) => now - (s.parkedAt ?? now) >= BACKGROUND_TTL_MS,
  );
  for (const s of old) disconnect(s.hostId);
  return old;
}

// 隐藏容器。TerminalDock 挂载时登记自己，没挂载时（单独渲染页面的测试）临时建一个。
let dock: HTMLElement | null = null;
export function setDock(el: HTMLElement | null) {
  dock = el;
}
export function parkingLot(): HTMLElement {
  if (dock?.isConnected) return dock;
  let el = document.getElementById("xc-term-dock-fallback");
  if (!el) {
    el = document.createElement("div");
    el.id = "xc-term-dock-fallback";
    hideDock(el);
    document.body.appendChild(el);
  }
  return el;
}

/** 隐藏容器放在屏幕外面，保留大小，xterm 照常收数据。 */
export function hideDock(el: HTMLElement) {
  el.setAttribute("aria-hidden", "true");
  Object.assign(el.style, {
    position: "fixed",
    left: "-10000px",
    top: "0",
    width: "960px",
    height: "540px",
    overflow: "hidden",
    visibility: "hidden",
    pointerEvents: "none",
  });
}
