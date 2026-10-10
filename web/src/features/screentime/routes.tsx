import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Hourglass } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./screentime.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const ScreenTimePage = lazy(() => import("./ScreenTimePage"));

registerCommands([
  {
    id: "screentime.open",
    title: "打开时间去向",
    group: "电脑时间",
    icon: Hourglass,
    keywords: "screen time usage app windows 时间 去向 电脑 使用 统计",
    run: ({ navigate }) => navigate("/screentime"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "screentime",
    element: <ScreenTimePage />,
    handle: { title: "Screen time" },
  },
];
