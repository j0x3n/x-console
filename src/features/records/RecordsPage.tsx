import type * as Model from "../../types/domain";
import React, { useState, useMemo } from "react";
import {
  peopleRecords,
  companyRecords,
  dealRecords,
} from "../../data/workspace";
import { matchesSavedView } from "./record-config";
import { agentMarks } from "../../data/catalogs";
import RecordRow from "./components/RecordRow";
import RecordsToolbar from "./components/RecordsToolbar";
import { Plus } from "lucide-react";
import RecordsTable from "./components/RecordsTable";
import RecordsSummary from "./components/RecordsSummary";

interface RecordsPageProps {
  kind: string;
  L: Model.Localize;
  language: Model.Language;
  navigate: Model.Navigate;
  openDelegate: Model.OpenDelegate;
  outcomes: Model.DecisionOutcomes;
}

export default function RecordsPage({
  kind,
  L,
  language,
  navigate,
  openDelegate,
  outcomes,
}: RecordsPageProps) {
  const isPeople = kind === "People";
  const rows: Model.RecordData[] = isPeople
    ? peopleRecords
    : companyRecords.map((row) =>
        row.name === "Brightwell Labs" && outcomes[3] === "approved"
          ? { ...row, stage: "Negotiation" }
          : row,
      );
  const [tab, setTab] = useState("All");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("All");
  const [sort, setSort] = useState("Last touch");
  const [group, setGroup] = useState("None");
  const [selected, setSelected] = useState<string[]>([]);
  const tabs = isPeople
    ? [
        ["All", L("All people", "全部联系人")],
        ["Champions", L("Champions", "关键支持者")],
        ["Committees", L("Buying committees", "采购决策组")],
        ["Cold", L("Going cold", "逐渐冷淡")],
      ]
    : [
        ["All", L("All companies", "全部公司")],
        ["Mine", L("My book", "我负责的")],
        ["Risk", L("At risk", "有风险")],
        ["Expansion", L("Expansion", "扩容机会")],
      ];
  const filtered = useMemo(
    () =>
      rows
        .filter((row) => {
          const company = isPeople
            ? companyRecords.find((item) => item.name === row.company)
            : row;
          const text =
            `${row.name} ${isPeople ? (row.company ?? "") : row.signal} ${row.role || ""}`.toLowerCase();
          if (!text.includes(query.toLowerCase())) return false;
          if (isPeople) {
            if (!matchesSavedView(row, tab, true)) return false;
            if (filter !== "All" && row.warmth !== filter) return false;
          } else {
            if (!matchesSavedView(row, tab, false)) return false;
            if (filter !== "All" && row.stage !== filter) return false;
          }
          return !!company;
        })
        .sort((a, b) =>
          sort === "Name"
            ? a.name.localeCompare(b.name)
            : sort === "Health"
              ? (b.health || 0) - (a.health || 0)
              : sort === "Value"
                ? (b.value || 0) - (a.value || 0)
                : 0,
        ),
    [rows, isPeople, query, tab, filter, sort],
  );
  const visibleCompanies = new Set(
    filtered.map((row) => (isPeople ? (row.company ?? "") : row.name)),
  );
  const visibleDeals = dealRecords.filter((deal) =>
    visibleCompanies.has(deal.company),
  ).length;
  const pipelineValue = filtered.reduce(
    (sum, row) => sum + (row.value || 0),
    0,
  );
  const pipelineLabel =
    pipelineValue >= 1000
      ? `$${(pipelineValue / 1000).toFixed(2)}M`
      : `$${pipelineValue}K`;
  const averageHealth = filtered.length
    ? Math.round(
        filtered.reduce((sum, row) => sum + (row.health || 0), 0) /
          filtered.length,
      )
    : 0;
  const crewTouched = filtered.filter((row) =>
    Boolean(agentMarks[row.agent ?? ""]),
  ).length;
  const warmPeople = filtered.filter((row) => row.warmth === "Warm").length;
  const crewKnown = filtered.filter((row) =>
    Boolean(agentMarks[row.knownBy ?? ""]),
  ).length;
  const allSelected =
    filtered.length > 0 && filtered.every((row) => selected.includes(row.name));
  const tableHeaders = isPeople
    ? [
        L("Person", "联系人"),
        L("Company", "公司"),
        L("Deal role", "商机角色"),
        L("Warmth", "联系热度"),
        L("Known by", "熟悉该联系人"),
        L("Last touch", "最近联系"),
        L("Next step", "下一步"),
      ]
    : [
        L("Company", "公司"),
        L("Stage", "阶段"),
        L("Owner", "负责人"),
        L("Value", "金额"),
        L("Health", "健康度"),
        L("Last touch", "最近动态"),
        L("Next step", "下一步"),
        L("Signals", "信号"),
        L("Segment", "客户类型"),
        "",
      ];
  const renderRow = (row: Model.RecordData) => {
    const company = isPeople
      ? companyRecords.find((item) => item.name === row.company)
      : row;
    return (
      <RecordRow
        key={row.name}
        row={row}
        navigate={navigate}
        L={L}
        selected={selected}
        setSelected={setSelected}
        isPeople={isPeople}
        company={company}
        openDelegate={openDelegate}
      />
    );
  };
  const groups: [string | null, Model.RecordData[]][] =
    group === "None"
      ? [[null, filtered]]
      : [
          ...new Set(
            filtered.map((row) =>
              group === "Stage"
                ? isPeople
                  ? (row.role ?? "")
                  : (row.stage ?? "")
                : isPeople
                  ? (row.company ?? "")
                  : (row.owner ?? ""),
            ),
          ),
        ].map((key) => [
          key,
          filtered.filter(
            (row) =>
              (group === "Stage"
                ? isPeople
                  ? (row.role ?? "")
                  : (row.stage ?? "")
                : isPeople
                  ? (row.company ?? "")
                  : (row.owner ?? "")) === key,
          ),
        ]);
  return (
    <div
      className={`workspace-page ws-records-page ${isPeople ? "ws-people-page" : "ws-companies-page"}`}
    >
      <h1 className="ws-screen-reader-only">
        {L(kind, isPeople ? "联系人" : "公司")}
      </h1>
      <div className="ws-records-topline">
        <div
          className="ws-tabs"
          role="tablist"
          aria-label={L("Saved views", "已保存视图")}
        >
          {tabs.map(([key, label]) => (
            <button
              key={key}
              role="tab"
              aria-selected={tab === key}
              className={tab === key ? "active" : ""}
              onClick={() => {
                setTab(key);
                setSelected([]);
              }}
            >
              {label}
              <span>
                {
                  rows.filter((row) => matchesSavedView(row, key, isPeople))
                    .length
                }
              </span>
            </button>
          ))}
        </div>
        <div className="ws-records-note">
          <span className="ws-live" />
          {isPeople
            ? L(
                "7 known best by your crew · 12 wrote to you this week",
                "7 位由智能团队最熟悉 · 本周 12 位给你发来消息",
              )
            : L(
                "18 of 24 last touched by your crew · updated 2m ago",
                "24 家公司中 18 家最近由智能团队更新 · 2 分钟前同步",
              )}
        </div>
      </div>
      <RecordsToolbar
        L={L}
        kind={kind}
        isPeople={isPeople}
        query={query}
        setQuery={setQuery}
        filter={filter}
        setFilter={setFilter}
        sort={sort}
        setSort={setSort}
        group={group}
        setGroup={setGroup}
        tableHeaders={tableHeaders}
        filtered={filtered}
      />
      {selected.length > 0 && (
        <div className="ws-selection-bar">
          <b>
            {selected.length} {L("selected", "项已选")}
          </b>
          <button onClick={openDelegate}>
            <Plus size={13} />
            {L("Delegate", "委派")}
          </button>
          <button onClick={() => setSelected([])}>{L("Clear", "清除")}</button>
        </div>
      )}
      <RecordsTable
        L={L}
        allSelected={allSelected}
        setSelected={setSelected}
        filtered={filtered}
        tableHeaders={tableHeaders}
        groups={groups}
        renderRow={renderRow}
      />
      <RecordsSummary
        filtered={filtered}
        L={L}
        isPeople={isPeople}
        visibleCompanies={visibleCompanies}
        visibleDeals={visibleDeals}
        warmPeople={warmPeople}
        pipelineLabel={pipelineLabel}
        crewKnown={crewKnown}
        averageHealth={averageHealth}
        crewTouched={crewTouched}
      />
    </div>
  );
}
