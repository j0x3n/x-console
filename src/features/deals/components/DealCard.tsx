import type * as Model from "../../../types/domain";
import React from "react";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import { forecastZh } from "../../../data/catalogs";
import Health from "../../../components/ui/Health";
import AgentIcon from "../../../components/ui/AgentIcon";
import { dealSignalMap } from "../deal-signals";

interface DealCardProps {
  deal: Model.DealRecord;
  navigate: Model.Navigate;
  L: Model.Localize;
  company: Model.CompanyRecord | undefined;
  pilotDecision: Model.Decision | undefined;
}

export default function DealCard({
  deal,
  navigate,
  L,
  company,
  pilotDecision,
}: DealCardProps) {
  return (
    <button
      className="ws-deal-card"
      key={deal.id}
      onClick={() => navigate(deal.company)}
    >
      <span className="ws-deal-title">
        <CompanyIcon company={deal.company} />
        <b>{deal.company}</b>
        <strong>${deal.value}K</strong>
      </span>
      <span className="ws-deal-sub">
        <span>{deal.title}</span>
        <em
          className={`ws-forecast-${deal.forecast.toLowerCase().replace(" ", "-")}`}
        >
          {L(deal.forecast, forecastZh[deal.forecast])}
        </em>
      </span>
      <span className="ws-deal-sub">
        <Health value={company?.health ?? 0} L={L} />
        <span>
          <AgentIcon name={deal.owner} />
          {deal.owner}
        </span>
        <time>{deal.close}</time>
      </span>
      {(deal.id === 26 && pilotDecision) ||
      dealSignalMap[`${deal.company}|${deal.titleEn}`] ? (
        <span className="ws-deal-signal">
          <AgentIcon
            name={
              deal.id === 26 && pilotDecision
                ? "Pilot"
                : dealSignalMap[`${deal.company}|${deal.titleEn}`][0]
            }
          />
          {deal.id === 26 && pilotDecision
            ? L("Proposes moving to Negotiation", "建议移至谈判阶段")
            : L(
                dealSignalMap[`${deal.company}|${deal.titleEn}`][1],
                dealSignalMap[`${deal.company}|${deal.titleEn}`][2],
              )}
        </span>
      ) : null}
    </button>
  );
}
