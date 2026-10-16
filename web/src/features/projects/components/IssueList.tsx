import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import { PRIORITY_LABELS, type IssueGroup } from "../logic";
import { ChecklistBadge, DueBadge, Members } from "./IssueCard";
import { LabelChip, PriorityIcon, StatusIcon } from "./Icons";

/**
 * 列表视图：按组显示，每行一个 Issue（B239 改成表格）。
 * 顶上一行表头，每一列上下对齐；组名是一条浅色色带，颜色跟着状态走；行之间不画线。
 */
export default function IssueList({
  groups,
  categoryNames,
  selectedKey,
  onSelect,
  onOpen,
  showProject = false,
}: {
  groups: IssueGroup[];
  /** 分类 id → “一级 / 二级”，分组不是按分类时显示在行里 */
  categoryNames?: Map<number, string>;
  selectedKey: string | null;
  onSelect: (key: string) => void;
  onOpen: (key: string) => void;
  showProject?: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const visible = groups.filter((g) => g.issues.length > 0);
  if (visible.length === 0)
    return <div className="projects-empty-lane">{t("No issues")}</div>;
  // 按分类分组时组名就是分类，不再单独占一列
  const showCategory =
    !groups.some((g) => g.raw) &&
    visible.some((g) =>
      g.issues.some((i) => i.categoryId && categoryNames?.has(i.categoryId)),
    );
  const showTime = !showProject;
  const cls = `projects-list${showCategory ? " with-category" : ""}${showTime ? " with-time" : ""}`;
  return (
    <div className={cls} role="list">
      <div className="projects-list-head" aria-hidden>
        <span className="name">{t("Name")}</span>
        {showCategory && <span>{t("Category")}</span>}
        <span>{t("Priority")}</span>
        <span className="labels">{t("Labels")}</span>
        <span>{t("Due date")}</span>
        <span className="members">{t("Members")}</span>
        {showTime && <span className="end">{t("Updated")}</span>}
      </div>
      {visible.map((group) => (
        <section key={group.id} className="projects-list-group">
          {groups.length > 1 && (
            <header data-status={group.status} data-priority={group.priority}>
              {group.status && <StatusIcon status={group.status} size={14} />}
              {group.priority !== undefined && (
                <PriorityIcon priority={group.priority} size={14} />
              )}
              <b>{group.raw ? group.label : t(group.label)}</b>
              <span>{group.issues.length}</span>
            </header>
          )}
          {group.issues.map((issue) => (
            <div
              key={issue.key}
              role="listitem"
              tabIndex={0}
              data-issue-key={issue.key}
              className={`projects-row${issue.key === selectedKey ? " selected" : ""}`}
              onClick={() => {
                onSelect(issue.key);
                onOpen(issue.key);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && e.target === e.currentTarget)
                  onOpen(issue.key);
              }}
            >
              <span className="projects-row-key xc-mono">{issue.key}</span>
              <span className="projects-row-name">
                <StatusIcon status={issue.status} size={14} />
                <span className="projects-row-title">{issue.title}</span>
                <ChecklistBadge issue={issue} />
              </span>
              {showCategory && (
                <span className="projects-row-category">
                  {(issue.categoryId && categoryNames?.get(issue.categoryId)) ||
                    ""}
                </span>
              )}
              <span className="projects-row-priority">
                <PriorityIcon priority={issue.priority} size={14} />
                <span className="projects-row-priority-label">
                  {issue.priority ? t(PRIORITY_LABELS[issue.priority]) : ""}
                </span>
              </span>
              <span className="projects-row-labels">
                {issue.labels.map((l) => (
                  <LabelChip key={l.id} label={l} />
                ))}
              </span>
              <span className="projects-row-due">
                <DueBadge issue={issue} />
              </span>
              <span className="projects-row-members">
                <Members issue={issue} />
              </span>
              {showTime && (
                <time className="projects-row-time" dateTime={issue.updatedAt}>
                  {relativeTime(issue.updatedAt, language)}
                </time>
              )}
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}
