import { useEffect, useMemo, useRef, useState } from "react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatTime } from "../../../lib/time";
import type { MetricsPoint } from "../api";
import { niceMax, splitSegments } from "../lib";

export interface ChartSeries {
  key: keyof MetricsPoint;
  label: string;
  /** CSS 颜色，用模块里的 --servers-series-* 变量 */
  color: string;
}

interface MetricChartProps {
  title: string;
  points: MetricsPoint[];
  stepSeconds: number;
  series: ChartSeries[];
  /** 纵轴固定上限，比如百分比用 100；不给就按数据取整 */
  max?: number;
  /** 按数据取整的方法，默认十进制；字节速率用 niceRateMax */
  nice?: (value: number) => number;
  format: (value: number) => string;
  /** 标题右边显示的当前值 */
  current?: string;
}

const HEIGHT = 150;
const PAD = { top: 8, right: 8, bottom: 20, left: 44 };

/** 细线曲线图：一根纵轴，悬停显示十字线和数值，离线的时段断开。 */
export default function MetricChart({
  title,
  points,
  stepSeconds,
  series,
  max,
  nice = (v) => niceMax(v, 1),
  format,
  current,
}: MetricChartProps) {
  const t = useT();
  const language = useLanguage();
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [hover, setHover] = useState<number | null>(null);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const times = useMemo(() => points.map((p) => new Date(p.at).getTime()), [points]);
  const top = useMemo(() => {
    if (max !== undefined) return max;
    let m = 0;
    for (const p of points) for (const s of series) m = Math.max(m, Number(p[s.key]));
    return nice(m * 1.1);
  }, [points, series, max, nice]);

  const ticks = [0, top / 2, top];
  // 纵轴留白按最长的刻度文字算，网速这类长数字不会被截掉。
  const left = Math.max(PAD.left, Math.max(...ticks.map((v) => format(v).length)) * 6.5 + 10);
  const plotW = Math.max(0, width - left - PAD.right);
  const plotH = HEIGHT - PAD.top - PAD.bottom;
  const t0 = times[0] ?? 0;
  const t1 = times[times.length - 1] ?? 1;
  const span = Math.max(t1 - t0, 1);
  const x = (time: number) => left + ((time - t0) / span) * plotW;
  const y = (v: number) => PAD.top + plotH - (Math.min(v, top) / top) * plotH;

  const segments = useMemo(() => splitSegments(points, stepSeconds), [points, stepSeconds]);
  const paths = series.map((s) =>
    segments
      .map((seg) =>
        seg
          .map((p, i) => `${i ? "L" : "M"}${x(new Date(p.at).getTime()).toFixed(1)},${y(Number(p[s.key])).toFixed(1)}`)
          .join(""),
      )
      .join(""),
  );

  const onMove = (event: React.PointerEvent<SVGSVGElement>) => {
    if (!points.length) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const time = t0 + ((event.clientX - rect.left - left) / plotW) * span;
    let best = 0;
    for (let i = 1; i < times.length; i++) {
      if (Math.abs(times[i] - time) < Math.abs(times[best] - time)) best = i;
    }
    setHover(best);
  };

  const hp = hover !== null ? points[hover] : undefined;
  const hx = hp ? x(times[hover!]) : 0;
  const showDate = span > 36 * 3600_000;
  const timeLabel = (ms: number) =>
    showDate
      ? new Date(ms).toLocaleDateString(language === "zh" ? "zh-CN" : "en", { month: "numeric", day: "numeric" })
      : formatTime(new Date(ms), language);

  return (
    <div className="servers-chart">
      <div className="servers-chart-head">
        <h3>{title}</h3>
        {series.length > 1 && (
          <div className="servers-legend" aria-label={t("Legend")}>
            {series.map((s) => (
              <span key={s.key}>
                <i style={{ background: s.color }} />
                {s.label}
              </span>
            ))}
          </div>
        )}
        {current && <strong className="servers-chart-current">{current}</strong>}
      </div>
      <div className="servers-chart-plot" ref={box}>
        {points.length < 2 ? (
          <div className="servers-chart-empty">{t("Not enough data yet")}</div>
        ) : (
          width > 0 && (
            <svg
              width={width}
              height={HEIGHT}
              role="img"
              aria-label={title}
              onPointerMove={onMove}
              onPointerLeave={() => setHover(null)}
            >
              {ticks.map((v) => (
                <g key={v}>
                  <line className="servers-gridline" x1={left} x2={width - PAD.right} y1={y(v)} y2={y(v)} />
                  <text className="servers-axis" x={left - 6} y={y(v) + 4} textAnchor="end">
                    {format(v)}
                  </text>
                </g>
              ))}
              <text className="servers-axis" x={left} y={HEIGHT - 4}>
                {timeLabel(t0)}
              </text>
              <text className="servers-axis" x={width - PAD.right} y={HEIGHT - 4} textAnchor="end">
                {timeLabel(t1)}
              </text>
              {paths.map((d, i) => (
                <path key={series[i].key} d={d} fill="none" stroke={series[i].color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
              ))}
              {hp && (
                <g>
                  <line className="servers-crosshair" x1={hx} x2={hx} y1={PAD.top} y2={PAD.top + plotH} />
                  {series.map((s) => (
                    <circle key={s.key} cx={hx} cy={y(Number(hp[s.key]))} r={4} fill={s.color} className="servers-dot" />
                  ))}
                </g>
              )}
            </svg>
          )
        )}
        {hp && (
          <div
            className="servers-tooltip"
            style={{ left: Math.min(hx + 10, Math.max(0, width - 160)) }}
          >
            <small>
              {showDate
                ? new Date(hp.at).toLocaleString(language === "zh" ? "zh-CN" : "en", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" })
                : formatTime(hp.at, language)}
            </small>
            {series.map((s) => (
              <span key={s.key}>
                <i style={{ background: s.color }} />
                {s.label}
                <b>{format(Number(hp[s.key]))}</b>
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
