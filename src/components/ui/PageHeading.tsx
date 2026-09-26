import type * as Model from "../../types/domain";
import React from "react";

interface PageHeadingProps {
  title: string;
  subtitle?: string;
  aside?: React.ReactNode;
}

export default function PageHeading({
  title,
  subtitle,
  aside,
}: PageHeadingProps) {
  return (
    <div className="ws-heading">
      <div>
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {aside}
    </div>
  );
}
