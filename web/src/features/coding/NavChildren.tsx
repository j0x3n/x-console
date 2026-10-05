import { useLocation } from "react-router";
import { Bot, FolderGit2, ListChecks } from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useAiAgents } from "../aiagents/api";

/**
 * 左栏“Agent”的二级菜单（B102）：全部 Agent、任务、仓库（和 Agent 页的标签一致），下面每个 Agent 一行。
 * 工作中的 Agent 名称右边转圈，数量大于 1 时再显示几个（B47、B86）。
 */
export default function CodingNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const agents = useAiAgents();
  const { pathname } = useLocation();
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/coding"
          icon={Bot}
          label={t("All agents")}
          count={agents.data?.length}
          active={pathname === "/coding"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/coding/tasks"
          icon={ListChecks}
          label={t("Tasks")}
          active={pathname === "/coding/tasks"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/coding/repos"
          icon={FolderGit2}
          label={t("Repositories")}
          active={pathname === "/coding/repos"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Agents")}>
        <NavChildLinks
          links={(agents.data ?? []).map((a) => ({
            key: String(a.id),
            to: `/coding/agents/${a.id}`,
            label: `${a.avatar ? `${a.avatar} ` : ""}${a.name}`,
            busy: a.runningTasks > 0,
            hint: a.runningTasks > 1 ? String(a.runningTasks) : undefined,
          }))}
          allTo="/coding"
          loading={agents.isPending}
          error={agents.isError}
          empty={t("No agents yet")}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
