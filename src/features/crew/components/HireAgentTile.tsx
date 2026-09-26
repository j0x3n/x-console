import type * as Model from "../../../types/domain";
import React from "react";
import { TemplateIcon } from "./TemplateIcon";
import { Plus } from "lucide-react";

interface HireAgentTileProps {
  setHireOpen: Model.Setter<boolean>;
  availableTemplates: Model.HireTemplate[];
  L: Model.Localize;
}

export default function HireAgentTile({
  setHireOpen,
  availableTemplates,
  L,
}: HireAgentTileProps) {
  return (
    <button
      className="ws-hire-tile"
      id="hire"
      onClick={() => setHireOpen(true)}
    >
      <span className="ws-hire-icons">
        {availableTemplates.map((template) => (
          <TemplateIcon template={template} key={template.id} />
        ))}
        <i>
          <Plus size={18} />
        </i>
      </span>
      <strong>{L("Hire an agent", "雇佣助手")}</strong>
      <span>
        {L(
          `${availableTemplates.length} templates: ${availableTemplates.map((template) => ({ relay: "inbound", sentry: "competitors", tally: "forecast", bridge: "handoffs" })[template.id]).join(", ")}. New hires start on Suggest only.`,
          `${availableTemplates.length} 个模板：${availableTemplates.map((template) => ({ relay: "新咨询", sentry: "竞品", tally: "预测", bridge: "交接" })[template.id]).join("、") || "已全部雇佣"}。新助手默认使用“仅建议”模式。`,
        )}
      </span>
      <em>
        <Plus size={13} />
        {L("Browse templates", "浏览模板")}
      </em>
    </button>
  );
}
