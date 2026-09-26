import type * as Model from "../../../types/domain";
import React from "react";
import { X, ArrowRight } from "lucide-react";
import AgentIcon from "../../../components/ui/AgentIcon";
import { taskTypeZh } from "../work-config";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import Progress from "../../../components/ui/Progress";

interface WorkDetailDrawerProps {
  setSelected: Model.Setter<Model.WorkItem | null>;
  L: Model.Localize;
  selectedItem: Model.WorkItem;
  navigate: Model.Navigate;
  labels: Model.StringMap;
  onAction: Model.DecisionHandler;
  setStatuses: Model.Setter<Model.WorkStatuses>;
}

export default function WorkDetailDrawer({
  setSelected,
  L,
  selectedItem,
  navigate,
  labels,
  onAction,
  setStatuses,
}: WorkDetailDrawerProps) {
  return (
    <div className="ws-drawer-backdrop" onMouseDown={() => setSelected(null)}>
      <aside
        className="ws-drawer"
        role="dialog"
        aria-modal="true"
        aria-label={L("Task details", "任务详情")}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="ws-drawer-top">
          <span>{L("Work details", "任务详情")}</span>
          <button onClick={() => setSelected(null)} title={L("Close", "关闭")}>
            <X size={17} />
          </button>
        </div>
        <div className="ws-drawer-body">
          <div className="ws-card-meta">
            <AgentIcon name={selectedItem.assignee} />
            {selectedItem.assignee} ·{" "}
            {L(
              selectedItem.type,
              taskTypeZh[selectedItem.type] || selectedItem.type,
            )}
          </div>
          <h2>
            {L(selectedItem.title, selectedItem.titleZh || selectedItem.title)}
          </h2>
          <button
            className="ws-record-link"
            onClick={() => {
              setSelected(null);
              navigate(
                selectedItem.company === "xcc" ? "Work" : selectedItem.company,
              );
            }}
          >
            <CompanyIcon company={selectedItem.company} />
            {selectedItem.company}
            <ArrowRight size={13} />
          </button>
          <div className="ws-detail-line">
            <span>{L("Status", "状态")}</span>
            <b>{labels[selectedItem.status]}</b>
          </div>
          <div className="ws-detail-line">
            <span>{L("Next step", "下一步")}</span>
            <b>{L(selectedItem.detail, selectedItem.detailZh)}</b>
          </div>
          {selectedItem.status === "Running" && (
            <Progress value={selectedItem.progress ?? 0} />
          )}
          {selectedItem.decision ? (
            <div className="ws-drawer-actions">
              <button
                className="ws-approve"
                onClick={() => {
                  if (selectedItem.decision)
                    onAction(selectedItem.decision, "approved");
                  setSelected(null);
                }}
              >
                {L("Approve", "批准")}
              </button>
              <button
                onClick={() => {
                  if (selectedItem.decision)
                    onAction(selectedItem.decision, "dismissed");
                  setSelected(null);
                }}
              >
                {L("Dismiss", "忽略")}
              </button>
            </div>
          ) : (
            selectedItem.status !== "Done today" && (
              <div className="ws-drawer-actions">
                <button
                  className="ws-approve"
                  onClick={() => {
                    setStatuses((prev) => ({
                      ...prev,
                      [selectedItem.id]:
                        selectedItem.status === "Queued"
                          ? "Running"
                          : "Done today",
                    }));
                    setSelected(null);
                  }}
                >
                  {selectedItem.status === "Queued"
                    ? L("Start work", "开始任务")
                    : L("Mark done", "标记完成")}
                </button>
              </div>
            )
          )}
        </div>
      </aside>
    </div>
  );
}
