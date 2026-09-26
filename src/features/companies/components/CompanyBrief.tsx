import type * as Model from "../../../types/domain";
import React from "react";
import AccountCard from "./AccountCard";
import AccountIcon from "./AccountIcon";

interface CompanyBriefProps {
  L: Model.Localize;
  detail: Model.CompanyDetail | undefined;
  company: Model.CompanyRecord;
  brief: string[];
  briefZh: string[];
}

export default function CompanyBrief({
  L,
  detail,
  company,
  brief,
  briefZh,
}: CompanyBriefProps) {
  return (
    <AccountCard title={L("Brief", "简报")} className="ws-account-brief">
      <div className="ws-account-byline">
        <AccountIcon name={detail?.briefAgent || company.agent} />
        {L("by", "由")} <b>{detail?.briefAgent || company.agent}</b> ·{" "}
        {detail?.briefTime || company.touch}
      </div>
      {brief.map((paragraph, index) => (
        <p key={index}>{L(paragraph, briefZh[index])}</p>
      ))}
      <div className="ws-account-sources">
        {(
          detail?.sources || [
            L("CRM activity", "CRM 动态"),
            L("Account signals", "客户信号"),
          ]
        ).map((source, index) => (
          <div key={source}>
            <span>{index + 1}</span>
            {source}
          </div>
        ))}
      </div>
    </AccountCard>
  );
}
