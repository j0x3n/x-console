import { useT } from "../../contexts/LanguageContext";
import { useTasks } from "../coding/api";
import { useAiAgents } from "./api";
import AgentAvatar from "./AgentAvatar";
import "./aiagents.css";

/**
 * 卡片上的 Agent 成员（B47）：头像，可选名字。传了 issueKey 时，这个 Agent
 * 在这张卡片上最近一次任务失败了就在头像上显示红点。
 */
export default function AgentMember({
  id,
  issueKey,
  withName,
  size = 20,
}: {
  id: string;
  issueKey?: string;
  withName?: boolean;
  size?: number;
}) {
  const t = useT();
  const agents = useAiAgents();
  const tasks = useTasks();
  const agent = agents.data?.find((a) => String(a.id) === id);
  let failed = false;
  if (issueKey) {
    const last = (tasks.data ?? [])
      .filter((x) => x.issueKey === issueKey && String(x.aiAgentId) === id)
      .sort((a, b) => b.id - a.id)[0];
    failed = last?.status === "failed";
  }
  const name = agent?.name ?? `Agent ${id}`;
  return (
    <span
      className="aiagent-member"
      title={failed ? `${name}：${t("Last task failed")}` : name}
      role={withName ? undefined : "img"}
      aria-label={
        withName
          ? undefined
          : failed
            ? `${name}：${t("Last task failed")}`
            : name
      }
    >
      <AgentAvatar
        agent={agent ?? { name, avatar: "", color: "" }}
        size={size}
        failed={failed}
      />
      {withName && <span>{name}</span>}
    </span>
  );
}
