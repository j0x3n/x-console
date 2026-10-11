import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Layers, Play, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavBadge } from "../../lib/navBadges";
import { useCardsBadge } from "./badge";
import "./i18n";
import "./cards.css";

registerNavBadge("/cards", useCardsBadge);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const CardsPage = lazy(() => import("./CardsPage"));

registerCommands([
  {
    id: "cards.open",
    title: "打开复习卡片",
    group: "复习卡片",
    icon: Layers,
    keywords: "cards review flashcards spaced repetition 复习 卡片 记忆",
    run: ({ navigate }) => navigate("/cards"),
  },
  {
    id: "cards.review",
    title: "开始复习",
    group: "复习卡片",
    icon: Play,
    keywords: "cards review start flashcards 复习 开始 卡片",
    run: ({ navigate }) => navigate("/cards"),
  },
  {
    id: "cards.new",
    title: "新建复习卡片",
    group: "复习卡片",
    icon: Plus,
    keywords: "cards new flashcard 新建 卡片",
    run: ({ navigate }) => navigate("/cards?new=1"),
  },
]);

export const routes: RouteObject[] = [
  { path: "cards", element: <CardsPage />, handle: { title: "Review cards" } },
];
