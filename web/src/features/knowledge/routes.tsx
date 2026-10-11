import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Library } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./knowledge.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const AskPage = lazy(() => import("./AskPage"));

registerCommands([
  {
    id: "knowledge.open",
    title: "打开资料问答",
    group: "资料问答",
    icon: Library,
    keywords: "ask knowledge search rag semantic 资料 问答 搜索 笔记 提问",
    run: ({ navigate }) => navigate("/ask"),
  },
]);

export const routes: RouteObject[] = [
  { path: "ask", element: <AskPage />, handle: { title: "Ask my data" } },
];
