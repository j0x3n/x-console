import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Inbox as InboxIcon } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavBadge } from "../../lib/navBadges";
import { useInboxBadge } from "./badge";
import "./i18n";
import "./inbox.css";

registerNavBadge("/inbox", useInboxBadge);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const InboxPage = lazy(() => import("./InboxPage"));

registerCommands([
  {
    id: "inbox.open",
    title: "打开收集箱",
    group: "收集箱",
    icon: InboxIcon,
    keywords: "inbox capture quick note voice 收集箱 随手记 语音 想法",
    run: ({ navigate }) => navigate("/inbox"),
  },
]);

export const routes: RouteObject[] = [
  { path: "inbox", element: <InboxPage />, handle: { title: "Capture inbox" } },
];
