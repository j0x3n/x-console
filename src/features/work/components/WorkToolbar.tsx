import type * as Model from "../../../types/domain";
import React from "react";
import AgentIcon from "../../../components/ui/AgentIcon";
import { Filter, List } from "lucide-react";
import { companyRecords } from "../../../data/workspace";

interface WorkToolbarProps {
  L: Model.Localize;
  assignee: string;
  setAssignee: Model.Setter<string>;
  type: string;
  setType: Model.Setter<string>;
  record: string;
  setRecord: Model.Setter<string>;
  mode: string;
  setWorkMode: (mode: string) => void;
}

export default function WorkToolbar({
  L,
  assignee,
  setAssignee,
  type,
  setType,
  record,
  setRecord,
  mode,
  setWorkMode,
}: WorkToolbarProps) {
  return (
    <div className="ws-toolbar">
      <div
        className="ws-agent-filters"
        aria-label={L("Filter by assignee", "按负责人筛选")}
      >
        <button
          className={assignee === "All" ? "active" : ""}
          onClick={() => setAssignee("All")}
          title={L("All assignees", "全部负责人")}
        >
          {L("All", "全部")}
        </button>
        {[
          "Scout",
          "Scribe",
          "Ledger",
          "Pilot",
          "Echo",
          "Theo",
          "Ines",
          "Kofi",
          "Ruth",
        ].map((name) => (
          <button
            key={name}
            className={assignee === name ? "active" : ""}
            onClick={() => setAssignee(assignee === name ? "All" : name)}
            title={name}
          >
            <AgentIcon name={name} />
          </button>
        ))}
      </div>
      <label className="ws-select">
        <Filter size={13} />
        <select
          aria-label={L("Type", "类型")}
          value={type}
          onChange={(e) => setType(e.target.value)}
        >
          {[
            "All",
            "Research",
            "Follow-up",
            "Pipeline",
            "Renewal",
            "Meeting prep",
            "Task",
          ].map((value) => (
            <option key={value} value={value}>
              {value === "All"
                ? L("Type: All", "类型：全部")
                : L(
                    value,
                    {
                      Research: "研究",
                      "Follow-up": "跟进",
                      Pipeline: "商机",
                      Renewal: "续约",
                      "Meeting prep": "会议准备",
                      Task: "任务",
                    }[value],
                  )}
            </option>
          ))}
        </select>
      </label>
      <label className="ws-select">
        <select
          aria-label={L("Record", "记录")}
          value={record}
          onChange={(e) => setRecord(e.target.value)}
        >
          <option value="All">{L("Record: All", "记录：全部")}</option>
          {companyRecords.map((row) => (
            <option key={row.name}>{row.name}</option>
          ))}
        </select>
      </label>
      <div className="ws-mode" role="group" aria-label={L("View", "视图")}>
        <button
          className={mode === "Board" ? "active" : ""}
          onClick={() => setWorkMode("Board")}
        >
          {L("Board", "看板")}
        </button>
        <button
          className={mode === "List" ? "active" : ""}
          onClick={() => setWorkMode("List")}
        >
          <List size={13} />
          {L("List", "列表")}
        </button>
      </div>
    </div>
  );
}
