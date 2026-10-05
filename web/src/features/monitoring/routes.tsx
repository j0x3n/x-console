import { lazy } from "react";
import type { RouteObject } from "react-router";
import { FileCode2, Globe, Plus, Receipt, ShieldCheck } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./monitoring.css";
import { registerNavAction, registerNavBadge } from "../../lib/navBadges";
import { registerNavChildren } from "../../lib/navChildren";
import { useMonitoringBadge } from "./badge";
import MonitoringNavChildren from "./NavChildren";

registerNavBadge("/monitoring", useMonitoringBadge);
// 二级菜单：四个视图和网站列表（用户 2026-10-05 要求）
registerNavChildren("/monitoring", MonitoringNavChildren);
registerNavAction("/monitoring", {
  icon: Plus,
  label: "New website",
  run: (navigate) => navigate("/monitoring?new=1"),
});

// 页面按需加载（B6），主包里只留路由、命令和样式。
const MonitoringPage = lazy(() => import("./MonitoringPage"));

registerCommands([
  {
    id: "monitoring.sites",
    title: "网站监控",
    group: "监控",
    icon: Globe,
    keywords: "monitor uptime website http",
    run: ({ navigate }) => navigate("/monitoring"),
  },
  {
    id: "monitoring.new-site",
    title: "添加网站监控",
    group: "监控",
    icon: Globe,
    keywords: "monitor new website",
    run: ({ navigate }) => navigate("/monitoring?new=1"),
  },
  {
    id: "monitoring.certs",
    title: "证书和域名到期",
    group: "监控",
    icon: ShieldCheck,
    keywords: "tls ssl certificate domain expiry",
    run: ({ navigate }) => navigate("/monitoring/certs"),
  },
  {
    id: "monitoring.scripts",
    title: "脚本库",
    group: "监控",
    icon: FileCode2,
    keywords: "script run bash",
    run: ({ navigate }) => navigate("/monitoring/scripts"),
  },
  {
    id: "monitoring.subscriptions",
    title: "订阅和续费",
    group: "监控",
    icon: Receipt,
    keywords: "subscription renewal bill 续费",
    run: ({ navigate }) => navigate("/monitoring/subscriptions"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "monitoring",
    element: <MonitoringPage />,
    handle: { title: "Monitoring" },
  },
  {
    path: "monitoring/:tab",
    element: <MonitoringPage />,
    handle: { title: "Monitoring" },
  },
];
