import { create } from "zustand";
import { registerCommands } from "../lib/commands";
import { PanelLeft } from "lucide-react";

/* 桌面上侧边栏可以收成一列图标（B20）。状态记在 localStorage。 */
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
    title: "折叠或展开侧边栏",
    group: "界面",
    keywords: "sidebar collapse expand 侧边栏 折叠 展开",
    icon: PanelLeft,
    run: () => useSidebar.getState().toggle(),
  },
]);
