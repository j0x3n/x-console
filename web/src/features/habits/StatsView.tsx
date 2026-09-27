import { useState } from "react";
import { BarChart3 } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { useHabitList, useHabitStats, type Habit, type HabitDay } from "./api";
import { barMax, formatAmount, heatLevel, isoWeekday } from "./progress";
import { colorVar } from "./TodayView";

const DAYS = 30;

export default function StatsView() {
  const t = useT();
  const habits = useHabitList();
  const [selected, setSelected] = useState<number | null>(null);
  if (habits.isPending) return <Loading />;
  if (habits.isError)
    return <ErrorState error={habits.error} onRetry={() => habits.refetch()} />;
  if (habits.data.length === 0)
    return (
      <EmptyState title={t("No habits yet")} icon={<BarChart3 size={28} />} />
    );
  const habit = habits.data.find((h) => h.id === selected) ?? habits.data[0];
  return (
    <div className="xc-stack">
      <label className="xc-field habits-stats-select">
        <span>{t("Habit")}</span>
        <select
          className="xc-select"
          value={habit.id}
          onChange={(e) => setSelected(Number(e.target.value))}
        >
          {habits.data.map((h) => (
            <option key={h.id} value={h.id}>
              {h.icon ? `${h.icon} ` : ""}
              {h.name}
              {h.archived ? ` (${t("archived")})` : ""}
            </option>
          ))}
        </select>
      </label>
      <HabitStatsPanel habit={habit} />
    </div>
  );
}

function HabitStatsPanel({ habit }: { habit: Habit }) {
  const t = useT();
  const stats = useHabitStats(habit.id, DAYS);
  if (stats.isPending) return <Loading />;
  if (stats.isError)
    return <ErrorState error={stats.error} onRetry={() => stats.refetch()} />;
  const s = stats.data;
  const color = colorVar(habit.color);
  return (
    <>
      <div className="habits-tiles">
        <Tile
          label={t("Current streak")}
          value={`${s.streak}`}
          unit={t("days")}
        />
        <Tile
          label={t("Best in 30 days")}
          value={`${s.bestStreak}`}
          unit={t("days")}
        />
        <Tile
          label={t("Days reached")}
          value={`${s.reachedDays}`}
          unit={`/ ${DAYS}`}
        />
        <Tile
          label={t("Total")}
          value={formatAmount(s.total)}
          unit={habit.unit}
        />
      </div>
      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Last 30 days")}</h2>
          <span className="xc-muted habits-legend">
            {t("Less")}
            {[0, 1, 2, 3, 4].map((l) => (
              <i
                key={l}
                className={`habits-heat l${l}`}
                style={{ ["--habit-color" as string]: color }}
              />
            ))}
            {t("Goal")}
          </span>
        </div>
        <Heatmap
          days={s.days}
          target={habit.dailyTarget}
          unit={habit.unit}
          color={color}
        />
      </div>
      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Per day")}</h2>
          <span className="xc-muted">
            {t("Goal")} {formatAmount(habit.dailyTarget)} {habit.unit}
          </span>
        </div>
        <Bars
          days={s.days}
          target={habit.dailyTarget}
          unit={habit.unit}
          color={color}
        />
      </div>
    </>
  );
}

function Tile({
  label,
  value,
  unit,
}: {
  label: string;
  value: string;
  unit: string;
}) {
  return (
    <div className="xc-card habits-tile">
      <span className="xc-muted">{label}</span>
      <strong>
        {value} <small>{unit}</small>
      </strong>
    </div>
  );
}

function parseDate(key: string): Date {
  const [y, m, d] = key.split("-").map(Number);
  return new Date(y, m - 1, d);
}

/** 按周排列的热力图：每列一周，从上到下是周一到周日。 */
function Heatmap({
  days,
  target,
  unit,
  color,
}: {
  days: HabitDay[];
  target: number;
  unit: string;
  color: string;
}) {
  const language = useLanguage();
  if (days.length === 0) return null;
  const lead = isoWeekday(parseDate(days[0].date)) - 1;
  const cells: (HabitDay | null)[] = [...Array<null>(lead).fill(null), ...days];
  const weekdayNames =
    language === "zh"
      ? ["一", "", "三", "", "五", "", "日"]
      : ["M", "", "W", "", "F", "", "S"];
  return (
    <div className="habits-heatmap-wrap">
      <div className="habits-heatmap-days">
        {weekdayNames.map((n, i) => (
          <span key={i}>{n}</span>
        ))}
      </div>
      <div
        className="habits-heatmap"
        style={{ ["--habit-color" as string]: color }}
      >
        {cells.map((d, i) =>
          d ? (
            <i
              key={d.date}
              className={`habits-heat l${heatLevel(d.amount, target)}`}
              title={`${d.date} · ${formatAmount(d.amount)} ${unit}`}
            />
          ) : (
            <i key={`pad-${i}`} className="habits-heat pad" />
          ),
        )}
      </div>
    </div>
  );
}

function Bars({
  days,
  target,
  unit,
  color,
}: {
  days: HabitDay[];
  target: number;
  unit: string;
  color: string;
}) {
  const max = barMax(
    days.map((d) => d.amount),
    target,
  );
  const w = 10;
  const h = 100;
  const targetY = h - (target / max) * h;
  return (
    <div className="habits-bars">
      <svg
        viewBox={`0 0 ${days.length * w} ${h}`}
        preserveAspectRatio="none"
        role="img"
        aria-label="daily amounts"
      >
        {days.map((d, i) => {
          const bh = (d.amount / max) * h;
          return (
            <rect
              key={d.date}
              x={i * w + 1.5}
              y={h - bh}
              width={w - 3}
              height={bh}
              rx={1.5}
              fill={d.reached ? color : "var(--xc-border-strong)"}
            >
              <title>{`${d.date} · ${formatAmount(d.amount)} ${unit}`}</title>
            </rect>
          );
        })}
        <line
          x1={0}
          x2={days.length * w}
          y1={targetY}
          y2={targetY}
          stroke="var(--xc-muted)"
          strokeDasharray="3 3"
          strokeWidth={0.8}
          vectorEffect="non-scaling-stroke"
        />
      </svg>
      <div className="habits-bars-axis">
        <span>{days[0]?.date.slice(5)}</span>
        <span>{days[days.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  );
}
