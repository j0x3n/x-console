import { create } from "zustand";
import { registerCommands } from "../lib/commands";
import { PanelLeft } from "lucide-react";

/* 桌面上二级菜单那一栏可以收起，只留图标栏（B20、B99）。状态记在 localStorage。 */
const KEY = "xc.sidebar.collapsed";

function read(): boolean {
  try {
    return localStorage.getItem(KEY) === "1";
  } catch {
    return false;
  }
}

interface SidebarState {
  collapsed: boolean;
  toggle: () => void;
}

export const useSidebar = create<SidebarState>()((set, get) => ({
  collapsed: read(),
  toggle: () => {
    const collapsed = !get().collapsed;
    try {
      localStorage.setItem(KEY, collapsed ? "1" : "0");
    } catch {
      /* 记不住就算了 */
    }
    set({ collapsed });
  },
}));

registerCommands([
  {
    id: "layout.toggle-sidebar",
    title: "收起或展开二级菜单",
    group: "界面",
    keywords: "sidebar collapse expand 侧边栏 二级菜单 折叠 展开",
    icon: PanelLeft,
    run: () => useSidebar.getState().toggle(),
  },
]);
