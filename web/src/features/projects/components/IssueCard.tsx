import type { DragEvent } from "react";
import { CalendarDays, ListChecks, MessageSquare } from "lucide-react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatDate, formatTime } from "../../../lib/time";
import { thumbnailSrc } from "../../../components/markdown/upload";
import {
  coverImage,
  issueDue,
  issueDueState,
  localTime,
  overdueBy,
  type Issue,
} from "../logic";
import { LabelChip, PriorityIcon } from "./Icons";
import AgentMember from "../../aiagents/AgentMember";

/** 截止时间：今天的写“今天 18:00”，过期的写“已过期 2 小时”。 */
export function DueBadge({ issue }: { issue: Issue }) {
  const t = useT();
  const language = useLanguage();
  const now = new Date();
  const at = issueDue(issue);
  const state = issueDueState(issue, now);
  if (!at || !state) return null;
  let label: string;
  if (state === "overdue") {
    const { value, unit } = overdueBy(at, now);
    label = `${t("Overdue for")} ${value} ${t(unit)}`;
  } else if (state === "today") {
    label = `${t("Today")} ${localTime(at)}`;
  } else {
    // 只有日期的旧数据是 23:59，这时不写时间。
    label =
      localTime(at) === "23:59"
        ? formatDate(at, language)
        : `${formatDate(at, language)} ${formatTime(at, language)}`;
  }
  return (
    <span
      className={`projects-due ${state}`}
      title={`${t("Due at")} ${formatDate(at, language)} ${formatTime(at, language)}`}
    >
      <CalendarDays size={12} />
      {label}
    </span>
  );
}

/** 检查清单进度 ☑ 3/5，全部完成时变绿。 */
export function ChecklistBadge({ issue }: { issue: Issue }) {
  const t = useT();
  const total = issue.checklistTotal ?? 0;
  if (!total) return null;
  const done = issue.checklistDone ?? 0;
  return (
    <span
      className={`projects-checklist-badge${done >= total ? " complete" : ""}`}
      title={t("Checklists")}
    >
      <ListChecks size={12} />
      {done}/{total}
    </span>
  );
}

/** 卡片成员的头像：“我”显示“我”，Agent 显示它的头像（B47）。 */
export function Members({ issue }: { issue: Issue }) {
  const t = useT();
  const members = issue.members ?? [];
  if (!members.length) return null;
  return (
    <span className="projects-members">
      {members.map((m) =>
        m.kind === "me" ? (
          <i key="me" className="projects-member me" title={t("Me")}>
            {t("Me")}
          </i>
        ) : (
          <span key={`agent:${m.id}`} className="projects-member agent">
            <AgentMember id={m.id} issueKey={issue.key} size={16} />
          </span>
        ),
      )}
    </span>
  );
}

/** 看板上的一张卡片。 */
export default function IssueCard({
  issue,
  category,
  selected,
  dragging,
  onClick,
  onDragStart,
  onDragOver,
}: {
  issue: Issue;
  /** “一级 / 二级” */
  category?: string;
  selected: boolean;
  dragging: boolean;
  onClick: () => void;
  onDragStart: (e: DragEvent<HTMLElement>) => void;
  onDragOver: (e: DragEvent<HTMLElement>) => void;
}) {
  // B55：描述里的第一张图做封面
  const cover = coverImage(issue.description);
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
      {cover && (
        <img
          className="projects-card-cover"
          src={thumbnailSrc(cover)}
          alt=""
          loading="lazy"
          draggable={false}
        />
      )}
      <div className="projects-card-meta">
        <PriorityIcon priority={issue.priority} size={13} />
        <span className="xc-mono">{issue.key}</span>
      </div>
      <h3>{issue.title}</h3>
      {category && <p className="projects-card-category">{category}</p>}
      {(issue.labels.length > 0 ||
        !!issue.checklistTotal ||
        !!issue.commentCount ||
        !!issue.members?.length ||
        !!issueDue(issue)) && (
        <div className="projects-card-labels">
          <DueBadge issue={issue} />
          <ChecklistBadge issue={issue} />
          {!!issue.commentCount && (
            <span className="projects-card-count">
              <MessageSquare size={12} />
              {issue.commentCount}
            </span>
          )}
          {issue.labels.map((l) => (
            <LabelChip key={l.id} label={l} />
          ))}
          <Members issue={issue} />
        </div>
      )}
    </article>
  );
}
