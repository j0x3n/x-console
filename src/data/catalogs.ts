import type { AgentDefinition, StringMap } from "../types/domain";
export const agents: AgentDefinition[] = [
  { name: "Scout", color: "teal", mark: "✦", status: "Running" },
  { name: "Scribe", color: "amber", mark: "◆", status: "Needs your call" },
  { name: "Ledger", color: "violet", mark: "Ⅱ", status: "Needs your call" },
  { name: "Pilot", color: "lime", mark: "✳", status: "Needs your call" },
  { name: "Echo", color: "pink", mark: "✺", status: "Running" },
];

export const agentMarks: StringMap = Object.fromEntries(
  agents.map((agent) => [agent.name, agent.mark]),
);

export const agentColors: StringMap = Object.fromEntries(
  agents.map((agent) => [agent.name, agent.color]),
);

export const stages = ["Discovery", "Evaluation", "Proposal", "Negotiation"];

export const stageZh: StringMap = {
  Discovery: "发现需求",
  Evaluation: "评估",
  Proposal: "方案",
  Negotiation: "谈判",
};

export const rolesZh: StringMap = {
  Champion: "支持者",
  "Economic buyer": "经济决策者",
  Legal: "法务",
  User: "使用者",
  "Decision maker": "决策者",
  Admin: "管理员",
};

export const forecastZh: StringMap = {
  Commit: "确定收入",
  "Best case": "最佳预期",
  Pipeline: "商机池",
};

export const ownerNames: StringMap = {
  jo: "jo",
  Theo: "Theo Park",
  Ines: "Ines Duarte",
  Kofi: "Kofi Mensah",
  Ruth: "Ruth Adler",
};
