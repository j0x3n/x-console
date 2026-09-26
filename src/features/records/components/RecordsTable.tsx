import type * as Model from "../../../types/domain";
import React from "react";
import { stageZh, rolesZh } from "../../../data/catalogs";

interface RecordsTableProps {
  L: Model.Localize;
  allSelected: boolean;
  setSelected: Model.Setter<string[]>;
  filtered: Model.RecordData[];
  tableHeaders: string[];
  groups: [string | null, Model.RecordData[]][];
  renderRow: (row: Model.RecordData) => React.ReactNode;
}

export default function RecordsTable({
  L,
  allSelected,
  setSelected,
  filtered,
  tableHeaders,
  groups,
  renderRow,
}: RecordsTableProps) {
  return (
    <div className="ws-table-wrap">
      <table className="ws-table ws-record-table">
        <thead>
          <tr>
            <th>
              <label className="ws-checkbox">
                <input
                  type="checkbox"
                  aria-label={L("Select all in view", "全选当前视图")}
                  checked={allSelected}
                  onChange={() =>
                    setSelected(
                      allSelected ? [] : filtered.map((row) => row.name),
                    )
                  }
                />
              </label>
              {tableHeaders[0]}
            </th>
            {tableHeaders.slice(1).map((heading) => (
              <th key={heading}>{heading}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.flatMap(([key, members]) => [
            key && (
              <tr className="ws-group-row" key={`group-${key}`}>
                <td colSpan={tableHeaders.length}>
                  {L(key, stageZh[key] || rolesZh[key] || key)}{" "}
                  <span>{members.length}</span>
                </td>
              </tr>
            ),
            ...members.map(renderRow),
          ])}
          {filtered.length === 0 && (
            <tr>
              <td colSpan={tableHeaders.length} className="ws-empty-table">
                {L("No matching records", "没有匹配的记录")}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
