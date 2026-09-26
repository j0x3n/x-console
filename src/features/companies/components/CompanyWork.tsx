import type * as Model from "../../../types/domain";
import React from "react";
import AccountCard from "./AccountCard";
import { ArrowRight, Check } from "lucide-react";
import AccountIcon from "./AccountIcon";
import { statusZh } from "../company-config";

interface CompanyWorkProps {
  L: Model.Localize;
  openWork: Model.WorkItem[];
  pending: Model.Decision | undefined;
  navigate: Model.Navigate;
  workStatuses: Model.WorkStatuses;
  doneWork: Model.WorkItem[];
}

export default function CompanyWork({
  L,
  openWork,
  pending,
  navigate,
  workStatuses,
  doneWork,
}: CompanyWorkProps) {
  return (
    <AccountCard
      title={L("Open work", "进行中的任务")}
      count={openWork.length + (pending ? 1 : 0)}
      action={
        <button
          onClick={() => navigate("Work")}
          aria-label={L("Open Work", "打开任务")}
        >
          <ArrowRight size={14} />
        </button>
      }
    >
      <div className="ws-account-list">
        {pending && (
          <button
            className="ws-account-work-row"
            onClick={() => navigate("Today")}
          >
            <AccountIcon name={pending.agent} />
            <span>
              <b>
                {L(
                  pending.kind === "draft"
                    ? "Follow-up to Priya Raman"
                    : pending.kind === "health"
                      ? "Oakline renewal check-in"
                      : "Move to Negotiation",
                  pending.kind === "draft"
                    ? "跟进 Priya Raman"
                    : pending.kind === "health"
                      ? "Oakline 续约沟通"
                      : "进入谈判阶段",
                )}
              </b>
              <small>
                <i className="ws-work-status-dot waiting" />
                {L("Needs your call", "待你决定")} · {pending.agent} ·{" "}
                {pending.confidence}
              </small>
            </span>
          </button>
        )}
        {openWork.map((item) => (
          <button
            className="ws-account-work-row"
            key={item.id}
            onClick={() => navigate("Work")}
          >
            <AccountIcon name={item.assignee} />
            <span>
              <b>{L(item.title, item.titleZh)}</b>
              <small>
                <i className="ws-work-status-dot" />
                {L(
                  workStatuses[item.id] || item.status,
                  statusZh[workStatuses[item.id] || item.status],
                )}{" "}
                · {item.assignee}
                {item.cost && ` · $${item.cost}`}
                {item.confidence && ` · ${item.confidence}`}
              </small>
            </span>
            {(workStatuses[item.id] || item.status) === "Running" && (
              <span className="ws-account-work-progress">
                <i />
              </span>
            )}
          </button>
        ))}
        {openWork.length === 0 && !pending && (
          <p className="ws-account-empty">
            {L("No open work", "暂无进行中的任务")}
          </p>
        )}
        {doneWork.length > 0 && (
          <>
            <div className="ws-account-subheading">
              {L("Done today", "今日完成")}
            </div>
            {doneWork.map((item) => (
              <button
                className="ws-account-work-row"
                key={item.id}
                onClick={() => navigate("Work")}
              >
                <AccountIcon name={item.assignee} />
                <span>
                  <b>{L(item.title, item.titleZh)}</b>
                  <small>
                    <Check size={11} />
                    {L("Done today", "今日完成")} · {item.assignee}
                  </small>
                </span>
              </button>
            ))}
          </>
        )}
      </div>
    </AccountCard>
  );
}
