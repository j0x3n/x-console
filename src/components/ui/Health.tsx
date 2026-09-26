import type * as Model from "../../types/domain";
import React from "react";
import SignalMeter from "./SignalMeter";

interface HealthProps {
  value: number;
  change?: number;
  L: Model.Localize;
}

export default function Health({ value, change, L }: HealthProps) {
  const tone = value < 50 ? "low" : value < 75 ? "mid" : "high";
  return (
    <span className="ws-health">
      <SignalMeter
        value={value}
        tone={tone}
        label={
          L
            ? L(`Health ${value} of 100`, `健康度 ${value} / 100`)
            : `Health ${value} of 100`
        }
      />
      <b>{value}</b>
      {change !== undefined && (
        <small className={change < 0 ? "down" : "up"}>
          {change > 0 ? "+" : ""}
          {change}
        </small>
      )}
    </span>
  );
}
