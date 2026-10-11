import { Navigate, type RouteObject } from "react-router";
import { Gauge } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./quotas.css";

registerCommands([
  {
    id: "quotas.open",
    title: "打开 AI 额度",
    group: "AI 额度",
    keywords:
      "quota usage limit claude codex grok deepseek balance 额度 余额 重置",
    icon: Gauge,
    run: ({ navigate }) => navigate("/monitoring/quotas"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "quotas",
    // B152：页面放进了监控，旧地址跳过去
    element: <Navigate to="/monitoring/quotas" replace />,
    handle: { title: "AI quotas" },
  },
];
