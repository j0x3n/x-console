import { lazy } from "react";
import type { RouteObject } from "react-router";
import { FlaskConical, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./experiments.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const ExperimentsPage = lazy(() => import("./ExperimentsPage"));
const ExperimentDetailPage = lazy(() => import("./ExperimentDetailPage"));

registerCommands([
  {
    id: "experiments.open",
    title: "打开自我实验",
    group: "自我实验",
    icon: FlaskConical,
    keywords: "experiment self n-of-1 ab test 实验 自我实验 对比",
    run: ({ navigate }) => navigate("/experiments"),
  },
  {
    id: "experiments.new",
    title: "新建自我实验",
    group: "自我实验",
    icon: Plus,
    keywords: "experiment new 新建 实验",
    run: ({ navigate }) => navigate("/experiments"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "experiments",
    element: <ExperimentsPage />,
    handle: { title: "Experiments" },
  },
  {
    path: "experiments/:id",
    element: <ExperimentDetailPage />,
    handle: { title: "Experiments" },
  },
];
