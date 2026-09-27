import type { ReactNode } from "react";

interface PageHeadingProps {
  title: ReactNode;
  /** 标题下面一行灰字，写这一页的概况。 */
  subtitle?: ReactNode;
  /** 右上角的按钮。 */
  aside?: ReactNode;
  /** 右侧、按钮下面的一句状态，比如“● 2 台在线”。 */
  meta?: ReactNode;
}

export default function PageHeading({
  title,
  subtitle,
  aside,
  meta,
}: PageHeadingProps) {
  return (
    <div className="xc-page-head">
      <div className="xc-page-title">
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {(aside || meta) && (
        <div className="xc-page-aside">
          {aside && <div className="xc-row">{aside}</div>}
          {meta && <div className="xc-page-meta">{meta}</div>}
        </div>
      )}
    </div>
  );
}
