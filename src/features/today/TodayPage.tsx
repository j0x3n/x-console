import type * as Model from "../../types/domain";
import React from "react";
import PipelinePulse from "./components/PipelinePulse";
import DecisionCard from "./components/DecisionCard";
import DecisionOutcomeRow from "../../components/decisions/DecisionOutcomeRow";
import { CheckCheck } from "lucide-react";
import Meetings from "./components/Meetings";
import ActivityPanel from "./components/ActivityPanel";

interface TodayPageProps {
  t: Model.Translate;
  navigate: Model.Navigate;
  language: Model.Language;
  decisions: Model.Decision[];
  exitingDecisions: Model.Decision[];
  activeDecision: number;
  onAction: Model.DecisionHandler;
  setEditItem: Model.Setter<Model.Decision | null>;
  decisionHistory: Model.DecisionEntry[];
  onUndoDecision: (key: string) => void;
  activity: Model.ActivityRow[];
}

export default function TodayPage({
  t,
  navigate,
  language,
  decisions,
  exitingDecisions,
  activeDecision,
  onAction,
  setEditItem,
  decisionHistory,
  onUndoDecision,
  activity,
}: TodayPageProps) {
  return (
    <div className="today-content">
      <div className="greeting-row">
        <div>
          <h1>{t("Good morning, jo")}</h1>
          <p>
            {t("While you were offline, your crew finished")}{" "}
            <button className="inline-link" onClick={() => navigate("Crew")}>
              {t("37 runs")}
            </button>
            {language === "zh" ? "，" : ". "}
            <strong>
              {decisions.length} {t("need your call.")}
            </strong>
          </p>
        </div>
        <time>{t("Thursday, September 24 · 9:12 AM")}</time>
      </div>
      <PipelinePulse navigate={navigate} />
      <div className="dashboard-grid">
        <section className="decisions-section">
          <div className="section-heading">
            <h2>
              {t("Needs your call")}{" "}
              <span className="count-pill">{decisions.length}</span>
            </h2>
            <span className="keyboard-help">
              <kbd>J</kbd>
              <kbd>K</kbd> {t("to move")} <kbd>↵</kbd> {t("to approve")}
            </span>
          </div>
          <div className="decision-list">
            {[...decisions, ...exitingDecisions]
              .sort((a, b) => a.id - b.id)
              .map((item, i) => (
                <DecisionCard
                  key={item.id}
                  item={item}
                  active={i === activeDecision}
                  exiting={exitingDecisions.some((row) => row.id === item.id)}
                  onAction={onAction}
                  onEdit={setEditItem}
                />
              ))}
            {decisionHistory
              .filter(
                (entry) =>
                  !exitingDecisions.some((row) => row.id === entry.item.id),
              )
              .map((entry) => (
                <DecisionOutcomeRow
                  key={entry.key}
                  entry={entry}
                  language={language}
                  navigate={navigate}
                  onUndo={onUndoDecision}
                />
              ))}
            {!decisions.length && !decisionHistory.length && (
              <div className="all-clear">
                <CheckCheck size={24} />
                <strong>{t("All caught up")}</strong>
                <span>{t("Your crew will bring new decisions here.")}</span>
              </div>
            )}
          </div>
        </section>
        <aside
          className="right-column"
          aria-label={t("Today's schedule and crew activity")}
        >
          <Meetings navigate={navigate} />
          <ActivityPanel navigate={navigate} activity={activity} />
        </aside>
      </div>
    </div>
  );
}
