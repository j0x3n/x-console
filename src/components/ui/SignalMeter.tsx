import type * as Model from "../../types/domain";
import React from "react";

interface SignalMeterProps {
  value: number;
  segments?: number;
  tone: string;
  label: string;
}

export default function SignalMeter({
  value,
  segments = 10,
  tone,
  label,
}: SignalMeterProps) {
  const filled = Math.max(
    0,
    Math.min(segments, Math.round((value / 100) * segments)),
  );
  return (
    <span className={`ws-meter-bars ${tone}`} role="img" aria-label={label}>
      {Array.from({ length: segments }, (_, i) => (
        <i key={i} className={i < filled ? "on" : ""} />
      ))}
    </span>
  );
}
