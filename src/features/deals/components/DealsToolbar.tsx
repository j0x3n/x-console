import type * as Model from "../../../types/domain";
import React from "react";
import AgentIcon from "../../../components/ui/AgentIcon";
import { forecastZh } from "../../../data/catalogs";
import { dealRecords } from "../../../data/workspace";

interface DealsToolbarProps {
  owner: string;
  setOwner: Model.Setter<string>;
  L: Model.Localize;
  forecast: string;
  setForecast: Model.Setter<string>;
  mode: string;
  setDealsMode: (mode: string) => void;
}

export default function DealsToolbar({
  owner,
  setOwner,
  L,
  forecast,
  setForecast,
  mode,
  setDealsMode,
}: DealsToolbarProps) {
  return (
    <div className="ws-toolbar">
      <div className="ws-agent-filters">
        <button
          className={owner === "All" ? "active" : ""}
          onClick={() => setOwner("All")}
        >
          {L("All", "全部")}
        </button>
        {["jo", "Theo", "Ines", "Kofi", "Ruth", "Scout"].map((name) => (
          <button
            key={name}
            title={name}
            className={owner === name ? "active" : ""}
            onClick={() => setOwner(owner === name ? "All" : name)}
          >
            <AgentIcon name={name} />
          </button>
        ))}
      </div>
      <div className="ws-forecast-filters">
        {["Commit", "Best case", "Pipeline"].map((value) => (
          <button
            key={value}
            className={forecast === value ? "active" : ""}
            onClick={() => setForecast(forecast === value ? "All" : value)}
          >
            {L(value, forecastZh[value])}{" "}
            <span>
              {dealRecords.filter((item) => item.forecast === value).length}
            </span>
          </button>
        ))}
      </div>
      <div className="ws-mode">
        <button
          className={mode === "Board" ? "active" : ""}
          onClick={() => setDealsMode("Board")}
        >
          {L("Board", "看板")}
        </button>
        <button
          className={mode === "Table" ? "active" : ""}
          onClick={() => setDealsMode("Table")}
        >
          {L("Table", "表格")}
        </button>
      </div>
    </div>
  );
}
