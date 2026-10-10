import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Bookmark, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavAction } from "../../lib/navBadges";
import "./i18n";
import "./readlater.css";

registerNavAction("/readlater", {
  icon: Plus,
  label: "Add link",
  run: (navigate) => navigate("/readlater?new=1"),
});

// 页面按需加载（B6），主包里只留路由、命令和样式。
const ReadLaterPage = lazy(() => import("./ReadLaterPage"));
const SharePage = lazy(() => import("./SharePage"));

registerCommands([
  {
    id: "readlater.open",
    title: "打开稍后阅读",
    group: "稍后阅读",
    icon: Bookmark,
    keywords: "read later bookmark link article 稍后阅读 收藏 文章 链接",
    run: ({ navigate }) => navigate("/readlater"),
  },
  {
    id: "readlater.add",
    title: "存一个链接到稍后阅读",
    group: "稍后阅读",
    icon: Plus,
    keywords: "read later add link save 稍后阅读 添加 链接 收藏",
    run: ({ navigate }) => navigate("/readlater?new=1"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "readlater",
    element: <ReadLaterPage />,
    handle: { title: "Read later" },
  },
  {
    path: "readlater/share",
    element: <SharePage />,
    handle: { title: "Read later" },
  },
];
