import { create } from "zustand";
import type { ToastMessage, ToastNotification } from "../types/domain";
import { reportErrorToast } from "../lib/errors";

/*
 * 全局提示。任何组件都可以调用 toast("已保存") 或
 * toast({ message: "保存失败", tone: "error" })。
 * 报错（tone: "error"）转到 lib/errors 的报错列表，不自动消失（B41）。
 */
interface ToastState {
  current: ToastNotification | null;
  show: (message: string | ToastMessage) => void;
  hide: () => void;
}

let sequence = 0;
let timer: number | undefined;

export const useToastStore = create<ToastState>()((set, get) => ({
  current: null,
  show: (message) => {
    const next = typeof message === "string" ? { message } : message;
    if (next.tone === "error") {
      reportErrorToast(next.message, next.subtitle);
      return;
    }
    window.clearTimeout(timer);
    set({ current: { ...next, id: ++sequence } });
    timer = window.setTimeout(() => get().hide(), 5000);
  },
  hide: () => {
    window.clearTimeout(timer);
    const current = get().current;
    if (!current) return;
    set({ current: { ...current, closing: true } });
    const id = current.id;
    window.setTimeout(() => {
      if (get().current?.id === id) set({ current: null });
    }, 170);
  },
}));

export const toast = (message: string | ToastMessage) =>
  useToastStore.getState().show(message);
