import type * as Model from "../../types/domain";
import React from "react";
import { agents } from "../../data/catalogs";

interface AgentBadgeProps {
  name: string;
  size?: string;
}

export default function AgentBadge({ name, size = "normal" }: AgentBadgeProps) {
  const agent = agents.find((item) => item.name === name) || agents[0];
  return (
    <span className={`agent-badge ${agent.color} ${size}`} aria-label={name}>
      {agent.mark}
    </span>
  );
}
