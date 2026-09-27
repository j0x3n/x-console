import { useSyncExternalStore, type ComponentType } from "react";

/*
 * 侧边栏的二级菜单。模块在自己的 routes.tsx 里登记：
 *   registerNavChildren("/projects", ProjectsNavChildren)
 * 组件只在展开时渲染，这时才去拉数据。onNavigate 在点了链接后调用（手机上关掉侧边栏）。
 * 每个模块最多列 NAV_CHILD_LIMIT 条，多出来的给一个“全部”链接。
 */
export interface NavChildrenProps {
  onNavigate: () => void;
}

export const NAV_CHILD_LIMIT = 5;

let registry: Record<string, ComponentType<NavChildrenProps>> = {};
const subscribers = new Set<() => void>();

export function registerNavChildren(
  path: string,
  component: ComponentType<NavChildrenProps>,
) {
  registry = { ...registry, [path]: component };
  subscribers.forEach((fn) => fn());
}

export function useNavChildren() {
  return useSyncExternalStore(
    (fn) => {
      subscribers.add(fn);
      return () => subscribers.delete(fn);
    },
    () => registry,
  );
}
