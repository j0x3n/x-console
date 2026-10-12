import type { LoaderFunctionArgs, RouteObject } from "react-router";
import { redirect, replace } from "react-router";
import { Settings2 } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { dialogIsOpen, settingsTarget } from "./nav";
import "./i18n";
import "./settings.css";

// 侧边栏里没有设置了（在左下角的个人菜单里），命令面板里保留一个入口。
registerCommands([
  {
    id: "settings.open",
    title: "设置",
    group: "设置",
    keywords: "settings preferences 设置 安全 通知",
    icon: Settings2,
    run: ({ navigate }) => navigate("/settings"),
  },
]);

// B180：设置改成弹窗。/settings/<id> 在加载前换成当前页面上的弹窗，
// 页面不会先跳走再跳回来。弹窗已经开着时只换标签，不多记一条历史。
function toDialog({ params, request }: LoaderFunctionArgs) {
  const target = settingsTarget(params.tab, new URL(request.url));
  return dialogIsOpen() ? replace(target) : redirect(target);
}

const Empty = () => null;

export const routes: RouteObject[] = [
  {
    path: "settings",
    loader: toDialog,
    HydrateFallback: Empty,
    element: null,
  },
  {
    path: "settings/:tab",
    loader: toDialog,
    HydrateFallback: Empty,
    element: null,
  },
];
