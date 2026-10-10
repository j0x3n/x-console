import { useState } from "react";
import { EmptyState } from "../../components/ui/States";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { bodySeries, type BodyMetric, type BodyStats } from "./personal";
import type { PersonalDay } from "./personalApi";

const RANGES = [30, 90, 180] as const;

const META: Record<
  BodyMetric,
  { label: string; unit: string; digits: number; bars?: boolean }
> = {
  weight: { label: "Body weight", unit: "kg", digits: 1 },
  sleep: { label: "Sleep", unit: "h", digits: 1 },
  restingHr: { label: "Resting heart rate", unit: "bpm", digits: 0 },
  steps: { label: "Steps", unit: "", digits: 0, bars: true },
};

const num = (v: number | null, digits: number) =>
  v === null
    ? "—"
    : v.toLocaleString("en-US", { maximumFractionDigits: digits });

const signed = (v: number | null, digits: number) =>
  v === null
    ? "—"
    : `${v > 0 ? "+" : ""}${v.toLocaleString("en-US", { maximumFractionDigits: digits })}`;

/** B119：体重、睡眠、静息心率、步数的趋势。结束日是页面选中的那天。 */
export default function BodyTrend({
  days,
  end,
}: {
  days: PersonalDay[];
  end: string;
}) {
  const t = useT();
  const [metric, setMetric] = useState<BodyMetric>("weight");
  const [range, setRange] = useState<number>(30);
  const meta = META[metric];
  const stats = bodySeries(days, metric, range, end);
  return (
    <section className="xc-card habits-body-trend">
      <div className="xc-card-head">
        <h2>{t("Body trend")}</h2>
      </div>
      <div className="habits-body-controls">
        <Segmented<BodyMetric>
          label={t("Body metric")}
          value={metric}
          onChange={setMetric}
          options={(Object.keys(META) as BodyMetric[]).map((value) => ({
            value,
            label: t(META[value].label),
          }))}
        />
        <Segmented<string>
          label={t("Time range")}
          value={String(range)}
          onChange={(v) => setRange(Number(v))}
          options={RANGES.map((n) => ({
            value: String(n),
            label: t(`${n} days`),
          }))}
        />
      </div>
      {stats.points.length >= 2 ? (
        <>
          <TrendChart
            stats={stats}
            metric={metric}
            range={range}
            end={end}
            title={t(meta.label)}
          />
          <dl className="habits-body-stats">
            {(
              [
                ["Latest", num(stats.latest, meta.digits)],
                ["Average", num(stats.average, meta.digits)],
                ["Lowest", num(stats.min, meta.digits)],
                ["Highest", num(stats.max, meta.digits)],
                ["Range change", signed(stats.change, meta.digits)],
              ] as const
            ).map(([label, value]) => (
              <div key={label}>
                <dt>{t(label)}</dt>
                <dd>
                  {value}
                  {meta.unit && value !== "—" ? ` ${meta.unit}` : ""}
                </dd>
              </div>
            ))}
          </dl>
        </>
      ) : (
        <EmptyState title={t("Record at least two days to see the trend")} />
      )}
      <p className="xc-muted">
        {t(
          "Only recorded values are used. Missing days are not filled with zero.",
        )}
      </p>
    </section>
  );
}

const W = 580,
  H = 155,
  LEFT = 40,
  RIGHT = 540,
  TOP = 20,
  BOTTOM = 120;

function TrendChart({
  stats,
  metric,
  range,
  end,
  title,
}: {
  stats: BodyStats;
  metric: BodyMetric;
  range: number;
  end: string;
  title: string;
}) {
  const meta = META[metric];
  const endMs = Date.parse(`${end}T12:00:00Z`);
  const startMs = endMs - (range - 1) * 86400000;
  const x = (date: string) =>
    LEFT +
    ((Date.parse(`${date}T12:00:00Z`) - startMs) / (endMs - startMs)) *
      (RIGHT - LEFT);
  let lo = stats.min!,
    hi = stats.max!;
  if (meta.bars) lo = 0;
  else {
    const pad = (hi - lo || Math.abs(hi) * 0.02 || 1) * 0.15;
    lo -= pad;
    hi += pad;
  }
  if (hi === lo) hi = lo + 1;
  const y = (v: number) => BOTTOM - ((v - lo) / (hi - lo)) * (BOTTOM - TOP);
  const label = (v: number) => num(v, meta.digits);
  const start = new Date(startMs).toISOString().slice(5, 10);
  return (
    <svg
      className="habits-weight-chart"
      viewBox={`0 0 ${W} ${H}`}
      role="img"
      aria-label={title}
    >
      <path
        d={`M${LEFT} ${TOP - 5}V${BOTTOM}H${RIGHT + 5}`}
        fill="none"
        stroke="var(--xc-border)"
      />
      {meta.bars ? (
        stats.points.map((p) => (
          <rect
            key={p.date}
            x={x(p.date) - Math.max(1.5, (RIGHT - LEFT) / range / 2.5)}
            y={y(p.value)}
            width={Math.max(3, (RIGHT - LEFT) / range / 1.25)}
            height={BOTTOM - y(p.value)}
            fill="var(--xc-accent)"
          >
            <title>
              {p.date}：{label(p.value)}
            </title>
          </rect>
        ))
      ) : (
        <>
          <polyline
            points={stats.points
              .map((p) => `${x(p.date)},${y(p.value)}`)
              .join(" ")}
            fill="none"
            stroke="var(--xc-accent)"
            strokeWidth="2"
          />
          {stats.points.map((p) => (
            <circle
              key={p.date}
              cx={x(p.date)}
              cy={y(p.value)}
              r="3"
              fill="var(--xc-accent)"
            >
              <title>
                {p.date}：{label(p.value)} {meta.unit}
              </title>
            </circle>
          ))}
        </>
      )}
      <text x={LEFT} y="149">
        {start}
      </text>
      <text x={RIGHT - 28} y="149">
        {end.slice(5)}
      </text>
      <text x="0" y={TOP + 5}>
        {label(hi)}
      </text>
      <text x="0" y={BOTTOM + 3}>
        {label(lo)}
      </text>
    </svg>
  );
}
