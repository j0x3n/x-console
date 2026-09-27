import type { ReactNode } from "react";
import { NavLink } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import { NAV_CHILD_LIMIT } from "../../lib/navChildren";

export interface NavChildLink {
  key: string | number;
  to: string;
  label: string;
  /** 名称左边的小标记，比如在线状态点、置顶图标。 */
  mark?: ReactNode;
  /** 名称右边的灰字，比如时间、数量。 */
  hint?: string;
  /** 链接带查询参数时自己判断是否选中；不传就按路径判断。 */
  active?: boolean;
}

/**
 * 二级菜单的通用列表：加载中、出错、空、超过上限时的“全部”链接。
 * 各模块只要把数据整理成 links 传进来。
 */
export default function NavChildLinks({
  links,
  total,
  allTo,
  loading,
  error,
  empty,
  onNavigate,
}: {
  links: NavChildLink[];
  total?: number;
  allTo: string;
  loading?: boolean;
  error?: boolean;
  empty: string;
  onNavigate: () => void;
}) {
  const t = useT();
  const count = total ?? links.length;
  if (loading) return <div className="nav-children-note">{t("Loading")}…</div>;
  if (error)
    return <div className="nav-children-note">{t("Something went wrong")}</div>;
  if (links.length === 0)
    return <div className="nav-children-note">{empty}</div>;
  return (
    <>
      {links.slice(0, NAV_CHILD_LIMIT).map((l) => (
        <NavLink
          key={l.key}
          to={l.to}
          end
          onClick={onNavigate}
          className={({ isActive }) =>
            `nav-child${(l.active ?? isActive) ? " selected" : ""}`
          }
          title={l.label}
        >
          {l.mark}
          <span>{l.label}</span>
          {l.hint && <small>{l.hint}</small>}
        </NavLink>
      ))}
      {count > NAV_CHILD_LIMIT && (
        <NavLink to={allTo} end onClick={onNavigate} className="nav-child more">
          <span>
            {t("See all")} {count}
          </span>
        </NavLink>
      )}
    </>
  );
}
