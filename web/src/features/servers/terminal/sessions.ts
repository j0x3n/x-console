import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { wsUrl } from "../../../api/client";
import {
  backgroundSessions,
  disconnect,
  MAX_BACKGROUND,
  parkingLot,
  patchSession,
  registerHandle,
  removeSession,
  sessionOf,
  unregisterHandle,
  useTerminals,
  type Handle,
  type TermInfo,
} from "./store";

/*
 * 终端会话（B249）。xterm 和 WebSocket 放在组件外面，页面卸载时不跟着关：
 * - 终端页显示时把会话的元素挪进页面的占位元素；
 * - 离开时选了“留在后台”就挪进隐藏容器（TerminalDock），不断开；
 * - 后台最多留 4 个，30 分钟没看自动断开（store.ts 的 expireSessions）。
 */

interface Live {
  node: HTMLDivElement;
  term: Terminal;
  fit: FitAddon;
  ws: WebSocket;
  lastOutputAt: number;
  handle: Handle;
}

const lives = new Map<string, Live>();

function cssVar(name: string, fallback: string) {
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return v || fallback;
}

function safeFit(fit: FitAddon) {
  try {
    fit.fit();
  } catch {
    /* 容器还没显示出来 */
  }
}

export interface OpenOptions {
  hostId: string;
  hostName: string;
  path: string;
  /** 新会话显示在哪里 */
  slot: HTMLElement;
  /** 连接断开时写进终端的一行，参数是服务端给的原因 */
  closedLine: (reason: string) => string;
}

/** 新开一个会话。这台机器已有的会话先关掉。 */
export function openSession({
  hostId,
  hostName,
  path,
  slot,
  closedLine,
}: OpenOptions) {
  disconnect(hostId);
  const node = document.createElement("div");
  node.className = "servers-terminal-screen";
  slot.appendChild(node);
  const term = new Terminal({
    cursorBlink: true,
    fontFamily: cssVar("--xc-mono", "monospace"),
    fontSize: 13,
    scrollback: 5000,
    theme: {
      background: cssVar("--servers-term-bg", "#0a0a0b"),
      foreground: cssVar("--servers-term-fg", "#ededf0"),
      cursor: cssVar("--xc-accent", "#cc7752"),
      selectionBackground: "#5c5c6480",
    },
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.open(node);
  safeFit(fit);
  const ws = new WebSocket(
    wsUrl(
      `/hosts/${encodeURIComponent(hostId)}/terminal?cols=${term.cols}&rows=${term.rows}`,
    ),
  );
  ws.binaryType = "arraybuffer";
  const live: Live = {
    node,
    term,
    fit,
    ws,
    lastOutputAt: 0,
    handle: {
      close: () => closeLive(hostId, live),
      lastOutputAt: () => live.lastOutputAt,
    },
  };
  lives.set(hostId, live);
  registerHandle(hostId, live.handle);
  useTerminals.setState((s) => ({
    sessions: {
      ...s.sessions,
      [hostId]: {
        hostId,
        hostName,
        path,
        status: "connecting",
        title: "",
        parkedAt: null,
      },
    },
  }));

  const encoder = new TextEncoder();
  ws.onopen = () => {
    patchSession(hostId, { status: "open" });
    term.focus();
  };
  ws.onmessage = (event) => {
    if (!(event.data instanceof ArrayBuffer)) return;
    live.lastOutputAt = Date.now();
    term.write(new Uint8Array(event.data));
  };
  ws.onclose = (event) => {
    if (lives.get(hostId) !== live) return;
    // 后台的连接断了就整个丢掉，后台列表里不再显示
    if (sessionOf(hostId)?.parkedAt != null) {
      closeLive(hostId, live);
      return;
    }
    patchSession(hostId, { status: "closed" });
    const reason =
      event.reason && event.reason !== "terminal closed" ? event.reason : "";
    term.write(closedLine(reason));
  };
  term.onData((data) => {
    if (ws.readyState === WebSocket.OPEN) ws.send(encoder.encode(data));
  });
  term.onBinary((data) => {
    if (ws.readyState !== WebSocket.OPEN) return;
    ws.send(Uint8Array.from(data, (c) => c.charCodeAt(0) & 0xff));
  });
  term.onResize(({ cols, rows }) => {
    if (ws.readyState === WebSocket.OPEN)
      ws.send(JSON.stringify({ type: "resize", cols, rows }));
  });
  term.onTitleChange((title) =>
    patchSession(hostId, { title: title.slice(0, 80) }),
  );
}

function closeLive(hostId: string, live: Live) {
  if (lives.get(hostId) === live) lives.delete(hostId);
  unregisterHandle(hostId, live.handle);
  live.ws.onclose = null;
  live.ws.close();
  live.term.dispose();
  live.node.remove();
  if (!lives.has(hostId)) removeSession(hostId);
}

// 每台机器有几个终端页正在显示。StrictMode 会卸载再挂载一次，离开时等一下再看还有没有人在看。
const viewers = new Map<string, number>();

/** 终端页显示时调用：有会话就挪进页面里，记成正在看。 */
export function showSession(hostId: string, slot: HTMLElement) {
  viewers.set(hostId, (viewers.get(hostId) ?? 0) + 1);
  const live = lives.get(hostId);
  if (!live) return;
  if (live.node.parentElement !== slot) slot.appendChild(live.node);
  patchSession(hostId, { parkedAt: null });
  requestAnimationFrame(() => safeFit(live.fit));
}

/**
 * 终端页卸载时调用。keep 留在后台，close 断开。
 * onEvicted：超过上限时关掉了哪个会话。
 */
export function leaveSession(
  hostId: string,
  choice: "keep" | "close",
  path: string,
  onEvicted: (info: TermInfo) => void,
) {
  viewers.set(hostId, Math.max(0, (viewers.get(hostId) ?? 1) - 1));
  setTimeout(() => {
    if ((viewers.get(hostId) ?? 0) > 0 || !lives.has(hostId)) return;
    if (choice === "close") {
      disconnect(hostId);
      return;
    }
    const evicted = parkSession(hostId, path);
    if (evicted) onEvicted(evicted);
  }, 0);
}

/**
 * 留在后台：挪进隐藏容器，不断开。返回因为超过上限被关掉的那个会话。
 * 只有连着的会话能留，其他的直接关掉。
 */
export function parkSession(
  hostId: string,
  path?: string,
  now = Date.now(),
): TermInfo | undefined {
  const live = lives.get(hostId);
  if (!live || sessionOf(hostId)?.status !== "open") {
    disconnect(hostId);
    return undefined;
  }
  parkingLot().appendChild(live.node);
  patchSession(hostId, { parkedAt: now, ...(path ? { path } : {}) });
  const parked = backgroundSessions(useTerminals.getState().sessions);
  if (parked.length <= MAX_BACKGROUND) return undefined;
  const oldest = parked[0];
  disconnect(oldest.hostId);
  return oldest;
}

export function fitSession(hostId: string) {
  const live = lives.get(hostId);
  if (live) safeFit(live.fit);
}

/** 脚本面板把一段输入送进终端，和键盘输入走同一条路。 */
export function sendInput(hostId: string, text: string) {
  const live = lives.get(hostId);
  if (live?.ws.readyState !== WebSocket.OPEN) return;
  live.ws.send(new TextEncoder().encode(text));
  live.term.focus();
}
