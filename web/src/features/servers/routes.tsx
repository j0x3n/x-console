import type { RouteObject } from "react-router";
import { BellRing, TerminalSquare } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./servers.css";
import ServersPage from "./ServersPage";
import HostDetailPage from "./HostDetailPage";

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
  { path: "servers/:hostId", element: <HostDetailPage />, handle: { title: "Servers" } },
  { path: "servers/:hostId/:tab", element: <HostDetailPage />, handle: { title: "Servers" } },
];
