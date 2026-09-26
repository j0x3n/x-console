import type * as Model from "../../types/domain";
import React from "react";
import SignalMeter from "./SignalMeter";

interface WarmthProps {
  value: string;
  L: Model.Localize;
}

export default function Warmth({ value, L }: WarmthProps) {
  const score = { Warm: 100, Neutral: 67, Cold: 33 }[value] || 33;
  const tone = { Warm: "high", Neutral: "mid", Cold: "low" }[value];
  const label = L(
    value,
    { Warm: "活跃", Neutral: "一般", Cold: "冷淡" }[value],
  );
  return (
    <span
      className="ws-warmth"
      aria-label={`${L("Warmth", "联系热度")}: ${label}`}
    >
      <SignalMeter
        value={score}
        segments={3}
        tone={tone ?? "low"}
        label={label}
      />
      <span>{label}</span>
    </span>
  );
}
