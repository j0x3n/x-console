import { NavLink, useParams, useSearchParams } from "react-router";
import { Plus } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { Ring, Segments, StatCard, StatStrip } from "../../components/ui/Stat";
import { useT } from "../../contexts/LanguageContext";
import { useHabitsToday } from "./api";
import { formatAmount, ratio } from "./progress";
import StatsView from "./StatsView";
import TodayView from "./TodayView";
import { TodayLogDialog } from "./FitnessModule";

const tabs = [
  { id: "", label: "Today", to: "/habits" },
  { id: "stats", label: "Stats", to: "/habits/stats" },
];

export default function HabitsPage() {
  const t = useT();
  const { tab = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const current = tabs.find((x) => x.id === tab) ?? tabs[0];
  const creating = params.get("new") === "1";
  const logging = params.get("log") === "workout";
  const closeLog = () => {
    const next = new URLSearchParams(params);
    next.delete("log");
    setParams(next, { replace: true });
  };
  const setCreating = (open: boolean) => {
    const next = new URLSearchParams(params);
    if (open) next.set("new", "1");
    else next.delete("new");
    setParams(next, { replace: true });
  };
  const today = useHabitsToday();
  const items = today.data ?? [];
  const reached = items.filter((h) => h.reached).length;
  const progress = items.length
    ? items.reduce(
        (sum, h) => sum + ratio(h.done, h.habit.dailyTarget ?? 0),
        0,
      ) / items.length
    : 0;
  const bestStreak = items.reduce((best, h) => Math.max(best, h.streak), 0);
  const checkins = items.reduce((sum, h) => sum + h.logs.length, 0);
  const behind = items
    .filter((h) => !h.reached)
    .sort(
      (a, b) =>
        ratio(a.done, a.habit.dailyTarget ?? 0) -
        ratio(b.done, b.habit.dailyTarget ?? 0),
    )[0];
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Habits")}
        subtitle={
          items.length ? (
            <>
              {t("Today")} <strong>{reached}</strong> / {items.length}{" "}
              {t("habits reached")}
            </>
          ) : undefined
        }
        aside={
          current.id === "" && (
            <button
              className="xc-btn primary"
              onClick={() => setCreating(true)}
            >
              <Plus size={15} /> {t("New habit")}
            </button>
          )
        }
      />
      {items.length > 0 && (
        <StatStrip label={t("Habits")}>
          <StatCard
            label={t("Today's progress")}
            caption={`${reached}/${items.length}`}
          >
            <div className="habits-stat-ring">
              <Ring
                value={progress}
                max={1}
                size={56}
                stroke={6}
                tone={progress >= 1 ? "ok" : "accent"}
              >
                {Math.round(progress * 100)}%
              </Ring>
              <span>
                {reached === items.length
                  ? t("All reached")
                  : `${items.length - reached} ${t("to go")}`}
              </span>
            </div>
          </StatCard>
          <StatCard
            label={t("Best streak")}
            value={bestStreak}
            unit={t("days")}
            foot={t("Days in a row, counting today")}
          />
          <StatCard
            label={t("Check-ins today")}
            value={checkins}
            foot={t("All habits")}
          >
            <Segments
              parts={items.map((h) => ({
                value: h.logs.length,
                tone: h.reached ? "ok" : "accent",
                label: h.habit.name,
              }))}
            />
          </StatCard>
          <StatCard
            label={t("Needs attention")}
            value={behind ? behind.habit.name : t("None")}
            foot={
              behind
                ? `${formatAmount(behind.done)} / ${formatAmount(behind.habit.dailyTarget ?? 0)} ${behind.habit.unit ?? ""}`
                : t("All reached")
            }
          />
        </StatStrip>
      )}
      <nav className="xc-tabs">
        {tabs.map((item) => (
          <NavLink
            key={item.id}
            to={item.to}
            end
            className={item.id === current.id ? "active" : ""}
          >
            {t(item.label)}
          </NavLink>
        ))}
      </nav>
      {current.id === "" && (
        <TodayView
          creating={creating}
          onCloseCreate={() => setCreating(false)}
        />
      )}
      {current.id === "stats" && <StatsView />}
      {logging && <TodayLogDialog open onClose={closeLog} />}
    </div>
  );
}
