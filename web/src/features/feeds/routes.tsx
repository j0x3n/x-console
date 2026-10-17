import { lazy } from "react";
import type { RouteObject } from "react-router";
import { MessageSquareText, Newspaper } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavChildren } from "../../lib/navChildren";
import NavChildren from "./NavChildren";
registerNavChildren("/feeds", NavChildren);
import "./i18n";
import "./feeds.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const NewsPage = lazy(() => import("./NewsPage"));
const TweetsPage = lazy(() => import("./TweetsPage"));

registerCommands([
  {
    id: "feeds.news",
    title: "打开资讯里的新闻",
    group: "资讯",
    icon: Newspaper,
    keywords: "feeds news ai 资讯 新闻 AI 新闻",
    run: ({ navigate }) => navigate("/feeds"),
  },
  {
    id: "feeds.tweets",
    title: "打开资讯里的推文",
    group: "资讯",
    icon: MessageSquareText,
    keywords: "feeds tweets x twitter 资讯 推文 推特",
    run: ({ navigate }) => navigate("/feeds/tweets"),
  },
]);

export const routes: RouteObject[] = [
  { path: "feeds", element: <NewsPage />, handle: { title: "Feeds" } },
  { path: "feeds/tweets", element: <TweetsPage />, handle: { title: "Feeds" } },
];
