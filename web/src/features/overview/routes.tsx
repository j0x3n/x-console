import type { RouteObject } from "react-router";
import "./i18n";
import "./overview.css";
import TodayPage from "./TodayPage";

// 模块入口：“今日”首页。命令面板里的跳转来自侧边栏，不用再注册。
export const routes: RouteObject[] = [
  { index: true, element: <TodayPage />, handle: { title: "My day" } },
];
