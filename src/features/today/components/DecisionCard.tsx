import type * as Model from "../../../types/domain";
import React, { useState, useRef, useEffect } from "react";
import { useT } from "../../../contexts/LanguageContext";
import AgentBadge from "../../../components/ui/AgentBadge";
import CompanyBadge from "../../../components/ui/CompanyBadge";
import { FileText, Check, PenLine, ChevronDown } from "lucide-react";
import DecisionPreview from "./DecisionPreview";

interface DecisionCardProps {
  item: Model.Decision;
  active: boolean;
  exiting: boolean;
  onAction: Model.DecisionHandler;
  onEdit: (item: Model.Decision) => void;
}

export default function DecisionCard({
  item,
  active,
  exiting,
  onAction,
  onEdit,
}: DecisionCardProps) {
  const t = useT();
  const [reassignOpen, setReassignOpen] = useState(false);
  const reassignRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!reassignOpen) return;
    const close = (event: PointerEvent) => {
      if (!reassignRef.current?.contains(event.target as Node))
        setReassignOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [reassignOpen]);
  return (
    <article
      className={`decision-card ${active ? "is-active" : ""} ${exiting ? "is-exiting" : ""} ${reassignOpen ? "menu-open" : ""}`}
    >
      <div className="decision-copy">
        <div className="decision-meta">
          <AgentBadge name={item.agent} />
          <b>{item.agent}</b>
          <span>{t(item.verb)}</span>
          <span className="dot">·</span>
          <time>{t(item.time)}</time>
        </div>
        <h3>{t(item.title)}</h3>
        <div className="company-line">
          <CompanyBadge initials={item.initials} color={item.companyColor} />
          <b>{item.company}</b>
          <span>· {t(item.detail)}</span>
        </div>
        <div className="receipts">
          <FileText size={13} />
          <span>{t(item.receipts)}</span>
          <em>{item.confidence}</em>
        </div>
        <div className="decision-actions">
          <button
            className={item.id === 1 ? "primary-action" : "approve-action"}
            onClick={() => onAction(item, "approved")}
          >
            <Check size={14} />
            {t(item.id === 1 ? "Send follow-up" : "Approve")}
            {item.id === 1 && <span className="return-key">↵</span>}
          </button>
          {item.id === 1 && (
            <button className="subtle-action" onClick={() => onEdit(item)}>
              <PenLine size={14} /> {t("Edit")}
            </button>
          )}
          {item.kind === "health" && (
            <div className="decision-reassign" ref={reassignRef}>
              <button
                className="subtle-action"
                aria-haspopup="menu"
                aria-expanded={reassignOpen}
                onClick={() => setReassignOpen((open) => !open)}
              >
                {t("Reassign")} <ChevronDown size={13} />
              </button>
              {reassignOpen && (
                <div
                  className="reassign-menu"
                  role="menu"
                  aria-label={t("Reassign")}
                >
                  <div className="reassign-menu-title">
                    {t("Who runs the check-in")}
                  </div>
                  {[
                    ["Ruth Adler", "Customer Success"],
                    ["Theo Park", "Account Executive"],
                    ["Ines Duarte", "Account Executive"],
                    ["Kofi Mensah", "Sales Development"],
                  ].map(([name, role]) => (
                    <button
                      key={name}
                      role="menuitem"
                      onClick={() => {
                        setReassignOpen(false);
                        onAction(item, "reassigned", { assignee: name });
                      }}
                    >
                      <span className="teammate-avatar">
                        {name
                          .split(" ")
                          .map((part) => part[0])
                          .join("")}
                      </span>
                      <span>
                        <b>{name}</b>
                        <small>{t(role)}</small>
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
          <button
            className="text-action"
            onClick={() => onAction(item, item.id === 3 ? "kept" : "dismissed")}
          >
            {t(
              item.id === 3 ? "Keep stage" : item.id === 1 ? "Skip" : "Dismiss",
            )}
          </button>
        </div>
      </div>
      <DecisionPreview kind={item.kind} openDraft={() => onEdit(item)} />
    </article>
  );
}
