import { lazy } from "react";
import type { RouteObject } from "react-router";
import "./i18n";
import "./projects.css";
import "./commands";
import ProjectsNavChildren from "./NavChildren";
import { registerNavChildren } from "../../lib/navChildren";
import { registerNavBadge } from "../../lib/navBadges";
import { useProjectsBadge } from "./badge";

registerNavBadge("/projects", useProjectsBadge);

// 侧边栏“/projects”的二级菜单。
registerNavChildren("/projects", ProjectsNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const IssuePage = lazy(() => import("./IssuePage"));
const ProjectPage = lazy(() => import("./ProjectPage"));
const ProjectsPage = lazy(() => import("./ProjectsPage"));
const ViewPage = lazy(() => import("./ViewPage"));

// 模块入口：路由、命令、事件订阅都从这里注册。
export const routes: RouteObject[] = [
  {
    path: "projects",
    element: <ProjectsPage />,
    handle: { title: "Projects" },
  },
  // B101：跨项目的视图。要放在 :projectKey 前面
  {
    path: "projects/views/:view",
    element: <ViewPage />,
    handle: { title: "Projects" },
  },
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
