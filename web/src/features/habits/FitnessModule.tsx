import { useMemo, useState } from "react";
import { CalendarRange, Dumbbell, History } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useWorkoutLogs, useWorkoutPlans, type WorkoutPlan } from "./api";
import { describeItem, isoWeekday, weekPlans } from "./progress";
import {
  LogDialog,
  NoticeSettings,
  RecentLogs,
  toItem,
  WeekEditor,
  weekdayLabels,
} from "./workout";

const shortDays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function localKey(d: Date) {
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

/** 本周一到周日的日期，YYYY-MM-DD。 */
function thisWeek(now: Date) {
  const monday = new Date(now);
  monday.setDate(now.getDate() - isoWeekday(now) + 1);
  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(monday);
    d.setDate(monday.getDate() + i);
    return localKey(d);
  });
}

/**
 * 健身概况：本周七天的完成情况、今天的计划、记录训练。
 * 习惯页和今日页都用它，compact 时不显示底部的计划和记录按钮。
 */
export function FitnessSummary({ compact }: { compact?: boolean }) {
  const t = useT();
  const plans = useWorkoutPlans();
  const logs = useWorkoutLogs(7);
  const [logging, setLogging] = useState(false);
  const [planning, setPlanning] = useState(false);
  const [history, setHistory] = useState(false);

  const now = new Date();
  const weekday = isoWeekday(now);
  const week = useMemo(
    () =>
      weekPlans(
        (plans.data ?? []).map((p) => ({ ...p, items: p.items.map(toItem) })),
      ),
    [plans.data],
  );
  const days = thisWeek(now);
  const doneDays = new Set((logs.data ?? []).map((l) => l.date));

  if (plans.isPending) return <Loading />;
  if (plans.isError)
    return <ErrorState error={plans.error} onRetry={() => plans.refetch()} />;

  const today = week[weekday - 1];
  const planned = week.filter((d) => d.items.length > 0 || d.title).length;
  const done = days.filter((d) => doneDays.has(d)).length;
  const todayPlans = plans.data.filter((p) => p.weekday === weekday);

  return (
    <div className={`habits-fitness${compact ? " is-compact" : ""}`}>
      <div className="habits-fitness-head">
        <div className="habits-fitness-title">
          <strong>{today.title || t("Rest day")}</strong>
          <span>
            {t("This week")} {done}
            {planned > 0 && ` / ${planned}`} {t("times")}
          </span>
        </div>
        <button
          className="xc-btn small primary"
          onClick={() => setLogging(true)}
        >
          <Dumbbell size={14} /> {t("Log workout")}
        </button>
      </div>
      <div className="habits-fitness-week" role="list">
        {week.map((d, i) => {
          const isDone = doneDays.has(days[i]);
          const hasPlan = d.items.length > 0 || !!d.title;
          const state = isDone ? "done" : hasPlan ? "planned" : "rest";
          return (
            <div
              role="listitem"
              key={d.weekday}
              className={`habits-fitness-day is-${state}${d.weekday === weekday ? " is-today" : ""}`}
              title={`${t(weekdayLabels[i])}${d.title ? ` · ${d.title}` : ""}`}
            >
              <span className="habits-fitness-dot" />
              <small>{t(shortDays[i])}</small>
            </div>
          );
        })}
      </div>
      {today.items.length > 0 ? (
        <ul className="habits-fitness-items">
          {today.items.map((it, i) => (
            <li key={i}>{describeItem(it)}</li>
          ))}
        </ul>
      ) : (
        <p className="habits-fitness-empty">
          {t("No plan for today. Rest or log a free workout.")}
        </p>
      )}
      {!compact && (
        <div className="habits-fitness-foot">
          <button className="xc-btn small" onClick={() => setPlanning(true)}>
            <CalendarRange size={14} /> {t("Weekly plan")}
          </button>
          <button
            className="xc-btn small ghost"
            onClick={() => setHistory(true)}
          >
            <History size={14} /> {t("Workout history")}
          </button>
        </div>
      )}
      <LogDialog
        open={logging}
        onClose={() => setLogging(false)}
        planId={todayPlans.length === 1 ? todayPlans[0].id : undefined}
        items={today.items}
      />
      {!compact && (
        <>
          <PlanDialog
            open={planning}
            onClose={() => setPlanning(false)}
            plans={plans.data}
          />
          <Dialog
            open={history}
            onClose={() => setHistory(false)}
            title={t("Workout history")}
            wide
          >
            <RecentLogs />
          </Dialog>
        </>
      )}
    </div>
  );
}

function PlanDialog({
  open,
  onClose,
  plans,
}: {
  open: boolean;
  onClose: () => void;
  plans: WorkoutPlan[];
}) {
  const t = useT();
  return (
    <Dialog open={open} onClose={onClose} title={t("Weekly plan")} wide>
      <div className="xc-stack habits-plan-dialog">
        <WeekEditor plans={plans} />
        <NoticeSettings />
      </div>
    </Dialog>
  );
}

/** 记录训练的弹窗，自动带上今天计划里的动作。 */
export function TodayLogDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const plans = useWorkoutPlans();
  const weekday = isoWeekday(new Date());
  const today = useMemo(
    () =>
      weekPlans(
        (plans.data ?? []).map((p) => ({ ...p, items: p.items.map(toItem) })),
      )[weekday - 1],
    [plans.data, weekday],
  );
  const todayPlans = (plans.data ?? []).filter((p) => p.weekday === weekday);
  return (
    <LogDialog
      open={open}
      onClose={onClose}
      planId={todayPlans.length === 1 ? todayPlans[0].id : undefined}
      items={today.items}
    />
  );
}
