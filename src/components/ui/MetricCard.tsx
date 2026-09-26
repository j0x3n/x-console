import type * as Model from "../../types/domain";
import React from "react";

interface MetricCardProps {
  label: string;
  caption?: string;
  children: React.ReactNode;
  onClick: React.MouseEventHandler<HTMLButtonElement>;
  className?: string;
}

export default function MetricCard({
  label,
  caption,
  children,
  onClick,
  className = "",
}: MetricCardProps) {
  return (
    <button className={`metric-card ${className}`} onClick={onClick}>
      <div className="metric-top">
        <span>{label}</span>
        <span>{caption}</span>
      </div>
      {children}
    </button>
  );
}
