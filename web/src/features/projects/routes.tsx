import type { RouteObject } from "react-router";
import "./i18n";
import "./projects.css";
import "./commands";
import IssuePage from "./IssuePage";
import ProjectPage from "./ProjectPage";
import ProjectsPage from "./ProjectsPage";

// 模块入口：路由、命令、事件订阅都从这里注册。
export const routes: RouteObject[] = [
  { path: "projects", element: <ProjectsPage />, handle: { title: "Projects" } },
  {
    path: "projects/:projectKey",
    element: <ProjectPage />,
    handle: { title: "Projects" },
  },
  {
    path: "projects/:projectKey/:number",
    element: <IssuePage />,
    handle: { title: "Projects" },
  },
];
