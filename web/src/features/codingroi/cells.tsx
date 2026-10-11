import type { ReactNode } from "react";
import { ArrowDown, ArrowUp } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { nextSort, type Sort } from "./format";

/** 没有记录的格子。服务端给 null，不能显示成 0。 */
export function NoRecord() {
  const t = useT();
  return <span className="roi-none">{t("No record")}</span>;
}

/** 不适用的格子，比如还没开始的任务没有费用。 */
export function NotApplicable() {
  return (
    <span className="roi-none" aria-hidden="true">
      –
    </span>
  );
}

/** 有值显示值，没有值显示“无记录”。 */
export function Value({
  text,
  atLeast,
  note,
}: {
  text: string | null;
  /** 合计里有任务没有记录时，数字只是下限。 */
  atLeast?: boolean;
  note?: ReactNode;
}) {
  const t = useT();
  if (text == null) return <NoRecord />;
  return (
    <>
      {atLeast && <small className="roi-atleast">{t("At least value")} </small>}
      {text}
      {note}
    </>
  );
}

/** 表头里点一下按这一列排序，再点一下反过来。 */
export function SortTh({
  label,
  col,
  sort,
  onSort,
  title,
  className = "",
}: {
  label: string;
  col: string;
  sort: Sort;
  onSort: (s: Sort) => void;
  title?: string;
  className?: string;
}) {
  const active = sort.key === col;
  return (
    <th
      className={className}
      aria-sort={active ? (sort.desc ? "descending" : "ascending") : "none"}
    >
      <button
        type="button"
        className="roi-sort"
        title={title}
        onClick={() => onSort(nextSort(sort, col))}
      >
        {label}
        {active &&
          (sort.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} />)}
      </button>
    </th>
  );
}
