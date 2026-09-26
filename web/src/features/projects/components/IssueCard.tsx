import type { DragEvent } from "react";
import { CalendarDays } from "lucide-react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatDate } from "../../../lib/time";
import { dueState, localDate, type Issue } from "../logic";
import { LabelChip, PriorityIcon } from "./Icons";

export function DueBadge({ issue }: { issue: Issue }) {
  const t = useT();
  const language = useLanguage();
  const state = dueState(issue.dueDate, localDate(), issue.status);
  if (!issue.dueDate || !state) return null;
  const label =
    state === "today"
      ? t("Today")
      : formatDate(`${issue.dueDate}T00:00:00`, language);
  return (
    <span className={`projects-due ${state}`} title={t("Due date")}>
      <CalendarDays size={12} />
      {label}
    </span>
  );
}

/** 看板上的一张卡片。 */
export default function IssueCard({
  issue,
  selected,
  dragging,
  onClick,
  onDragStart,
  onDragOver,
}: {
  issue: Issue;
  selected: boolean;
  dragging: boolean;
  onClick: () => void;
  onDragStart: (e: DragEvent<HTMLElement>) => void;
  onDragOver: (e: DragEvent<HTMLElement>) => void;
}) {
  return (
    <article
      className={`projects-card${selected ? " selected" : ""}${dragging ? " dragging" : ""}`}
      draggable
      tabIndex={0}
      data-issue-key={issue.key}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" && e.target === e.currentTarget) onClick();
      }}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
    >
      <div className="projects-card-meta">
        <PriorityIcon priority={issue.priority} size={13} />
        <span className="xc-mono">{issue.key}</span>
        <span className="xc-spacer" />
        <DueBadge issue={issue} />
      </div>
      <h3>{issue.title}</h3>
      {issue.labels.length > 0 && (
        <div className="projects-card-labels">
          {issue.labels.map((l) => (
            <LabelChip key={l.id} label={l} />
          ))}
        </div>
      )}
    </article>
  );
}
