import type * as Model from "../../../types/domain";
import React, { useState, useRef, useEffect } from "react";
import {
  hireTemplates,
  radioKeys,
  sourceNames,
  modeLabel,
} from "../hire-config";
import { createPortal } from "react-dom";
import { X, Check } from "lucide-react";
import { TemplateIcon } from "./TemplateIcon";

interface HireAgentDialogProps {
  language: Model.Language;
  hired: Model.HiredAgent[];
  onHire: (agent: Model.HiredAgent) => void;
  onClose: () => void;
}

export function HireAgentDialog({
  language,
  hired,
  onHire,
  onClose,
}: HireAgentDialogProps) {
  const L = (en: string, zh?: string) => (language === "zh" ? (zh ?? en) : en);
  const available = hireTemplates.filter(
    (template) => !hired.some((agent) => agent.template === template.id),
  );
  const [templateId, setTemplateId] = useState(available[0]?.id);
  const template = available.find((item) => item.id === templateId);
  const [name, setName] = useState(template?.name || "");
  const [sources, setSources] = useState(template?.sources || []);
  const [mode, setMode] = useState<Model.AgentMode>("Suggest only");
  const [budget, setBudget] = useState(4);
  const [closing, setClosing] = useState(false);
  const dialogRef = useRef<HTMLElement>(null);
  const nameRef = useRef<HTMLInputElement>(null);
  const closingRef = useRef(false);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const requestClose = () => {
    if (closingRef.current) return;
    closingRef.current = true;
    setClosing(true);
    closeTimer.current = setTimeout(onClose, 150);
  };
  useEffect(() => {
    const previousFocus = document.activeElement;
    const appRoot = document.getElementById("root");
    const previousInert = appRoot?.inert;
    if (appRoot) appRoot.inert = true;
    nameRef.current?.focus();
    return () => {
      if (closeTimer.current !== null) clearTimeout(closeTimer.current);
      if (appRoot) appRoot.inert = previousInert ?? false;
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected)
        previousFocus.focus();
    };
  }, []);
  const submit = () => {
    if (!template || !name.trim() || closingRef.current) return;
    onHire({
      id: `hired-${template.id}-${Date.now()}`,
      template: template.id,
      name: name.trim(),
      sources: [...sources],
      mode,
      budget,
    });
  };
  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    event.stopPropagation();
    if (event.key === "Escape") {
      event.stopPropagation();
      requestClose();
    }
    if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
      event.preventDefault();
      submit();
    }
    if (event.key === "Tab") {
      const controls = [
        ...(dialogRef.current?.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), [tabindex="0"]',
        ) ?? []),
      ];
      const first = controls[0],
        last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    }
  };
  const totalBudget =
    30 + hired.reduce((sum, agent) => sum + agent.budget, 0) + budget;
  return createPortal(
    <div
      className={`hire-backdrop${closing ? " is-closing" : ""}`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) requestClose();
      }}
      onKeyDown={onKeyDown}
    >
      <section
        ref={dialogRef}
        className="hire-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="hire-dialog-title"
        aria-describedby="hire-dialog-description"
      >
        <header className="hire-dialog-head">
          <h2 id="hire-dialog-title">{L("Hire an agent", "雇佣助手")}</h2>
          <p id="hire-dialog-description">
            {L(
              "Pick a job. It joins the crew with a name, a budget and the autonomy you set.",
              "选择一项工作，为助手设置名称、预算与自主模式，然后加入智能团队。",
            )}
          </p>
          <button
            className="hire-close"
            aria-label={L("Close", "关闭")}
            onClick={requestClose}
          >
            <X size={16} />
          </button>
        </header>
        {template ? (
          <div className="hire-dialog-body">
            <div
              className="hire-template-picker"
              role="radiogroup"
              aria-label={L("Templates", "模板")}
              onKeyDown={radioKeys}
            >
              {available.map((item) => (
                <button
                  key={item.id}
                  role="radio"
                  aria-checked={template.id === item.id}
                  className={template.id === item.id ? "selected" : ""}
                  onClick={() => {
                    setTemplateId(item.id);
                    setName(item.name);
                    setSources([...item.sources]);
                  }}
                >
                  <TemplateIcon template={item} />
                  <span>
                    <b>{item.name}</b>
                    <small>{L(...item.job)}</small>
                  </span>
                </button>
              ))}
            </div>
            <div className="hire-template-config">
              <div className="hire-name-row">
                <TemplateIcon template={template} large />
                <div>
                  <label className="hire-sr-only" htmlFor="hire-name">
                    {L("Name", "名称")}
                  </label>
                  <input
                    id="hire-name"
                    ref={nameRef}
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    aria-invalid={!name.trim()}
                  />
                  <p className={!name.trim() ? "hire-name-error" : ""}>
                    {name.trim()
                      ? L(...template.job)
                      : L("Give your agent a name.", "请为助手填写名称。")}
                  </p>
                </div>
              </div>
              <div className="hire-config-section">
                <span className="hire-label">
                  {L("From day one", "从第一天开始")}
                </span>
                <ul>
                  {template.duties.map((duty) => (
                    <li key={duty[0]}>
                      <Check size={14} />
                      <span>{L(...duty)}</span>
                    </li>
                  ))}
                </ul>
              </div>
              <div className="hire-config-section">
                <span className="hire-label">
                  {L("Sources it can use", "可使用的资料")}
                </span>
                <div className="hire-sources">
                  {Object.keys(sourceNames).map((source) => (
                    <button
                      key={source}
                      role="checkbox"
                      aria-checked={sources.includes(source)}
                      className={sources.includes(source) ? "selected" : ""}
                      onClick={() =>
                        setSources((current) =>
                          current.includes(source)
                            ? current.filter((item) => item !== source)
                            : [...current, source],
                        )
                      }
                    >
                      {sources.includes(source) && <Check size={12} />}
                      {L(source, sourceNames[source])}
                    </button>
                  ))}
                </div>
              </div>
              <div className="hire-config-columns">
                <div>
                  <span className="hire-label">
                    {L("Starts on", "初始自主模式")}
                  </span>
                  <div
                    className="hire-segment"
                    role="radiogroup"
                    aria-label={L("Starting autonomy", "初始自主模式")}
                    onKeyDown={radioKeys}
                  >
                    {(["Suggest only", "Ask first"] as const).map((option) => (
                      <button
                        key={option}
                        role="radio"
                        aria-checked={mode === option}
                        className={mode === option ? "selected" : ""}
                        onClick={() => setMode(option)}
                      >
                        {modeLabel(option, language)}
                      </button>
                    ))}
                  </div>
                  <p>
                    {mode === "Suggest only"
                      ? L(
                          "Proposes on Today. Nothing leaves Keel until you act. Autopilot opens after a week of receipts.",
                          "在“今日”提出建议，只有你确认后才会发送。一周执行记录后开放自主执行。",
                        )
                      : L(
                          `Does the work, then asks. About ${template.calls} calls a day on Today. Autopilot opens after a week of receipts.`,
                          `完成工作后询问你，每天约 ${template.calls} 项待决定事项显示在“今日”。一周执行记录后开放自主执行。`,
                        )}
                  </p>
                </div>
                <div>
                  <span className="hire-label">
                    {L("Daily budget", "每日预算")}
                  </span>
                  <div
                    className="hire-segment"
                    role="radiogroup"
                    aria-label={L("Daily budget", "每日预算")}
                    onKeyDown={radioKeys}
                  >
                    {[2, 4, 6].map((amount) => (
                      <button
                        key={amount}
                        role="radio"
                        aria-checked={budget === amount}
                        className={budget === amount ? "selected" : ""}
                        onClick={() => setBudget(amount)}
                      >
                        ${amount}
                      </button>
                    ))}
                  </div>
                  <p>
                    {L(
                      `≈ ${template.runs} runs a day for ≈ $${template.cost.toFixed(2)}. The crew’s budget goes to $${totalBudget}.`,
                      `每天约 ${template.runs} 次执行，花费约 $${template.cost.toFixed(2)}。团队每日预算将增至 $${totalBudget}。`,
                    )}
                  </p>
                </div>
              </div>
            </div>
          </div>
        ) : (
          <p className="hire-empty">
            {L(
              "All four templates have joined your crew.",
              "四个模板都已加入智能团队。",
            )}
          </p>
        )}
        <footer className="hire-dialog-footer">
          <p>
            {template &&
              L(
                `${name.trim() || template.name} starts at 9:30 AM. You can change anything on its card.`,
                `${name.trim() || template.name} 将于上午 9:30 开始执行。你可以在助手卡片上调整设置。`,
              )}
          </p>
          <button className="hire-cancel" onClick={requestClose}>
            {L("Cancel", "取消")}
          </button>
          <button
            className="hire-submit"
            disabled={!template || !name.trim()}
            onClick={submit}
          >
            {L(
              `Hire ${name.trim() || template?.name || ""}`,
              `雇佣 ${name.trim() || template?.name || ""}`,
            )}
            <kbd>⌘↵</kbd>
          </button>
        </footer>
      </section>
    </div>,
    document.body,
  );
}
