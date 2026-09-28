import { useEffect, type ReactNode } from "react";
import { usePageTitle, type Crumb } from "../../stores/page-title";
import PageActions from "../layout/PageActions";

interface PageHeadingProps {
  /**
   * 页面标题。默认不显示大标题（左上角已经有模块名），只给读屏软件用。
   * 是字符串时同时显示在左上角，比如详情页的项目名。
   */
  title: ReactNode;
  /** 这一页的概况灰字，显示在左上角标题后面（B20）。 */
  subtitle?: ReactNode;
  /** 页面按钮，显示在顶栏中段（B20）。等同于 <PageActions>。 */
  aside?: ReactNode;
  /** 一句状态，比如“● 2 台在线”，跟在概况后面。 */
  meta?: ReactNode;
  /** 显示大标题。只有标题本身是内容时才用，比如今日页的问候语。这时概况和 meta 留在页面里。 */
  showTitle?: boolean;
  /** 左上角模块名和标题之间的层级，比如 Issue 页的项目。 */
  parents?: Crumb[];
}

export default function PageHeading({
  title,
  subtitle,
  aside,
  meta,
  showTitle,
  parents,
}: PageHeadingProps) {
  const setTitle = usePageTitle((s) => s.setTitle);
  const setSubtitle = usePageTitle((s) => s.setSubtitle);
  const parentsKey = JSON.stringify(parents ?? []);
  useEffect(() => {
    if (showTitle || typeof title !== "string") return;
    setTitle(title, parents);
    return () => setTitle("");
    // parents 按内容比较
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title, showTitle, setTitle, parentsKey]);

  const topSubtitle = showTitle ? null : subtitle || meta ? (
    <>
      {subtitle}
      {subtitle && meta ? " · " : null}
      {meta}
    </>
  ) : null;
  useEffect(() => {
    setSubtitle(topSubtitle);
  });
  useEffect(() => () => setSubtitle(null), [setSubtitle]);

  return (
    <>
      <div className={`xc-page-head${showTitle ? "" : " empty"}`}>
        <div className="xc-page-title">
          <h1 className={showTitle ? undefined : "xc-sr-only"}>{title}</h1>
          {showTitle && subtitle && <p>{subtitle}</p>}
        </div>
        {showTitle && meta && (
          <div className="xc-page-aside">
            <div className="xc-page-meta">{meta}</div>
          </div>
        )}
      </div>
      {aside && <PageActions>{aside}</PageActions>}
    </>
  );
}
