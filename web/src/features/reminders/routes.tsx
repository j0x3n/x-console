import { lazy } from "react";
import type { RouteObject } from "react-router";
import { AlarmClockPlus, BellRing } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./reminders.css";
import RemindersNavChildren from "./NavChildren";
import { registerNavChildren } from "../../lib/navChildren";
import { registerNavBadge } from "../../lib/navBadges";
import { useRemindersBadge } from "./badge";

registerNavBadge("/reminders", useRemindersBadge);

// 侧边栏“/reminders”的二级菜单。
registerNavChildren("/reminders", RemindersNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const RemindersPage = lazy(() => import("./RemindersPage"));

registerCommands([
  {
    id: "reminders.new",
    title: "新建提醒",
    group: "提醒",
    icon: AlarmClockPlus,
    keywords: "reminder new remind",
    run: ({ navigate }) => navigate("/reminders?new=1"),
  },
  {
    id: "reminders.notify-settings",
    title: "通知设置",
    group: "提醒",
    icon: BellRing,
    keywords: "notification telegram bark push",
    run: ({ navigate }) => navigate("/settings/notifications"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "reminders",
    element: <RemindersPage />,
    handle: { title: "Reminders" },
  },
];
