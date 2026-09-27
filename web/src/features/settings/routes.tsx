import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Settings2 } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./settings.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const SettingsPage = lazy(() => import("./SettingsPage"));

// 侧边栏里没有设置了（在左下角的个人菜单里），命令面板里保留一个入口。
registerCommands([
  {
    id: "settings.open",
    title: "设置",
    group: "设置",
    keywords: "settings preferences 设置 安全 通知",
    icon: Settings2,
    run: ({ navigate }) => navigate("/settings"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "settings",
    element: <SettingsPage />,
    handle: { title: "Settings" },
  },
  {
    path: "settings/:tab",
    element: <SettingsPage />,
    handle: { title: "Settings" },
  },
];
