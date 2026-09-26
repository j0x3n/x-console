import type * as Model from "../../../types/domain";
import React from "react";
import { companyRecords } from "../../../data/workspace";
import { marks, colors, names } from "../company-config";

interface AccountIconProps {
  name: string;
  company?: boolean;
}

export default function AccountIcon({
  name,
  company = false,
}: AccountIconProps) {
  if (company) {
    const row = companyRecords.find((item) => item.name === name);
    return (
      <span className={`ws-company ${row?.color || "terra"}`} aria-label={name}>
        {row?.initials || name.slice(0, 2).toUpperCase()}
      </span>
    );
  }
  if (marks[name])
    return (
      <span className={`ws-agent ${colors[name]}`} aria-label={name}>
        {marks[name]}
      </span>
    );
  return (
    <span className="ws-person" aria-label={names[name] || name}>
      {(names[name] || name)
        .split(" ")
        .map((part) => part[0])
        .join("")
        .slice(0, 2)}
    </span>
  );
}
