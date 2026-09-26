import type * as Model from "../../../types/domain";
import React from "react";
import AgentIcon from "../../../components/ui/AgentIcon";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import { taskTypeZh } from "../work-config";
import DecisionOutcomeRow from "../../../components/decisions/DecisionOutcomeRow";

interface WorkListProps {
  L: Model.Localize;
  groupBy: string;
  changeGroup: (group: string) => void;
  listGroups: string[];
  groupItems: (group: string) => Model.WorkItem[];
  labels: Model.StringMap;
  listMeta: (item: Model.WorkItem) => (string | undefined)[];
  setSelected: Model.Setter<Model.WorkItem | null>;
  navigate: Model.Navigate;
  onEdit: (item: Model.Decision) => void;
  onAction: Model.DecisionHandler;
  visibleHistory: Model.DecisionEntry[];
  exitingDecisions: Model.Decision[];
  language: Model.Language;
  onUndoDecision: (key: string) => void;
}

export default function WorkList({
  L,
  groupBy,
  changeGroup,
  listGroups,
  groupItems,
  labels,
  listMeta,
  setSelected,
  navigate,
  onEdit,
  onAction,
  visibleHistory,
  exitingDecisions,
  language,
  onUndoDecision,
}: WorkListProps) {
  return (
    <div className="ws-list-view">
      <label className="ws-list-group-control">
        {L("Group", "分组")}
        <select
          value={groupBy}
          onChange={(event) => changeGroup(event.target.value)}
          aria-label={L("Group work list", "任务列表分组")}
        >
          <option value="Status">{L("Status", "状态")}</option>
          <option value="Assignee">{L("Assignee", "负责人")}</option>
          <option value="Record">{L("Record", "记录")}</option>
        </select>
      </label>
      <div className="ws-table-wrap">
        <table className="ws-table ws-work-list-table">
          <thead>
            <tr>
              {[
                L("Delegation", "委派任务"),
                L("Record", "记录"),
                L("Assignee", "负责人"),
                L("Progress", "进度"),
                L("When", "时间"),
                L("Cost", "成本"),
                L("Conf.", "置信度"),
              ].map((heading) => (
                <th key={heading}>{heading}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {listGroups.map((group) => {
              const rows = groupItems(group);
              if (!rows.length) return null;
              return (
                <React.Fragment key={group}>
                  <tr className="ws-group-row">
                    <td colSpan={7}>
                      {groupBy === "Status" ? (
                        <span
                          className={`ws-lane-dot ${group.toLowerCase().replaceAll(" ", "-")}`}
                        />
                      ) : groupBy === "Assignee" ? (
                        <AgentIcon name={group} />
                      ) : (
                        <CompanyIcon company={group} />
                      )}
                      <b>
                        {labels[group] || L(group, taskTypeZh[group] || group)}
                      </b>
                      <span>{rows.length}</span>
                      {groupBy === "Status" && (
                        <small>
                          {group === "Queued"
                            ? L("≈10m · $1.52", "约 10 分钟 · $1.52")
                            : group === "Running"
                              ? L("$0.87 so far", "已花费 $0.87")
                              : group === "Needs your call"
                                ? L("since 6:40 AM", "上午 6:40 起")
                                : "$1.42 · 94%"}
                        </small>
                      )}
                    </td>
                  </tr>
                  {rows.map((item) => {
                    const [when, cost, confidence] = listMeta(item);
                    return (
                      <tr
                        key={item.id}
                        className={item.isExiting ? "is-exiting" : ""}
                        onClick={() => setSelected(item)}
                        tabIndex={0}
                        onKeyDown={(event) => {
                          if (
                            event.key === "Enter" &&
                            event.target === event.currentTarget
                          )
                            setSelected(item);
                        }}
                      >
                        <td>
                          <span className="ws-list-task">
                            <span
                              className={`ws-lane-dot ${item.status.toLowerCase().replaceAll(" ", "-")}`}
                            />
                            <b>{L(item.title, item.titleZh || item.title)}</b>
                            <small>
                              {L(item.type, taskTypeZh[item.type] || item.type)}
                            </small>
                          </span>
                        </td>
                        <td>
                          <button
                            className="ws-list-record"
                            onClick={(event) => {
                              event.stopPropagation();
                              navigate(
                                item.company === "xcc" ? "Work" : item.company,
                              );
                            }}
                          >
                            <CompanyIcon company={item.company} />
                            {item.company}
                          </button>
                        </td>
                        <td>
                          <span className="ws-list-assignee">
                            <AgentIcon name={item.assignee} />
                            {item.assignee}
                          </span>
                        </td>
                        <td>
                          {item.decision ? (
                            <button
                              className="ws-list-action"
                              onClick={(event) => {
                                event.stopPropagation();
                                if (!item.decision) return;
                                if (item.decision.kind === "draft")
                                  onEdit(item.decision);
                                else onAction(item.decision, "approved");
                              }}
                            >
                              {item.decision.kind === "draft"
                                ? L("Review draft", "审阅草稿")
                                : L("Approve", "批准")}
                            </button>
                          ) : item.status === "Running" ? (
                            <span className="ws-list-progress">
                              <span className="ws-list-progress-track">
                                <i style={{ width: `${item.progress}%` }} />
                              </span>
                              <span>{L(item.detail, item.detailZh)}</span>
                            </span>
                          ) : item.status === "Done today" ? (
                            L("Finished with receipts", "已完成，依据可查")
                          ) : (
                            L(item.detail, item.detailZh)
                          )}
                        </td>
                        <td>{L(when, when === "yesterday" ? "昨天" : when)}</td>
                        <td>{cost}</td>
                        <td>{confidence}</td>
                      </tr>
                    );
                  })}
                </React.Fragment>
              );
            })}
            {visibleHistory
              .filter(
                (entry) =>
                  !exitingDecisions.some((row) => row.id === entry.item.id),
              )
              .map((entry) => (
                <tr key={entry.key} className="ws-outcome-table-row">
                  <td colSpan={7}>
                    <DecisionOutcomeRow
                      entry={entry}
                      language={language}
                      navigate={navigate}
                      onUndo={onUndoDecision}
                    />
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
