import { lazy } from "react";
import type { RouteObject } from "react-router";
import { ClipboardPaste } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavChildren } from "../../lib/navChildren";
import PcNavChildren from "./NavChildren";
import "../servers/servers.css";
import "../servers/i18n";
import "./i18n";
import "./pc.css";

// 侧边栏“/pc”的二级菜单：每台电脑
registerNavChildren("/pc", PcNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const PcPage = lazy(() => import("./PcPage"));

registerCommands([
  {
    id: "pc.clipboard",
    title: "发送到电脑剪贴板",
    group: "本机",
    keywords: "clipboard pc copy paste",
    icon: ClipboardPaste,
    run: ({ navigate }) => navigate("/pc"),
  },
]);

export const routes: RouteObject[] = [
  { path: "pc", element: <PcPage />, handle: { title: "Computer" } },
  { path: "pc/:tab", element: <PcPage />, handle: { title: "Computer" } },
];
