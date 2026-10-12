import { useSearchParams } from "react-router";
import { ChevronLeft, ChevronRight, Download, Scale } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { localToday, rangeLabel, shiftRange } from "../screentime/format";
import {
  roiCsvUrl,
  useRoiReport,
  type RoiGroup,
  type RoiGroupBy,
  type RoiPeriod,
  type RoiReport,
} from "./api";
import GroupTable from "./GroupTable";
import TaskTable from "./TaskTable";
import {
  minutesText,
  moneyText,
  percentText,
  secondsText,
  tokensText,
} from "./format";
import "./i18n";
import "./codingroi.css";

const PERIODS: RoiPeriod[] = ["week", "month"];
const GROUPS: RoiGroupBy[] = ["repo", "tool", "agent"];
const GROUP_LABELS: Record<RoiGroupBy, string> = {
  repo: "Repository",
  tool: "Coding tool",
  agent: "Agents",
};
const PERIOD_LABELS: Record<RoiPeriod, string> = {
  week: "Week",
  month: "Month",
};

/**
 * AI 编码投入产出对账（B129）：一周或一个月里每个编码任务花了多少、AI 跑了多久、
 * 你审查花了多久、最后有没有合并。没有记录的格子写“无记录”，不当成 0。
 */
export default function RoiPage() {
  const t = useT();
  const language = useLanguage();
  const [params, setParams] = useSearchParams();
  const periodParam = params.get("period");
  const period: RoiPeriod = PERIODS.includes(periodParam as RoiPeriod)
    ? (periodParam as RoiPeriod)
    : "week";
  const groupParam = params.get("group");
  const groupBy: RoiGroupBy = GROUPS.includes(groupParam as RoiGroupBy)
    ? (groupParam as RoiGroupBy)
    : "repo";
  const date = params.get("date") ?? "";
  const set = (patch: Record<string, string>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );

  const q = useRoiReport(period, date, groupBy);
  const r = q.data;
  const atEnd = r ? r.to >= localToday() : true;

  let body;
  if (q.isPending) body = <Loading />;
  else if (q.isError && isNotLive(q.error))
    body = <NotLive name={t("Return report")} icon={<Scale size={28} />} />;
  else if (q.isError)
    body = <ErrorState error={q.error} onRetry={() => q.refetch()} />;
  else if (q.data.tasks.length === 0)
    body = (
      <EmptyState
        title={t("No coding tasks in this period")}
        icon={<Scale size={28} />}
      >
        <p>{t("Tasks are counted by the day they were created.")}</p>
      </EmptyState>
    );
  else body = <Body report={q.data} groupBy={groupBy} />;

  return (
    <div className="xc-page roi-page">
      <PageHeading
        title={t("Return report")}
        subtitle={
          r && r.tasks.length > 0
            ? `${r.totals.tasks} ${t("Tasks")} · ${r.totals.merged} ${t("Merged")}`
            : undefined
        }
        aside={
          <a
            className="xc-btn"
            href={roiCsvUrl(period, date, groupBy)}
            download
            title={t("Export the tasks below as CSV")}
          >
            <Download size={15} /> {t("Export CSV")}
          </a>
        }
      />
      <p className="roi-intro">
        {t(
          "Whether AI pays off can only be judged from your own data. Research results differ from person to person.",
        )}
      </p>
      {r && r.tasks.length > 0 && <Totals totals={r.totals} />}
      <Toolbar
        start={
          <div className="roi-nav">
            <Segmented
              label={t("Return period")}
              value={period}
              options={PERIODS.map((p) => ({
                value: p,
                label: t(PERIOD_LABELS[p]),
              }))}
              onChange={(p) => set({ period: p === "week" ? "" : p, date: "" })}
            />
            <div className="roi-pager">
              <button
                className="xc-btn small icon"
                aria-label={t("Previous period")}
                title={t("Previous period")}
                disabled={!r}
                onClick={() =>
                  r && set({ date: shiftRange(r.from, period, -1) })
                }
              >
                <ChevronLeft size={15} />
              </button>
              <span className="roi-range">
                {r ? rangeLabel(period, r.from, r.to, language) : "…"}
              </span>
              <button
                className="xc-btn small icon"
                aria-label={t("Next period")}
                title={t("Next period")}
                disabled={!r || atEnd}
                onClick={() =>
                  r && set({ date: shiftRange(r.from, period, 1) })
                }
              >
                <ChevronRight size={15} />
              </button>
            </div>
          </div>
        }
        end={
          <Segmented
            label={t("Return group by")}
            value={groupBy}
            options={GROUPS.map((g) => ({
              value: g,
              label: t(GROUP_LABELS[g]),
            }))}
            onChange={(g) => set({ group: g === "repo" ? "" : g })}
          />
        }
      />
      {body}
    </div>
  );
}

/** 概要卡片：数字只加有记录的，缺的任务数写在下面。 */
function Totals({ totals: g }: { totals: RoiGroup }) {
  const t = useT();
  const missing = (n: number) =>
    n > 0 ? t("{n} without a record").replace("{n}", String(n)) : undefined;
  const dash = (text: string | null) => text ?? "–";
  const tokens =
    g.inputTokens == null
      ? null
      : tokensText(g.inputTokens + (g.outputTokens ?? 0));
  return (
    <StatStrip label={t("Return report")}>
      <StatCard
        label={t("Tasks")}
        value={g.tasks}
        foot={
          [
            g.waiting > 0 && `${g.waiting} ${t("Waiting to merge")}`,
            g.active > 0 && `${g.active} ${t("In progress")}`,
          ]
            .filter(Boolean)
            .join(" · ") || undefined
        }
      />
      <StatCard
        label={t("Merged")}
        value={g.merged}
        tone={g.merged > 0 ? "ok" : undefined}
        foot={
          g.mergeRate != null
            ? `${t("Merge rate")} ${percentText(g.mergeRate)}`
            : undefined
        }
      />
      <StatCard
        label={t("Cost")}
        value={dash(moneyText(g.cost))}
        foot={
          [tokens && `${tokens} ${t("Tokens")}`, missing(g.costMissing)]
            .filter(Boolean)
            .join(" · ") || undefined
        }
      />
      <StatCard
        label={t("AI run time")}
        value={dash(secondsText(t, g.runSeconds))}
        foot={missing(g.runMissing)}
      />
      <StatCard
        label={t("Review time (estimate)")}
        value={dash(minutesText(t, g.reviewMinutes))}
        foot={missing(g.reviewMissing)}
      />
    </StatStrip>
  );
}

function Body({ report, groupBy }: { report: RoiReport; groupBy: RoiGroupBy }) {
  const t = useT();
  return (
    <div className="roi-body">
      {!report.screenAvailable && (
        <p className="roi-note">
          {t("Screen time has no record, so the review time shows no record.")}
        </p>
      )}
      <GroupTable
        groups={report.groups}
        totals={report.totals}
        groupBy={groupBy}
      />
      <TaskTable tasks={report.tasks} />
      <p className="roi-note">
        {t(
          "Review time is an estimate. It counts the coding minutes between the end of a task and its merge or discard. Some of them may have nothing to do with the task.",
        )}
      </p>
    </div>
  );
}
