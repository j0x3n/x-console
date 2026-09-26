import type * as Model from "../../../types/domain";
import { stages, stageZh, forecastZh } from "../../../data/catalogs";
import React from "react";
import { companyRecords } from "../../../data/workspace";
import { dealSignalMap } from "../deal-signals";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import AgentIcon from "../../../components/ui/AgentIcon";
import Health from "../../../components/ui/Health";

interface DealsTableProps {
  L: Model.Localize;
  visible: Model.DealRecord[];
  formatTotal: (value: number) => string;
  pilotDecision: Model.Decision | undefined;
  navigate: Model.Navigate;
  decisions: Model.Decision[];
}

export default function DealsTable({
  L,
  visible,
  formatTotal,
  pilotDecision,
  navigate,
  decisions,
}: DealsTableProps) {
  return (
    <div className="ws-table-wrap">
      <table className="ws-table ws-deal-table">
        <thead>
          <tr>
            {[
              L("Deal", "商机"),
              L("Stage", "阶段"),
              L("Forecast", "预测"),
              L("Owner", "负责人"),
              L("Close", "结单"),
              L("Health", "健康度"),
              L("Value", "金额"),
              L("Agent signal", "智能体信号"),
            ].map((heading) => (
              <th key={heading}>{heading}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {[...stages].reverse().map((stage) => {
            const rows = visible.filter((deal) => deal.stage === stage);
            if (!rows.length) return null;
            const total = rows.reduce((sum, deal) => sum + deal.value, 0);
            return (
              <React.Fragment key={stage}>
                <tr className="ws-group-row">
                  <td colSpan={8}>
                    <b>{L(stage, stageZh[stage])}</b>
                    <span>{rows.length}</span>
                    <small>
                      {stage === "Negotiation"
                        ? L("all commit", "全部为确定收入")
                        : stage === "Proposal"
                          ? L("$248K commit", "$248K 确定收入")
                          : ""}
                    </small>
                    <strong>{formatTotal(total)}</strong>
                  </td>
                </tr>
                {rows.map((deal) => {
                  const company = companyRecords.find(
                    (row) => row.name === deal.company,
                  );
                  const signalData =
                    dealSignalMap[`${deal.company}|${deal.titleEn}`];
                  const signal =
                    deal.id === 26 && pilotDecision
                      ? L("Proposes moving to Negotiation", "建议移至谈判阶段")
                      : signalData
                        ? L(signalData[1], signalData[2])
                        : "";
                  const signalAgent =
                    deal.id === 26 && pilotDecision ? "Pilot" : signalData?.[0];
                  return (
                    <tr
                      key={deal.id}
                      id={
                        deal.id === 26 && pilotDecision
                          ? "ws-pilot-proposal"
                          : undefined
                      }
                      onClick={() => navigate(deal.company)}
                      tabIndex={0}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") navigate(deal.company);
                      }}
                    >
                      <td>
                        <span className="ws-deal-table-name">
                          <CompanyIcon company={deal.company} />
                          <b>{deal.company}</b>
                          <span>{deal.title}</span>
                        </span>
                      </td>
                      <td>
                        <span className="ws-deal-stage-chip">
                          ▥ {L(deal.stage, stageZh[deal.stage])}
                        </span>
                        {deal.id === 26 && pilotDecision && (
                          <span className="ws-deal-stage-hint">▲</span>
                        )}
                      </td>
                      <td>
                        <span
                          className={`ws-deal-forecast ws-forecast-label-${deal.forecast.toLowerCase().replace(" ", "-")}`}
                        >
                          {L(deal.forecast, forecastZh[deal.forecast])}
                        </span>
                      </td>
                      <td>
                        <span className="ws-list-assignee">
                          <AgentIcon name={deal.owner} />
                          {deal.owner}
                        </span>
                      </td>
                      <td>{deal.close}</td>
                      <td>
                        <Health value={company?.health || 0} L={L} />
                      </td>
                      <td>
                        <b>${deal.value}K</b>
                      </td>
                      <td>
                        {signal ? (
                          <span className="ws-deal-table-signal">
                            <AgentIcon name={signalAgent} />
                            {signal}
                            {decisions.some(
                              (decision) => decision.company === deal.company,
                            ) &&
                              ((deal.company === "Brightwell Labs" &&
                                deal.titleEn === "Platform") ||
                                (deal.company === "Halcyon Robotics" &&
                                  deal.titleEn === "Fleet platform") ||
                                (deal.company === "Oakline Freight" &&
                                  deal.titleEn === "Renewal")) && (
                                <i
                                  className="ws-signal-pending"
                                  title={L("Needs your call", "待你决定")}
                                />
                              )}
                          </span>
                        ) : (
                          "—"
                        )}
                      </td>
                    </tr>
                  );
                })}
              </React.Fragment>
            );
          })}
        </tbody>
      </table>
      <div className="ws-table-footer">
        <span>
          {L(
            `${visible.length} deals · soonest close first in each stage`,
            `${visible.length} 笔商机 · 各阶段按最近结单日期排序`,
          )}
        </span>
        <strong>
          {formatTotal(visible.reduce((sum, deal) => sum + deal.value, 0))}
        </strong>
      </div>
    </div>
  );
}
