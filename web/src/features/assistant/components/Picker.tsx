import {
  Fragment,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Check, ChevronDown } from "lucide-react";

export interface PickerOption {
  value: string;
  label: string;
  /** 选项下面的一行说明 */
  hint?: string;
  /** 分组标题，和上一项不同时显示 */
  group?: string;
}

const GAP = 6;
const EDGE = 8;

/**
 * 输入框底部的小下拉（B194）：按钮是图标加当前值，点开后菜单在按钮上方。
 * 菜单挂在 body 上，浮窗的 overflow 截不掉。
 */
export default function Picker({
  label,
  icon,
  value,
  options,
  display,
  tone,
  title,
  disabled,
  fluid,
  onChange,
}: {
  /** 读屏文字和菜单标题 */
  label: string;
  icon?: ReactNode;
  value: string;
  options: PickerOption[];
  /** 按钮上显示的文字，不传时用选中项的 label */
  display?: string;
  tone?: "warn";
  title?: string;
  disabled?: boolean;
  /** 位置不够时这个按钮的文字先省略，比如模型名 */
  fluid?: boolean;
  onChange: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState<CSSProperties>({ visibility: "hidden" });
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const current = options.find((o) => o.value === value);

  useLayoutEffect(() => {
    if (!open) return;
    const b = buttonRef.current?.getBoundingClientRect();
    const m = menuRef.current?.getBoundingClientRect();
    if (!b || !m) return;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const left = Math.max(EDGE, Math.min(b.left, vw - EDGE - m.width));
    // 默认在按钮上方，上方放不下时放到下方
    const above = b.top - GAP - m.height >= EDGE || b.top > vh - b.bottom;
    setStyle(
      above
        ? { left, bottom: vh - b.top + GAP, maxHeight: b.top - GAP - EDGE }
        : { left, top: b.bottom + GAP, maxHeight: vh - b.bottom - GAP - EDGE },
    );
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      const target = e.target as Node;
      if (
        menuRef.current?.contains(target) ||
        buttonRef.current?.contains(target)
      )
        return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        setOpen(false);
        buttonRef.current?.focus();
      }
    };
    const onResize = () => setOpen(false);
    document.addEventListener("pointerdown", close, true);
    document.addEventListener("scroll", close, true);
    window.addEventListener("keydown", onKey, true);
    window.addEventListener("resize", onResize);
    return () => {
      document.removeEventListener("pointerdown", close, true);
      document.removeEventListener("scroll", close, true);
      window.removeEventListener("keydown", onKey, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open]);

  // 打开后焦点放到选中项上，方向键可以上下移动
  useEffect(() => {
    if (!open) return;
    const menu = menuRef.current;
    const on =
      menu?.querySelector<HTMLElement>("[aria-selected='true']") ??
      menu?.querySelector<HTMLElement>("[role='option']");
    on?.focus();
  }, [open]);

  const onMenuKey = (e: ReactKeyboardEvent) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLElement>("[role='option']") ?? [],
    );
    const i = items.indexOf(document.activeElement as HTMLElement);
    const next = e.key === "ArrowDown" ? i + 1 : i - 1;
    items[(next + items.length) % items.length]?.focus();
  };

  const pick = (v: string) => {
    setOpen(false);
    buttonRef.current?.focus();
    if (v !== value) onChange(v);
  };

  let lastGroup: string | undefined;
  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className={`ai-chip${fluid ? " fluid" : ""}${tone ? ` ${tone}` : ""}${open ? " on" : ""}`}
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        title={title ?? label}
        disabled={disabled}
        onClick={() => {
          setStyle({ visibility: "hidden" });
          setOpen((v) => !v);
        }}
      >
        {icon}
        <span>{display ?? current?.label ?? ""}</span>
        <ChevronDown size={12} className="ai-chip-caret" />
      </button>
      {open &&
        createPortal(
          <div
            ref={menuRef}
            className="ai-picker"
            role="listbox"
            aria-label={label}
            style={style}
            onKeyDown={onMenuKey}
          >
            <div className="ai-picker-title">{label}</div>
            {options.map((o) => {
              const head = o.group && o.group !== lastGroup ? o.group : null;
              lastGroup = o.group;
              const on = o.value === value;
              return (
                <Fragment key={o.value}>
                  {head && <div className="ai-picker-group">{head}</div>}
                  <button
                    type="button"
                    role="option"
                    aria-selected={on}
                    className={on ? "on" : undefined}
                    onClick={() => pick(o.value)}
                  >
                    <span className="ai-picker-text">
                      <span>{o.label}</span>
                      {o.hint && <small>{o.hint}</small>}
                    </span>
                    {on && <Check size={14} />}
                  </button>
                </Fragment>
              );
            })}
          </div>,
          document.body,
        )}
    </>
  );
}
