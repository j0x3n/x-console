import { lazy } from "react";
import type { RouteObject } from "react-router";
import { CalendarClock } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./plan.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const PlanPage = lazy(() => import("./PlanPage"));

registerCommands([
  {
    id: "dayplan.open",
    title: "打开今日计划",
    group: "今日计划",
    icon: CalendarClock,
    keywords: "plan day today schedule time block 计划 今天 排程 要事",
    run: ({ navigate }) => navigate("/plan"),
  },
]);

export const routes: RouteObject[] = [
  { path: "plan", element: <PlanPage />, handle: { title: "Day plan" } },
];
