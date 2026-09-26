import type * as Model from "../../../types/domain";
import React from "react";
import AccountIcon from "./AccountIcon";
import { stageZh, stageNames, shortDate } from "../company-config";
import AccountHealth from "./AccountHealth";
import { Check } from "lucide-react";

interface CompanyOverviewProps {
  L: Model.Localize;
  pending: Model.Decision | undefined;
  company: Model.CompanyRecord;
  deals: Model.DealRecord[];
  detail: Model.CompanyDetail | undefined;
  leadingDeal: Model.DealRecord | null;
  stepAccepted: boolean;
  setStepAccepted: Model.Setter<boolean>;
}

export default function CompanyOverview({
  L,
  pending,
  company,
  deals,
  detail,
  leadingDeal,
  stepAccepted,
  setStepAccepted,
}: CompanyOverviewProps) {
  return (
    <section className="ws-account-overview">
      <div className="ws-account-stage">
        <div className="ws-account-metric-label">
          {L("Stage", "阶段")}
          {pending?.kind === "stage" && (
            <span className="ws-stage-proposal">
              <AccountIcon name="Pilot" />
              {L("Pilot proposes Negotiation", "Pilot 建议进入谈判")}
            </span>
          )}
        </div>
        <ol
          aria-label={L(
            `Stage: ${company.stage}`,
            `阶段：${stageZh[company.stage]}`,
          )}
        >
          {stageNames.map((stage, index) => (
            <li key={stage}>
              {index > 0 && <i className="ws-stage-line" />}
              <span
                className={
                  stage === company.stage
                    ? "current"
                    : index < stageNames.indexOf(company.stage)
                      ? "passed"
                      : "future"
                }
              >
                {L(stage, stageZh[stage])}
              </span>
            </li>
          ))}
        </ol>
      </div>
      <div className="ws-account-metrics">
        <div className="ws-account-metric">
          <div className="ws-account-metric-label">
            {L("Health", "健康度")}
            <span>
              {L(
                `${company.health >= 75 ? "Healthy" : company.health >= 50 ? "Watch" : "At risk"} · ${company.change > 0 ? "+" : ""}${company.change} in 14d`,
                `${company.health >= 75 ? "健康" : company.health >= 50 ? "需关注" : "有风险"} · 14 天${company.change > 0 ? "+" : ""}${company.change}`,
              )}
            </span>
          </div>
          <AccountHealth value={company.health} change={company.change} L={L} />
        </div>
        <div className="ws-account-metric">
          <div className="ws-account-metric-label">
            {L("Open pipeline", "进行中商机")}
            <span>
              {deals.length}{" "}
              {L(deals.length === 1 ? "deal" : "deals", "笔商机")}
            </span>
          </div>
          <div className="ws-account-metric-value">
            <strong>${company.value}K</strong>
            <span>{detail?.bestCase || L("open", "进行中")}</span>
          </div>
        </div>
        <div className="ws-account-metric">
          <div className="ws-account-metric-label">
            {L(
              company.name === "Oakline Freight" ? "ARR" : "Next close",
              company.name === "Oakline Freight"
                ? "年度经常性收入"
                : "下次结单",
            )}
          </div>
          <div className="ws-account-metric-value">
            <strong>
              {company.name === "Oakline Freight"
                ? "$184K"
                : shortDate(detail?.nextClose || leadingDeal?.close || "—", L)}
            </strong>
            <span>
              {company.name === "Oakline Freight"
                ? L("renews in 67 days", "67 天后续约")
                : leadingDeal
                  ? `$${leadingDeal.value}K · ${L(leadingDeal.title, leadingDeal.titleZh)}`
                  : "—"}
            </span>
          </div>
        </div>
      </div>
      <div className="ws-account-next">
        <span className="ws-account-metric-label">
          {L("Next step", "下一步")}
        </span>
        <p>
          {pending && pending.kind !== "draft" && (
            <AccountIcon name={pending.agent} />
          )}
          {L(company.next, company.nextZh)}{" "}
          <small>
            ·{" "}
            {L(
              pending && pending.kind !== "draft"
                ? `suggested by ${pending.agent}`
                : `set by ${company.agent}`,
              pending && pending.kind !== "draft"
                ? `由 ${pending.agent} 建议`
                : `由 ${company.agent} 设置`,
            )}
            {(detail?.nextDue || leadingDeal?.close) &&
              ` · ${L("due", "截止")} ${detail?.nextDue === "today, 10:00 AM" ? L("today, 10:00 AM", "今天 10:00") : shortDate(detail?.nextDue || leadingDeal?.close, L)}`}
          </small>
        </p>
        {pending && pending.kind !== "draft" && !stepAccepted && (
          <button onClick={() => setStepAccepted(true)}>
            {L("Accept step", "接受下一步")}
          </button>
        )}
        {stepAccepted && (
          <span className="ws-step-accepted">
            <Check size={13} />
            {L("Accepted", "已接受")}
          </span>
        )}
      </div>
    </section>
  );
}
