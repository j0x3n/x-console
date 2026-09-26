import type * as Model from "../../types/domain";
import React from "react";
import { useT } from "../../contexts/LanguageContext";

interface LineChartProps {
  points: string;
  color?: string;
  fill?: boolean;
}

export default function LineChart({
  points,
  color = "#70b5f7",
  fill = false,
}: LineChartProps) {
  const t = useT();
  return (
    <svg
      className="line-chart"
      viewBox="0 0 160 35"
      preserveAspectRatio="none"
      role="img"
      aria-label={t("Trend chart")}
    >
      {fill && (
        <polygon points={`0,35 ${points} 160,35`} fill="rgba(93,166,235,.12)" />
      )}
      <polyline
        points={points}
        fill="none"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle
        cx="160"
        cy={points.split(" ").at(-1)?.split(",")[1] ?? 0}
        r="3.5"
        fill={color}
        stroke="var(--chart-dot-stroke, #17171a)"
        strokeWidth="2"
      />
    </svg>
  );
}
