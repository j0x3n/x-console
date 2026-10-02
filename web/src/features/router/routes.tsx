import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Router as RouterIcon, Settings } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./router.css";
import { registerNavStatus } from "../../lib/navBadges";
import { useRouterNavStatus } from "./navStatus";

// B93：左栏显示 WAN、在线设备数和速率
registerNavStatus("/router", useRouterNavStatus);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const RouterPage = lazy(() => import("./RouterPage"));

registerCommands([
  {
    id: "router.open",
    title: "打开路由器",
    group: "路由器",
    keywords: "router openwrt wan 网络 在线设备 流量",
    icon: RouterIcon,
    run: ({ navigate }) => navigate("/router"),
  },
  {
    id: "router.settings",
    title: "路由器设置",
    group: "路由器",
    keywords: "router openwrt ubus",
    icon: Settings,
    run: ({ navigate }) => navigate("/settings/router"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "router/*",
    element: <RouterPage />,
    handle: { title: "Router" },
  },
];
