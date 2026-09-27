import { lazy } from "react";
import type { RouteObject } from "react-router";
import { FileCode2, Globe, Receipt, ShieldCheck } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./monitoring.css";

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
  { path: "monitoring", element: <MonitoringPage />, handle: { title: "Monitoring" } },
  { path: "monitoring/:tab", element: <MonitoringPage />, handle: { title: "Monitoring" } },
];
