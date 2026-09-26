import { useEffect, useState, useSyncExternalStore } from "react";
import { create } from "zustand";

/** 媒体查询是否匹配，比如手机宽度。 */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (fn) => {
      if (typeof window === "undefined" || !window.matchMedia) return () => {};
      const mq = window.matchMedia(query);
      mq.addEventListener("change", fn);
      return () => mq.removeEventListener("change", fn);
    },
    () =>
      typeof window !== "undefined" &&
      !!window.matchMedia &&
      window.matchMedia(query).matches,
    () => false,
  );
}

/** 当前时间，每 period 毫秒更新一次。enabled 为 false 时不走表。 */
export function useNow(period: number, enabled = true): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    if (!enabled) return;
    setNow(new Date());
    const id = window.setInterval(() => setNow(new Date()), period);
    return () => window.clearInterval(id);
  }, [period, enabled]);
  return now;
}

/** 页头番茄钟弹框的开关，命令面板和 Issue 页面也能打开它。 */
interface FocusPanelState {
  open: boolean;
  issueKey: string;
  setOpen: (open: boolean) => void;
  openFor: (issueKey: string) => void;
}

export const useFocusPanel = create<FocusPanelState>()((set) => ({
  open: false,
  issueKey: "",
  setOpen: (open) => set({ open }),
  openFor: (issueKey) => set({ open: true, issueKey }),
}));
