import type * as Model from "../../types/domain";
import React from "react";

interface CompanyBadgeProps {
  initials: string;
  color: string;
}

export default function CompanyBadge({ initials, color }: CompanyBadgeProps) {
  return <span className={`company-badge ${color}`}>{initials}</span>;
}
