import { useState, type DragEvent } from "react";
import { Plus } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import {
  STATUSES,
  STATUS_LABELS,
  column,
  isNoopMove,
  planMove,
  type DropTarget,
  type Issue,
  type IssueStatus,
  type MovePlan,
} from "../logic";
import IssueCard from "./IssueCard";
import { StatusIcon } from "./Icons";

/** 看板：六列状态，拖动卡片改状态和顺序。 */
export default function Board({
  issues,
  selectedKey,
  onSelect,
  onOpen,
  onMove,
  onAdd,
}: {
  issues: Issue[];
  selectedKey: string | null;
  onSelect: (key: string) => void;
  onOpen: (key: string) => void;
  onMove: (key: string, plan: MovePlan) => void;
  onAdd: (status: IssueStatus) => void;
}) {
  const t = useT();
  const [dragKey, setDragKey] = useState<string | null>(null);
  const [drop, setDrop] = useState<DropTarget | null>(null);

  const reset = () => {
    setDragKey(null);
    setDrop(null);
  };
  const finish = (e: DragEvent) => {
    e.preventDefault();
    const key = dragKey ?? e.dataTransfer.getData("text/plain");
    if (key && drop) {
      const plan = planMove(issues, key, drop);
      if (!isNoopMove(issues, key, plan)) onMove(key, plan);
    }
    reset();
  };

  return (
    <div className="projects-board" onDragEnd={reset}>
      {STATUSES.map((status) => {
        const cards = column(issues, status);
        const others = cards.filter((i) => i.key !== dragKey);
        const dropIndex = drop?.status === status ? drop.index : -1;
        const line = (i: number) =>
          dropIndex === i ? <div className="projects-drop-line" key={`line-${i}`} /> : null;
        return (
          <section
            key={status}
            className={`projects-lane ${dropIndex >= 0 ? "drag-over" : ""}`}
            onDragOver={(e) => {
              if (!dragKey) return;
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
              // Over empty space: drop at the end of the column.
              if (e.target === e.currentTarget || (e.target as HTMLElement).classList.contains("projects-lane-list"))
                setDrop({ status, index: others.length });
            }}
            onDrop={finish}
          >
            <header className="projects-lane-head">
              <StatusIcon status={status} size={14} />
              <h2>{t(STATUS_LABELS[status])}</h2>
              <em>{cards.length}</em>
              <span className="xc-spacer" />
              <button
                className="icon-button projects-lane-add"
                aria-label={`${t("New issue")} · ${t(STATUS_LABELS[status])}`}
                onClick={() => onAdd(status)}
              >
                <Plus size={14} />
              </button>
            </header>
            <div className="projects-lane-list">
              {cards.map((issue) => {
                const index = others.findIndex((o) => o.key === issue.key);
                return [
                  index >= 0 ? line(index) : null,
                  <IssueCard
                    key={issue.key}
                    issue={issue}
                    selected={issue.key === selectedKey}
                    dragging={issue.key === dragKey}
                    onClick={() => {
                      onSelect(issue.key);
                      onOpen(issue.key);
                    }}
                    onDragStart={(e) => {
                      e.dataTransfer.setData("text/plain", issue.key);
                      e.dataTransfer.effectAllowed = "move";
                      setDragKey(issue.key);
                      onSelect(issue.key);
                    }}
                    onDragOver={(e) => {
                      if (!dragKey) return;
                      e.preventDefault();
                      e.stopPropagation();
                      if (issue.key === dragKey) return;
                      const rect = e.currentTarget.getBoundingClientRect();
                      const before = e.clientY < rect.top + rect.height / 2;
                      setDrop({ status, index: index + (before ? 0 : 1) });
                    }}
                  />,
                ];
              })}
              {line(others.length)}
              {cards.length === 0 && dropIndex < 0 && (
                <div className="projects-empty-lane">{t("No issues")}</div>
              )}
            </div>
          </section>
        );
      })}
    </div>
  );
}
