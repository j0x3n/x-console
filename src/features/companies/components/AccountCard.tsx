import type * as Model from "../../../types/domain";
import React from "react";

interface AccountCardProps {
  title: string;
  count?: number;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}

export default function AccountCard({
  title,
  count,
  action,
  children,
  className = "",
}: AccountCardProps) {
  return (
    <section className={`ws-account-card ${className}`}>
      <div className="ws-account-card-title">
        <h2>{title}</h2>
        {count != null && <span>{count}</span>}
        {action}
      </div>
      {children}
    </section>
  );
}
