import type * as Model from "../../types/domain";
import React from "react";
import { Check, Undo2 } from "lucide-react";
import { agentMarks } from "../../data/catalogs";
import { outcomeText } from "../../lib/outcomeText";

interface DecisionOutcomeRowProps {
  entry: Model.DecisionEntry;
  language: Model.Language;
  navigate: Model.Navigate;
  onUndo: (key: string) => void;
}

export default function DecisionOutcomeRow({
  entry,
  language,
  navigate,
  onUndo,
}: DecisionOutcomeRowProps) {
  const status =
    entry.action === "reassigned"
      ? ["Reassigned", "已转交"]
      : entry.action === "approved"
        ? [
            entry.item.kind === "draft" ? "Sent" : "Approved",
            entry.item.kind === "draft" ? "已发送" : "已批准",
          ]
        : entry.action === "kept"
          ? ["Kept", "已保持"]
          : entry.item.kind === "health"
            ? ["Dismissed", "已忽略"]
            : ["Skipped", "已跳过"];
  return (
    <div
      className="decision-outcome"
      role="status"
      aria-label={outcomeText(entry, language)}
    >
      <span className="decision-outcome-check">
        <Check size={12} strokeWidth={2.6} />
      </span>
      <span
        className={`decision-outcome-agent ${entry.item.agent.toLowerCase()}`}
        aria-label={entry.item.agent}
      >
        {agentMarks[entry.item.agent] || entry.item.agent.slice(0, 1)}
      </span>
      <span className="decision-outcome-label">
        {outcomeText(entry, language)}
      </span>
      <span className="decision-outcome-status">
        {status[language === "zh" ? 1 : 0]}
      </span>
      <button
        className="decision-outcome-company"
        onClick={() => navigate(entry.item.company)}
      >
        {entry.item.company}
      </button>
      <button
        className="decision-outcome-undo"
        onClick={() => onUndo(entry.key)}
      >
        <Undo2 size={14} />
        {language === "zh" ? "撤销" : "Undo"}
      </button>
    </div>
  );
}
