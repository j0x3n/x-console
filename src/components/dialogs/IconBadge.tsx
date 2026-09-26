import type * as Model from "../../types/domain";
import React from "react";
import { agentMarks } from "./dialog-config";

interface IconBadgeProps {
  name: string;
}

export default function IconBadge({ name }: IconBadgeProps) {
  return (
    <span className={`overlay-agent-icon ${name.toLowerCase()}`}>
      {agentMarks[name] ||
        name
          .split(" ")
          .map((part) => part[0])
          .join("")}
    </span>
  );
}
