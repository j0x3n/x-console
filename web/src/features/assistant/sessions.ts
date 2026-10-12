import type { AiSession } from "./api";

/*
 * B183：AI 窗口左边的会话列表。三种来源：面板对话（chat）、服务器 Agent（host）、
 * Agent 执行记录（agent）。这里放筛选、分组和跳转地址，组件只管画。
 */

export type SessionFilter = "all" | "active" | "host" | "agent" | "chat";

export const SESSION_FILTERS: { value: SessionFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "active", label: "At work" },
  { value: "host", label: "Server" },
  { value: "agent", label: "AI agent" },
  { value: "chat", label: "Chat" },
];

/** 正在工作或等你确认。 */
export function isActive(s: Pick<AiSession, "status">) {
  return s.status === "working" || s.status === "waiting";
}

export function filterSessions(
  list: AiSession[],
  filter: SessionFilter,
  query: string,
): AiSession[] {
  const q = query.trim().toLowerCase();
  return list.filter(
    (s) =>
      (filter === "all" ||
        (filter === "active" ? isActive(s) : s.kind === filter)) &&
      (!q ||
        s.title.toLowerCase().includes(q) ||
        s.sourceName.toLowerCase().includes(q) ||
        (s.issueKey ?? "").toLowerCase().includes(q)),
  );
}

/** 分成“进行中”和“最近”，各自按最后变化时间倒序。 */
export function groupSessions(list: AiSession[]) {
  const byTime = (a: AiSession, b: AiSession) =>
    b.updatedAt.localeCompare(a.updatedAt);
  return {
    active: list.filter(isActive).sort(byTime),
    recent: list.filter((s) => !isActive(s)).sort(byTime),
  };
}

/** 顶栏按钮上的标记：等确认的数量，和有没有在工作的。 */
export function sessionBadge(list: AiSession[] | undefined) {
  let pending = 0;
  let working = 0;
  for (const s of list ?? []) {
    if (s.status === "waiting") pending += Math.max(1, s.pendingCount);
    if (s.status === "working") working++;
  }
  return { pending, working };
}

export const STATUS_LABELS: Record<AiSession["status"], string> = {
  working: "At work",
  waiting: "Waiting for you",
  idle: "Idle",
  done: "Done",
  failed: "Failed",
  canceled: "Canceled",
};

export function statusTone(status: AiSession["status"]) {
  switch (status) {
    case "working":
      return "accent";
    case "waiting":
      return "warn";
    case "failed":
      return "danger";
    case "done":
      return "ok";
    default:
      return "";
  }
}

/** “打开来源”去哪里。面板对话没有来源页。 */
export function sessionSource(
  s: Pick<AiSession, "kind" | "refId" | "hostId" | "agentId">,
): string | null {
  if (s.kind === "host" && s.hostId)
    return `/servers/${encodeURIComponent(s.hostId)}/agent?c=${s.refId}`;
  if (s.kind === "agent" && s.agentId)
    return `/coding/agents/${s.agentId}?run=${s.refId}`;
  return null;
}

/** 列表里的选中项：对话按会话 id，执行记录按执行 id。 */
export function isSelected(
  s: Pick<AiSession, "kind" | "refId">,
  conversationId: number | null,
  runId: number | null,
) {
  return s.kind === "agent" ? s.refId === runId : s.refId === conversationId;
}
