import type * as Model from "../../../types/domain";
import AgentIcon from "../../../components/ui/AgentIcon";
import React from "react";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import { FileText } from "lucide-react";

interface RecentRunsProps {
  L: Model.Localize;
  recentRuns: Model.CrewRun[];
  agentFilter: string;
  setAgentFilter: Model.Setter<string>;
  agentNames: string[];
  visibleRuns: Model.CrewRun[];
  firstOffline: number;
  navigate: Model.Navigate;
  decisions: Model.Decision[];
  filteredActivity: Model.CrewRun[];
  setShowAll: Model.Setter<boolean>;
  showAll: boolean;
}

export default function RecentRuns({
  L,
  recentRuns,
  agentFilter,
  setAgentFilter,
  agentNames,
  visibleRuns,
  firstOffline,
  navigate,
  decisions,
  filteredActivity,
  setShowAll,
  showAll,
}: RecentRunsProps) {
  return (
    <section className="ws-recent" id="recent-runs">
      <div className="ws-section-heading">
        <h2>
          {L("Recent runs", "最近执行")} <span>{recentRuns.length}</span>
        </h2>
        <div className="ws-agent-filters">
          <button
            className={agentFilter === "All" ? "active" : ""}
            onClick={() => setAgentFilter("All")}
          >
            {L("All", "全部")}
          </button>
          {agentNames.map((name) => (
            <button
              key={name}
              className={agentFilter === name ? "active" : ""}
              onClick={() => setAgentFilter(name)}
              title={name}
            >
              <AgentIcon name={name} />
              <span>{name}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="ws-table-wrap">
        <table className="ws-table ws-runs-table">
          <thead>
            <tr>
              {[
                L("Time", "时间"),
                L("Agent", "助手"),
                L("Run", "执行内容"),
                L("Record", "记录"),
                L("Receipt", "依据"),
                L("Took", "耗时"),
                L("Cost", "成本"),
              ].map((label) => (
                <th key={label}>{label}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRuns.map((row, index) => (
              <React.Fragment key={row.id}>
                {index === firstOffline && (
                  <tr className="ws-group-row ws-offline-row">
                    <td colSpan={7}>
                      <span>{L("While you were offline", "你离线期间")}</span>
                      <small>{L("37 runs overnight", "夜间执行 37 次")}</small>
                    </td>
                  </tr>
                )}
                <tr onClick={() => navigate(row.record)}>
                  <td>{row.time}</td>
                  <td>
                    <AgentIcon name={row.agent} />
                    {row.agent}
                  </td>
                  <td>
                    {L(row.run, row.runZh)}
                    {row.decision &&
                      decisions.some((item) => item.id === row.decision) && (
                        <button
                          className="ws-run-call"
                          onClick={(event) => {
                            event.stopPropagation();
                            navigate("Today");
                          }}
                        >
                          {L("Your call", "待你决定")}
                        </button>
                      )}
                  </td>
                  <td>
                    <CompanyIcon company={row.record} />
                    {row.record}
                  </td>
                  <td>
                    <FileText size={12} />
                    {L(row.receipt, row.receiptZh)}
                  </td>
                  <td>{row.took}</td>
                  <td>{row.cost === null ? "—" : `$${row.cost.toFixed(2)}`}</td>
                </tr>
              </React.Fragment>
            ))}
          </tbody>
        </table>
      </div>
      <p className="ws-runs-summary">
        {L(
          `Latest ${filteredActivity.length} of 142 runs today`,
          `今日 142 次执行中的最近 ${filteredActivity.length} 次`,
        )}{" "}
        · $
        {filteredActivity
          .reduce((sum, row) => sum + (row.cost || 0), 0)
          .toFixed(2)}{" "}
        · {L("median 58s", "中位耗时 58 秒")}
      </p>
      {filteredActivity.length > 12 && (
        <button className="ws-show-more" onClick={() => setShowAll(!showAll)}>
          {showAll
            ? L("Show less", "收起")
            : L(
                `Show ${filteredActivity.length - 12} more`,
                `再显示 ${filteredActivity.length - 12} 项`,
              )}
        </button>
      )}
    </section>
  );
}
