import { useEffect, type ReactNode } from "react";
import { usePageTitle } from "../../stores/page-title";

interface PageHeadingProps {
  /**
   * 页面标题。默认不显示大标题（左上角已经有模块名），只给读屏软件用。
   * 是字符串时同时显示在左上角，比如详情页的项目名。
   */
  title: ReactNode;
  /** 标题下面一行灰字，写这一页的概况。 */
  subtitle?: ReactNode;
  /** 右上角的按钮。 */
  aside?: ReactNode;
  /** 右侧、按钮下面的一句状态，比如“● 2 台在线”。 */
  meta?: ReactNode;
  /** 显示大标题。只有标题本身是内容时才用，比如今日页的问候语。 */
  showTitle?: boolean;
}

export default function PageHeading({
  title,
  subtitle,
  aside,
  meta,
  showTitle,
}: PageHeadingProps) {
  const setTitle = usePageTitle((s) => s.setTitle);
  useEffect(() => {
    if (showTitle || typeof title !== "string") return;
    setTitle(title);
    return () => setTitle("");
  }, [title, showTitle, setTitle]);

  const hasRow = showTitle || subtitle || aside || meta;
  return (
    <div className={`xc-page-head${hasRow ? "" : " empty"}`}>
      <div className="xc-page-title">
        <h1 className={showTitle ? undefined : "xc-sr-only"}>{title}</h1>
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
