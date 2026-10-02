import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Dumbbell, HeartPulse, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavChildren } from "../../lib/navChildren";
import HabitsNavChildren from "./NavChildren";
import "./i18n";
import "./habits.css";
import "./personal.css";
import "./personalI18n";

// 侧边栏“/habits”的二级菜单：今天（带每个习惯）和健身
registerNavChildren("/habits", HabitsNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const HabitsPage = lazy(() => import("./HabitsPage"));

registerCommands([
  {
    id: "habits.today",
    title: "今天的习惯",
    group: "习惯",
    icon: HeartPulse,
    keywords: "habit checkin 打卡",
    run: ({ navigate }) => navigate("/habits"),
  },
  {
    id: "habits.new",
    title: "新建习惯",
    group: "习惯",
    icon: Plus,
    keywords: "habit new",
    run: ({ navigate }) => navigate("/habits?new=1"),
  },
  {
    id: "habits.workout",
    title: "记录训练",
    group: "习惯",
    icon: Dumbbell,
    keywords: "workout gym 健身",
    run: ({ navigate }) => navigate("/habits?log=workout"),
  },
]);

export const routes: RouteObject[] = [
  { path: "habits", element: <HabitsPage />, handle: { title: "Habits" } },
  { path: "habits/:tab", element: <HabitsPage />, handle: { title: "Habits" } },
];
