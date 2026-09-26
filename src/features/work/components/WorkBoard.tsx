import type * as Model from "../../../types/domain";
import React from "react";
import WorkCard from "./WorkCard";
import DecisionOutcomeRow from "../../../components/decisions/DecisionOutcomeRow";

interface WorkBoardProps {
  lanes: string[];
  labels: Model.StringMap;
  visible: Model.WorkItem[];
  L: Model.Localize;
  navigate: Model.Navigate;
  setSelected: Model.Setter<Model.WorkItem | null>;
  onAction: Model.DecisionHandler;
  onEdit: (item: Model.Decision) => void;
  visibleHistory: Model.DecisionEntry[];
  exitingDecisions: Model.Decision[];
  language: Model.Language;
  onUndoDecision: (key: string) => void;
}

export default function WorkBoard({
  lanes,
  labels,
  visible,
  L,
  navigate,
  setSelected,
  onAction,
  onEdit,
  visibleHistory,
  exitingDecisions,
  language,
  onUndoDecision,
}: WorkBoardProps) {
  return (
    <div className="ws-board">
      {lanes.map((lane) => (
        <section className="ws-lane" key={lane}>
          <div className="ws-lane-heading">
            <h2>
              <span
                className={`ws-lane-dot ${lane.toLowerCase().replaceAll(" ", "-")}`}
              />
              {labels[lane]}{" "}
              <em>{visible.filter((item) => item.status === lane).length}</em>
            </h2>
            <small>
              {lane === "Queued"
                ? L("≈ 10m · $1.52", "约 10 分钟 · $1.52")
                : lane === "Running"
                  ? L("$0.87 so far", "已花费 $0.87")
                  : lane === "Needs your call"
                    ? L("since 6:40 AM", "上午 6:40 起")
                    : L("$1.42 · 94%", "$1.42 · 94%")}
            </small>
          </div>
          <div className="ws-lane-list">
            {visible
              .filter((item) => item.status === lane)
              .map((item) => (
                <WorkCard
                  key={item.id}
                  item={item}
                  L={L}
                  navigate={navigate}
                  onSelect={setSelected}
                  onAction={onAction}
                  onEdit={onEdit}
                />
              ))}
            {lane === "Needs your call" &&
              visibleHistory
                .filter(
                  (entry) =>
                    !exitingDecisions.some((row) => row.id === entry.item.id),
                )
                .map((entry) => (
                  <DecisionOutcomeRow
                    key={entry.key}
                    entry={entry}
                    language={language}
                    navigate={navigate}
                    onUndo={onUndoDecision}
                  />
                ))}
            {!visible.some((item) => item.status === lane) &&
              !(lane === "Needs your call" && visibleHistory.length) && (
                <div className="ws-empty-lane">
                  {L("No matching work", "没有匹配的任务")}
                </div>
              )}
          </div>
        </section>
      ))}
    </div>
  );
}
