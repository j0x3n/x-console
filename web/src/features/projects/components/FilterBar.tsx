import { Search, X } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { Label, Milestone } from "../api";
import {
  PRIORITIES,
  PRIORITY_LABELS,
  STATUSES,
  STATUS_LABELS,
  emptyFilter,
  isFilterActive,
  type IssueFilter,
  type IssueStatus,
} from "../logic";
import { StatusIcon } from "./Icons";

/** 筛选栏：状态（可多选）、优先级、标签、里程碑、搜索。 */
export default function FilterBar({
  filter,
  onChange,
  labels,
  milestones,
  showStatus = true,
}: {
  filter: IssueFilter;
  onChange: (filter: IssueFilter) => void;
  labels: Label[];
  milestones: Milestone[];
  showStatus?: boolean;
}) {
  const t = useT();
  const toggleStatus = (status: IssueStatus) =>
    onChange({
      ...filter,
      statuses: filter.statuses.includes(status)
        ? filter.statuses.filter((s) => s !== status)
        : [...filter.statuses, status],
    });
  return (
    <div className="projects-filters">
      <label className="projects-search">
        <Search size={14} />
        <input
          value={filter.q}
          onChange={(e) => onChange({ ...filter, q: e.target.value })}
          placeholder={t("Search issues")}
          aria-label={t("Search issues")}
        />
      </label>
      {showStatus && (
        <div className="projects-status-filter" role="group" aria-label={t("Status")}>
          {STATUSES.map((s) => (
            <button
              key={s}
              className={filter.statuses.includes(s) ? "on" : ""}
              aria-pressed={filter.statuses.includes(s)}
              title={t(STATUS_LABELS[s])}
              onClick={() => toggleStatus(s)}
            >
              <StatusIcon status={s} size={14} />
            </button>
          ))}
        </div>
      )}
      <select
        className="xc-select projects-filter-select"
        value={filter.priority ?? ""}
        aria-label={t("Priority")}
        onChange={(e) =>
          onChange({ ...filter, priority: e.target.value === "" ? null : Number(e.target.value) })
        }
      >
        <option value="">{t("Any priority")}</option>
        {PRIORITIES.map((p) => (
          <option key={p} value={p}>
            {t(PRIORITY_LABELS[p])}
          </option>
        ))}
      </select>
      {labels.length > 0 && (
        <select
          className="xc-select projects-filter-select"
          value={filter.labelId ?? ""}
          aria-label={t("Label")}
          onChange={(e) =>
            onChange({ ...filter, labelId: e.target.value ? Number(e.target.value) : null })
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
            onChange({ ...filter, milestoneId: e.target.value ? Number(e.target.value) : null })
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
      {isFilterActive(filter) && (
        <button className="xc-btn ghost small" onClick={() => onChange(emptyFilter)}>
          <X size={13} /> {t("Clear")}
        </button>
      )}
    </div>
  );
}
