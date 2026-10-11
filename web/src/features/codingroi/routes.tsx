import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Scale } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const RoiPage = lazy(() => import("./RoiPage"));

registerCommands([
  {
    id: "codingroi.open",
    title: "打开投入产出",
    group: "Agent 任务",
    icon: Scale,
    keywords:
      "ai coding cost return report roi claude codex 花费 投入 产出 对账 审查 合并",
    run: ({ navigate }) => navigate("/coding/roi"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "coding/roi",
    element: <RoiPage />,
    handle: { title: "Agents" },
  },
];
