import { lazy } from "react";
import type { RouteObject } from "react-router";
import "./i18n";
import "./overview.css";
import { registerNavIcon } from "../../lib/navBadges";
import { useTodayNavIcon } from "./navIcon";

// B88：左栏“今日”的图标跟着天气变
registerNavIcon("/", useTodayNavIcon);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const TodayPage = lazy(() => import("./TodayPage"));

// 模块入口：“今日”首页。命令面板里的跳转来自侧边栏，不用再注册。
export const routes: RouteObject[] = [
  { index: true, element: <TodayPage />, handle: { title: "My day" } },
];
