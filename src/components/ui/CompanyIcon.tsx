import type * as Model from "../../types/domain";
import React from "react";
import { companyRecords } from "../../data/workspace";

interface CompanyIconProps {
  company: string;
}

export default function CompanyIcon({ company }: CompanyIconProps) {
  const row = companyRecords.find((item) => item.name === company);
  return (
    <span className={`ws-company ${row?.color || "terra"}`}>
      {row?.initials || company.slice(0, 2).toUpperCase()}
    </span>
  );
}
