import { useState, type ReactNode } from "react";
import { NavLink } from "react-router";
import { ChevronRight } from "lucide-react";
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
  /** 缩进一级，比如项目下面的分类。 */
  nested?: boolean;
  /** 行尾的小按钮，比如习惯的“+1”。点它不会跳转 */
  action?: ReactNode;
}

// 三级菜单（缩进的子项）展开了哪些，记在 localStorage。默认收起（B68）。
const OPEN3_KEY = "xc.nav.open3";
function readOpen3(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(OPEN3_KEY) ?? "[]");
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
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
  limit = NAV_CHILD_LIMIT,
}: {
  links: NavChildLink[];
  total?: number;
  /** 最多显示几条，默认 NAV_CHILD_LIMIT。插了缩进的子链接时调大，“全部”仍按 total 判断。 */
  limit?: number;
  allTo: string;
  loading?: boolean;
  error?: boolean;
  empty: string;
  onNavigate: () => void;
}) {
  const t = useT();
  const [open3, setOpen3] = useState<string[]>(readOpen3);
  const count = total ?? links.length;
  if (loading) return <div className="nav-children-note">{t("Loading")}…</div>;
  if (error)
    return <div className="nav-children-note">{t("Something went wrong")}</div>;
  if (links.length === 0)
    return <div className="nav-children-note">{empty}</div>;
  const toggle = (id: string) =>
    setOpen3((prev) => {
      const next = prev.includes(id)
        ? prev.filter((x) => x !== id)
        : [...prev, id];
      try {
        localStorage.setItem(OPEN3_KEY, JSON.stringify(next));
      } catch {
        /* 记不住就算了 */
      }
      return next;
    });
  // 缩进的子项跟在它上面那个不缩进的链接后面，归到一组。
  const groups: { parent: NavChildLink; nested: NavChildLink[] }[] = [];
  for (const l of links.slice(0, limit)) {
    const last = groups[groups.length - 1];
    if (l.nested && last) last.nested.push(l);
    else groups.push({ parent: l, nested: [] });
  }
  const render = (l: NavChildLink, toggleButton?: ReactNode) => {
    const link = (
      <NavLink
        key={l.key}
        to={l.to}
        end
        onClick={onNavigate}
        className={({ isActive }) =>
          `nav-child${l.nested ? " nested" : ""}${(l.active ?? isActive) ? " selected" : ""}`
        }
        title={l.label}
      >
        {l.mark}
        <span>{l.label}</span>
        {l.hint && <small>{l.hint}</small>}
      </NavLink>
    );
    return l.action || toggleButton ? (
      <div key={l.key} className="nav-child-row">
        {link}
        {l.action}
        {toggleButton}
      </div>
    ) : (
      link
    );
  };
  return (
    <>
      {groups.map(({ parent, nested }) => {
        if (nested.length === 0) return render(parent);
        const id = `${allTo}|${parent.key}`;
        const isOpen = open3.includes(id);
        return [
          render(
            parent,
            <button
              key="toggle"
              type="button"
              className={`nav-child-action nav-child-toggle${isOpen ? " open" : ""}`}
              aria-expanded={isOpen}
              aria-label={`${isOpen ? t("Collapse menu") : t("Expand menu")} ${parent.label}`}
              title={isOpen ? t("Collapse menu") : t("Expand menu")}
              onClick={() => toggle(id)}
            >
              <ChevronRight size={13} />
            </button>,
          ),
          ...(isOpen ? nested.map((l) => render(l)) : []),
        ];
      })}
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
