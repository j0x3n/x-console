import { lazy } from "react";
import type { RouteObject } from "react-router";
import "./i18n";
import "./settings.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const SettingsPage = lazy(() => import("./SettingsPage"));

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
