import { ArrowDown, ArrowUp } from "lucide-react";
import { nextSort, type Sort, type SortKey } from "../logic";

/** 列表表头里能点的排序列。云盘和外部网盘共用，手机上靠 span 的位置隐藏大小和时间列。 */
export default function SortHeader({
  label,
  col,
  sort,
  setSort,
}: {
  label: string;
  col: SortKey;
  sort: Sort;
  setSort: (s: Sort) => void;
}) {
  const active = sort.key === col;
  return (
    <span
      role="columnheader"
      className={col === "name" ? "" : "drive-col"}
      aria-sort={active ? (sort.desc ? "descending" : "ascending") : "none"}
    >
      <button
        className="drive-sort"
        onClick={() => setSort(nextSort(sort, col))}
      >
        {label}
        {active &&
          (sort.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} />)}
      </button>
    </span>
  );
}
