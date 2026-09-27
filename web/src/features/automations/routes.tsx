import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Workflow } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./automations.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const AutomationsPage = lazy(() => import("./AutomationsPage"));
const AutomationEditor = lazy(() => import("./AutomationEditor"));

registerCommands([
  {
    id: "automations.new",
    title: "新建自动化规则",
    group: "自动化",
    keywords: "automation rule new trigger",
    icon: Workflow,
    run: ({ navigate }) => navigate("/automations/new"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "automations",
    element: <AutomationsPage />,
    handle: { title: "Automations" },
  },
  {
    path: "automations/:id",
    element: <AutomationEditor />,
    handle: { title: "Automations" },
  },
];
