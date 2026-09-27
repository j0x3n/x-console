import { lazy } from "react";
import type { RouteObject } from "react-router";
import { CalendarDays, CalendarPlus, Newspaper, Timer } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./calendar.css";
import { useFocusPanel } from "./hooks";
import CalendarNavChildren from "./NavChildren";
import { registerNavChildren } from "../../lib/navChildren";

// 侧边栏“/calendar”的二级菜单。
registerNavChildren("/calendar", CalendarNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const CalendarShell = lazy(() => import("./CalendarShell"));
const CalendarView = lazy(() => import("./CalendarView"));
const BriefsPage = lazy(() => import("./BriefsPage"));
const FocusPage = lazy(() => import("./FocusPage"));
const CalendarsManager = lazy(() => import("./CalendarsManager"));

// M11：日历、每日早报、番茄钟。后端是 calendar、brief、focus 三个模块。
registerCommands([
  {
    id: "calendar.today",
    title: "今天的日程",
    group: "日历",
    icon: CalendarDays,
    keywords: "calendar today schedule agenda",
    run: ({ navigate }) => navigate("/calendar?view=day"),
  },
  {
    id: "calendar.add",
    title: "添加日历",
    group: "日历",
    icon: CalendarPlus,
    keywords: "calendar ics caldav subscribe",
    run: ({ navigate }) => navigate("/calendar/calendars?new=1"),
  },
  {
    id: "calendar.brief",
    title: "查看早报",
    group: "日历",
    icon: Newspaper,
    keywords: "brief morning daily weather",
    run: ({ navigate }) => navigate("/calendar/briefs"),
  },
  {
    id: "focus.start",
    title: "开始番茄钟",
    group: "日历",
    icon: Timer,
    keywords: "focus pomodoro timer",
    run: () => useFocusPanel.getState().openFor(""),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "calendar",
    element: <CalendarShell />,
    handle: { title: "Calendar" },
    children: [
      { index: true, element: <CalendarView /> },
      { path: "briefs", element: <BriefsPage /> },
      { path: "focus", element: <FocusPage /> },
      { path: "calendars", element: <CalendarsManager /> },
    ],
  },
];
