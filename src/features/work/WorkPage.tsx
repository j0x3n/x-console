import type * as Model from "../../types/domain";
import React, { useState, useEffect } from "react";
import { workItems } from "../../data/workspace";
import WorkToolbar from "./components/WorkToolbar";
import WorkBoard from "./components/WorkBoard";
import WorkList from "./components/WorkList";
import WorkDetailDrawer from "./components/WorkDetailDrawer";

interface WorkPageProps {
  L: Model.Localize;
  language: Model.Language;
  navigate: Model.Navigate;
  openDelegate: Model.OpenDelegate;
  tasks: Model.DelegatedTask[];
  decisions: Model.Decision[];
  decisionHistory: Model.DecisionEntry[];
  exitingDecisions: Model.Decision[];
  onAction: Model.DecisionHandler;
  onUndoDecision: (key: string) => void;
  onEdit: (item: Model.Decision) => void;
  statuses: Model.WorkStatuses;
  setStatuses: Model.Setter<Model.WorkStatuses>;
}

export default function WorkPage({
  L,
  language,
  navigate,
  openDelegate,
  tasks,
  decisions,
  decisionHistory,
  exitingDecisions,
  onAction,
  onUndoDecision,
  onEdit,
  statuses,
  setStatuses,
}: WorkPageProps) {
  const [assignee, setAssignee] = useState("All");
  const [type, setType] = useState("All");
  const [record, setRecord] = useState("All");
  const [mode, setMode] = useState(() =>
    window.location.pathname === "/work" &&
    new URLSearchParams(window.location.search).get("view") === "list"
      ? "List"
      : "Board",
  );
  const [groupBy, setGroupBy] = useState(
    () =>
      ({ assignee: "Assignee", record: "Record" })[
        new URLSearchParams(window.location.search).get("group") ?? ""
      ] || "Status",
  );
  useEffect(() => {
    const syncFromUrl = () => {
      if (window.location.pathname !== "/work") return;
      const params = new URLSearchParams(window.location.search);
      setMode(params.get("view") === "list" ? "List" : "Board");
      setGroupBy(
        { assignee: "Assignee", record: "Record" }[params.get("group") ?? ""] ||
          "Status",
      );
    };
    window.addEventListener("popstate", syncFromUrl);
    return () => window.removeEventListener("popstate", syncFromUrl);
  }, []);
  const [selected, setSelected] = useState<Model.WorkItem | null>(null);
  const dynamicTasks: Model.WorkItem[] = tasks.map((task, index) => ({
    id: `delegated-${index}`,
    title: typeof task === "string" ? task : task.title,
    titleZh: typeof task === "string" ? task : task.title,
    assignee: typeof task === "string" ? "Scout" : task.assignee,
    company: typeof task === "string" ? "X Console" : task.company,
    status: "Queued",
    type: "Task",
    progress: 0,
    detail: "Assigned just now",
    detailZh: "刚刚委派",
  }));
  const review: Model.WorkItem[] = [...decisions, ...exitingDecisions].map(
    (decision) => ({
      id: `decision-${decision.id}`,
      title:
        decision.kind === "draft"
          ? "Follow-up to Priya Raman"
          : decision.kind === "health"
            ? "Oakline renewal check-in"
            : "Move to Negotiation",
      titleZh:
        decision.kind === "draft"
          ? "跟进 Priya Raman"
          : decision.kind === "health"
            ? "Oakline 续约沟通"
            : "移至谈判阶段",
      assignee: decision.agent,
      company: decision.company,
      status: "Needs your call",
      type:
        decision.kind === "draft"
          ? "Follow-up"
          : decision.kind === "health"
            ? "Renewal"
            : "Pipeline",
      detail: decision.receipts,
      detailZh: L(decision.receipts),
      decision,
      isExiting: exitingDecisions.some((entry) => entry.id === decision.id),
    }),
  );
  const items = [...dynamicTasks, ...workItems, ...review].map((item) => ({
    ...item,
    status: statuses[item.id] || item.status,
  }));
  const visible = items.filter(
    (item) =>
      (assignee === "All" || item.assignee === assignee) &&
      (type === "All" || item.type === type) &&
      (record === "All" || item.company === record),
  );
  const visibleHistory = decisionHistory.filter((entry) => {
    const decisionType = {
      draft: "Follow-up",
      health: "Renewal",
      stage: "Pipeline",
    }[entry.item.kind];
    return (
      (assignee === "All" || entry.item.agent === assignee) &&
      (type === "All" || decisionType === type) &&
      (record === "All" || entry.item.company === record)
    );
  });
  const lanes = ["Queued", "Running", "Needs your call", "Done today"];
  const labels = {
    Queued: L("Queued", "排队中"),
    Running: L("Running", "进行中"),
    "Needs your call": L("Needs your call", "待你决定"),
    "Done today": L("Done today", "今日完成"),
  };
  const selectedItem =
    selected && items.find((item) => item.id === selected.id);
  const workListMeta = [
    ["1h", "$0.60", "—"],
    ["11:00 AM", "$0.25", "—"],
    ["yesterday", "—", "—"],
    ["12:00 PM", "$0.12", "—"],
    ["1h", "—", "—"],
    ["10:30 AM", "$0.55", "—"],
    ["02:04", "$0.42", "—"],
    ["05:04", "$0.14", "—"],
    ["1h 12m", "$0.31", "—"],
    ["32m", "—", "—"],
    ["yesterday", "—", "—"],
    ["8:55 AM", "$0.61", "94%"],
    ["8:49 AM", "$0.16", "97%"],
    ["8:42 AM", "$0.02", "98%"],
    ["8:37 AM", "$0.19", "90%"],
    ["8:30 AM", "$0.12", "93%"],
    ["8:12 AM", "$0.14", "91%"],
    ["8:05 AM", "—", "—"],
    ["7:22 AM", "$0.18", "96%"],
  ];
  const listMeta = (item: Model.WorkItem) =>
    item.decision
      ? [
          item.decision.time || "1h",
          item.decision.kind === "draft"
            ? "$0.18"
            : item.decision.kind === "health"
              ? "$0.34"
              : "$0.04",
          item.decision.confidence,
        ]
      : workListMeta[Number(item.id.replace("work-", ""))] || [
          L("just now", "刚刚"),
          "—",
          "—",
        ];
  const setWorkMode = (next: string) => {
    setMode(next);
    window.history.pushState(
      {},
      "",
      next === "List"
        ? `/work?view=list${groupBy === "Status" ? "" : `&group=${groupBy.toLowerCase()}`}`
        : "/work",
    );
  };
  const changeGroup = (next: string) => {
    setGroupBy(next);
    window.history.pushState(
      {},
      "",
      `/work?view=list${next === "Status" ? "" : `&group=${next.toLowerCase()}`}`,
    );
  };
  const listGroups =
    groupBy === "Status"
      ? lanes
      : groupBy === "Assignee"
        ? [
            "Scout",
            "Scribe",
            "Ledger",
            "Pilot",
            "Echo",
            "Theo",
            "Ines",
            "Kofi",
            "Ruth",
          ].filter((name) => visible.some((item) => item.assignee === name))
        : [...new Set(visible.map((item) => item.company))];
  const groupItems = (group: string) =>
    groupBy === "Status"
      ? visible.filter((item) => item.status === group)
      : visible
          .filter(
            (item) =>
              item[groupBy === "Assignee" ? "assignee" : "company"] === group,
          )
          .sort((a, b) => lanes.indexOf(a.status) - lanes.indexOf(b.status));
  return (
    <div className="workspace-page ws-work-page">
      <div className="ws-work-heading">
        <h1>
          {L(
            `${decisions.length} need your call`,
            `${decisions.length} 项等待你决定`,
          )}
        </h1>
        <div className="ws-work-meta">
          <p>
            {L(
              "22 delegations today: 17 with your crew, 5 with your team.",
              "今日 22 项委派：17 项由智能团队处理，5 项由同事处理。",
            )}
          </p>
          <span className="ws-summary-line">
            <span className="ws-live" />
            {L(
              "5 running now: 3 crew, 2 team",
              "目前 5 项进行中：智能团队 3 项，同事 2 项",
            )}
          </span>
        </div>
      </div>
      <WorkToolbar
        L={L}
        assignee={assignee}
        setAssignee={setAssignee}
        type={type}
        setType={setType}
        record={record}
        setRecord={setRecord}
        mode={mode}
        setWorkMode={setWorkMode}
      />
      {mode === "Board" ? (
        <WorkBoard
          lanes={lanes}
          labels={labels}
          visible={visible}
          L={L}
          navigate={navigate}
          setSelected={setSelected}
          onAction={onAction}
          onEdit={onEdit}
          visibleHistory={visibleHistory}
          exitingDecisions={exitingDecisions}
          language={language}
          onUndoDecision={onUndoDecision}
        />
      ) : (
        <WorkList
          L={L}
          groupBy={groupBy}
          changeGroup={changeGroup}
          listGroups={listGroups}
          groupItems={groupItems}
          labels={labels}
          listMeta={listMeta}
          setSelected={setSelected}
          navigate={navigate}
          onEdit={onEdit}
          onAction={onAction}
          visibleHistory={visibleHistory}
          exitingDecisions={exitingDecisions}
          language={language}
          onUndoDecision={onUndoDecision}
        />
      )}
      {selectedItem && (
        <WorkDetailDrawer
          setSelected={setSelected}
          L={L}
          selectedItem={selectedItem}
          navigate={navigate}
          labels={labels}
          onAction={onAction}
          setStatuses={setStatuses}
        />
      )}
    </div>
  );
}
