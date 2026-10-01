import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Ellipsis } from "lucide-react";

export interface MoreMenuItem {
  key: string;
  label: string;
  icon?: ReactNode;
  danger?: boolean;
  onSelect: () => void;
}

/** 菜单和按钮之间、菜单和窗口边缘之间的距离。 */
const GAP = 4;
const EDGE = 8;

/** 桌面上菜单的位置：默认在按钮下方右对齐，下方放不下时放到上方。 */
export function menuPosition(
  button: { top: number; bottom: number; left: number; right: number },
  menu: { width: number; height: number },
  view: { width: number; height: number },
): CSSProperties {
  const below = button.bottom + GAP;
  const fitsBelow = below + menu.height <= view.height - EDGE;
  const fitsAbove = button.top - GAP - menu.height >= EDGE;
  const top =
    fitsBelow || !fitsAbove
      ? Math.max(EDGE, Math.min(below, view.height - EDGE - menu.height))
      : button.top - GAP - menu.height;
  const left = Math.max(
    EDGE,
    Math.min(button.right - menu.width, view.width - EDGE - menu.width),
  );
  return { top, left };
}

const mobileQuery = "(max-width: 640px)";

/*
 * “更多”菜单（…按钮）。桌面上是按钮下方的下拉，手机上从底部弹出，
 * 顶部显示 title（通常是条目名称）。列表里每一行的操作都用它。
 * 菜单挂在 body 上（B54），外层的 overflow 截不掉它。
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
  const [style, setStyle] = useState<CSSProperties>({ visibility: "hidden" });
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // 按按钮的位置放菜单。页面滚动时跟着按钮走，按钮滚出窗口就关掉。
  const place = useCallback(() => {
    if (window.matchMedia?.(mobileQuery).matches) {
      setStyle({});
      return true;
    }
    const b = buttonRef.current?.getBoundingClientRect();
    const m = menuRef.current?.getBoundingClientRect();
    if (!b || !m) return true;
    if (b.bottom < 0 || b.top > window.innerHeight) return false;
    setStyle(
      menuPosition(b, m, {
        width: window.innerWidth,
        height: window.innerHeight,
      }),
    );
    return true;
  }, []);

  useLayoutEffect(() => {
    if (open) place();
  }, [open, place]);

  useEffect(() => {
    if (!open) return;
    const follow = (e: Event) => {
      // 菜单自己滚动时不动
      if (e.target instanceof Node && menuRef.current?.contains(e.target))
        return;
      if (!place()) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("scroll", follow, true);
    window.addEventListener("resize", follow);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("scroll", follow, true);
      window.removeEventListener("resize", follow);
      window.removeEventListener("keydown", onKey);
    };
  }, [open, place]);

  const toggle = () => {
    setStyle({ visibility: "hidden" });
    setOpen((v) => !v);
  };

  return (
    <div className={`xc-more ${className}`}>
      <button
        ref={buttonRef}
        type="button"
        className="xc-btn ghost small xc-more-button"
        aria-label={label}
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation();
          toggle();
        }}
      >
        <Ellipsis size={15} />
      </button>
      {open &&
        createPortal(
          <>
            <div
              className="xc-more-backdrop"
              onClick={(e) => {
                e.stopPropagation();
                setOpen(false);
              }}
            />
            <div
              ref={menuRef}
              className="xc-more-menu"
              role="menu"
              style={style}
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
          </>,
          document.body,
        )}
    </div>
  );
}
