import type { ReactNode } from "react";

interface PageHeadingProps {
  title: string;
  subtitle?: string;
  aside?: ReactNode;
}

export default function PageHeading({
  title,
  subtitle,
  aside,
}: PageHeadingProps) {
  return (
    <div className="xc-page-head">
      <div>
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {aside && <div className="xc-row">{aside}</div>}
    </div>
  );
}
