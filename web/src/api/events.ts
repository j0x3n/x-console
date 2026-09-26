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

function dispatch(event: ServerEvent) {
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
    };
  }, []);
}
