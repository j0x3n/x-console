import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Scale } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./forecasts.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const ForecastsPage = lazy(() => import("./ForecastsPage"));

registerCommands([
  {
    id: "forecasts.open",
    title: "打开预测",
    group: "预测",
    icon: Scale,
    keywords:
      "forecast prediction decision brier calibration 预测 决策 把握 校准",
    run: ({ navigate }) => navigate("/forecasts"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "forecasts",
    element: <ForecastsPage />,
    handle: { title: "Forecasts" },
  },
];
