import { useRef, type DragEvent, type MouseEvent } from "react";
import {
  CalendarDays,
  Check,
  Ellipsis,
  FolderGit2,
  ListChecks,
  MessageSquare,
  Pencil,
  RotateCcw,
} from "lucide-react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatDate, formatTime } from "../../../lib/time";
import { thumbnailSrc } from "../../../components/markdown/upload";
import {
  coverImage,
  issueDue,
  issueDueState,
  localTime,
  overdueBy,
  PRIORITY_LABELS,
  type Issue,
} from "../logic";
import { LabelChip, PriorityIcon, StatusIcon } from "./Icons";
import AgentMember from "../../aiagents/AgentMember";

/** 截止时间：今天的写“今天 18:00”，过期的写“已过期 2 小时”。 */
export function DueBadge({
  issue,
  onClick,
}: {
  issue: Issue;
  onClick?: (e: MouseEvent<HTMLButtonElement>) => void;
}) {
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
  const title = `${t("Due at")} ${formatDate(at, language)} ${formatTime(at, language)}`;
  const body = (
    <>
      <CalendarDays size={12} />
      {label}
    </>
  );
  if (!onClick)
    return (
      <span className={`projects-due ${state}`} title={title}>
        {body}
      </span>
    );
  return (
    <button
      type="button"
      draggable
      className={`projects-due projects-card-chip ${state}`}
      title={title}
      onClick={(e) => {
        e.stopPropagation();
        onClick(e);
      }}
      onDragStart={cancelDrag}
    >
      {body}
    </button>
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
export function Members({
  issue,
  onClick,
}: {
  issue: Issue;
  onClick?: (e: MouseEvent<HTMLButtonElement>) => void;
}) {
  const t = useT();
  const members = issue.members ?? [];
  if (!members.length) return null;
  const avatars = members.map((m) =>
    m.kind === "me" ? (
      <i key="me" className="projects-member me" title={t("Me")}>
        {t("Me")}
      </i>
    ) : (
      <span key={`agent:${m.id}`} className="projects-member agent">
        <AgentMember id={m.id} issueKey={issue.key} size={16} />
      </span>
    ),
  );
  if (!onClick) return <span className="projects-members">{avatars}</span>;
  return (
    <button
      type="button"
      draggable
      className="projects-members projects-card-chip"
      aria-label={t("Assign")}
      onClick={(e) => {
        e.stopPropagation();
        onClick(e);
      }}
      onDragStart={cancelDrag}
    >
      {avatars}
    </button>
  );
}

function cancelDrag(e: DragEvent<HTMLElement>) {
  e.preventDefault();
  e.stopPropagation();
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
  onMenu,
  onToggleDone,
  onField,
}: {
  issue: Issue;
  /** “一级 / 二级” */
  category?: string;
  selected: boolean;
  dragging: boolean;
  onClick: () => void;
  onDragStart: (e: DragEvent<HTMLElement>) => void;
  onDragOver: (e: DragEvent<HTMLElement>) => void;
  /** B85：右键或长按时弹出卡片菜单，参数是位置 */
  onMenu?: (at: { x: number; y: number }) => void;
  onToggleDone?: () => void;
  onField?: (
    field: "priority" | "due" | "assign",
    at: { x: number; y: number },
  ) => void;
}) {
  const t = useT();
  // B55：描述里的第一张图做封面
  const cover = coverImage(issue.description);
  // B85：手机上长按 500 毫秒弹出菜单，弹出后这次点击不再打开卡片
  const press = useRef<{ timer: number; fired: boolean } | null>(null);
  const cancelPress = () => {
    if (press.current) window.clearTimeout(press.current.timer);
  };
  const synced =
    issue.externalSource === "github" || issue.externalSource === "forgejo";
  return (
    <article
      className={`projects-card${selected ? " selected" : ""}${dragging ? " dragging" : ""}${issue.status === "done" ? " done" : ""}${issue.color ? ` has-color card-color-${issue.color}` : ""}`}
      draggable
      tabIndex={0}
      data-issue-key={issue.key}
      onClick={() => {
        if (press.current?.fired) {
          press.current = null;
          return;
        }
        onClick();
      }}
      onContextMenu={
        onMenu
          ? (e) => {
              e.preventDefault();
              onMenu({ x: e.clientX, y: e.clientY });
            }
          : undefined
      }
      onTouchStart={
        onMenu
          ? (e) => {
              const touch = e.touches[0];
              const x = touch.clientX;
              const y = touch.clientY;
              cancelPress();
              press.current = {
                fired: false,
                timer: window.setTimeout(() => {
                  if (press.current) press.current.fired = true;
                  onMenu({ x, y });
                }, 500),
              };
            }
          : undefined
      }
      onTouchMove={cancelPress}
      onTouchEnd={cancelPress}
      onKeyDown={(e) => {
        if (e.key === "Enter" && e.target === e.currentTarget) onClick();
      }}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
    >
      {onToggleDone && (
        <div className="projects-card-hover">
          <button
            type="button"
            draggable
            aria-label={t(issue.status === "done" ? "Reopen" : "Mark complete")}
            title={t(issue.status === "done" ? "Reopen" : "Mark complete")}
            onClick={(e) => {
              e.stopPropagation();
              onToggleDone();
            }}
            onDragStart={cancelDrag}
          >
            {issue.status === "done" ? (
              <RotateCcw size={13} />
            ) : (
              <Check size={13} />
            )}
          </button>
          <button
            type="button"
            draggable
            aria-label={t("Quick edit")}
            title={t("Quick edit")}
            onClick={(e) => {
              e.stopPropagation();
              onClick();
            }}
            onDragStart={cancelDrag}
          >
            <Pencil size={12} />
          </button>
          {onMenu && (
            <button
              type="button"
              draggable
              aria-label={t("More")}
              title={t("More")}
              onClick={(e) => {
                e.stopPropagation();
                onMenu({ x: e.clientX, y: e.clientY });
              }}
              onDragStart={cancelDrag}
            >
              <Ellipsis size={13} />
            </button>
          )}
        </div>
      )}
      {cover && (
        <img
          className="projects-card-cover"
          src={thumbnailSrc(cover)}
          alt=""
          loading="lazy"
          draggable={false}
        />
      )}
      {issue.labels.length > 0 && (
        <div className="projects-card-labels projects-card-tags">
          {issue.labels.map((l) => (
            <LabelChip key={l.id} label={l} />
          ))}
        </div>
      )}
      <h3 className="projects-card-title" title={issue.key}>
        {issue.status === "done" && <StatusIcon status="done" size={14} />}
        <span>{issue.title}</span>
      </h3>
      {category && <p className="projects-card-category">{category}</p>}
      {(issue.checklistTotal ?? 0) > 0 && (
        <div className="projects-card-sub">
          <ChecklistBadge issue={issue} />
          <span className="projects-card-bar">
            <i
              className={
                (issue.checklistDone ?? 0) >= (issue.checklistTotal ?? 0)
                  ? "full"
                  : undefined
              }
              style={{
                width: `${Math.round(((issue.checklistDone ?? 0) / (issue.checklistTotal ?? 1)) * 100)}%`,
              }}
            />
          </span>
        </div>
      )}
      <div className="projects-card-meta">
        {onField ? (
          <button
            type="button"
            draggable
            className="projects-card-chip"
            aria-label={t(PRIORITY_LABELS[issue.priority])}
            onClick={(e) => {
              e.stopPropagation();
              onField("priority", { x: e.clientX, y: e.clientY });
            }}
            onDragStart={cancelDrag}
          >
            <PriorityIcon priority={issue.priority} size={13} />
          </button>
        ) : (
          <PriorityIcon priority={issue.priority} size={13} />
        )}
        {synced && (
          <span
            className="projects-card-count"
            title={t("Synced from the repository")}
          >
            <FolderGit2 size={12} />
          </span>
        )}
        <DueBadge
          issue={issue}
          onClick={
            onField
              ? (e) => onField("due", { x: e.clientX, y: e.clientY })
              : undefined
          }
        />
        {!!issue.commentCount && (
          <span className="projects-card-count">
            <MessageSquare size={12} />
            {issue.commentCount}
          </span>
        )}
        <Members
          issue={issue}
          onClick={
            onField
              ? (e) => onField("assign", { x: e.clientX, y: e.clientY })
              : undefined
          }
        />
      </div>
    </article>
  );
}
