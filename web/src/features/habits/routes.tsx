import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Dumbbell, HeartPulse, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./habits.css";

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
    run: ({ navigate }) => navigate("/habits/workout"),
  },
]);

export const routes: RouteObject[] = [
  { path: "habits", element: <HabitsPage />, handle: { title: "Habits" } },
  { path: "habits/:tab", element: <HabitsPage />, handle: { title: "Habits" } },
];
