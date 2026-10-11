import { lazy } from "react";
import type { RouteObject } from "react-router";
import { ClipboardCheck, Plus, Target } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./goals.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const GoalsPage = lazy(() => import("./GoalsPage"));
const ReviewPage = lazy(() => import("./ReviewPage"));

registerCommands([
  {
    id: "goals.open",
    title: "打开目标",
    group: "目标",
    icon: Target,
    keywords: "goals okr quarter progress 目标 季度 进度 关键结果",
    run: ({ navigate }) => navigate("/goals"),
  },
  {
    id: "goals.new",
    title: "新建目标",
    group: "目标",
    icon: Plus,
    keywords: "goal new okr 新建 目标 季度",
    run: ({ navigate }) => navigate("/goals?new=1"),
  },
  {
    id: "goals.review",
    title: "打开每周复盘",
    group: "目标",
    icon: ClipboardCheck,
    keywords: "weekly review retrospective 每周 复盘 总结 周报",
    run: ({ navigate }) => navigate("/goals/review"),
  },
]);

export const routes: RouteObject[] = [
  { path: "goals", element: <GoalsPage />, handle: { title: "Goals" } },
  { path: "goals/review", element: <ReviewPage />, handle: { title: "Goals" } },
];
