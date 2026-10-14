import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Plus, ScanEye } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./watchers.css";

// 列表页在监控页里（/monitoring/watchers），这里只放详情和接管页。
const WatcherPage = lazy(() => import("./WatcherPage"));
const LivePage = lazy(() => import("./LivePage"));

registerCommands([
  {
    id: "watchers.list",
    title: "网页盯梢",
    group: "监控",
    icon: ScanEye,
    keywords: "watch web page ai 盯 网页 政务 办理 状态",
    run: ({ navigate }) => navigate("/monitoring/watchers"),
  },
  {
    id: "watchers.new",
    title: "新建网页盯梢",
    group: "监控",
    icon: Plus,
    keywords: "watch web page new 盯 网页",
    run: ({ navigate }) => navigate("/monitoring/watchers?new=1"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "monitoring/watchers/:watcherId",
    element: <WatcherPage />,
    handle: { title: "Monitoring" },
  },
  {
    path: "monitoring/watchers/:watcherId/live",
    element: <LivePage />,
    handle: { title: "Monitoring" },
  },
];
