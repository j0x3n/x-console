import { useSyncExternalStore } from "react";

/*
 * B93：路由器状态多久刷新一次。路由器页、左栏和今日页共用，存在这台浏览器上。
 * 0 表示暂停。
 */
export const INTERVALS = [1000, 2000, 5000, 10_000, 30_000, 60_000, 0] as const;
export type RouterInterval = (typeof INTERVALS)[number];

const KEY = "xc.router.interval";
const DEFAULT: RouterInterval = 5000;
const listeners = new Set<() => void>();

function read(): RouterInterval {
  try {
    const v = Number(localStorage.getItem(KEY));
    return (INTERVALS as readonly number[]).includes(v) &&
      localStorage.getItem(KEY) !== null
      ? (v as RouterInterval)
      : DEFAULT;
  } catch {
    return DEFAULT;
  }
}

let current = read();

export function setRouterInterval(v: RouterInterval) {
  current = v;
  try {
    localStorage.setItem(KEY, String(v));
  } catch {
    /* 存不了就只在这次打开的页面里生效 */
  }
  listeners.forEach((fn) => fn());
}

export function useRouterInterval(): RouterInterval {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    () => current,
  );
}

/** “每 5 秒”“每 1 分钟”“已暂停”。 */
export function intervalLabel(v: number): string {
  if (v === 0) return "已暂停";
  if (v < 60_000) return `每 ${v / 1000} 秒`;
  return `每 ${v / 60_000} 分钟`;
}
