import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { create } from "zustand";

/*
 * 页面自己的按钮（新建、调整布局、同步……）显示在顶栏中段，
 * 和右边的全局按钮用一条竖线隔开（B20）。
 * 页面里写 <PageActions>按钮</PageActions>，离开页面时自动清掉。
 * 手机上按钮只显示图标，所以每个按钮都要有图标和 aria-label 或 title。
 */
interface SlotState {
  el: HTMLElement | null;
  setEl: (el: HTMLElement | null) => void;
}

const useSlot = create<SlotState>()((set) => ({
  el: null,
  setEl: (el) => set({ el }),
}));

export default function PageActions({ children }: { children: ReactNode }) {
  const el = useSlot((s) => s.el);
  // 没有顶栏时（比如单独渲染页面的测试）就地显示。
  if (!el) return <div className="page-actions-inline xc-row">{children}</div>;
  return createPortal(children, el);
}

/** 顶栏里放页面按钮的位置。只在 Topbar 里用一次。 */
export function PageActionsSlot() {
  const setEl = useSlot((s) => s.setEl);
  return <div className="page-actions" ref={setEl} />;
}
