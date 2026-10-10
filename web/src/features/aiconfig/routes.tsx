import { lazy } from "react";
import type { RouteObject } from "react-router";
import { SlidersHorizontal } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const AIConfigPage = lazy(() => import("./AIConfigPage"));

registerCommands([
  {
    id: "aiconfig.open",
    title: "打开配置下发",
    group: "Agent 任务",
    icon: SlidersHorizontal,
    keywords:
      "config deliver claude codex rules permissions mcp 配置 下发 规则 权限 同步",
    run: ({ navigate }) => navigate("/coding/config"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "coding/config",
    element: <AIConfigPage />,
    handle: { title: "Agents" },
  },
];
