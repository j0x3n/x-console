import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { Search, type LucideIcon } from "lucide-react";

/*
 * 左栏二级菜单的公共部件（B102）。各模块的 NavChildren 用它们拼出：
 *   <NavPanelStack>
 *     <NavPanelSearch …/>
 *     <NavPanelGroup><NavPanelLink …/>…</NavPanelGroup>
 *     <NavPanelGroup label="标签">…</NavPanelGroup>
 *   </NavPanelStack>
 * 选中不按地址自动判断，由模块传 active，因为很多入口只差查询参数。
 */

export function NavPanelStack({ children }: { children: ReactNode }) {
  return <div className="nav-panel-stack">{children}</div>;
}

/** 一组菜单项。第一组不写 label；后面的组上方有分隔线。 */
export function NavPanelGroup({
  label,
  children,
}: {
  label?: string;
  children: ReactNode;
}) {
  return (
    <div className={`nav-panel-group${label ? " labeled" : ""}`}>
      {label && <div className="nav-panel-label">{label}</div>}
      {children}
    </div>
  );
}

export function NavPanelLink({
  to,
  icon: Icon,
  mark,
  label,
  count,
  danger,
  active,
  onNavigate,
}: {
  to: string;
  icon?: LucideIcon;
  /** 不用图标时放的小标记，比如颜色点 */
  mark?: ReactNode;
  label: string;
  count?: number | null;
  /** 数量用红色，比如已过期 */
  danger?: boolean;
  active: boolean;
  onNavigate: () => void;
}) {
  return (
    <Link
      to={to}
      onClick={onNavigate}
      aria-current={active ? "page" : undefined}
      className={`nav-child${active ? " selected" : ""}`}
    >
      {Icon ? <Icon size={16} className="nav-child-mark" /> : mark}
      <span>{label}</span>
      {count != null && (
        <small className={danger && count > 0 ? "danger" : ""}>{count}</small>
      )}
    </Link>
  );
}

/** 二级菜单顶上的搜索框，回车时交给模块处理（一般是跳到列表页并带上搜索词）。 */
export function NavPanelSearch({
  placeholder,
  onSubmit,
}: {
  placeholder: string;
  onSubmit: (query: string) => void;
}) {
  const [query, setQuery] = useState("");
  return (
    <form
      className="nav-panel-search"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(query.trim());
      }}
    >
      <Search size={14} />
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
      />
    </form>
  );
}
