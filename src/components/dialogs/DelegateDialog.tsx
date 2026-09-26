import type * as Model from "../../types/domain";
import React, { useState } from "react";
import { agents, teammates } from "./dialog-config";
import { X, ChevronDown, Lightbulb, Check, Sparkles } from "lucide-react";
import IconBadge from "./IconBadge";

interface DelegateDialogProps {
  language: Model.Language;
  instruction: string;
  setInstruction: Model.Setter<string>;
  companies: Model.CompanyRecord[];
  onClose: () => void;
  onDelegate: (task: Model.DelegatedTask) => void;
  closing: boolean;
  initialAssignee?: string;
}

export function DelegateDialog({
  language,
  instruction,
  setInstruction,
  companies,
  onClose,
  onDelegate,
  closing,
  initialAssignee = "Scout",
}: DelegateDialogProps) {
  const zh = language === "zh";
  const L = (en: string, cn?: string) => (zh ? (cn ?? en) : en);
  const [assignee, setAssignee] = useState(initialAssignee);
  const [company, setCompany] = useState("Halcyon Robotics");
  const [sources, setSources] = useState(["Email", "Web"]);
  const [askFirst, setAskFirst] = useState(true);
  const selectedAgent = agents.find(([name]) => name === assignee);
  const suggested = L(
    `Map ${company}’s buying committee and list any signals from the last 30 days.`,
    `梳理 ${company} 的采购决策人员，并列出最近 30 天的相关信号。`,
  );
  const sourceNames = [
    ["Email", "邮件"],
    ["Calls", "通话"],
    ["Calendar", "日历"],
    ["Web", "网页"],
    ["Usage", "使用数据"],
    ["Billing", "账单"],
  ];
  return (
    <div
      className={"modal-backdrop" + (closing ? " is-closing" : "")}
      onMouseDown={onClose}
    >
      <div
        className="form-dialog delegate-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={L("Delegate", "委派")}
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (
            (event.metaKey || event.ctrlKey) &&
            event.key === "Enter" &&
            instruction.trim()
          ) {
            event.preventDefault();
            onDelegate({
              title: instruction.trim(),
              assignee,
              company,
              sources,
              askFirst,
            });
          }
        }}
      >
        <div className="delegate-head">
          <h2>{L("Delegate", "委派")}</h2>
          <button
            className="overlay-close"
            aria-label={L("Close", "关闭")}
            onClick={onClose}
          >
            <X size={16} />
          </button>
        </div>
        <p className="delegate-intro">
          {L(
            "Hand work to an agent or a teammate. You’ll see receipts when it’s done.",
            "把任务交给智能团队或同事。完成后你会看到执行依据。",
          )}
        </p>
        <div className="delegate-body">
          <div className="delegate-label">{L("Assign to", "委派给")}</div>
          <div
            className="assignee-grid agents-grid"
            role="radiogroup"
            aria-label={L("Agents", "智能团队")}
          >
            {agents.map(([name, policy, policyZh]) => (
              <button
                key={name}
                role="radio"
                aria-checked={assignee === name}
                className={assignee === name ? "selected" : ""}
                onClick={() => {
                  setAssignee(name);
                  setAskFirst(policy !== "Autopilot");
                }}
              >
                <IconBadge name={name} />
                <span>
                  <b>{name}</b>
                  <small>{zh ? policyZh : policy}</small>
                </span>
              </button>
            ))}
          </div>
          <div
            className="assignee-grid teammates-grid"
            role="radiogroup"
            aria-label={L("Teammates", "同事")}
          >
            {teammates.map(([name, short]) => (
              <button
                key={name}
                role="radio"
                aria-checked={assignee === name}
                className={assignee === name ? "selected" : ""}
                onClick={() => {
                  setAssignee(name);
                  setAskFirst(false);
                }}
              >
                <IconBadge name={name} />
                <span>
                  <b>{short}</b>
                </span>
              </button>
            ))}
          </div>
          <div className="delegate-on">
            <span>{L("On", "关联")}</span>
            <label>
              <span className="company-mini">
                {company
                  .split(" ")
                  .map((part) => part[0])
                  .join("")}
              </span>
              <select
                aria-label={L("Record", "记录")}
                value={company}
                onChange={(event) => setCompany(event.target.value)}
              >
                {companies.map((item) => (
                  <option key={item.name}>{item.name}</option>
                ))}
              </select>
              <ChevronDown size={13} />
            </label>
          </div>
          <label className="delegate-label" htmlFor="delegate-instruction">
            {L("Instruction", "任务说明")}
          </label>
          <textarea
            id="delegate-instruction"
            aria-label={L("Instruction", "任务说明")}
            autoFocus
            placeholder={L(
              `What should ${assignee.split(" ")[0]} do?`,
              `${assignee.split(" ")[0]} 应该做什么？`,
            )}
            value={instruction}
            onChange={(event) => setInstruction(event.target.value)}
          />
          <button
            className="delegate-suggestion"
            onClick={() => setInstruction(suggested)}
          >
            <Lightbulb size={14} />
            <span>
              {L("Suggested: ", "建议：")}
              {suggested}
            </span>
          </button>
          <div className="delegate-label source-label">
            {L(
              `Sources ${assignee.split(" ")[0]} can use`,
              `${assignee.split(" ")[0]} 可使用的资料`,
            )}
          </div>
          <div className="delegate-sources">
            {sourceNames.map(([source, label]) => (
              <button
                key={source}
                aria-pressed={sources.includes(source)}
                onClick={() =>
                  setSources((current) =>
                    current.includes(source)
                      ? current.filter((item) => item !== source)
                      : [...current, source],
                  )
                }
              >
                {sources.includes(source) && <Check size={11} />}
                {zh ? label : source}
              </button>
            ))}
          </div>
          <label className="delegate-ask">
            <span>
              <b>{L("Ask before sending", "发送前询问")}</b>
              <small>
                {selectedAgent?.[1] === "Suggests"
                  ? L(
                      `${assignee} only suggests. Nothing leaves X Console until you act.`,
                      `${assignee} 只会提出建议，未经你确认不会发送。`,
                    )
                  : L(
                      "Review outbound actions before they are sent.",
                      "对外发送前先审核。",
                    )}
              </small>
            </span>
            <input
              type="checkbox"
              role="switch"
              checked={askFirst}
              disabled={selectedAgent?.[1] === "Suggests"}
              onChange={(event) => setAskFirst(event.target.checked)}
            />
            <span className="switch-track" />
          </label>
        </div>
        <div className="delegate-footer">
          <span>
            {instruction.trim()
              ? L("Ready to delegate.", "可以委派。")
              : L(
                  "Add an instruction to delegate.",
                  "输入任务说明后即可委派。",
                )}
          </span>
          <button className="delegate-cancel" onClick={onClose}>
            {L("Cancel", "取消")}
          </button>
          <button
            className="delegate-submit"
            disabled={!instruction.trim()}
            onClick={() =>
              onDelegate({
                title: instruction.trim(),
                assignee,
                company,
                sources,
                askFirst,
              })
            }
          >
            <Sparkles size={13} />
            {L(
              `Delegate to ${assignee.split(" ")[0]}`,
              `委派给 ${assignee.split(" ")[0]}`,
            )}
            <kbd>⌘↵</kbd>
          </button>
        </div>
      </div>
    </div>
  );
}
