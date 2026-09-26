import type * as Model from "../../types/domain";
import React, { useState, useEffect } from "react";
import { dealRecords } from "../../data/workspace";
import PageHeading from "../../components/ui/PageHeading";
import DealsToolbar from "./components/DealsToolbar";
import DealsBoard from "./components/DealsBoard";
import DealsTable from "./components/DealsTable";

interface DealsPageProps {
  L: Model.Localize;
  language: Model.Language;
  navigate: Model.Navigate;
  decisions: Model.Decision[];
  decisionHistory: Model.DecisionEntry[];
  exitingDecisions: Model.Decision[];
  onAction: Model.DecisionHandler;
  onUndoDecision: (key: string) => void;
  outcomes: Model.DecisionOutcomes;
}

export default function DealsPage({
  L,
  language,
  navigate,
  decisions,
  decisionHistory,
  exitingDecisions,
  onAction,
  onUndoDecision,
  outcomes,
}: DealsPageProps) {
  const [owner, setOwner] = useState("All");
  const [forecast, setForecast] = useState("All");
  const [mode, setMode] = useState(() =>
    window.location.pathname === "/records/deals" &&
    new URLSearchParams(window.location.search).get("view") === "table"
      ? "Table"
      : "Board",
  );
  useEffect(() => {
    const syncFromUrl = () => {
      if (window.location.pathname === "/records/deals")
        setMode(
          new URLSearchParams(window.location.search).get("view") === "table"
            ? "Table"
            : "Board",
        );
    };
    window.addEventListener("popstate", syncFromUrl);
    return () => window.removeEventListener("popstate", syncFromUrl);
  }, []);
  const pilotDecision = [...decisions, ...exitingDecisions].find(
    (item) => item.kind === "stage",
  );
  const pilotOutcome = decisionHistory.find(
    (entry) => entry.item.kind === "stage",
  );
  const visible = dealRecords
    .map((item) => ({
      ...item,
      titleEn: item.title,
      stage:
        item.id === 26 && outcomes[3] === "approved"
          ? "Negotiation"
          : item.stage,
      title: L(item.title, item.titleZh),
      close: L(item.close, item.closeZh),
    }))
    .filter(
      (item) =>
        (owner === "All" || item.owner === owner) &&
        (forecast === "All" || item.forecast === forecast),
    );
  const setDealsMode = (next: string) => {
    setMode(next);
    window.history.pushState(
      {},
      "",
      next === "Table" ? "/records/deals?view=table" : "/records/deals",
    );
  };
  const jumpToPilot = () =>
    document
      .getElementById("ws-pilot-proposal")
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
  const formatTotal = (value: number) =>
    value >= 1000 ? `$${(value / 1000).toFixed(2)}M` : `$${value}K`;
  return (
    <div className="workspace-page ws-deals-page">
      <PageHeading
        title={L("$4.82M open across 38 deals", "38 笔商机，进行中金额 $4.82M")}
        subtitle={L(
          "$1.31M committed for Q4 is 82% of the $1.60M target; best case reaches $1.94M.",
          "第四季度确定收入 $1.31M，达到 $1.60M 目标的 82%；最佳预期为 $1.94M。",
        )}
        aside={
          <div className="ws-deals-pulse">
            <div className="ws-deals-aside">
              <span className="ws-pulse-pilot" aria-hidden="true">
                ▲
              </span>
              <span>
                {L(
                  "Pilot kept 5 deals current since midnight",
                  "Pilot 自午夜起维护了 5 笔商机",
                )}
              </span>
              <i className="ws-pulse-separator" />
              <button onClick={jumpToPilot} disabled={!pilotDecision}>
                {pilotDecision
                  ? L("1 move needs your call", "1 项阶段调整待你决定")
                  : L("No moves need your call", "暂无待决定的阶段调整")}
              </button>
            </div>
            <div
              className="ws-forecast-track"
              role="img"
              aria-label={L(
                "Q4 forecast: commit $1.31M, best case $1.94M, target $1.60M",
                "第四季度预测：确定收入 $1.31M，最佳预期 $1.94M，目标 $1.60M",
              )}
            >
              <i className="ws-forecast-commit" />
              <i className="ws-forecast-best" />
              <i className="ws-forecast-target" />
            </div>
            <div className="ws-forecast-scale">
              <span>$0</span>
              <span>{L("$1.60M target", "$1.60M 目标")}</span>
            </div>
          </div>
        }
      />
      <DealsToolbar
        owner={owner}
        setOwner={setOwner}
        L={L}
        forecast={forecast}
        setForecast={setForecast}
        mode={mode}
        setDealsMode={setDealsMode}
      />
      {mode === "Board" ? (
        <DealsBoard
          visible={visible}
          L={L}
          navigate={navigate}
          pilotDecision={pilotDecision}
          owner={owner}
          forecast={forecast}
          exitingDecisions={exitingDecisions}
          onAction={onAction}
          pilotOutcome={pilotOutcome}
          language={language}
          onUndoDecision={onUndoDecision}
        />
      ) : (
        <DealsTable
          L={L}
          visible={visible}
          formatTotal={formatTotal}
          pilotDecision={pilotDecision}
          navigate={navigate}
          decisions={decisions}
        />
      )}
    </div>
  );
}
