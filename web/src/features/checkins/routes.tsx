import { lazy } from "react";
import type { RouteObject } from "react-router";
import { HeartPulse } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./checkins.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const CheckinsPage = lazy(() => import("./CheckinsPage"));

registerCommands([
  {
    id: "checkins.open",
    title: "打开状态打卡",
    group: "状态打卡",
    icon: HeartPulse,
    keywords: "checkin mood energy stress 打卡 精力 压力 心情 状态",
    run: ({ navigate }) => navigate("/checkins"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "checkins",
    element: <CheckinsPage />,
    handle: { title: "Check-ins" },
  },
];
