import { useSyncExternalStore, type ComponentType } from "react";

/*
 * 侧边栏一级菜单右边的数量标记（B76）和行内按钮（B72 的“+”）。
 * 模块在自己的 routes.tsx 里登记：
 *   registerNavBadge("/mail", useMailBadge)
 *   registerNavAction("/notes", { icon: Plus, label: "New note", run })
 * 标记的 Hook 在侧边栏每次渲染时调用，只能用已有的查询（和页面共用缓存），不要轮询。
 */

export interface NavBadge {
  count: number;
  /** danger 浅红底（默认），warn 浅黄底 */
  tone?: "danger" | "warn";
  /** 鼠标悬停的说明，比如“3 封未读” */
  title: string;
}

export type NavBadgeHook = () => NavBadge | null;

export interface NavAction {
  icon: ComponentType<{ size?: number }>;
  /** 英文原文，中文在 i18n */
  label: string;
  run: (navigate: (to: string) => void) => void;
}

interface Registry {
  badges: Record<string, NavBadgeHook>;
  actions: Record<string, NavAction>;
}

let registry: Registry = { badges: {}, actions: {} };
const subscribers = new Set<() => void>();
const emit = () => subscribers.forEach((fn) => fn());

export function registerNavBadge(path: string, hook: NavBadgeHook) {
  registry = { ...registry, badges: { ...registry.badges, [path]: hook } };
  emit();
}

export function registerNavAction(path: string, action: NavAction) {
  registry = { ...registry, actions: { ...registry.actions, [path]: action } };
  emit();
}

export function useNavExtras() {
  return useSyncExternalStore(
    (fn) => {
      subscribers.add(fn);
      return () => subscribers.delete(fn);
    },
    () => registry,
  );
}

/** 10 以上只显示一个点（用户要求：超过 9 个显示红点）。 */
export function badgeLabel(count: number): string {
  return count > 9 ? "" : String(count);
}
