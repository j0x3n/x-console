import type * as Model from "../../types/domain";
import React from "react";
import { agentMarks, agentColors, ownerNames } from "../../data/catalogs";

interface AgentIconProps {
  name: string;
}

export default function AgentIcon({ name }: AgentIconProps) {
  return agentMarks[name] ? (
    <span className={`ws-agent ${agentColors[name]}`} aria-label={name}>
      {agentMarks[name]}
    </span>
  ) : (
    <span className="ws-person" aria-label={ownerNames[name] || name}>
      {name.slice(0, 2).toUpperCase()}
    </span>
  );
}
