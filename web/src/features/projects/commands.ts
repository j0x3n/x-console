import { useEffect } from "react";
import { FolderKanban, Hash, SquareKanban, SquarePlus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import type { Issue, Project } from "./api";
import { issuePath } from "./logic";

/*
 * 命令面板：固定命令在模块加载时注册；项目和 Issue 的跳转命令在数据加载后注册，
 * 这样在面板里输入 key（比如 XC-12）就能直接跳过去。
 */
registerCommands([
  {
    id: "projects.new-issue",
    title: "新建 Issue",
    group: "项目",
    keywords: "new issue create task",
    icon: SquarePlus,
    run: ({ navigate }) => navigate("/projects?new=1"),
  },
  {
    id: "projects.new-project",
    title: "新建项目",
    group: "项目",
    keywords: "new project",
    icon: FolderKanban,
    run: ({ navigate }) => navigate("/projects?newProject=1"),
  },
  {
    id: "projects.goto-issue",
    title: "跳到 Issue…",
    group: "项目",
    keywords: "go to issue key",
    icon: Hash,
    run: ({ navigate }) => navigate("/projects?goto=1"),
  },
]);

export function useProjectCommands(projects: Project[] | undefined) {
  useEffect(() => {
    if (!projects) return;
    registerCommands(
      projects.map((p) => ({
        id: `projects.open:${p.key}`,
        title: `${p.name} (${p.key})`,
        group: "项目",
        keywords: `project ${p.key}`,
        icon: SquareKanban,
        run: ({ navigate }) => navigate(`/projects/${p.key}`),
      })),
    );
  }, [projects]);
}

const MAX_ISSUE_COMMANDS = 500;

export function useIssueCommands(issues: Issue[] | undefined) {
  useEffect(() => {
    if (!issues) return;
    registerCommands(
      issues.slice(0, MAX_ISSUE_COMMANDS).map((i) => ({
        id: `projects.issue:${i.key}`,
        title: `${i.key} ${i.title}`,
        group: "Issue",
        keywords: i.key,
        icon: Hash,
        run: ({ navigate }) => navigate(issuePath(i.key)),
      })),
    );
  }, [issues]);
}
