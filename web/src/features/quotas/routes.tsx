import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Gauge } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./quotas.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const QuotasPage = lazy(() => import("./QuotasPage"));

registerCommands([
  {
    id: "quotas.open",
    title: "打开 AI 额度",
    group: "AI 额度",
    keywords:
      "quota usage limit claude codex grok deepseek balance 额度 余额 重置",
    icon: Gauge,
    run: ({ navigate }) => navigate("/quotas"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "quotas",
    element: <QuotasPage />,
    handle: { title: "AI quotas" },
  },
];
