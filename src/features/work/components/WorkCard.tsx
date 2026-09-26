import type * as Model from "../../../types/domain";
import React from "react";
import {
  MoreHorizontal,
  FileText,
  ArrowRight,
  Check,
  Clock3,
} from "lucide-react";
import AgentIcon from "../../../components/ui/AgentIcon";
import { taskTypeZh } from "../work-config";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import Progress from "../../../components/ui/Progress";

interface WorkCardProps {
  item: Model.WorkItem;
  L: Model.Localize;
  navigate: Model.Navigate;
  onSelect: Model.Setter<Model.WorkItem | null>;
  onAction: Model.DecisionHandler;
  onEdit: (item: Model.Decision) => void;
}

export default function WorkCard({
  item,
  L,
  navigate,
  onSelect,
  onAction,
  onEdit,
}: WorkCardProps) {
  const decision = item.decision;
  return (
    <article
      className={`ws-work-card ${decision ? "review" : ""} ${item.isExiting ? "is-exiting" : ""}`}
    >
      <button
        className="ws-card-open"
        aria-label={L(
          `Details: ${item.title}`,
          `查看详情：${item.titleZh || item.title}`,
        )}
        onClick={() => onSelect(item)}
      >
        <MoreHorizontal size={15} />
      </button>
      <div className="ws-card-meta">
        <AgentIcon name={item.assignee} />
        <b>{item.assignee}</b>
        <span>· {L(item.type, taskTypeZh[item.type] || item.type)}</span>
        {decision && <em>{decision.confidence}</em>}
      </div>
      <h3>{L(item.title, item.titleZh || item.title)}</h3>
      <button
        className="ws-record-link"
        onClick={() => navigate(item.company === "xcc" ? "Work" : item.company)}
      >
        <CompanyIcon company={item.company} />
        {item.company}
      </button>
      {decision?.kind === "draft" && (
        <button className="ws-draft-strip" onClick={() => onEdit(decision)}>
          <FileText size={13} />
          <span>
            {L(
              "Two pricing options before our 10:00",
              "10 点会议前的两种报价方案",
            )}
          </span>
        </button>
      )}
      {decision?.kind === "health" && (
        <div className="ws-inline-health">
          <span>
            {L("Health", "健康度")} <b>81 → 63</b>
          </span>
          <Progress value={63} color="gold" />
          <small>{L("Seats in use 118 → 92", "活跃席位 118 → 92")}</small>
        </div>
      )}
      {decision?.kind === "stage" && (
        <div className="ws-stage-change">
          {L("Proposal", "方案")} <ArrowRight size={13} />{" "}
          <b>{L("Negotiation", "谈判")}</b>
        </div>
      )}
      {decision ? (
        <div className="ws-review-actions">
          <button
            className="ws-approve"
            onClick={() => onAction(decision, "approved")}
          >
            <Check size={13} />
            {L(
              decision.kind === "draft" ? "Send" : "Approve",
              decision.kind === "draft" ? "发送" : "批准",
            )}
          </button>
          {decision.kind === "draft" && (
            <button onClick={() => onEdit(decision)}>
              {L("Review draft", "审阅草稿")}
            </button>
          )}
          <button
            onClick={() =>
              onAction(
                decision,
                decision.kind === "stage" ? "kept" : "dismissed",
              )
            }
          >
            {L(
              decision.kind === "stage" ? "Keep stage" : "Dismiss",
              decision.kind === "stage" ? "保持阶段" : "忽略",
            )}
          </button>
        </div>
      ) : item.status === "Running" ? (
        <div className="ws-card-foot">
          <Progress value={item.progress ?? 0} />
          <small>
            {item.progress}% · {L(item.detail, item.detailZh)}
          </small>
        </div>
      ) : (
        <div className="ws-card-foot">
          <Clock3 size={12} />
          <small>{L(item.detail, item.detailZh)}</small>
        </div>
      )}
    </article>
  );
}
