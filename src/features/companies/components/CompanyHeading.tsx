import type * as Model from "../../../types/domain";
import React from "react";
import AccountIcon from "./AccountIcon";
import { industryZh, names } from "../company-config";
import { Check, Link2, SquarePen } from "lucide-react";

interface CompanyHeadingProps {
  company: Model.CompanyRecord;
  L: Model.Localize;
  accountType: string;
  domain: string;
  detail: Model.CompanyDetail | undefined;
  copyLink: () => Promise<void>;
  copied: boolean;
  openDelegate: Model.OpenDelegate;
}

export default function CompanyHeading({
  company,
  L,
  accountType,
  domain,
  detail,
  copyLink,
  copied,
  openDelegate,
}: CompanyHeadingProps) {
  return (
    <header className="ws-company-heading">
      <AccountIcon name={company.name} company />
      <div className="ws-account-identity">
        <div className="ws-account-title">
          <h1>{company.name}</h1>
          <span className="ws-account-tag">
            {L(
              company.segment,
              {
                Enterprise: "企业客户",
                "Mid-market": "中型客户",
                Startup: "初创企业",
              }[company.segment],
            )}
          </span>
          <span className="ws-account-tag filled">
            <i />
            {L(
              accountType,
              accountType === "Customer" ? "现有客户" : "潜在客户",
            )}
          </span>
        </div>
        <p>
          <span className="ws-account-domain">{domain}</span>
          <span>·</span>
          {detail?.industry && (
            <>
              <span>{L(detail.industry, industryZh[detail.industry])}</span>
              <span>·</span>
            </>
          )}
          {detail?.size && (
            <>
              <span>{L(`${detail.size} people`, `${detail.size} 人`)}</span>
              <span>·</span>
            </>
          )}
          {detail?.location && <span>{detail.location}</span>}
        </p>
      </div>
      <div className="ws-account-heading-actions">
        <span className="ws-owner">
          <AccountIcon name={company.owner} />
          <span>{L("Owner", "负责人")}</span>
          <b>{names[company.owner] || company.owner}</b>
        </span>
        <button
          className="ws-secondary ws-copy-button"
          onClick={copyLink}
          title={L("Copy link", "复制链接")}
          aria-label={L("Copy link", "复制链接")}
        >
          {copied ? <Check size={15} /> : <Link2 size={15} />}
        </button>
        <button className="ws-secondary" onClick={openDelegate}>
          <SquarePen size={15} />
          {L("Delegate", "委派")}
        </button>
      </div>
    </header>
  );
}
