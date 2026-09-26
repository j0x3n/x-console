import type * as Model from "../../../types/domain";
import React from "react";

interface RecordsSummaryProps {
  filtered: Model.RecordData[];
  L: Model.Localize;
  isPeople: boolean;
  visibleCompanies: Set<string | undefined>;
  visibleDeals: number;
  warmPeople: number;
  pipelineLabel: string;
  crewKnown: number;
  averageHealth: number;
  crewTouched: number;
}

export default function RecordsSummary({
  filtered,
  L,
  isPeople,
  visibleCompanies,
  visibleDeals,
  warmPeople,
  pipelineLabel,
  crewKnown,
  averageHealth,
  crewTouched,
}: RecordsSummaryProps) {
  return (
    <div className="ws-table-footer">
      <span>
        {filtered.length}{" "}
        {L(isPeople ? "people" : "companies", isPeople ? "位联系人" : "家公司")}
      </span>
      <span>
        {isPeople
          ? L(
              `${visibleCompanies.size} companies`,
              `${visibleCompanies.size} 家公司`,
            )
          : L(`${visibleDeals} open deals`, `${visibleDeals} 笔进行中的商机`)}
      </span>
      <span>
        {isPeople
          ? L(`${warmPeople} warm`, `${warmPeople} 位活跃`)
          : pipelineLabel}
      </span>
      <span>
        {isPeople
          ? L(`${crewKnown} known by crew`, `${crewKnown} 位由智能团队熟悉`)
          : L(
              `Average health ${averageHealth} · ${crewTouched} by your crew`,
              `平均健康度 ${averageHealth} · ${crewTouched} 家由智能团队更新`,
            )}
      </span>
    </div>
  );
}
