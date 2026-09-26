import type * as Model from "../../../types/domain";
import React from "react";
import AccountCard from "./AccountCard";
import { forecastZh, shortDate, stageZh } from "../company-config";

interface CompanyDealsProps {
  L: Model.Localize;
  deals: Model.DealRecord[];
  company: Model.CompanyRecord;
  navigate: Model.Navigate;
  leadingDeal: Model.DealRecord | null;
}

export default function CompanyDeals({
  L,
  deals,
  company,
  navigate,
  leadingDeal,
}: CompanyDealsProps) {
  return (
    <AccountCard
      title={L("Deals", "商机")}
      count={deals.length}
      action={
        <strong className="ws-account-deals-total">${company.value}K</strong>
      }
    >
      <div className="ws-account-list">
        {deals
          .slice()
          .sort((a, b) => b.value - a.value)
          .map((deal) => (
            <button
              className="ws-account-deal-row"
              key={deal.id}
              onClick={() => navigate("Deals")}
            >
              <span>
                <b>{L(deal.title, deal.titleZh)}</b>
                <small>
                  {L(
                    deal.title === "Renewal"
                      ? "Renewal"
                      : deal.value === leadingDeal?.value
                        ? "New business"
                        : "Expansion",
                    deal.title === "Renewal"
                      ? "续约"
                      : deal.value === leadingDeal?.value
                        ? "新业务"
                        : "扩展",
                  )}{" "}
                  · {L(deal.forecast, forecastZh[deal.forecast])} ·{" "}
                  {L("closes", "预计结单")} {shortDate(deal.close, L)}
                </small>
              </span>
              <em>{L(deal.stage, stageZh[deal.stage])}</em>
              <strong>${deal.value}K</strong>
            </button>
          ))}
      </div>
    </AccountCard>
  );
}
