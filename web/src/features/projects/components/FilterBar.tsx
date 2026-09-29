import type { ReactNode } from "react";
import { X } from "lucide-react";
import { SearchBox } from "../../../components/ui/Toolbar";
import { useT } from "../../../contexts/LanguageContext";
import type { Label, Milestone } from "../api";
import {
  OPEN_STATUSES,
  PRIORITIES,
  PRIORITY_LABELS,
  STATUSES,
  STATUS_LABELS,
  emptyFilter,
  isFilterActive,
  type Category,
  type DueFilter,
  type IssueFilter,
  type IssueStatus,
} from "../logic";
import CategorySelect from "./CategorySelect";

const DUE_LABELS: Record<DueFilter, string> = {
  today: "Due today",
  week: "Next 7 days",
  overdue: "Overdue",
};

/** 状态下拉的值：全部、未完成，或者某一个状态。 */
function statusValue(statuses: IssueStatus[]): string {
  if (statuses.length === 1) return statuses[0];
  if (
    statuses.length === OPEN_STATUSES.length &&
    OPEN_STATUSES.every((s) => statuses.includes(s))
  )
    return "open";
  return "";
}

/**
 * 筛选：搜索、状态（只在列表里）、优先级、分类、标签、里程碑、截止时间。
 * 返回搜索框和一行下拉，放在 Toolbar 的 start 里。
 */
export default function FilterBar({
  filter,
  onChange,
  labels,
  milestones,
  categories = [],
  showStatus = true,
  extra,
}: {
  filter: IssueFilter;
  onChange: (filter: IssueFilter) => void;
  labels: Label[];
  milestones: Milestone[];
  categories?: Category[];
  showStatus?: boolean;
  /** 放在筛选后面的其他下拉，比如分组和排序 */
  extra?: ReactNode;
}) {
  const t = useT();
  return (
    <>
      <SearchBox
        className="projects-search"
        value={filter.q}
        onChange={(q) => onChange({ ...filter, q })}
        placeholder={t("Search issues")}
        clearLabel={t("Clear")}
      />
      {/* 窄屏时这一行可以左右滑动 */}
      <div className="projects-filters">
        {showStatus && (
          <select
            className="xc-select projects-filter-select"
            value={statusValue(filter.statuses)}
            aria-label={t("Status")}
            onChange={(e) => {
              const v = e.target.value;
              onChange({
                ...filter,
                statuses:
                  v === ""
                    ? []
                    : v === "open"
                      ? [...OPEN_STATUSES]
                      : [v as IssueStatus],
              });
            }}
          >
            <option value="">{t("Any status")}</option>
            <option value="open">{t("Open issues")}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(STATUS_LABELS[s])}
              </option>
            ))}
          </select>
        )}
        <select
          className="xc-select projects-filter-select"
          value={filter.priority ?? ""}
          aria-label={t("Priority")}
          onChange={(e) =>
            onChange({
              ...filter,
              priority: e.target.value === "" ? null : Number(e.target.value),
            })
          }
        >
          <option value="">{t("Any priority")}</option>
          {PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {t(PRIORITY_LABELS[p])}
            </option>
          ))}
        </select>
        {categories.length > 0 && (
          <CategorySelect
            mode="filter"
            className="xc-select projects-filter-select"
            categories={categories}
            value={filter.categoryId}
            onChange={(categoryId) => onChange({ ...filter, categoryId })}
          />
        )}
        {labels.length > 0 && (
          <select
            className="xc-select projects-filter-select"
            value={filter.labelId ?? ""}
            aria-label={t("Label")}
            onChange={(e) =>
              onChange({
                ...filter,
                labelId: e.target.value ? Number(e.target.value) : null,
              })
            }
          >
            <option value="">{t("Any label")}</option>
            {labels.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        )}
        {milestones.length > 0 && (
          <select
            className="xc-select projects-filter-select"
            value={filter.milestoneId ?? ""}
            aria-label={t("Milestone")}
            onChange={(e) =>
              onChange({
                ...filter,
                milestoneId: e.target.value ? Number(e.target.value) : null,
              })
            }
          >
            <option value="">{t("Any milestone")}</option>
            {milestones.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </select>
        )}
        <select
          className="xc-select projects-filter-select"
          value={filter.due ?? ""}
          aria-label={t("Due date")}
          onChange={(e) =>
            onChange({
              ...filter,
              due: (e.target.value || null) as DueFilter | null,
            })
          }
        >
          <option value="">{t("Any due date")}</option>
          {(Object.keys(DUE_LABELS) as DueFilter[]).map((d) => (
            <option key={d} value={d}>
              {t(DUE_LABELS[d])}
            </option>
          ))}
        </select>
        {isFilterActive(filter) && (
          <button
            className="xc-btn ghost small"
            onClick={() => onChange(emptyFilter)}
          >
            <X size={13} /> {t("Clear")}
          </button>
        )}
        {extra && <span className="projects-filter-extra">{extra}</span>}
      </div>
    </>
  );
}
