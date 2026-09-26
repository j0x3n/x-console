import type * as Model from "../../../types/domain";
import React from "react";

interface AccountHealthProps {
  value: number;
  change?: number;
  L: Model.Localize;
}

export default function AccountHealth({
  value,
  change = 0,
  L,
}: AccountHealthProps) {
  const tone = value >= 75 ? "high" : value >= 50 ? "mid" : "low";
  const values = Array.from({ length: 15 }, (_, index) =>
    Math.max(
      5,
      Math.min(
        95,
        value - change + (change * index) / 14 + Math.sin(index * 1.8) * 2,
      ),
    ),
  );
  const path = values
    .map(
      (point, index) =>
        `${index ? "L" : "M"}${(index / 14) * 100} ${24 - point * 0.2}`,
    )
    .join(" ");
  return (
    <div className={`ws-account-health-visual ${tone}`}>
      <strong>{value}</strong>
      <span
        className={`ws-meter-bars ${tone}`}
        role="img"
        aria-label={L(`Health ${value} of 100`, `健康度 ${value} / 100`)}
      >
        {Array.from({ length: 10 }, (_, index) => (
          <i
            key={index}
            className={index < Math.round(value / 10) ? "on" : ""}
          />
        ))}
      </span>
      <svg
        viewBox="0 0 100 24"
        preserveAspectRatio="none"
        role="img"
        aria-label={L(`Health trend over 14 days`, `14 天健康度趋势`)}
      >
        <path d={`${path} L100 24 L0 24 Z`} className="ws-account-spark-fill" />
        <path d={path} className="ws-account-spark-line" />
      </svg>
    </div>
  );
}
