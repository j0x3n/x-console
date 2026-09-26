import type * as Model from "../../types/domain";
import React from "react";

interface ProgressProps {
  value: number;
  color?: string;
}

export default function Progress({ value, color = "mint" }: ProgressProps) {
  return (
    <span
      className={`ws-progress ${color}`}
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <i style={{ width: `${value}%` }} />
    </span>
  );
}
