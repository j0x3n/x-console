import { useEffect } from "react";
import type { QueryKey } from "@tanstack/react-query";
import { create } from "zustand";
import { wsUrl } from "./client";
import { queryClient } from "./query";

/*
 * 服务端事件。后端每次 events.Bus.Publish 都会推到这里。
 * 模块用 invalidateOn("issue.", ["projects"]) 声明：
 * 收到 issue.* 事件时刷新 queryKey 以 ["projects"] 开头的查询。
 * 需要逐条处理时用 onServerEvent。
 */
export interface ServerEvent<T = unknown> {
  topic: string;
  data: T;
  at: string;
}

type Listener = (event: ServerEvent) => void;
const listeners = new Set<Listener>();
const invalidations: Array<[string, QueryKey]> = [];
const topics = new Map<string, number>();
let activeSocket: WebSocket | null = null;

function sendControl(message: object) {
  if (activeSocket?.readyState === WebSocket.OPEN)
    activeSocket.send(JSON.stringify(message));
}

function sendTopics() {
  sendControl({ type: "subscribe", topics: [...topics.keys()] });
}

/*
 * 主机详情的刷新周期（B26）：1 秒、5 秒、30 秒。服务端对同一台机器取所有浏览器里最小的，
 * 告诉代理按这个周期上报。旧服务端不认识这条消息，会忽略，按 5 秒刷新。
 */
const intervals = new Map<string, number>();

function sendInterval(hostId: string, ms: number) {
  sendControl({ type: "interval", hostId, ms });
}

/** 组件挂载期间把这台机器的刷新周期设成 ms，离开时恢复默认。 */
export function useMetricsInterval(hostId: string, ms: number) {
  useEffect(() => {
    if (!hostId) return;
    intervals.set(hostId, ms);
    sendInterval(hostId, ms);
    return () => {
      intervals.delete(hostId);
      sendInterval(hostId, 0);
    };
  }, [hostId, ms]);
}

/** 组件挂载期间订阅高频主题，离开时释放。 */
export function useEventTopic(topic: string) {
  useEffect(() => {
    if (!topic) return;
    topics.set(topic, (topics.get(topic) ?? 0) + 1);
    sendTopics();
    return () => {
      const count = (topics.get(topic) ?? 1) - 1;
      if (count > 0) topics.set(topic, count);
      else topics.delete(topic);
      sendTopics();
    };
  }, [topic]);
}

export function onServerEvent(fn: Listener) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

export function useServerEvent(prefix: string, fn: Listener) {
  useEffect(
    () =>
      onServerEvent((event) => {
        if (event.topic.startsWith(prefix)) fn(event);
      }),
    [prefix, fn],
  );
}

export function invalidateOn(prefix: string, key: QueryKey) {
  invalidations.push([prefix, key]);
}

interface ConnectionState {
  connected: boolean;
  setConnected: (connected: boolean) => void;
}
export const useEventConnection = create<ConnectionState>()((set) => ({
  connected: false,
  setConnected: (connected) => set({ connected }),
}));

/*
 * 高频事件（监控数据、智能家居状态）在页面切到后台时不处理，省 CPU。
 * 记下漏掉的前缀，切回来时按前缀刷新一次对应的查询。
 */
const backgroundSkip: Array<[string, QueryKey]> = [
  ["host.metrics", ["hosts"]],
  ["ha.state_changed", ["ha"]],
];
const skipped = new Set<string>();
if (typeof document !== "undefined")
  document.addEventListener("visibilitychange", () => {
    sendControl({ type: document.hidden ? "pause" : "resume" });
    if (document.hidden) return;
    for (const [prefix, key] of backgroundSkip)
      if (
        skipped.has(prefix) ||
        (prefix === "host.metrics"
          ? [...topics.keys()].some((topic) => topic.startsWith("host.metrics"))
          : topics.has("ha."))
      )
        queryClient.invalidateQueries({ queryKey: key });
    if (
      [...topics.keys()].some((topic) =>
        topic.startsWith("coding_task.output:"),
      )
    )
      queryClient.invalidateQueries({ queryKey: ["coding", "events"] });
    skipped.clear();
  });

function dispatch(event: ServerEvent) {
  if (typeof document !== "undefined" && document.hidden) {
    const skip = backgroundSkip.find(([p]) => event.topic.startsWith(p));
    if (skip) {
      skipped.add(skip[0]);
      return;
    }
  }
  const keys = new Map<string, QueryKey>();
  for (const [prefix, key] of invalidations) {
    if (event.topic.startsWith(prefix)) keys.set(JSON.stringify(key), key);
  }
  keys.forEach((key) => queryClient.invalidateQueries({ queryKey: key }));
  listeners.forEach((fn) => fn(event));
}

/** 在已登录的布局里调用一次。断线后自动重连。 */
export function useServerEvents() {
  useEffect(() => {
    let socket: WebSocket | null = null;
    let timer: number | undefined;
    let delay = 1000;
    let stopped = false;
    const connect = () => {
      socket = new WebSocket(wsUrl("/events"));
      socket.onopen = () => {
        activeSocket = socket;
        sendTopics();
        intervals.forEach((ms, hostId) => sendInterval(hostId, ms));
        if (document.hidden) sendControl({ type: "pause" });
        delay = 1000;
        useEventConnection.getState().setConnected(true);
        // 断线期间可能漏了事件，重连后整体刷新一次。
        queryClient.invalidateQueries();
      };
      socket.onmessage = (message) => {
        try {
          dispatch(JSON.parse(message.data));
        } catch {
          /* 忽略格式不对的消息 */
        }
      };
      socket.onclose = () => {
        if (activeSocket === socket) activeSocket = null;
        useEventConnection.getState().setConnected(false);
        if (stopped) return;
        timer = window.setTimeout(connect, delay);
        delay = Math.min(delay * 2, 30_000);
      };
    };
    connect();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
      socket?.close();
      if (activeSocket === socket) activeSocket = null;
    };
  }, []);
}
