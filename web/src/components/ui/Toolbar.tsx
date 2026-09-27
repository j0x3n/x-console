import { forwardRef, type ReactNode } from "react";
import { Search, X, type LucideIcon } from "lucide-react";

/*
 * 列表页的工具栏：左边是分类标签或筛选，右边是搜索和视图切换。
 * 窄屏时自动换成上下两行。用法：
 *   <Toolbar start={<Tabs .../>} end={<><SearchBox .../><Segmented .../></>} />
 */
export function Toolbar({
  start,
  end,
  className = "",
}: {
  start?: ReactNode;
  end?: ReactNode;
  className?: string;
}) {
  return (
    <div className={`xc-toolbar ${className}`}>
      <div className="xc-toolbar-start">{start}</div>
      <div className="xc-toolbar-end">{end}</div>
    </div>
  );
}

/** 搜索框：左边放大镜，有内容时右边出现清除按钮。 */
export const SearchBox = forwardRef<
  HTMLInputElement,
  {
    value: string;
    onChange: (value: string) => void;
    placeholder: string;
    clearLabel?: string;
    className?: string;
  }
>(function SearchBox(
  { value, onChange, placeholder, clearLabel = "清除", className = "" },
  ref,
) {
  return (
    <label className={`xc-search ${className}`}>
      <Search size={14} />
      <input
        ref={ref}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
      />
      {value && (
        <button
          type="button"
          aria-label={clearLabel}
          onClick={() => onChange("")}
        >
          <X size={13} />
        </button>
      )}
    </label>
  );
});

export interface SegmentedOption<T extends string> {
  value: T;
  label: string;
  icon?: LucideIcon;
  /** 只显示图标，文字放进提示和读屏。 */
  iconOnly?: boolean;
}

/** 分段切换：比如列表 / 网格、编辑 / 预览。选中的一段有底色。 */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: SegmentedOption<T>[];
  onChange: (value: T) => void;
  label: string;
}) {
  return (
    <div className="xc-segmented" role="group" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          className={o.value === value ? "on" : ""}
          aria-pressed={o.value === value}
          title={o.label}
          aria-label={o.iconOnly ? o.label : undefined}
          onClick={() => onChange(o.value)}
        >
          {o.icon && <o.icon size={15} />}
          {!o.iconOnly && <span>{o.label}</span>}
        </button>
      ))}
    </div>
  );
}
