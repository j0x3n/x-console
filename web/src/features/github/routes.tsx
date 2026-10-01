import { lazy } from "react";
import type { RouteObject } from "react-router";
import { GitPullRequest, Settings } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./github.css";
import { registerNavBadge } from "../../lib/navBadges";
import { useGitHubBadge } from "./badge";
import { registerNavChildren } from "../../lib/navChildren";
import ReposNavChildren from "./NavChildren";

registerNavBadge("/github", useGitHubBadge);
registerNavChildren("/github", ReposNavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const GitHubPage = lazy(() => import("./GitHubPage"));

registerCommands([
  {
    id: "github.open",
    title: "打开仓库",
    group: "仓库",
    keywords: "github forgejo repo pr pull request ci actions commit",
    icon: GitPullRequest,
    run: ({ navigate }) => navigate("/github"),
  },
  {
    id: "github.runs",
    title: "查看 CI 运行",
    group: "仓库",
    keywords: "github ci actions workflow",
    icon: GitPullRequest,
    run: ({ navigate }) => navigate("/github?tab=runs"),
  },
  {
    id: "github.settings",
    title: "Git 与仓库设置",
    group: "仓库",
    keywords: "github git forgejo token repo account",
    icon: Settings,
    run: ({ navigate }) => navigate("/settings/git"),
  },
  {
    id: "linear.settings",
    title: "Linear 同步设置",
    group: "仓库",
    keywords: "linear sync team",
    icon: Settings,
    run: ({ navigate }) => navigate("/settings/linear"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "github/*",
    element: <GitHubPage />,
    handle: { title: "Repositories" },
  },
];
