import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Plus,
  Target,
} from "lucide-react";
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
import { useT } from "../../contexts/LanguageContext";
import { useGoals, type Goal, type GoalPeriod } from "./api";
import GoalCard from "./GoalCard";
import GoalDialog from "./GoalDialog";
import GoalTabs from "./GoalTabs";
import {
  currentPeriod,
  fmtPercent,
  quarterLabel,
  samePeriod,
  shiftPeriod,
  summarize,
} from "./format";
import "./i18n";
import "./goals.css";

type Filter = "active" | "all";

/** 目标（B128）：季度目标和关键结果，进度和按时间应该到哪的对比。 */
export default function GoalsPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const today = currentPeriod();
  const [period, setPeriod] = useState<GoalPeriod>(today);
  const [filter, setFilter] = useState<Filter>("all");
  const [editing, setEditing] = useState<Goal | null>(null);
  const list = useGoals(period);

  const creating = params.get("new") === "1";
  const closeDialog = () => {
    setEditing(null);
    if (creating)
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete("new");
          return next;
        },
        { replace: true },
      );
  };
  const openNew = () =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set("new", "1");
        return next;
      },
      { replace: true },
    );

  const all = list.data?.items ?? [];
  const shown =
    filter === "active" ? all.filter((g) => g.status === "active") : all;
  const sum = summarize(all);
  const expected = all[0]?.expected;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Goals")} icon={<Target size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (all.length === 0)
    body = (
      <EmptyState
        title={t("No goals in this quarter")}
        icon={<Target size={28} />}
      >
        <span>
          写下这个季度想做到的事，再给每个目标定几个可以查数的关键结果。能从习惯、专注、电脑时间和
          GitHub 取数的，进度会自动算。
        </span>
        <button className="xc-btn primary small" onClick={openNew}>
          <Plus size={14} /> {t("New goal")}
        </button>
      </EmptyState>
    );
  else if (shown.length === 0)
    body = (
      <EmptyState title={t("No active goals")} icon={<Target size={28} />} />
    );
  else
    body = (
      <div className="goals-grid">
        {shown.map((g) => (
          <GoalCard key={g.id} goal={g} onEdit={() => setEditing(g)} />
        ))}
      </div>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Goals")}
        subtitle={
          list.data
            ? sum.behind > 0
              ? `${quarterLabel(period)} · ${sum.behind} ${t("goals are behind")}`
              : quarterLabel(period)
            : undefined
        }
        aside={
          <>
            <Link
              className="xc-btn"
              to="/goals/review"
              title={t("Weekly review")}
            >
              <ClipboardCheck size={15} /> {t("Weekly review")}
            </Link>
            <button
              className="xc-btn primary"
              title={t("New goal")}
              onClick={openNew}
            >
              <Plus size={15} /> {t("New goal")}
            </button>
          </>
        }
      />
      <StatStrip label={t("Goals")}>
        <StatCard
          label={t("Active goals")}
          value={list.data ? sum.active : "–"}
        />
        <StatCard
          label={t("Average progress")}
          value={list.data ? fmtPercent(sum.average) : "–"}
          tone="accent"
        />
        <StatCard
          label={t("Behind")}
          value={list.data ? sum.behind : "–"}
          tone={sum.behind > 0 ? "warn" : undefined}
        />
        <StatCard
          label={t("Quarter gone")}
          value={expected == null ? "–" : fmtPercent(expected)}
          foot={t("Where progress should be if it grows evenly")}
        />
      </StatStrip>
      <Toolbar
        start={
          <>
            <GoalTabs active="goals" />
            <div className="goals-quarter">
              <button
                type="button"
                className="xc-btn ghost small"
                aria-label={t("Previous quarter")}
                title={t("Previous quarter")}
                onClick={() => setPeriod(shiftPeriod(period, -1))}
              >
                <ChevronLeft size={15} />
              </button>
              <strong>{quarterLabel(period)}</strong>
              <button
                type="button"
                className="xc-btn ghost small"
                aria-label={t("Next quarter")}
                title={t("Next quarter")}
                onClick={() => setPeriod(shiftPeriod(period, 1))}
              >
                <ChevronRight size={15} />
              </button>
              {!samePeriod(period, today) && (
                <button
                  type="button"
                  className="xc-btn small"
                  onClick={() => setPeriod(today)}
                >
                  {t("This quarter")}
                </button>
              )}
            </div>
          </>
        }
        end={
          <Segmented<Filter>
            label={t("Show")}
            value={filter}
            onChange={setFilter}
            options={[
              { value: "all", label: t("All goals") },
              { value: "active", label: t("Active only") },
            ]}
          />
        }
      />
      {body}
      <p className="goals-footnote">
        记下进度、再把“什么时候做什么”写具体，通常有助于把事做成。相关研究多数来自健康类目标，工作和学习上的效果因人而异。
      </p>
      <GoalDialog
        open={creating || editing != null}
        onClose={closeDialog}
        goal={editing}
        period={period}
      />
    </div>
  );
}
