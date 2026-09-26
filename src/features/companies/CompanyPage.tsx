import type * as Model from "../../types/domain";
import React, { useState } from "react";
import {
  companyDetailData,
  companyProfileData,
} from "../../data/company-details";
import { peopleRecords, dealRecords, workItems } from "../../data/workspace";
import { names, stageZh, localizedTime } from "./company-config";
import CompanyHeading from "./components/CompanyHeading";
import CompanyOverview from "./components/CompanyOverview";
import AccountIcon from "./components/AccountIcon";
import { FileText, SquarePen } from "lucide-react";
import DecisionOutcomeRow from "../../components/decisions/DecisionOutcomeRow";
import CompanyTimeline from "./components/CompanyTimeline";
import CompanyBrief from "./components/CompanyBrief";
import CompanyWork from "./components/CompanyWork";
import CompanySignals from "./components/CompanySignals";
import CompanyPeople from "./components/CompanyPeople";
import CompanyDeals from "./components/CompanyDeals";

interface CompanyPageProps {
  company: Model.CompanyRecord;
  L: Model.Localize;
  language: Model.Language;
  navigate: Model.Navigate;
  openDelegate: Model.OpenDelegate;
  decisions: Model.Decision[];
  decisionHistory: Model.DecisionEntry[];
  exitingDecisions: Model.Decision[];
  onAction: Model.DecisionHandler;
  onUndoDecision: (key: string) => void;
  onEdit: (item: Model.Decision) => void;
  activity: Model.ActivityRow[];
  workStatuses: Model.WorkStatuses;
}

