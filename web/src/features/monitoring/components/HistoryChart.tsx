import { useMemo } from "react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import type { MonitorResult } from "../api";
import { latencyPoints, stripSegments } from "../lib";
import { shortDateTime } from "./common";

const W = 600;
const H = 110;

/** 响应时间曲线，失败的检查画成红色竖线；下面一条是正常和失败的色带。 */
export default function HistoryChart({ items }: { items: MonitorResult[] }) {
  const t = useT();
  const language = useLanguage();
  const { points, max } = useMemo(() => latencyPoints(items, W, H), [items]);
  if (points.length === 0)
    return (
      <div className="monitoring-chart-empty">
        {t("No checks in this period")}
      </div>
    );
  const line = points
    .map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`)
    .join(" ");
  const failed = points.filter((p) => !p.ok);
  return (
    <figure className="monitoring-chart">
      <div className="monitoring-chart-axis">
        <span>{max} ms</span>
        <span>0</span>
      </div>
      <div className="monitoring-chart-plot">
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          role="img"
          aria-label={t("Response time")}
        >
          <line
            x1="0"
            x2={W}
            y1={H / 2}
            y2={H / 2}
            className="monitoring-chart-grid"
          />
          {failed.map((p, i) => (
            <line
              key={i}
              x1={p.x}
              x2={p.x}
              y1="0"
              y2={H}
              className="monitoring-chart-fail"
            />
          ))}
          <polyline points={line} className="monitoring-chart-line" />
          {points.length === 1 && (
            <circle
              cx={points[0].x}
              cy={points[0].y}
              r="3"
              className="monitoring-chart-dot"
            />
          )}
        </svg>
        <div className="monitoring-strip" aria-hidden="true">
          <svg viewBox={`0 0 ${W} 6`} preserveAspectRatio="none">
            {stripSegments(
              points.map((p) => p.x),
              W,
            ).map(([x, w], i) => (
              <rect
                key={i}
                x={x}
                width={w}
                y="0"
                height="6"
                className={points[i].ok ? "ok" : "fail"}
              />
            ))}
          </svg>
        </div>
        <div className="monitoring-chart-times">
          <span>{shortDateTime(items[0].at, language)}</span>
          <span>{shortDateTime(items[items.length - 1].at, language)}</span>
        </div>
      </div>
    </figure>
  );
}
