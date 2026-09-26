import type * as Model from "../../types/domain";
import React from "react";

interface PersonIconProps {
  name: string;
}

export default function PersonIcon({ name }: PersonIconProps) {
  return (
    <span className="ws-person">
      {name
        .split(" ")
        .map((part) => part[0])
        .join("")
        .slice(0, 2)}
    </span>
  );
}
