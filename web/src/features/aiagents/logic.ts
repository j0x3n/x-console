import type { AiAgent, AiAgentKind } from "./api";

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
