import { lazy } from "react";
import type { RouteObject } from "react-router";
import { BookOpen } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./journal.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const JournalPage = lazy(() => import("./JournalPage"));

registerCommands([
  {
    id: "journal.open",
    title: "打开每日时间线",
    group: "每日时间线",
    icon: BookOpen,
    keywords: "journal timeline diary history 时间线 日记 回顾 今天做了什么",
    run: ({ navigate }) => navigate("/journal"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "journal",
    element: <JournalPage />,
    handle: { title: "Journal" },
  },
];
