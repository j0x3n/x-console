import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Files, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavAction } from "../../lib/navBadges";
import { registerNavChildren } from "../../lib/navChildren";
import "./i18n";
import "./documents.css";
import DocumentsNavChildren from "./NavChildren";

registerNavChildren("/documents", DocumentsNavChildren);
registerNavAction("/documents", {
  icon: Plus,
  label: "New document",
  run: (navigate) => navigate("/documents?new=1"),
});

// 页面按需加载（B6），主包里只留路由、命令和样式。
const DocumentsPage = lazy(() => import("./DocumentsPage"));

registerCommands([
  {
    id: "documents.open",
    title: "打开证件档案",
    group: "证件档案",
    icon: Files,
    keywords:
      "document passport id visa contract insurance warranty 证件 护照 合同 保修 到期",
    run: ({ navigate }) => navigate("/documents"),
  },
  {
    id: "documents.new",
    title: "新建证件档案",
    group: "证件档案",
    icon: Plus,
    keywords: "document new passport 新建 证件",
    run: ({ navigate }) => navigate("/documents?new=1"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "documents",
    element: <DocumentsPage />,
    handle: { title: "Documents" },
  },
];
