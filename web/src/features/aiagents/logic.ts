import type { Task, TaskStatus } from "../coding/logic";
import type { AiAgent, AiAgentKind, AiAgentRun } from "./api";

export const KINDS: { value: AiAgentKind; label: string; hint: string }[] = [
  {
    value: "claude_code",
    label: "Claude Code",
    hint: "Runs Claude Code on a machine. Good for changing code.",
  },
  {
    value: "codex",
    label: "Codex",
    hint: "Runs Codex CLI on a machine. Good for changing code.",
  },
  {
    value: "builtin",
    label: "Built-in",
    hint: "Uses your AI providers and the console's tools. Good for cards, notes and summaries. Does not touch code.",
  },
];

export const COLORS = [
  "#c2653d",
  "#2f7fd1",
  "#3a9a5b",
  "#8a5cc7",
  "#c79a2a",
  "#4d8f99",
  "#c24d7a",
  "#6b7280",
];

export const AVATARS = ["🤖", "🛠️", "🧹", "🔍", "📝", "🧪", "🚀", "🐛"];

export function kindLabel(kind: AiAgentKind) {
  return KINDS.find((k) => k.value === kind)?.label ?? kind;
}

export function isCLI(kind: AiAgentKind) {
  return kind !== "builtin";
}

/** "agent:3" -> 3 */
export function authorAgentId(author?: string | null) {
  if (!author?.startsWith("agent:")) return undefined;
  const id = Number(author.slice(6));
  return Number.isFinite(id) ? id : undefined;
}

export function findAgent(agents: AiAgent[] | undefined, id?: number | string) {
  if (id === undefined || id === "") return undefined;
  return agents?.find((a) => String(a.id) === String(id));
}

/** 本月费用和预算，例如 "$1.20 / $5"。 */
export function costText(
  a: Pick<AiAgent, "monthCostUsd" | "monthlyBudgetUsd">,
) {
  const cost = `$${a.monthCostUsd.toFixed(2)}`;
  return a.monthlyBudgetUsd != null ? `${cost} / $${a.monthlyBudgetUsd}` : cost;
}

/** Agent 卡片上绑定的机器：最多列 3 个，多了写“等 N 台”（B60）。 */
export function hostsText(
  names: string[],
  more: (n: number) => string,
): string {
  if (names.length <= 3) return names.join("、");
  return `${names.slice(0, 3).join("、")} ${more(names.length)}`;
}

/** B86：执行日志里的一条，执行记录和编码任务统一成这个样子。 */
export type RunStatus = AiAgentRun["status"];

export interface RunEntry {
  key: string;
  runId?: number;
  taskId?: number;
  agentName: string;
  issueKey: string;
  title: string;
  status: RunStatus;
  active: boolean;
  summary: string;
  prUrl?: string;
  at: string;
}

/** 编码任务的状态换成执行记录的状态。 */
export function taskRunStatus(status: TaskStatus): RunStatus {
  switch (status) {
    case "queued":
      return "queued";
    case "running":
      return "running";
    case "review":
      return "waiting";
    case "failed":
      return "failed";
    case "canceled":
    case "discarded":
      return "canceled";
    case "pr_opened":
      return "pr_opened";
    default:
      return "done";
  }
}

const ACTIVE: RunStatus[] = ["queued", "running", "waiting"];

/**
 * 执行记录接口上线后只用它（它已经包含 CLI 类型对应的编码任务）；
 * 没上线时（runs 为 undefined）用编码任务凑出来。新的在前。
 */
export function runEntries({
  runs,
  tasks,
}: {
  runs?: AiAgentRun[];
  tasks: Task[];
}): RunEntry[] {
  if (Array.isArray(runs))
    return runs.map((r) => ({
      key: `r${r.id}`,
      runId: r.id,
      taskId: r.taskId,
      agentName: r.agentName,
      issueKey: r.issueKey,
      title: r.issueTitle,
      status: r.status,
      // 编码任务做完等审查时不算“在跑”，不给“停止”
      active: r.status === "queued" || r.status === "running",
      summary: r.summary ?? "",
      prUrl: r.prUrl,
      at: r.startedAt ?? r.createdAt,
    }));
  return [...tasks]
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
    .map((x) => {
      const status = taskRunStatus(x.status);
      return {
        key: `t${x.id}`,
        taskId: x.id,
        agentName: "",
        issueKey: x.issueKey ?? "",
        title: x.title,
        status,
        active: status === "queued" || status === "running",
        summary: x.error,
        prUrl: x.prUrl || undefined,
        at: x.startedAt ?? x.createdAt,
      };
    });
}

export function isRunActive(status: RunStatus) {
  return ACTIVE.includes(status);
}

const RUN_LABELS: Record<RunStatus, string> = {
  queued: "Queued",
  running: "Running",
  waiting: "Waiting for your decision",
  pr_opened: "PR opened",
  done: "Done",
  failed: "Failed",
  canceled: "Canceled",
};

export function runStatusLabel(status: RunStatus): string {
  return RUN_LABELS[status] ?? status;
}

export function runTone(status: RunStatus): string {
  switch (status) {
    case "running":
      return "info";
    case "waiting":
      return "accent";
    case "failed":
      return "danger";
    case "done":
    case "pr_opened":
      return "ok";
    default:
      return "";
  }
}
