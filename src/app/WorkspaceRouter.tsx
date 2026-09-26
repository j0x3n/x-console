import type * as Model from "../types/domain";
import React from "react";
import { companyRecords, peopleRecords } from "../data/workspace";
import CompanyPage from "../features/companies/CompanyPage";
import PersonPage from "../features/people/PersonPage";
import WorkPage from "../features/work/WorkPage";
import CrewPage from "../features/crew/CrewPage";
import DealsPage from "../features/deals/DealsPage";
import RecordsPage from "../features/records/RecordsPage";

interface WorkspaceRouterProps {
  view: string;
  language: Model.Language;
  navigate: Model.Navigate;
  openDelegate: Model.OpenDelegate;
  tasks: Model.DelegatedTask[];
  decisions: Model.Decision[];
  decisionHistory: Model.DecisionEntry[];
  exitingDecisions: Model.Decision[];
  outcomes: Model.DecisionOutcomes;
  onAction: Model.DecisionHandler;
  onUndoDecision: (key: string) => void;
  onEdit: (item: Model.Decision) => void;
  activity: Model.ActivityRow[];
  t: Model.Translate;
  workStatuses: Model.WorkStatuses;
  setWorkStatuses: Model.Setter<Model.WorkStatuses>;
  hireRequest: number;
  onHireRequestHandled: () => void;
  hiredAgents: Model.HiredAgent[];
  setHiredAgents: Model.Setter<Model.HiredAgent[]>;
  onNotify: Model.Notify;
}

export default function WorkspaceRouter({
  view,
  language,
  navigate,
  openDelegate,
  tasks,
  decisions,
  decisionHistory,
  exitingDecisions,
  outcomes,
  onAction,
  onUndoDecision,
  onEdit,
  activity,
  t,
  workStatuses,
  setWorkStatuses,
  hireRequest,
  onHireRequestHandled,
  hiredAgents,
  setHiredAgents,
  onNotify,
}: WorkspaceRouterProps) {
  const L: Model.Localize = (en, zh) =>
    language === "zh"
      ? zh && zh !== en
        ? String(zh)
        : t(en)
      : String(en ?? "");
  const rawCompany = companyRecords.find((item) => item.name === view);
  const company: Model.CompanyRecord | undefined =
    rawCompany?.name === "Brightwell Labs" && outcomes[3] === "approved"
      ? { ...rawCompany, stage: "Negotiation" }
      : rawCompany;
  const person = peopleRecords.find((item) => item.name === view);
  if (company)
    return (
      <CompanyPage
        key={view}
        company={company}
        L={L}
        language={language}
        navigate={navigate}
        openDelegate={openDelegate}
        decisions={decisions}
        decisionHistory={decisionHistory}
        exitingDecisions={exitingDecisions}
        onAction={onAction}
        onUndoDecision={onUndoDecision}
        onEdit={onEdit}
        activity={activity}
        workStatuses={workStatuses}
      />
    );
  if (person)
    return (
      <PersonPage
        key={view}
        person={person}
        L={L}
        navigate={navigate}
        openDelegate={openDelegate}
        activity={activity}
      />
    );
  if (view === "Work")
    return (
      <WorkPage
        key={view}
        L={L}
        language={language}
        navigate={navigate}
        openDelegate={openDelegate}
        tasks={tasks}
        decisions={decisions}
        decisionHistory={decisionHistory}
        exitingDecisions={exitingDecisions}
        onAction={onAction}
        onUndoDecision={onUndoDecision}
        onEdit={onEdit}
        statuses={workStatuses}
        setStatuses={setWorkStatuses}
      />
    );
  if (view === "Crew")
    return (
      <CrewPage
        key={view}
        L={L}
        language={language}
        navigate={navigate}
        openDelegate={openDelegate}
        activity={activity}
        decisions={decisions}
        hireRequest={hireRequest}
        onHireRequestHandled={onHireRequestHandled}
        hired={hiredAgents}
        setHired={setHiredAgents}
        onNotify={onNotify}
      />
    );
  if (view === "Deals")
    return (
      <DealsPage
        key={view}
        L={L}
        language={language}
        navigate={navigate}
        decisions={decisions}
        decisionHistory={decisionHistory}
        exitingDecisions={exitingDecisions}
        outcomes={outcomes}
        onAction={onAction}
        onUndoDecision={onUndoDecision}
      />
    );
  if (view === "People" || view === "Companies")
    return (
      <RecordsPage
        key={view}
        kind={view}
        L={L}
        language={language}
        navigate={navigate}
        openDelegate={openDelegate}
        outcomes={outcomes}
      />
    );
  return null;
}