export default function CompanyPage({
  company,
  L,
  language,
  navigate,
  openDelegate,
  decisions,
  decisionHistory,
  exitingDecisions,
  onAction,
  onUndoDecision,
  onEdit,
  activity,
  workStatuses,
}: CompanyPageProps) {
  const [timelineFilter, setTimelineFilter] = useState("All");
  const [copied, setCopied] = useState(false);
  const [stepAccepted, setStepAccepted] = useState(false);
  const detail =
    companyDetailData[company.name] || companyProfileData[company.name];
  const people = peopleRecords.filter((item) => item.company === company.name);
  const deals = dealRecords.filter((item) => item.company === company.name);
  const work = workItems.filter((item) => item.company === company.name);
  const openWork = work.filter(
    (item) => (workStatuses[item.id] || item.status) !== "Done today",
  );
  const doneWork = work.filter(
    (item) => (workStatuses[item.id] || item.status) === "Done today",
  );
  const pending = [...decisions, ...exitingDecisions].find(
    (item) => item.company === company.name,
  );
  const leadingDeal =
    deals.find((deal) => deal.value === detail?.nextCloseValue) ||
    deals.reduce<Model.DealRecord | null>(
      (best, deal) => (!best || deal.value > best.value ? deal : best),
      null,
    );
  const isRisk = company.change < 0;
  const accountType = detail?.type || (isRisk ? "Customer" : "Prospect");
  const domain =
    detail?.domain ||
    `${company.name.toLowerCase().replace(/[^a-z0-9]/g, "")}.com`;
  const genericTimeline: Model.AccountTimelineEntry[] = [
    ...activity
      .filter((row) => row[2] === company.name)
      .map<Model.AccountTimelineEntry>((row) => [
        "Today",
        row[0],
        `${row[0]} ${row[1]}`,
        row[3],
        row[4],
        company.signal,
      ]),
    [
      "Yesterday",
      company.owner,
      `${names[company.owner] || company.owner} reviewed the next step`,
      company.next,
      "3:00 PM",
      "Account activity",
      "People",
    ],
    [
      "Tue, Sep 22",
      company.agent,
      `${company.agent} found a signal: ${company.signal}`,
      "",
      "9:30 AM",
      "Company research",
    ],
  ];
  const entries = detail?.timeline || genericTimeline;
  const counts = {
    All: entries.length,
    Crew: entries.filter((item) => item[6] !== "People").length,
    People: entries.filter((item) => item[6] === "People").length,
  };
  const shownEntries = entries.filter(
    (item) =>
      timelineFilter === "All" ||
      (timelineFilter === "People") === (item[6] === "People"),
  );
  const groups = [...new Set(shownEntries.map((item) => item[0]))];
  const signals: Model.AccountSignal[] = detail?.signals || [
    [
      company.signal,
      company.signalZh,
      isRisk ? "risk" : "positive",
      company.agent,
      "Account activity",
      company.touch,
    ],
  ];
  const brief = detail?.brief || [
    `${company.name} is in ${company.stage.toLowerCase()} with $${company.value}K open pipeline. ${company.signal}.`,
    `The next step is ${company.next.charAt(0).toLowerCase() + company.next.slice(1)}.`,
  ];
  const briefZh = detail?.briefZh || [
    `${company.name} 处于${stageZh[company.stage]}阶段，进行中金额 $${company.value}K。${company.signalZh}。`,
    `下一步：${company.nextZh}。`,
  ];
  const copyLink = async () => {
    try {
      await navigator.clipboard.writeText(window.location.href);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };
  return (
    <div className="workspace-page ws-company-page">
      <CompanyHeading
        company={company}
        L={L}
        accountType={accountType}
        domain={domain}
        detail={detail}
        copyLink={copyLink}
        copied={copied}
        openDelegate={openDelegate}
      />

      <CompanyOverview
        L={L}
        pending={pending}
        company={company}
        deals={deals}
        detail={detail}
        leadingDeal={leadingDeal}
        stepAccepted={stepAccepted}
        setStepAccepted={setStepAccepted}
      />

      {pending && (
        <section
          className={
            "ws-account-decision" +
            (exitingDecisions.some((item) => item.id === pending.id)
              ? " is-exiting"
              : "")
          }
          aria-label={pending.title}
        >
          <div className="ws-account-decision-meta">
            <AccountIcon name={pending.agent} />
            <b>{pending.agent}</b>
            <span>{L(pending.verb)}</span>
            <span>· {localizedTime(pending.time || "recently", L)}</span>
          </div>
          <h2>{L(pending.title)}</h2>
          <p>
            {L(
              pending.subtitle ||
                pending.detail ||
                (pending.kind === "draft"
                  ? "VP Engineering · call at 10:00"
                  : "Review the suggested action"),
            )}
          </p>
          <div className="ws-account-decision-bottom">
            <span>
              <FileText size={13} />
              {L(pending.receipts)} <em>{pending.confidence}</em>
            </span>
            <div className="ws-review-actions">
              <button
                className="ws-approve"
                onClick={() => onAction(pending, "approved")}
              >
                {L(
                  pending.kind === "draft" ? "Send follow-up" : "Approve",
                  pending.kind === "draft" ? "发送跟进邮件" : "批准",
                )}
              </button>
              {pending.kind === "draft" && (
                <button onClick={() => onEdit(pending)}>
                  <SquarePen size={13} />
                  {L("Edit", "编辑")}
                </button>
              )}
              <button
                onClick={() =>
                  onAction(
                    pending,
                    pending.kind === "stage" ? "kept" : "dismissed",
                  )
                }
              >
                {L(
                  pending.kind === "stage"
                    ? "Keep stage"
                    : pending.kind === "draft"
                      ? "Skip"
                      : "Dismiss",
                  pending.kind === "stage"
                    ? "保持阶段"
                    : pending.kind === "draft"
                      ? "跳过"
                      : "忽略",
                )}
              </button>
            </div>
          </div>
        </section>
      )}
      {decisionHistory
        .filter(
          (entry) =>
            entry.item.company === company.name &&
            !exitingDecisions.some((item) => item.id === entry.item.id),
        )
        .map((entry) => (
          <div className="ws-account-outcome" key={entry.key}>
            <DecisionOutcomeRow
              entry={entry}
              language={language}
              navigate={navigate}
              onUndo={onUndoDecision}
            />
          </div>
        ))}

      <CompanyTimeline
        L={L}
        counts={counts}
        timelineFilter={timelineFilter}
        setTimelineFilter={setTimelineFilter}
        groups={groups}
        shownEntries={shownEntries}
      />

      <div className="ws-account-knowledge">
        <h2>
          {L(
            `What your crew knows about ${company.name}`,
            `智能团队了解到的 ${company.name}`,
          )}
        </h2>
        <CompanyBrief
          L={L}
          detail={detail}
          company={company}
          brief={brief}
          briefZh={briefZh}
        />
        <CompanyWork
          L={L}
          openWork={openWork}
          pending={pending}
          navigate={navigate}
          workStatuses={workStatuses}
          doneWork={doneWork}
        />
        <CompanySignals L={L} signals={signals} />
        <CompanyPeople
          L={L}
          people={people}
          navigate={navigate}
          detail={detail}
        />
        <CompanyDeals
          L={L}
          deals={deals}
          company={company}
          navigate={navigate}
          leadingDeal={leadingDeal}
        />
      </div>
    </div>
  );
}
