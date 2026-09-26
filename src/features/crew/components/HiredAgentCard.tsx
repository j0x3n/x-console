import type * as Model from "../../../types/domain";
import React, { useState, useRef, useEffect } from "react";
import { hireTemplates, sourceNames, modeLabel } from "../hire-config";
import { TemplateIcon } from "./TemplateIcon";
import { MoreHorizontal } from "lucide-react";

interface HiredAgentCardProps {
  agent: Model.HiredAgent;
  language: Model.Language;
  onChange: (agent: Model.HiredAgent) => void;
  onRemove: () => void;
}

export function HiredAgentCard({
  agent,
  language,
  onChange,
  onRemove,
}: HiredAgentCardProps) {
  const template = hireTemplates.find((item) => item.id === agent.template);
  const L = (en: string, zh?: string) => (language === "zh" ? (zh ?? en) : en);
  const [menu, setMenu] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!menu) return;
    const outside = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenu(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenu(false);
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", escape);
    };
  }, [menu]);
  if (!template) return null;
  return (
    <section className="ws-crew-card hire-new-card" id={agent.id}>
      <div className="ws-crew-head">
        <TemplateIcon template={template} />
        <div>
          <h2>{agent.name}</h2>
          <p>{L(...template.job)}</p>
        </div>
        <span className="hire-new-label">
          {L("New", "新加入")}
          <small>{L("Starts 9:30 AM", "上午 9:30 开始")}</small>
        </span>
        <div className="ws-card-menu" ref={menuRef}>
          <button
            aria-label={L(`${agent.name} actions`, `${agent.name} 操作`)}
            aria-haspopup="menu"
            aria-expanded={menu}
            onClick={() => setMenu(!menu)}
          >
            <MoreHorizontal size={16} />
          </button>
          {menu && (
            <div role="menu">
              <button role="menuitem" onClick={onRemove}>
                {L("Remove from crew", "从团队移除")}
              </button>
            </div>
          )}
        </div>
      </div>
      <div className="ws-agent-stats hire-new-stats">
        <div>
          <small>
            {L("RUNS TODAY", "今日执行")}
            <em>{L("first at 9:30", "9:30 首次执行")}</em>
          </small>
          <strong>0</strong>
          <span>
            {L(`≈ ${template.runs} a day`, `每天约 ${template.runs} 次`)}
          </span>
        </div>
        <div>
          <small>
            {L("SUCCESS", "成功率")}
            <em>{L("30 days", "近 30 天")}</em>
          </small>
          <strong>—</strong>
          <span>{L("after 10 runs", "执行 10 次后统计")}</span>
        </div>
        <div>
          <small>
            {L("SPEND", "花费")}
            <em>{L(`of $${agent.budget}`, `预算 $${agent.budget}`)}</em>
          </small>
          <strong>$0.00</strong>
          <span className="hire-zero-ticks" aria-hidden="true">
            {Array.from({ length: 10 }, (_, i) => (
              <i key={i} />
            ))}
          </span>
        </div>
        <div>
          <small>
            {L("MEDIAN RUN", "执行中位时长")}
            <em>{L("today", "今天")}</em>
          </small>
          <strong>—</strong>
          <span>{L("no runs yet", "尚未执行")}</span>
        </div>
      </div>
      <div className="hire-next">
        <small>{L("Next", "下一步")}</small>
        <span>
          {L(
            `Reads its sources: ${agent.sources.map((source) => source.toLowerCase()).join(", ") || "none selected"}`,
            `读取资料：${agent.sources.map((source) => sourceNames[source]).join("、") || "未选择"}`,
          )}
        </span>
      </div>
      <div
        className="ws-autonomy"
        role="radiogroup"
        aria-label={L(`${agent.name} autonomy`, `${agent.name} 自主模式`)}
      >
        {(["Suggest only", "Ask first"] as const).map((mode) => (
          <button
            key={mode}
            role="radio"
            aria-checked={agent.mode === mode}
            className={agent.mode === mode ? "active" : ""}
            onClick={() => onChange({ ...agent, mode })}
          >
            {modeLabel(mode, language)}
          </button>
        ))}
      </div>
      <p className="ws-mode-note">
        {agent.mode === "Suggest only"
          ? L(
              "Proposes on Today. Nothing is sent or changed until you act. Autopilot opens after a week of receipts.",
              "在“今日”提出建议，只有你确认后才会发送或修改。一周执行记录后开放自主执行。",
            )
          : L(
              "Does the work, then waits for your OK before anything is sent or changed. Autopilot opens after a week of receipts.",
              "完成工作后等待你的批准，才会发送或修改。一周执行记录后开放自主执行。",
            )}
      </p>
    </section>
  );
}
