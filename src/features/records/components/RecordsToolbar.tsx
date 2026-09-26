import type * as Model from "../../../types/domain";
import React from "react";
import { Search, Filter, SlidersHorizontal, Download } from "lucide-react";
import { stages, stageZh } from "../../../data/catalogs";
import { exportCsv } from "../../../lib/exportCsv";
import { companyRecords } from "../../../data/workspace";

interface RecordsToolbarProps {
  L: Model.Localize;
  kind: string;
  isPeople: boolean;
  query: string;
  setQuery: Model.Setter<string>;
  filter: string;
  setFilter: Model.Setter<string>;
  sort: string;
  setSort: Model.Setter<string>;
  group: string;
  setGroup: Model.Setter<string>;
  tableHeaders: string[];
  filtered: Model.RecordData[];
}

export default function RecordsToolbar({
  L,
  kind,
  isPeople,
  query,
  setQuery,
  filter,
  setFilter,
  sort,
  setSort,
  group,
  setGroup,
  tableHeaders,
  filtered,
}: RecordsToolbarProps) {
  return (
    <div className="ws-toolbar ws-records-toolbar">
      <label className="ws-search">
        <Search size={15} />
        <input
          aria-label={L(
            `Search ${kind.toLowerCase()}`,
            `搜索${isPeople ? "联系人" : "公司"}`,
          )}
          placeholder={L(
            `Search ${kind.toLowerCase()}...`,
            `搜索${isPeople ? "联系人" : "公司"}...`,
          )}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </label>
      <label className="ws-select">
        <Filter size={13} />
        <select
          aria-label={L("Filter", "筛选")}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        >
          {(isPeople
            ? ["All", "Warm", "Neutral", "Cold"]
            : ["All", ...stages]
          ).map((item) => (
            <option key={item} value={item}>
              {item === "All"
                ? L("Filter: All", "筛选：全部")
                : L(
                    item,
                    stageZh[item] ||
                      { Warm: "活跃", Neutral: "一般", Cold: "冷淡" }[item],
                  )}
            </option>
          ))}
        </select>
      </label>
      <span className="ws-toolbar-spacer" />
      <label className="ws-select">
        <SlidersHorizontal size={13} />
        <select
          aria-label={L("Sort", "排序")}
          value={sort}
          onChange={(e) => setSort(e.target.value)}
        >
          {["Last touch", "Name", ...(isPeople ? [] : ["Health", "Value"])].map(
            (item) => (
              <option key={item} value={item}>
                {L(
                  item,
                  {
                    "Last touch": "最近动态",
                    Name: "名称",
                    Health: "健康度",
                    Value: "金额",
                  }[item],
                )}
              </option>
            ),
          )}
        </select>
      </label>
      <label className="ws-select">
        <select
          aria-label={L("Group", "分组")}
          value={group}
          onChange={(e) => setGroup(e.target.value)}
        >
          {["None", "Stage", "Owner"].map((item) => (
            <option key={item} value={item}>
              {L(
                item,
                {
                  None: "不分组",
                  Stage: isPeople ? "按角色" : "按阶段",
                  Owner: isPeople ? "按公司" : "按负责人",
                }[item],
              )}
            </option>
          ))}
        </select>
      </label>
      <button
        className="ws-icon-button"
        title={L("Export CSV", "导出 CSV")}
        onClick={() =>
          exportCsv(`${kind.toLowerCase()}.csv`, [
            isPeople ? tableHeaders : tableHeaders.slice(0, -1),
            ...filtered.map((row) =>
              isPeople
                ? [
                    row.name,
                    row.company,
                    row.role,
                    row.warmth,
                    row.knownBy,
                    row.touch,
                    companyRecords.find((item) => item.name === row.company)
                      ?.next || "",
                  ]
                : [
                    row.name,
                    row.stage,
                    row.owner,
                    `$${row.value}K`,
                    row.health,
                    `${row.agent} ${row.touch}`,
                    row.next,
                    row.signal,
                    row.segment,
                  ],
            ),
          ])
        }
      >
        <Download size={15} />
      </button>
    </div>
  );
}
