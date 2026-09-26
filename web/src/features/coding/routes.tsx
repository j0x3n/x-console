import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Bot, FolderGit2 } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./coding.css";

const CodingPage = lazy(() => import("./CodingPage"));
const ReposPage = lazy(() => import("./ReposPage"));
const TaskPage = lazy(() => import("./TaskPage"));

registerCommands([
  {
    id: "coding.new-task",
    title: "新建编码任务",
    group: "编码任务",
    keywords: "coding task claude codex new",
    icon: Bot,
    run: ({ navigate }) => navigate("/coding?new=1"),
  },
  {
    id: "coding.repos",
    title: "管理编码仓库",
    group: "编码任务",
    keywords: "coding repositories git repo",
    icon: FolderGit2,
    run: ({ navigate }) => navigate("/coding/repos"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "coding",
    element: <CodingPage />,
    handle: { title: "Coding tasks" },
  },
  {
    path: "coding/repos",
    element: <ReposPage />,
    handle: { title: "Coding tasks" },
  },
  {
    path: "coding/:taskId",
    element: <TaskPage />,
    handle: { title: "Coding tasks" },
  },
];
