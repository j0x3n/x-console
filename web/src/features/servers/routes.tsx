import { lazy } from "react";
import type { RouteObject } from "react-router";
import { BellRing, TerminalSquare } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./servers.css";
import ServersNavChildren from "./NavChildren";
import { registerNavChildren } from "../../lib/navChildren";
import { registerNavBadge } from "../../lib/navBadges";
import { useServersBadge } from "./badge";

registerNavBadge("/servers", useServersBadge);

// 侧边栏“/servers”的二级菜单。
registerNavChildren("/servers", ServersNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const ServersPage = lazy(() => import("./ServersPage"));
const HostDetailPage = lazy(() => import("./HostDetailPage"));

registerCommands([
  {
    id: "servers.ssh-hosts",
    title: "添加 SSH 主机",
    group: "服务器",
    keywords: "ssh server add",
    icon: TerminalSquare,
    run: ({ navigate }) => navigate("/servers?ssh=1"),
  },
  {
    id: "servers.alerts",
    title: "服务器告警",
    group: "服务器",
    keywords: "alert rules",
    icon: BellRing,
    run: ({ navigate }) => navigate("/servers?alerts=1"),
  },
]);

export const routes: RouteObject[] = [
  { path: "servers", element: <ServersPage />, handle: { title: "Servers" } },
  {
    path: "servers/:hostId",
    element: <HostDetailPage />,
    handle: { title: "Servers" },
  },
  {
    path: "servers/:hostId/:tab",
    element: <HostDetailPage />,
    handle: { title: "Servers" },
  },
];
