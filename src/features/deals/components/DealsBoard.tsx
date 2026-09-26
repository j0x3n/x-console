import type * as Model from "../../../types/domain";
import React from "react";
import { stages, stageZh } from "../../../data/catalogs";
import { companyRecords } from "../../../data/workspace";
import DealCard from "./DealCard";
import AgentIcon from "../../../components/ui/AgentIcon";
import { ArrowRight, Check } from "lucide-react";
import DecisionOutcomeRow from "../../../components/decisions/DecisionOutcomeRow";

interface DealsBoardProps {
  visible: Model.DealRecord[];
  L: Model.Localize;
  navigate: Model.Navigate;
  pilotDecision: Model.Decision | undefined;
  owner: string;
  forecast: string;
  exitingDecisions: Model.Decision[];
  onAction: Model.DecisionHandler;
  pilotOutcome: Model.DecisionEntry | undefined;
  language: Model.Language;
  onUndoDecision: (key: string) => void;
}

export default function DealsBoard({
  visible,
  L,
  navigate,
  pilotDecision,
  owner,
  forecast,
  exitingDecisions,
  onAction,
  pilotOutcome,
  language,
  onUndoDecision,
}: DealsBoardProps) {
  return (
    <div className="ws-board ws-deal-board">
      {stages.map((stage) => {
        const rows = visible.filter((item) => item.stage === stage);
        const total = rows.reduce((sum, item) => sum + item.value, 0);
        return (
          <section className="ws-lane" key={stage}>
            <div className="ws-lane-heading">
              <h2>
                {L(stage, stageZh[stage])} <em>{rows.length}</em>
              </h2>
              <small>
                ${total >= 1000 ? `${(total / 1000).toFixed(2)}M` : `${total}K`}
              </small>
            </div>
            <div className="ws-stage-meter">
              <i style={{ width: `${Math.min(100, total / 16)}%` }} />
            </div>
            <div className="ws-lane-list">
              {rows.map((deal) => {
                const company = companyRecords.find(
                  (item) => item.name === deal.company,
                );
                return (
                  <DealCard
                    key={deal.id}
                    deal={deal}
                    navigate={navigate}
                    L={L}
                    company={company}
                    pilotDecision={pilotDecision}
                  />
                );
              })}
              {stage === "Negotiation" &&
                pilotDecision &&
                (owner === "All" || owner === "Ines") &&
                (forecast === "All" || forecast === "Commit") && (
                  <div
                    id="ws-pilot-proposal"
                    className={`ws-work-card ws-proposal-card ${exitingDecisions.some((item) => item.id === pilotDecision.id) ? "is-exiting" : ""}`}
                  >
                    <div className="ws-card-meta">
                      <AgentIcon name="Pilot" />
                      <b>Pilot</b>
                      <span>{L("proposes a move", "建议调整阶段")}</span>
                      <em>95%</em>
                    </div>
                    <h3>Brightwell Labs · $248K</h3>
                    <div className="ws-stage-change">
                      {L("Proposal", "方案")} <ArrowRight size={13} />{" "}
                      <b>{L("Negotiation", "谈判")}</b>
                    </div>
                    <div className="ws-review-actions">
                      <button
                        className="ws-approve"
                        onClick={() => onAction(pilotDecision, "approved")}
                      >
                        <Check size={13} />
                        {L("Approve", "批准")}
                      </button>
                      <button onClick={() => onAction(pilotDecision, "kept")}>
                        {L("Keep stage", "保持阶段")}
                      </button>
                    </div>
                  </div>
                )}
              {stage === "Negotiation" &&
                pilotOutcome &&
                !exitingDecisions.some(
                  (item) => item.id === pilotOutcome.item.id,
                ) &&
                (owner === "All" || owner === "Ines") &&
                (forecast === "All" || forecast === "Commit") && (
                  <DecisionOutcomeRow
                    entry={pilotOutcome}
                    language={language}
                    navigate={navigate}
                    onUndo={onUndoDecision}
                  />
                )}
            </div>
          </section>
        );
      })}
    </div>
  );
}
