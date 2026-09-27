import { useState, type ReactNode } from "react";
import { Ellipsis } from "lucide-react";

export interface MoreMenuItem {
  key: string;
  label: string;
  icon?: ReactNode;
  danger?: boolean;
  onSelect: () => void;
}

/*
 * “更多”菜单（…按钮）。桌面上是按钮下方的下拉，手机上从底部弹出，
 * 顶部显示 title（通常是条目名称）。列表里每一行的操作都用它。
 */
export default function MoreMenu({
  items,
  title,
  label,
  className = "",
}: {
  items: MoreMenuItem[];
  /** 手机上弹出时显示在最上面，比如文件名。 */
  title?: string;
  /** 按钮的读屏文字，比如“更多：notes.md”。 */
  label: string;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className={`xc-more ${className}`}>
      <button
        type="button"
        className="xc-btn ghost small xc-more-button"
        aria-label={label}
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
      >
        <Ellipsis size={15} />
      </button>
      {open && (
        <>
          <div
            className="xc-more-backdrop"
            onClick={(e) => {
              e.stopPropagation();
              setOpen(false);
            }}
          />
          <div
            className="xc-more-menu"
            role="menu"
            onClick={(e) => e.stopPropagation()}
          >
            {title && <div className="xc-more-title">{title}</div>}
            {items.map((item) => (
              <button
                key={item.key}
                type="button"
                role="menuitem"
                className={item.danger ? "danger" : ""}
                onClick={() => {
                  setOpen(false);
                  item.onSelect();
                }}
              >
                {item.icon}
                {item.label}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
