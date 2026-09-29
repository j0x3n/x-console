import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import type { IssueGroup } from "../logic";
import { ChecklistBadge, DueBadge } from "./IssueCard";
import { LabelChip, PriorityIcon, StatusIcon } from "./Icons";

/** 列表视图：按组显示，每行一个 Issue。 */
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
  return (
    <div className="projects-list" role="list">
      {visible.map((group) => (
        <section key={group.id} className="projects-list-group">
          {groups.length > 1 && (
            <header>
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
              <PriorityIcon priority={issue.priority} size={14} />
              <span className="projects-row-key xc-mono">{issue.key}</span>
              <StatusIcon status={issue.status} size={14} />
              <span className="projects-row-title">{issue.title}</span>
              {issue.categoryId &&
                categoryNames?.get(issue.categoryId) &&
                !group.raw && (
                  <span className="projects-row-category">
                    {categoryNames.get(issue.categoryId)}
                  </span>
                )}
              <ChecklistBadge issue={issue} />
              <span className="projects-row-labels">
                {issue.labels.map((l) => (
                  <LabelChip key={l.id} label={l} />
                ))}
              </span>
              <DueBadge issue={issue} />
              {showProject || (
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
