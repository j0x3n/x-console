import NavChildLinks from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useAiAgents } from "../aiagents/api";

/** 侧边栏“Agent”下面：每个 Agent 一行，右边是正在跑的任务数（B47）。 */
export default function CodingNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const agents = useAiAgents();
  return (
    <NavChildLinks
      links={(agents.data ?? []).map((a) => ({
        key: String(a.id),
        to: `/coding/agents/${a.id}`,
        label: `${a.avatar ? `${a.avatar} ` : ""}${a.name}`,
        hint: a.runningTasks > 0 ? String(a.runningTasks) : undefined,
        active: false,
      }))}
      allTo="/coding"
      error={agents.isError}
      empty={t("No agents yet")}
      onNavigate={onNavigate}
    />
  );
}
