import { lazy } from "react";
import type { RouteObject } from "react-router";
import { GraduationCap } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./skills.css";

// 页面按需加载（B6），主包里只留路由、命令和样式。
const SkillsPage = lazy(() => import("./SkillsPage"));

registerCommands([
  {
    id: "skills.open",
    title: "打开技能",
    group: "技能",
    icon: GraduationCap,
    keywords: "skills practice learning deliberate 技能 练习 学习 英语 时长",
    run: ({ navigate }) => navigate("/skills"),
  },
]);

export const routes: RouteObject[] = [
  { path: "skills", element: <SkillsPage />, handle: { title: "Skills" } },
];
