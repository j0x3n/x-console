import { create } from "zustand";
import type { ToastMessage, ToastNotification } from "../types/domain";

/*
 * 全局提示。任何组件都可以调用 toast("已保存") 或
 * toast({ message: "保存失败", tone: "error" })。
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
    window.clearTimeout(timer);
    const next = typeof message === "string" ? { message } : message;
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
