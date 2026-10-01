import { lazy } from "react";
import type { RouteObject } from "react-router";
import { GitPullRequest, Settings } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./github.css";
import { registerNavBadge } from "../../lib/navBadges";
import { useGitHubBadge } from "./badge";

registerNavBadge("/github", useGitHubBadge);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const GitHubPage = lazy(() => import("./GitHubPage"));

registerCommands([
  {
    id: "github.open",
    title: "打开 GitHub",
    group: "GitHub",
    keywords: "github pr pull request ci actions",
    icon: GitPullRequest,
    run: ({ navigate }) => navigate("/github"),
  },
  {
    id: "github.runs",
    title: "查看 CI 运行",
    group: "GitHub",
    keywords: "github ci actions workflow",
    icon: GitPullRequest,
    run: ({ navigate }) => navigate("/github?tab=runs"),
  },
  {
    id: "github.settings",
    title: "Git 与 GitHub 设置",
    group: "GitHub",
    keywords: "github git forgejo token repo account",
    icon: Settings,
    run: ({ navigate }) => navigate("/settings/git"),
  },
  {
    id: "linear.settings",
    title: "Linear 同步设置",
    group: "GitHub",
    keywords: "linear sync team",
    icon: Settings,
    run: ({ navigate }) => navigate("/settings/linear"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "github/*",
    element: <GitHubPage />,
    handle: { title: "GitHub" },
  },
];
